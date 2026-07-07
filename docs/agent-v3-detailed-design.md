# Agent 层 v3.0 — Graph 流程控制 + ChatModelAgent Phase 执行

> **核心思路**: Graph 控制"能不能"（Guard），ChatModelAgent 控制"怎么做"（ReAct）  
> **修复 D1-D4**: 通过 Eino checkpoint 让子图 State 跨 Guard 重试持久化

---

## 1. 架构图

```
┌─────────────────────────────────────────────────────────────────┐
│  Graph (确定性流程控制)                                          │
│                                                                  │
│  START → phase1_agent ──→ phase1_guard ──┬─→ phase2 ──→ ... ──→ END
│                        ↑                 │                        │
│                        └─── fail ────────┘                        │
│                                                                  │
│  每个 Phase 节点 = ChatModelAgent (ADK 原生)                      │
│  Guard 节点      = Lambda (检查 ReimbursementState)               │
│  Branch          = 路由决策                                       │
└─────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────┐
│  ChatModelAgent (自主决策) — 每个 Phase 内部                     │
│                                                                  │
│  Init → ChatModel ⇄ ToolsNode → AfterToolCalls                  │
│           ↑                        │                             │
│           └──── 循环 ──────────────┘                             │
│                                                                  │
│  typedState.Messages = 全部消息历史 (永不丢失)                    │
│  Checkpoint: 自动存/取 → Guard 重试时恢复                        │
└─────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────┐
│  ReimbursementState (业务流程状态) — 父图 WithGenLocalState      │
│                                                                  │
│  票据列表 / 合规结果 / 预算结果 / 确认标记                        │
│  Guard 节点检查此状态决定跳转                                     │
│  工具执行时通过 ProcessState 更新                                 │
└─────────────────────────────────────────────────────────────────┘
```

### 与 v2.1 的关键差异

| | v2.1 | v3.0 |
|---|---|---|
| Phase 实现 | 手工 `buildReActPhase()` | `adk.ChatModelAgent` |
| 消息历史存储 | `phaseState` (每次重入清空) | `typedState.Messages` (checkpoint 恢复) |
| 工具事件 SSE | 无 | 内置 event sender |
| Checkpoint | 无 | gob 序列化，MySQL 持久化 |
| 控制权 | Graph Guard | Graph Guard (不变) |
| 决策权 | 手工 ReAct 循环 | ChatModelAgent 引擎 |

---

## 2. 核心修复: Checkpoint 让 State 跨 Guard 重试

### 2.1 原理

```
Phase1 第一次进入:
  WithGenLocalState → new phaseState{Messages: []}
  ChatModel#1 → "请上传票据"
  → 到达 END (子图出口)
  → gob serialize phaseState → CheckpointStore.Set("phase1", bytes)
  → Guard: fail → Branch → route back to phase1

Phase1 被重新进入 (Guard 路由回来):
  WithGenLocalState → ⚠️ v2.1: new phaseState{Messages: []} (丢失!)
                      ✅ v3.0: CheckpointStore.Get("phase1") → gob deserialize → 恢复!
  ChatModel#2 → 看到完整历史: [userMsg, assistantMsg("请上传票据"), userMsg(guard提示)]
  → 继续对话, 上下文完整!
```

### 2.2 需要做的事情

1. **Phase State 类型注册**: 让 gob 能序列化 `phaseState`
2. **编译时加载 CheckpointStore**: `g.Compile(ctx, WithCheckPointStore(store), ...)`
3. **Checkpoint ID 命名**: 每个 Phase 子图用不同的 key（`phase1_collect`, `phase2_validate` 等）

---

## 3. Phase 节点实现

### 3.1 使用 ChatModelAgent 作为 Phase 节点

每个 Phase 创建独立的 ChatModelAgent 实例，工具集不同：

```go
// react_phase_v3.go — 新文件

// NewPhase1Agent 创建 Phase 1 (信息收集) 的 ChatModelAgent
func NewPhase1Agent(ctx context.Context, deps PhaseAgentDeps) (*adk.ChatModelAgent, error) {
    return adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
        Name:        "phase1_collect",
        Description: "收集票据信息, OCR识别, 用户确认",
        Instruction: agent.BuildPhase1Instruction(),  // 系统提示词
        Model:       deps.ChatModel,
        ToolsConfig: adk.ToolsConfig{
            ToolsNodeConfig: compose.ToolsNodeConfig{
                Tools: []tool.BaseTool{deps.OCR, deps.Compliance},
            },
        },
        MaxIterations: deps.Config.MaxPhaseTurns,
        Handlers: []adk.ChatModelAgentMiddleware{
            &stateUpdater{}, // ← AfterModel: 更新 ReimbursementState
        },
    })
}
```

### 3.2 stateUpdater Middleware: 桥接工具结果 → ReimbursementState

ChatModelAgent 的 State 是私有的。我们需要把工具结果（如 OCR 识别结果）同步到父图的 `ReimbursementState` 中，Guard 才能检查。

```go
// middleware_state_sync.go — 新文件

// stateUpdater 在每次工具调用完成后，将工具结果同步到父图的 ReimbursementState
type stateUpdater struct {
    *adk.BaseChatModelAgentMiddleware
}

func (s *stateUpdater) AfterModelRewriteState(
    ctx context.Context,
    state *adk.ChatModelAgentState,
    mc *adk.ModelContext,
) (context.Context, *adk.ChatModelAgentState, error) {
    // 从 Agent State 中提取最后一条 assistant 消息
    // 如果 LLM 已确认票据 → 解析确认信息 → 更新 ReimbursementState
    // (具体逻辑根据工具返回的 JSON 内容解析)
    return ctx, state, nil
}
```

**更好的方式**: 让工具实现直接操作 `ReimbursementState`。工具通过 `compose.ProcessState` 访问父图的 State：

```go
// 工具内部 (如 OCR 工具):
func (t *OCRTool) InvokableRun(ctx context.Context, argsJSON string) (string, error) {
    result := doOCR(argsJSON)
    
    // 同步更新父图的 ReimbursementState
    _ = compose.ProcessState(ctx, func(ctx context.Context, rs *agent.ReimbursementState) error {
        rs.Invoices = append(rs.Invoices, agent.InvoiceState{
            Amount:   result.Amount,
            Category: result.Category,
        })
        rs.TotalAmount += result.Amount
        return nil
    })
    
    return result.ToJSON(), nil
}
```

这样不需要 middleware 桥接 —— 工具直接修改 Guard 能读到的状态。

---

## 4. 父图重构

```go
// reimbursement_v3.go — 重写 reimbursement.go

func NewReimbursementGraph(ctx context.Context, deps ReimbursementGraphDeps) (...) {
    
    // ── 创建 Phase Agent 实例 ──
    phase1Agent, _ := NewPhase1Agent(ctx, deps)  // ChatModelAgent
    phase2Agent, _ := NewPhase2Agent(ctx, deps)
    phase3Agent, _ := NewPhase3Agent(ctx, deps)
    
    // ── 父图: 流程控制 ──
    g := compose.NewGraph[*schema.Message, *schema.Message](
        compose.WithGenLocalState(func(ctx context.Context) *agent.ReimbursementState {
            return restoreOrCreate(ctx)  // 从 context 恢复或新建
        }),
    )
    
    // ── Phase 节点: 用 Lambda 包装 ChatModelAgent.Run() ──
    g.AddLambdaNode("phase1", compose.InvokableLambda(
        func(ctx context.Context, msg *schema.Message) (*schema.Message, error) {
            return runPhaseAgent(ctx, phase1Agent, msg, deps.SessionStore, input.SessionID)
        }),
    ))
    // ... phase2, phase3 同理
    
    // ── Guard 节点 (不变) ──
    g.AddLambdaNode("phase1_guard", ...)
    g.AddLambdaNode("phase2_guard", ...)
    
    // ── Branch (不变) ──
    g.AddBranch("phase1_guard", compose.NewGraphBranch(
        func(ctx context.Context, msg *schema.Message) (string, error) {
            var passed bool
            compose.ProcessState(ctx, func(rs *agent.ReimbursementState) error {
                passed = phase.Phase1Guard(rs).Passed
                return nil
            })
            if passed { return "phase2", nil }
            return "phase1", nil
        },
        map[string]bool{"phase1": true, "phase2": true},
    ))
    
    // ── 编译 (带 Checkpoint!) ──
    runnable, err := g.Compile(ctx,
        compose.WithGraphName("reimbursement_workflow"),
        compose.WithMaxRunSteps(100),
        compose.WithCheckPointStore(deps.CheckpointStore),  // ← 关键!
        compose.WithSerializer(&gobSerializer{}),
    )
}
```

### 4.1 runPhaseAgent 辅助函数

```go
// runPhaseAgent 运行一个 ChatModelAgent 并返回最终回复 *Message
func runPhaseAgent(
    ctx context.Context,
    agent *adk.ChatModelAgent,
    input *schema.Message,
    store infra.SessionStore,
    sessionID string,
) (*schema.Message, error) {
    // 1. 加载对话历史
    history, _ := store.GetHistory(ctx, sessionID, 40)
    
    // 2. 构建消息列表
    messages := append(history, input)
    
    // 3. 运行 Agent
    runner := adk.NewRunner(ctx, adk.RunnerConfig{
        Agent:           agent,
        EnableStreaming: true,
    })
    iter := runner.Query(ctx, messages)
    
    // 4. 消费事件流，收集最终回复
    var finalText string
    for {
        event, ok := iter.Next()
        if !ok { break }
        if event.Err != nil { return nil, event.Err }
        if event.Output != nil && event.Output.MessageOutput != nil {
            mv := event.Output.MessageOutput
            if !mv.IsStreaming {
                finalText = mv.Message.Content
            } else {
                // 流式 → 累积
                // ⚠️ 这里需要处理 SSE 输出
                // 由外层 Runner 统一管理
            }
        }
    }
    
    return schema.AssistantMessage(finalText, nil), nil
}
```

---

## 5. AgentRunner 整合

```go
// runner.go — 修改 StreamChat

func (r *AgentRunner) StreamChat(ctx context.Context, input AgentInput, sseWriter SSEWriter) error {
    // 1. thinking 事件
    _ = sseWriter.WriteEvent(NewThinkingEvent("正在处理..."))
    _ = sseWriter.Flush()
    
    // 2. 加载 State + 历史
    ctx = loadState(ctx, r.sessionStore, input)
    history, _ := r.sessionStore.GetHistory(ctx, input.SessionID, 40)
    
    // 3. 保存用户消息
    userMsg := schema.UserMessage(input.Message)
    _ = r.sessionStore.SaveMessages(ctx, input.SessionID, []*schema.Message{userMsg})
    
    // 4. 意图分类 (复用现有 root dispatcher)
    route := classifyIntent(ctx, input)
    
    switch route {
    case "new_reimbursement":
        // 5. 执行报销 Graph (带 checkpoint)
        result, err := r.reimbGraph.Invoke(ctx, userMsg)
        if err != nil { ... }
        
        // 6. SSE 输出
        _ = sseWriter.WriteEvent(NewMessageEvent(result.Content, false))
        
    case "query_progress":
        // 简单子流程: ChatModel 直出
        ...
    }
    
    // 7. 持久化 + State 保存 + done
    _ = sseWriter.WriteEvent(NewDoneEvent())
    _ = sseWriter.Flush()
    
    saveState(ctx, r.sessionStore, input)
    return nil
}
```

---

## 6. 四种数据流的正确管理

| 数据类型 | 存储位置 | 生命周期 | 谁管理 |
|---------|---------|---------|--------|
| 对话消息 | ChatModelAgent 的 `typedState.Messages` | Phase 内 ReAct 循环 | ChatModelAgent 引擎 |
| 业务流程状态 | 父图 `ReimbursementState` | 整个报销流程 | 父图 `WithGenLocalState` |
| 跨请求持久化 | `SessionStore` (MySQL) | Session 级别 | `AgentRunner` |
| Graph 执行状态 | `CheckpointStore` (MySQL) | Guard 重试 | Eino checkpoint 引擎 |

### 6.1 对话消息的完整生命周期

```
请求 1: "我要报销"
  Phase1 Agent 第一次进入:
    typedState.Messages = [UserMsg("我要报销")]
    ChatModel#1 → "请上传票据"
    → 到达 END → checkpoint 保存 typedState
    → Guard: fail → Branch → phase1

请求 1 (同一 Graph 执行内, Guard 重试):
  Phase1 Agent 重新进入:
    checkpoint 恢复 → typedState.Messages = [UserMsg("我要报销"), AssistantMsg("请上传票据")]
    ChatModel#2 输入 = 完整历史!
    → "请上传票据" (Guard 未满足)

请求 2: "已上传发票"
  新的 Graph 执行:
    Phase1 Agent 重新进入:
    ⚠️ Checkpoint ID 不同 (新的 Graph 执行)
    → typedState.Messages = [UserMsg("已上传发票")]
    → 需要从 SessionStore 恢复对话上下文
```

### 6.2 Checkpoint Key 设计

```
Key 格式: {GraphName}:{SessionID}:{NodeName}
示例:
  reimbursement_workflow:abc123:phase1_collect
  reimbursement_workflow:abc123:phase2_validate
```

同一个 Graph 执行内 (Guard 重试): 同一个 key, checkpoint 恢复 ✅  
不同 Graph 执行 (新 HTTP 请求): 不同 key (?), 不走 checkpoint, 从 SessionStore 恢复历史

---

## 7. 文件变更

### 7.1 新增

| 文件 | 说明 |
|------|------|
| `graph/phase_agent.go` | `NewPhase1Agent()` / `NewPhase2Agent()` / `NewPhase3Agent()` |
| `graph/phase_runner.go` | `runPhaseAgent()` — 包装 ChatModelAgent.Run() |
| `graph/reimbursement_v3.go` | `NewReimbursementGraph()` — 父图+Guard+Checkpoint |
| `graph/gob_register.go` | gob 注册 `phaseState` 等类型 |

### 7.2 修改

| 文件 | 变更 |
|------|------|
| `runner.go` | `StreamChat` 集成新 Graph |
| `checkpoint.go` | 实现 Eino 的 `compose.CheckPointStore` 接口 (已有, 确认兼容) |

### 7.3 不变

| 文件 | 原因 |
|------|------|
| `dto.go` | ReimbursementState 结构不变 |
| `phase/guard.go` | Guard 逻辑不变 |
| `prompt.go` | 系统提示词微调 |
| 所有 `tools/*.go` | 工具实现不变 (增加 ProcessState 写 ReimbursementState) |
| `sse.go` | SSE 事件格式不变 |
| `service.go` | HTTP 层不变 |

### 7.4 废弃 (保留但不再使用)

| 文件 | 替代 |
|------|------|
| `graph/react_phase.go` | ChatModelAgent 替代手工 ReAct |
| `graph/provider.go` | 简化，只保留 Wire 绑定 |

---

## 8. D1-D4 修复对照

| ID | 缺陷 | v3.0 修复方式 |
|----|------|--------------|
| D1 | phaseState 在 Guard 重试时清空 | Eino checkpoint: gob serialize → restore on re-entry |
| D2 | saveSessionState 无法读取嵌套图 State | 工具通过 ProcessState 直接写父图 ReimbursementState |
| D3 | 类型桥接 (*Message ↔ []*Message) | 父图统一使用 *Message 类型 |
| D4 | 工具事件未通过 SSE 发送 | ChatModelAgent 内置 event sender |
