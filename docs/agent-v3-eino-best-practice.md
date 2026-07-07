# Agent 层 v3.0 — Eino 最佳实践数据流重构

> **目标**: 按照 Eino 官方标准模式重新设计数据流，消除当前 4 个架构缺陷  
> **核心变更**: 从手动 Graph 编排 → ADK ChatModelAgent + BeforeModelRewriteState

---

## 1. 官方最佳实践的核心原则

### 1.1 来自 `adk/react.go` 的关键发现

```go
// Eino 官方 ReAct 图的数据流:
// 消息永远不通过边传递 → 始终存储在 State 中，通过 StatePreHandler 注入 ChatModel

typedState {
    Messages []Message  // ← 唯一消息来源！累积所有 user/assistant/tool 消息
    RemainingIterations int
    ToolInfos []*schema.ToolInfo
}

// Init 节点: 将入口消息追加到 State
g.AddLambdaNode("Init", compose.InvokableLambda(func(ctx, input) {
    st.Messages = append(st.Messages, input.Messages...)
    return input.Messages, nil
}))

// ChatModel 节点: 从 State 读取完整消息列表
g.AddChatModelNode("ChatModel", model,
    compose.WithStatePreHandler(func(ctx, input, st) {
        return st.Messages, nil  // ← 直接返回 State 中的消息列表
    }))

// AfterToolCalls 节点: 工具结果追加回 State
g.AddLambdaNode("AfterToolCalls", compose.InvokableLambda(func(ctx, toolResults) {
    st.Messages = append(st.Messages, toolResults...)
    return toolResults, nil
}))
```

### 1.2 原则对照

| 原则 | 官方做法 | 我们的当前做法 | 问题 |
|------|---------|---------------|------|
| 消息存储 | 集中在 `State.Messages` | 分散在 `phaseState`(子图) + Guard输出(父图) | D1: Guard重试丢失 |
| 消息传递 | 只通过 State，不通过边 | 通过边传递 `[]*Message` | D3: 类型桥接 |
| 状态持久化 | ChatModelAgent 内置 gob checkpoint | 手动 SessionStore, runner 层 ProcessState 读不到 | D2: 持久化失败 |
| 工具事件 | 内置 event sender middleware | 无 | D4: 无 tool_call/tool_result SSE |

---

## 2. v3.0 架构设计

### 2.1 整体结构

```
AgentRunner.StreamChat()
    │
    ▼
adk.NewRunner(ctx, RunnerConfig{
    Agent:      reimbAgent,     // ← 单个 ChatModelAgent, 持有全部 9 个工具
    EnableStreaming: true,
    CheckPointStore: checkpointStore,  // ← 自动持久化 State
})
    │
    ▼
adk.ChatModelAgent {
    Instruction: BuildReimbInstruction()   // ← 系统提示词 (含三阶段规则)
    Model:       chatModel
    ToolsConfig: {
        Tools: [OCR, Compliance, Budget, CreateReimb, SubmitReimb, PDF, Email, Progress, Query]
    }
    MaxIterations: 30
    Handlers: [
        phaseToolFilter,    // ← BeforeModelRewriteState: 动态过滤工具列表
        stateBridge,        // ← BeforeModel: 注入 SessionStore 中的 ReimbursementState
    ]
}
    │
    ▼
Agent 内部的 ReAct 循环 (adk/react.go):
    Init → ChatModel ⇄ ToolsNode → AfterToolCalls
              ↑                      │
              └──────── 循环 ────────┘
    所有消息累积在 typedState.Messages 中
    状态通过 gob checkpoint 自动持久化
```

### 2.2 关键组件

#### 2.2.1 系统提示词（替代 Graph Guard）

```go
func BuildReimbInstruction() string {
    return `你是 Reimbee，企业财务报销智能助手。

## 核心流程（你必须引导用户逐阶段完成）

### 阶段 1: 信息收集
- 引导用户上传票据图片
- 用户上传后，调用 recognize_invoice 进行 OCR 识别
- 展示识别结果并请用户确认
- 用户可以继续添加更多票据
- 所有票据确认后，汇总展示总金额
- ⚠️ 在本阶段完成前，不要调用 create_reimbursement 或 submit_reimbursement

### 阶段 2: 校验确认
- 调用 check_compliance 对每张票据执行合规检查
- 调用 check_budget 检查部门预算余额
- 展示检查结果，请用户最终确认
- ⚠️ 用户说"确认提交"之前，不要调用 create_reimbursement

### 阶段 3: 执行提交
- 用户确认后，严格按顺序调用:
  1. create_reimbursement
  2. submit_reimbursement
  3. generate_pdf
  4. send_email
- 告知用户报销单号和后续步骤

## 行为规范
- 一次一步，每次只引导一个操作
- 涉及金额必须让用户明确确认
- 如果信息不足，向用户提问，不要猜测`
}
```

#### 2.2.2 BeforeModelRewriteState: 动态工具过滤

```go
// phaseToolFilter 根据对话上下文决定当前阶段可见的工具
// 替代了 Graph 模式的 Phase-by-Phase 工具隔离
var phaseToolFilter = &PhaseToolFilterMiddleware{}

type PhaseToolFilterMiddleware struct {
    *adk.BaseChatModelAgentMiddleware
}

func (m *PhaseToolFilterMiddleware) BeforeModelRewriteState(
    ctx context.Context,
    state *adk.ChatModelAgentState,
    mc *adk.ModelContext,
) (context.Context, *adk.ChatModelAgentState, error) {
    // 分析对话历史，推断当前阶段
    phase := inferPhase(state.Messages)

    // 根据阶段过滤工具
    state.ToolInfos = filterToolsByPhase(state.ToolInfos, phase)

    return ctx, state, nil
}

func inferPhase(messages []*schema.Message) string {
    // 简单启发式: 搜索消息中的关键字
    // 如果已调用过 submit_reimbursement → phase3
    // 如果已调用过 check_compliance → phase2
    // 否则 → phase1
    for _, msg := range messages {
        if msg.Role == schema.Tool && msg.ToolName == "submit_reimbursement" {
            return "phase3"
        }
    }
    for _, msg := range messages {
        if msg.Role == schema.Tool && (msg.ToolName == "check_compliance" || msg.ToolName == "check_budget") {
            return "phase2"
        }
    }
    return "phase1"
}

func filterToolsByPhase(infos []*schema.ToolInfo, phase string) []*schema.ToolInfo {
    switch phase {
    case "phase1":
        return keepTools(infos, "recognize_invoice", "check_compliance")
    case "phase2":
        return keepTools(infos, "check_compliance", "check_budget")
    case "phase3":
        return keepTools(infos, "create_reimbursement", "submit_reimbursement", "generate_pdf", "send_email")
    }
    return infos
}
```

#### 2.2.3 BeforeAgent: 注入 SessionStore 中的 State

```go
// stateBridge 在 Agent 启动时从 SessionStore 恢复 ReimbursementState，
// 并将关键信息注入到系统提示词中
var stateBridge = &StateBridgeMiddleware{}

type StateBridgeMiddleware struct {
    *adk.BaseChatModelAgentMiddleware
    store infra.SessionStore
}

func (m *StateBridgeMiddleware) BeforeAgent(
    ctx context.Context,
    runCtx *adk.ChatModelAgentContext,
) (context.Context, *adk.ChatModelAgentContext, error) {
    // 从 context 读取 sessionID（由 Runner 层注入）
    sessionID := getSessionID(ctx)

    var state agent.ReimbursementState
    found, _ := m.store.GetState(ctx, sessionID, "reimbursement", &state)
    if found {
        // 将业务状态摘要注入系统提示词
        summary := agent.BuildStateSummary(&state)
        runCtx.Instruction += "\n\n## 当前报销上下文\n" + summary
    }
    return ctx, runCtx, nil
}
```

### 2.3 AgentRunner 简化

```go
// 使用 ADK Runner 替代手动 StreamChat 流程
func (r *AgentRunner) StreamChat(ctx context.Context, input AgentInput, sseWriter SSEWriter) error {
    // 1. 加载会话历史（ADK runner 需要外部管理历史）
    history, _ := r.sessionStore.GetHistory(ctx, input.SessionID, r.config.MaxHistoryTurns*2)

    // 2. 构建消息列表
    messages := make([]*schema.Message, 0, len(history)+1)
    messages = append(messages, history...)
    messages = append(messages, schema.UserMessage(input.Message))

    // 3. 创建 ADK Runner（内置流式 + checkpoint）
    adkRunner := adk.NewRunner(ctx, adk.RunnerConfig{
        Agent:           r.reimbAgent,
        EnableStreaming: true,
        CheckPointStore: r.checkpoint, // ← 自动持久化 State
    })

    // 4. 执行
    iter := adkRunner.Query(ctx, messages)

    // 5. 消费事件流 → SSE
    for {
        event, ok := iter.Next()
        if !ok { break }
        switch {
        case event.Err != nil:
            _ = sseWriter.WriteEvent(NewErrorEvent(...))
        case event.Output != nil:
            handleAgentEvent(sseWriter, event.Output)
        }
    }

    // 6. 持久化消息 + done
    _ = sseWriter.WriteEvent(NewDoneEvent())
    _ = sseWriter.Flush()
    return nil
}
```

### 2.4 消息历史的多轮管理

Eino 官方 Quick Start Ch2 的模式：调用方维护历史。

```go
// 每次请求:
history = append(history, msgops.NewUser(用户消息))
events := runner.Run(ctx, msgops.NormalizeMessagesForModelInput(history))
result := helpers.PrintAndCollect(events, ...)
history = append(history, msgops.NewAssistant(assistantText, toolCalls))
// 保存到 SessionStore
```

---

## 3. 数据流对比

### 3.1 当前 v2.1 数据流（4 个缺陷）

```
HTTP → Service → Runner → Root Graph dispatcher
                          │ AgentInput → *Message
                          │
                          Reimb Graph (父图) [][]*Message → *Message]
                            │
                            Phase1 ReAct 子图 [][]*Message → *Message]
                            │  ⚠️ phaseState.Messages: 只在单次入口存活
                            │     Guard fail → 重新进入 → Messages 清空 (D1)
                            │
                            Guard Lambda [][]*Message → []*Message]
                            │  ⚠️ 边传递消息数组 (D3)
                            │
                            Phase2 ReAct 子图 (重复 D1)
                            │
                            RUNNER 层 ProcessState 读取 ReimbursementState
                            │  ⚠️ 读取嵌套图状态 → 失败 (D2)
                            │
                            SSE 输出 → 无 tool_call/tool_result 事件 (D4)
```

### 3.2 v3.0 数据流（0 个缺陷）

```
HTTP → Service → Runner → ADK ChatModelAgent
                            │ Instruction + ToolsConfig
                            │
                            Agent 内部 State (typedState):
                            │ Messages: []Message  ← 唯一消息来源!
                            │ RemainingIterations: int
                            │
                            Init → ChatModel → Branch(ToolCalls?)
                            │         ↑              │
                            │         │              ├─ YES → ToolsNode
                            │         │              │         │
                            │         │              │    AfterToolCalls
                            │         │              │    → append to State.Messages
                            │         │              │         │
                            │         └── 循环 ─────┘         │
                            │                                 │
                            │         ├─ NO → END (文本回复)   │
                            │
                            └─ 所有消息在 State.Messages 中
                               从不丢失 (D1 ✅)
                               
                            ┌─ BeforeModelRewriteState: 动态过滤工具 (替代 Guard)
                            │  → State.ToolInfos 被过滤
                            │
                            ┌─ BeforeAgent: 注入 SessionStore 中的业务状态
                            │  → runCtx.Instruction += 业务上下文
                            │
                            ┌─ gob checkpoint: 自动持久化 State (D2 ✅)
                            │
                            ┌─ 内置 event sender: tool_call/tool_result SSE (D4 ✅)
                            │
                            └─ 无嵌套图 → 无类型桥接问题 (D3 ✅)
```

---

## 4. 需要修改的文件

### 4.1 新增

| 文件 | 说明 |
|------|------|
| `internal/domain/agent/react_agent.go` | `NewReimbAgent()` — 创建 ChatModelAgent 实例 |
| `internal/domain/agent/middleware_phase_filter.go` | `PhaseToolFilterMiddleware` — 动态工具过滤 |
| `internal/domain/agent/middleware_state_bridge.go` | `StateBridgeMiddleware` — 业务状态注入 |

### 4.2 修改

| 文件 | 变更 |
|------|------|
| `internal/domain/agent/runner.go` | `StreamChat` 使用 ADK Runner |
| `internal/domain/agent/config.go` | 增加 `UseADKAgent bool` 开关 |
| `internal/domain/agent/prompt.go` | 新增 `BuildReimbInstruction()` |

### 4.3 删除/废弃

| 文件 | 原因 |
|------|------|
| `graph/root.go` | ADK ChatModelAgent 替代 dispatcher |
| `graph/reimbursement.go` | ChatModelAgent 替代手工 Graph |
| `graph/react_phase.go` | 不再需要手动 ReAct 构建器 |
| `graph/provider.go` | Graph 组装移到 react_agent.go |
| `phase/guard.go` | Guard 由 BeforeModelRewriteState 替代 |
| `tools/provider.go` 中的阶段分组 | 所有工具统一提供 |

### 4.4 保留

| 文件 | 原因 |
|------|------|
| `service.go` | HTTP 层不变 |
| `sse.go` | SSE 事件格式不变 |
| `dto.go` | ReimbursementState 结构保留（传递给 SessionStore） |
| `config.go` | 配置加载保留 |
| `llm.go` | ChatModel 工厂保留 |
| `checkpoint.go` | Checkpoint 保留（传给 ADK Runner） |
| 所有 `tools/*.go` | 工具实现不变 |
| `prompt.go` | 意图分类 prompt 保留（root dispatcher 的意图分类逻辑） |

---

## 5. 消息流详细走读

```
请求 1: "我要报销差旅费"

① Service: AgentInput{SessionID:"abc", Message:"我要报销", ...}
② Runner: history = GetHistory("abc") → [空]
③ Runner: messages = [UserMsg("我要报销")]
④ Runner: adkRunner.Query(ctx, messages)
⑤ ChatModelAgent.Run():
    Init: st.Messages = [UserMsg("我要报销")]
    ChatModel#1: input=st.Messages=[SystemPrompt+UserMsg]
                 output="请上传票据图片"
    → 无 ToolCall → END
⑥ Runner 消费事件流:
    MessageOutput.IsStreaming=true
    chunk-by-chunk → SSE: message(delta=true)
⑦ Runner: history = [UserMsg, AssistantMsg("请上传票据")]
⑧ SessionStore.SaveMessages(history)
⑨ ⚠️ ChatModelAgent 的 gob checkpoint 自动保存 State

──────────────────────────────────────

请求 2: "已上传 /uploads/t1.png"

① Service: AgentInput{SessionID:"abc", Message:"已上传 /uploads/t1.png"}
② Runner: history = GetHistory("abc") → [UserMsg, AssistantMsg]
③ Runner: messages = [UserMsg, AssistantMsg, UserMsg("已上传...")]
④ Runner: adkRunner.Query(ctx, messages)
⑤ ChatModelAgent.Run():
    Init: st.Messages = [UserMsg, AssistantMsg, UserMsg("已上传...")]
    ChatModel#1: SystemPrompt + 完整历史
                 output = {ToolCalls:[{recognize_invoice}]}
    → Branch: ToolCalls 非空 → ToolsNode
    ToolsNode: 执行 recognize_invoice → "识别成功: 差旅-交通 ¥500"
    AfterToolCalls: st.Messages.append(toolResult)
    ChatModel#2: SystemPrompt + 完整历史(含tool结果)
                 output = "已识别票据: 差旅-交通 ¥500.00。请确认。"
    → 无 ToolCall → END ← BeforeModelRewriteState 会过滤掉 phase3 工具!
⑥ SSE: tool_call → tool_result → message(流式) → done
⑦ SessionStore 保存消息 → 下一次请求继续
```

---

## 6. 优势汇总

| 维度 | v2.1 (Graph) | v3.0 (ChatModelAgent) |
|------|-------------|----------------------|
| 消息历史管理 | 手动，多层级分散 | 框架内置，单一 State.Messages |
| 跨请求持久化 | 手动 SessionStore，有 bug | 内置 gob checkpoint |
| 工具事件 SSE | 无 | 内置 event sender |
| 中断/恢复 | 无 | 内置 checkpoint/resume |
| 重试/故障转移 | 无 | 内置 ModelRetry/ModelFailover |
| 上下文压缩 | 无 | Summarization middleware 可用 |
| 工具动态加载 | 无 | ToolSearch middleware 可用 |
| 代码量 | ~2000行 (graph层) | ~300行 (配置+middleware) |
| 配置切换 | 无 | `agent.use_adk: true` |
