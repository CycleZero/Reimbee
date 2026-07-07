# Agent 层 v3.0 — Eino 原生组件 + 显式流程控制

> **原则**：Eino 有的不自己写，Eino 没有的业务逻辑我们自己写  
> **流控方式**：PrepareAgent 根据 ReimbursementState 选择 Phase Agent（物理隔离工具集）

---

## 1. 架构全景

```
┌──────────────────────────────────────────────────────────────────┐
│ TurnLoop (Eino adk) — 多轮对话运行时                              │
│                                                                   │
│  Push(msg) ──→ GenInput ──→ PrepareAgent ──→ Agent.Run()         │
│                    │              │                  │            │
│              加载 State ←── SessionStore           AgentEvent     │
│              加载历史 ←── SessionStore                 │            │
│                                          ┌──────────────┤         │
│                                          ▼              ▼         │
│                                   OnAgentEvents    Checkpoint      │
│                                        │            (gob 自动)     │
│                                     SSE 输出                       │
└──────────────────────────────────────────────────────────────────┘
         │
         ▼  PrepareAgent 选哪个?
┌──────────────────────────────────────────────────────────────────┐
│                                                                   │
│  switch rs.CurrentPhase:                                          │
│  ┌──────────────────────────────────────────────────────────┐    │
│  │ Phase1 Agent                                              │    │
│  │ Tools: [recognize_invoice, check_compliance]              │    │
│  │ Instruction: "引导用户上传票据并确认"                       │    │
│  │ → LLM 物理上不知道 submit_reimbursement 的存在!            │    │
│  └──────────────────────────────────────────────────────────┘    │
│  ┌──────────────────────────────────────────────────────────┐    │
│  │ Phase2 Agent                                              │    │
│  │ Tools: [check_compliance, check_budget]                   │    │
│  │ Instruction: "执行合规+预算校验，请用户最终确认"             │    │
│  └──────────────────────────────────────────────────────────┘    │
│  ┌──────────────────────────────────────────────────────────┐    │
│  │ Phase3 Agent                                              │    │
│  │ Tools: [create_reimb, submit_reimb, generate_pdf, ...]   │    │
│  │ Instruction: "创建报销单→提交→PDF→邮件"                    │    │
│  └──────────────────────────────────────────────────────────┘    │
└──────────────────────────────────────────────────────────────────┘
         │
         ▼ 工具内部
┌──────────────────────────────────────────────────────────────────┐
│ ReimbursementState (共享业务状态, SessionStore 持久化)             │
│                                                                   │
│ 工具通过 compose.ProcessState 读写:                               │
│   OCR工具:    rs.Invoices = append(...), rs.TotalAmount += ...   │
│   合规工具:   rs.ComplianceResult = {...}                         │
│   预算工具:   rs.BudgetResult = {...}                             │
│                                                                   │
│ PrepareAgent 读取 rs.CurrentPhase 决定用哪个 Agent                 │
│ 阶段切换由工具隐式触发: OCR 填完票据 → UserConfirmed=true          │
│                       合规通过 → FinalConfirmed=true               │
│                       提交完成 → 单号写入 rs.ReimbursementNo       │
└──────────────────────────────────────────────────────────────────┘
```

## 2. 流控机制：PrepareAgent 选 Agent = 物理隔离

### 为什么不能用 BeforeModel？

```
BeforeModel("过滤掉 submit 工具")
  → LLM 调用 Generate(messages)
  → LLM 回复: "好的，我已提交报销单！"  ← 幻觉! LLM 看不到工具但能"假装"调用过
  → 用户以为提交了，实际没有
  → 无法阻止
```

### PrepareAgent 选 Agent：物理隔离

```
PrepareAgent 读到 rs.CurrentPhase="phase1_collect"
  → return phase1Agent  ← 这个 Agent 的工具集根本没有 submit_reimbursement
  → LLM 的 WithTools(toolInfos) 传入的 schema 中就没有 submit
  → LLM 不可能生成 submit_reimbursement 的 ToolCall
  → 物理上不可能跳过阶段
```

### 对比

| 方式 | 跳阶段 | LLM 幻觉 | 用户体验 |
|------|--------|---------|---------|
| BeforeModel 过滤工具 | 可能（LLM 假装调过） | 可能 | 困惑 |
| **PrepareAgent 选 Agent** | **物理不可能** | **不可能** | 正确引导 |

## 3. TurnLoop 完整实现

```go
// agent/loop.go — 新文件

type ReimbLoop struct {
    phase1Agent *adk.ChatModelAgent  // Phase 1: OCR + Compliance
    phase2Agent *adk.ChatModelAgent  // Phase 2: Compliance + Budget
    phase3Agent *adk.ChatModelAgent  // Phase 3: Create + Submit + PDF + Email
    chatAgent   *adk.ChatModelAgent  // 通用对话 (general_chat)
    // progressAgent, budgetAgent, policyAgent, modifyAgent...

    sessionStore infra.SessionStore
    sseWriter    SSEWriter
    logger       *log.Logger
}

func (l *ReimbLoop) Run(ctx context.Context, sessionID string, userMsg string) error {
    loop := adk.NewTurnLoop(adk.TurnLoopConfig[string, *schema.Message]{

        // ── GenInput: 加载 State + 历史 ──
        GenInput: func(ctx context.Context, _ *adk.TurnLoop[string, *schema.Message],
            items []string) (*adk.GenInputResult[string, *schema.Message], error) {

            // 加载对话历史
            history, _ := l.sessionStore.GetHistory(ctx, sessionID, 40)

            // 加载业务状态 (首次为空)
            var rs agent.ReimbursementState
            l.sessionStore.GetState(ctx, sessionID, "reimbursement", &rs)

            // 保存用户消息
            userMsg := schema.UserMessage(items[0])
            l.sessionStore.SaveMessages(ctx, sessionID, []*schema.Message{userMsg})

            // 构建消息列表
            msgs := append(history, userMsg)

            return &adk.GenInputResult[string, *schema.Message]{
                Input:    &adk.AgentInput{Messages: msgs, EnableStreaming: true},
                Consumed: items,
            }, nil
        },

        // ── PrepareAgent: 选 Phase Agent ──
        PrepareAgent: func(ctx context.Context, _ *adk.TurnLoop[string, *schema.Message],
            consumed []string) (adk.Agent, error) {

            // 读取当前业务状态
            var rs agent.ReimbursementState
            l.sessionStore.GetState(ctx, sessionID, "reimbursement", &rs)

            // 意图分类
            route := classifyIntent(consumed[0])
            l.logger.Debug("意图分类完成",
                zap.String("意图", route),
                zap.String("当前阶段", rs.CurrentPhase))

            switch route {
            case "new_reimbursement":
                return l.selectPhaseAgent(&rs), nil
            case "query_progress":
                return l.progressAgent, nil
            case "query_budget":
                return l.budgetAgent, nil
            case "policy_question":
                return l.policyAgent, nil
            default:
                return l.chatAgent, nil
            }
        },

        // ── OnAgentEvents: AgentEvent → SSE ──
        OnAgentEvents: func(ctx context.Context, tc *adk.TurnContext[string, *schema.Message],
            events *adk.AsyncIterator[*adk.AgentEvent]) error {

            var fullContent string
            for {
                event, ok := events.Next()
                if !ok { break }

                if event.Err != nil {
                    l.sseWriter.WriteEvent(NewErrorEvent(event.Err.Error(), false, "agent_error"))
                    return event.Err
                }

                if event.Output != nil && event.Output.MessageOutput != nil {
                    mv := event.Output.MessageOutput
                    switch mv.Role {
                    case schema.Assistant:
                        // LLM 文本输出 (流式)
                        if mv.IsStreaming {
                            // chunk by chunk
                        } else {
                            fullContent = mv.Message.Content
                            l.sseWriter.WriteEvent(NewMessageEvent(mv.Message.Content, false))
                        }
                    case schema.Tool:
                        // 工具执行结果
                        l.sseWriter.WriteEvent(NewToolResultEvent(mv.ToolName, mv.Message.Content))
                    }
                }
            }

            // 持久化 assistant 回复
            l.sessionStore.SaveMessages(ctx, sessionID,
                []*schema.Message{schema.AssistantMessage(fullContent, nil)})

            // 保存业务状态
            // ⚠️ 工具通过 ProcessState 更新了 ReimbursementState
            // 这里从 ChatModelAgent 的 State 无法读取 → 工具直接写 SessionStore
            l.saveReimbState(ctx, sessionID)

            l.sseWriter.WriteEvent(NewDoneEvent())
            l.sseWriter.Flush()
            return nil
        },

        Store:        l.checkpointStore,
        CheckpointID: sessionID,
    })

    loop.Push(userMsg)
    loop.Run(ctx)
    return nil
}

// ── selectPhaseAgent: 根据 ReimbursementState 选 Agent ──
func (l *ReimbLoop) selectPhaseAgent(rs *agent.ReimbursementState) adk.Agent {
    switch {
    case rs.ReimbursementNo != "":
        // 已提交 → 回到通用对话
        return l.chatAgent
    case rs.FinalConfirmed:
        // 用户已确认 → Phase 3: 执行提交
        return l.phase3Agent
    case rs.UserConfirmed:
        // 票据已确认 → Phase 2: 校验
        return l.phase2Agent
    default:
        // Phase 1: 收集
        return l.phase1Agent
    }
}
```

## 4. Phase Agent 创建

```go
// agent/phase_agents.go — 新文件

func (l *ReimbLoop) initPhaseAgents(ctx context.Context) error {
    // Phase 1: 信息收集
    l.phase1Agent, _ = adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
        Name:        "phase1_collect",
        Description: "收集票据信息并OCR识别",
        Instruction: agent.BuildSystemPrompt("phase1_collect", nil),
        Model:       l.chatModel,
        ToolsConfig: adk.ToolsConfig{
            ToolsNodeConfig: compose.ToolsNodeConfig{
                Tools: []tool.BaseTool{l.toolSet.OCR, l.toolSet.Compliance},
            },
        },
        MaxIterations: 10,
    })

    // Phase 2: 校验确认
    l.phase2Agent, _ = adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
        Name:        "phase2_validate",
        Description: "合规检查与预算校验",
        Instruction: agent.BuildSystemPrompt("phase2_validate", nil),
        Model:       l.chatModel,
        ToolsConfig: adk.ToolsConfig{
            ToolsNodeConfig: compose.ToolsNodeConfig{
                Tools: []tool.BaseTool{l.toolSet.Compliance, l.toolSet.Budget},
            },
        },
        MaxIterations: 10,
    })

    // Phase 3: 执行提交
    l.phase3Agent, _ = adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
        Name:        "phase3_execute",
        Description: "创建报销单并提交审批",
        Instruction: agent.BuildSystemPrompt("phase3_execute", nil),
        Model:       l.chatModel,
        ToolsConfig: adk.ToolsConfig{
            ToolsNodeConfig: compose.ToolsNodeConfig{
                Tools: []tool.BaseTool{
                    l.toolSet.CreateReimb, l.toolSet.SubmitReimb,
                    l.toolSet.PDF, l.toolSet.Email, l.toolSet.Progress,
                },
            },
        },
        MaxIterations: 10,
    })

    return nil
}
```

## 5. 阶段切换的正确路径

```
请求序列                         CurrentPhase     选中的 Agent       工具集
─────────────────────────────────────────────────────────────────────
"我要报销"                       phase1_collect    Phase1 Agent      [OCR, Compliance]
  → LLM: "请上传票据"                (不变)          (不变)            (不变)

"已上传 /uploads/t1.png"         phase1_collect    Phase1 Agent      [OCR, Compliance]
  → LLM调用OCR → 识别成功             (不变)          (不变)            (不变)
  → 工具写 rs.Invoices.append(...)                                          
  → LLM: "识别到 ¥500, 确认?"       (不变)          (不变)            (不变)

"确认"                           phase1_collect    Phase1 Agent      [OCR, Compliance]
  → LLM设置 UserConfirmed=true       (不变)          (不变)            (不变)
  → rs.UserConfirmed = true                                                  

"继续"                           phase2_validate   Phase2 Agent      [Compliance, Budget]
  → PrepareAgent 读到 UserConfirmed=true, Phase=phase2_validate
  → 返回 Phase2 Agent!
  → LLM调用 check_compliance → pass
  → LLM调用 check_budget → 余额5000
  → LLM: "合规通过, 预算充足, 确认提交?"

"确认提交"                         phase2_validate   Phase2 Agent      [Compliance, Budget]
  → LLM设置 FinalConfirmed=true      (不变)          (不变)            (不变)

"提交"                            phase3_execute    Phase3 Agent      [Create, Submit, PDF, Email]
  → PrepareAgent 读到 FinalConfirmed=true, Phase=phase3_execute
  → 返回 Phase3 Agent!
  → LLM调用 create → submit → pdf → email
  → rs.ReimbursementNo = "REIMB-2026-0001"

"查询进度"                         (通用)           chatAgent         []
  → PrepareAgent: rs.ReimbursementNo != "" → 选 chatAgent
```

## 6. 文件变更清单

### 新增

| 文件 | 说明 |
|------|------|
| `agent/loop.go` | `ReimbLoop` — TurnLoop 配置 + GenInput + PrepareAgent + OnAgentEvents |
| `agent/phase_agents.go` | `initPhaseAgents()` — 创建 3 个 ChatModelAgent + 4 个子流程 Agent |

### 修改

| 文件 | 变更 |
|------|------|
| `agent/runner.go` | 简化为 `ReimbLoop` 的包装 |
| `agent/config.go` | 去掉 Graph 相关配置, 保留 LLM/Session 配置 |

### 删除 (被 Eino 内置替代)

| 文件 | 替代 |
|------|------|
| `graph/react_phase.go` | `adk.ChatModelAgent` |
| `graph/reimbursement.go` | `PrepareAgent` 选 Agent |
| `graph/provider.go` | Wire 绑定简化 |
| `graph/root.go` | TurnLoop GenInput 中的意图分类 |
| `phase/guard.go` | `selectPhaseAgent()` 函数 |

### 保留

| 文件 | 原因 |
|------|------|
| `tools/*.go` (全部) | 报销业务专属 |
| `dto.go` | ReimbursementState |
| `sse.go` | 前后端协议 |
| `service.go` | HTTP 层 (微调) |
| `llm.go` | ChatModel 工厂 |
| `prompt.go` | 提示词模板 |
| `checkpoint.go` | MySQL CheckpointStore |
| `infra/session*.go` | SessionStore (Eino 不提供) |
| `graph/progress/budget/policy/modify.go` | 简单子流程 (可渐进迁移) |

## 7. Guard 逻辑去哪了？

| v2.1 Guard | v3.0 等价逻辑 | 位置 |
|-----------|-------------|------|
| `Phase1Guard`: ≥1票据 + 有金额 + 有类别 + UserConfirmed | `selectPhaseAgent`: `rs.UserConfirmed == true` → 切换到 Phase2 Agent | `loop.go:selectPhaseAgent()` |
| `Phase2Guard`: ComplianceResult≠nil + !error + FinalConfirmed | `selectPhaseAgent`: `rs.FinalConfirmed == true` → 切换到 Phase3 Agent | 同上 |
| `PhaseXTurns` 计数器 | ChatModelAgent 的 `MaxIterations` 配置 | `phase_agents.go` |
| 防死循环 | TurnLoop 的 CheckpointID + 重新 Push | Eino 内置 |

## 8. 代码量

| | v2.1 | v3.0 |
|---|---|---|
| Graph 层 | ~800 行 | 0 (被 Eino 替代) |
| Runner 层 | ~220 行 | ~80 行 (TurnLoop 配置) |
| Phase Agent 创建 | — | ~80 行 |
| 新增 | — | ~200 行 (loop.go + phase_agents.go) |
| 保留 | — | ~600 行 (tools + dto + sse + prompt + session + checkpoint) |
| **总计** | **~1600 行** | **~950 行 (-40%)** |
