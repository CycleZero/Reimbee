# Agent 层 v2 重构设计文档

> **状态**: 待审核  
> **日期**: 2026-07-06  
> **作者**: Sisyphus  
> **目标**: 完全重写报销子流程，修复工具执行 Bug，引入 Eino 标准 ReAct 模式

---

## 目录

1. [执行摘要](#1-执行摘要)
2. [当前架构问题清单](#2-当前架构问题清单)
3. [目标架构总览](#3-目标架构总览)
4. [Phase I — 工具层重构](#4-phase-i--工具层重构)
5. [Phase II — 报销子流程重写（ReAct 模式）](#5-phase-ii--报销子流程重写react-模式)
6. [Phase III — 根路由适配](#6-phase-iii--根路由适配)
7. [Phase IV — 简单子流程修复](#7-phase-iv--简单子流程修复)
8. [Phase V — Runner 层流式适配](#8-phase-v--runner-层流式适配)
9. [数据流全景](#9-数据流全景)
10. [文件变更清单](#10-文件变更清单)
11. [迁移风险与缓解](#11-迁移风险与缓解)

---

## 1. 执行摘要

当前 Agent 层的核心问题：**LLM 返回的 ToolCalls 从未被执行**。根因是使用了 `InvokableLambda` 内手动调用 `chatModel.Generate(WithTools())` 的方式，而非 Eino 框架的 `AddChatModelNode + AddToolsNode + AddBranch` 标准模式。

本次重构采用 **Eino 标准 ReAct 图模式**（基于官方 `flow/agent/react/react.go` 实现）重写报销子流程，同时修复所有已识别 Bug。

### 关键指标

| 指标 | 修复前 | 修复后 |
|------|--------|--------|
| 工具执行 | ❌ 从不执行 | ✅ ToolsNode 自动执行 |
| 真正的流式输出 | ❌ Invoke fallback | ✅ ChatModelNode 原生 Stream |
| 意图分类阈值 | ❌ 硬编码 0.7 | ✅ 读取 AgentConfig |
| Phase 计数器 | ❌ Phase1Turns 在 Phase2 中递增 | ✅ 各自独立 |
| User 元数据传递 | ❌ adapter 丢失全部字段 | ✅ 通过 State 传递 |

---

## 2. 当前架构问题清单

### P0 — 阻塞级

| ID | 文件 | 问题 | 影响 |
|----|------|------|------|
| **B2** | `graph/reimbursement.go:100` | LLM 返回的 ToolCalls 从未执行——缺少 ToolsNode + Branch 循环 | 报销流程完全不可用 |
| **B1** | `graph/provider.go:94-97` | `agentInputAdapter` 丢弃 SessionID/UserID/EmployeeID/Role | 所有子流程缺少用户上下文 |

### P1 — 高级

| ID | 文件 | 问题 | 影响 |
|----|------|------|------|
| **B6** | `graph/root.go:42` | `InvokableLambda` 不支持 Stream，真流式是死代码 | 前端收到完整回复而非 token-by-token |
| **B5** | `graph/reimbursement.go:131-150` | Phase 3 无验证循环 | 执行不完整无法检测 |

### P2 — 中级

| ID | 文件 | 问题 |
|----|------|------|
| **B3** | `graph/reimbursement.go:113` | Phase1Turns 在所有 Phase 中被递增 |
| **B4** | `graph/root.go:158` | classifyIntent 硬编码阈值 0.7 |
| **B7** | `graph/root.go:73` | dispatchToWorkflow 只传 msg.Content |

### P3 — 低级

| ID | 文件 | 问题 |
|----|------|------|
| **B8** | `graph/root.go:203-208` | `truncate()` 按字节截断中文 |
| **B9** | `tools/email_tool.go:70` | `context.Background()` 被 `%d` 格式化 |
| **B10** | `config.yaml` | `${}` 语法 Viper 不解析 |

---

## 3. 目标架构总览

```
                           ┌──────────────────────────────┐
                           │     HTTP SSE Request          │
                           │  GET /api/chat/stream         │
                           └─────────────┬────────────────┘
                                         │
                           ┌─────────────▼────────────────┐
                           │  AgentService.HandleChat()    │
                           │  → 解析参数 → 提取JWT claims  │
                           │  → 创建GinSSEWriter           │
                           └─────────────┬────────────────┘
                                         │
                           ┌─────────────▼────────────────┐
                           │  AgentRunner.StreamChat()     │
                           │  → 加载Session历史             │
                           │  → 持久化用户消息               │
                           │  → rootGraph.Stream(input)    │ ← 现在真正流式!
                           └─────────────┬────────────────┘
                                         │
                           ┌─────────────▼────────────────┐
                           │  Root Graph                   │
                           │  [AgentInput → *Message]      │
                           │                               │
                           │  dispatcher (StreamableLambda)│
                           │   ├─ classifyIntent()         │
                           │   └─ switch(route):           │
                           │       ├─ new_reimbursement    │
                           │       │    → NewReimbGraph    │ ← 完全重写
                           │       ├─ query_progress       │
                           │       │    → ProgressAgent    │ ← 简单修复
                           │       ├─ query_budget         │
                           │       │    → BudgetAgent      │ ← 简单修复
                           │       ├─ policy_question      │
                           │       │    → PolicyAgent      │ ← 简单修复
                           │       ├─ modify_reimbursement │
                           │       │    → ModifyAgent      │ ← 简单修复
                           │       └─ general_chat         │
                           │            → ChatModel 直出   │ ← 不变
                           └──────────────────────────────┘
                                         │
         ┌───────────────────────────────┼───────────────────────────────┐
         │                               │                               │
         ▼                               ▼                               ▼
┌─────────────────────┐    ┌─────────────────────┐    ┌─────────────────────┐
│ Reimbursement Graph  │    │ Progress/Budget/     │    │ General Chat        │
│ (ReAct 三阶段)       │    │ Policy/Modify Graphs │    │ (ChatModel 直出)    │
│                     │    │ (保留现有模式+修复)    │    │                     │
│ START               │    │                     │    │                     │
│  │                  │    │ START                │    │                     │
│  ▼                  │    │  │                   │    │                     │
│ Phase1:             │    │  ▼                   │    │                     │
│  ChatModel ←──┐     │    │ build_prompt(λ)      │    │                     │
│    │ Branch   │     │    │  │                   │    │                     │
│    ▼ (tools?)│     │    │  ▼                   │    │                     │
│  ToolsNode ──┘     │    │ ChatModel             │    │                     │
│    │                │    │  │                   │    │                     │
│  Phase1Guard(λ)     │    │  ▼                   │    │                     │
│  ├─ pass → Phase2   │    │ END                  │    │                     │
│  └─ fail → Phase1   │    └─────────────────────┘    └─────────────────────┘
│                     │
│ Phase2: (同 Phase1) │
│  ChatModel ←──┐     │
│    │ Branch    │     │
│    ▼           │     │
│  ToolsNode ───┘     │
│    │                │
│  Phase2Guard(λ)     │
│  ├─ pass → Phase3   │
│  └─ fail → Phase2   │
│                     │
│ Phase3: (同 Phase1) │
│  ChatModel ←──┐     │
│    │ Branch    │     │
│    ▼           │     │
│  ToolsNode ───┘     │
│    │                │
│  END                │
└─────────────────────┘

共享状态: ReimbursementState (via compose.WithGenLocalState)
  ├─ SessionID, UserID, EmployeeID, Role  ← 从 AgentInput 注入
  ├─ Invoices, TotalAmount, UserConfirmed  ← Phase1 填充
  ├─ ComplianceResult, BudgetResult        ← Phase2 填充
  ├─ FinalConfirmed, NeedSpecialApproval   ← Phase2 填充
  └─ Phase1Turns, Phase2Turns, Phase3Turns ← 各自独立递增
```

### 核心变化

| 层级 | 旧方式 | 新方式 |
|------|--------|--------|
| 子图编译 | 全部在 `graph/provider.go` 中 | 各子图独立编译，通过 `NewXxxGraph()` |
| Phase 节点 | `InvokableLambda` + 手动 for 循环 | `AddChatModelNode` + `AddToolsNode` + `AddBranch` |
| 工具执行 | ❌ `chatModel.Generate(WithTools())` | ✅ `AddToolsNode` 自动执行 ToolCalls |
| Phase 循环 | 手动 for (turn < maxTurns) | Eino 图引擎驱动的 ReAct 循环 (AddBranch) |
| Guard 检查 | `compose.ProcessState` 在 Lambda 内 | `AddBranch` 条件 + Guard Lambda 节点 |
| 流式输出 | `InvokableLambda` (退化为 Invoke) | `AddChatModelNode` (原生 Stream) |
| 用户上下文 | `agentInputAdapter` (丢失) | `WithGenLocalState` (全量保持) |

---

## 4. Phase I — 工具层重构

### 4.1 目标

当前 `tools/` 包的工具通过 `ToolSet.GetPhaseXTools()` 返回 `[]tool.InvokableTool`。需要新增方法返回 `[]tool.BaseTool`（兼容 `compose.ToolsNodeConfig.Tools`）。

### 4.2 变更

**文件**: `internal/domain/agent/tools/provider.go`

```go
// 新增方法——为每个 Phase 返回 []tool.BaseTool（用于 AddToolsNode）
func (ts *ToolSet) GetPhase1BaseTools() []tool.BaseTool {
    return []tool.BaseTool{ts.OCR, ts.Compliance}
}

func (ts *ToolSet) GetPhase2BaseTools() []tool.BaseTool {
    return []tool.BaseTool{ts.Compliance, ts.Budget}
}

func (ts *ToolSet) GetPhase3BaseTools() []tool.BaseTool {
    return []tool.BaseTool{ts.PDF, ts.Email, ts.Progress}
}

func (ts *ToolSet) GetProgressTools() []tool.BaseTool {
    return []tool.BaseTool{ts.Progress, ts.QueryRecords}
}

func (ts *ToolSet) GetBudgetTools() []tool.BaseTool {
    return []tool.BaseTool{ts.Budget}
}
```

### 4.3 不变部分

- `ToolSet` 结构体不变
- 各工具实现文件的 `NewOCRTool` 等函数不变
- `Wire ProviderSet` 不变

---

## 5. Phase II — 报销子流程重写（ReAct 模式）

### 5.1 设计目标

将 `graph/reimbursement.go` 从手动 Lambda 循环完全重写为 **Eino 标准 ReAct 图**。

### 5.2 图拓扑

```
                        ┌──────────────────────────────────────────┐
                        │     NewReimbursementGraph()               │
                        │     Type: [*schema.Message → *schema.Message] │
                        │     State: *agent.ReimbursementState     │
                        └──────────────────────────────────────────┘

    START
      │
      ▼
┌──────────────┐      ┌─────────────────────────────────┐
│ phase1_chat  │◄─────│ ToolsNode 返回结果后回到 ChatModel │
│ (ChatModel)  │      └─────────────────────────────────┘
│              │
│ Branch: has  │── YES ──► ┌───────────────┐
│ ToolCalls?   │           │ phase1_tools   │
│              │           │ (ToolsNode)    │
│     NO       │           │ Tools: OCR,    │
│     │        │           │   Compliance   │
│     ▼        │           └───────┬───────┘
│ phase1_guard │                   │
│ (Lambda)     │         Branch: always back to
│              │         phase1_chat (loop!)
│ Pass? ── YES ──► Phase2
│  │
│  NO ──► phase1_chat (retry)
│
▼
┌──────────────┐      ┌─────────────────────────────────┐
│ phase2_chat  │◄─────│ ToolsNode 返回结果后回到 ChatModel │
│ (ChatModel)  │      └─────────────────────────────────┘
│              │
│ Branch: has  │── YES ──► ┌───────────────┐
│ ToolCalls?   │           │ phase2_tools   │
│              │           │ (ToolsNode)    │
│     NO       │           │ Tools:         │
│     │        │           │   Compliance,  │
│     ▼        │           │   Budget       │
│ phase2_guard │           └───────┬───────┘
│ (Lambda)     │                   │
│              │         Branch: always back to
│ Pass? ── YES ──► Phase3
│  │
│  NO ──► phase2_chat (retry)
│
▼
┌──────────────┐      ┌─────────────────────────────────┐
│ phase3_chat  │◄─────│ ToolsNode 返回结果后回到 ChatModel │
│ (ChatModel)  │      └─────────────────────────────────┘
│              │
│ Branch: has  │── YES ──► ┌───────────────┐
│ ToolCalls?   │           │ phase3_tools   │
│              │           │ (ToolsNode)    │
│     NO       │           │ Tools:         │
│     │        │           │   PDF, Email,  │
│     ▼        │           │   Progress     │
│     END      │           └───────────────┘
└──────────────┘
```

### 5.3 关键代码结构

```go
// === internal/domain/agent/graph/reimbursement.go (新版) ===

// 阶段状态类型——用于 accumulate message history
type phaseState struct {
    Messages []*schema.Message
}

// PhaseConfig 单个阶段的配置参数
type PhaseConfig struct {
    PhaseName   string          // "phase1_collect" | "phase2_validate" | "phase3_execute"
    SystemPrompt string         // 系统提示词
    Tools       []tool.BaseTool // 该阶段可用工具
    MaxTurns    int             // 最大 ReAct 循环轮次（即 tools 调用次数）
}

// buildReActPhase 构建单个阶段的 ReAct 子图
// 图类型: [*schema.Message, *schema.Message]
// 内部拓扑: ChatModel → (Branch: ToolCalls?) → ToolsNode → (loop back to ChatModel) → END
func buildReActPhase(
    ctx context.Context,
    chatModel model.ToolCallingChatModel,
    logger *log.Logger,
    config PhaseConfig,
    guardFn func(*agent.ReimbursementState) *agent.GuardResult,
) (*compose.Graph[*schema.Message, *schema.Message], error) {

    // 获取工具信息用于绑定到 ChatModel
    toolInfos := make([]*schema.ToolInfo, 0, len(config.Tools))
    for _, t := range config.Tools {
        info, err := t.Info(ctx)
        if err != nil {
            return nil, fmt.Errorf("获取工具信息失败(%s): %w", t.Info, err)
        }
        toolInfos = append(toolInfos, info)
    }

    // 绑定工具到模型（关键！工具不绑定，LLM 不知道有哪些工具可用）
    chatModelWithTools, err := chatModel.WithTools(toolInfos)
    if err != nil {
        return nil, fmt.Errorf("绑定工具到ChatModel失败(%s): %w", config.PhaseName, err)
    }

    // 创建 ToolsNode——Eino 的官方工具执行器
    toolsNode, err := compose.NewToolNode(ctx, &compose.ToolsNodeConfig{
        Tools: config.Tools,
    })
    if err != nil {
        return nil, fmt.Errorf("创建ToolsNode失败(%s): %w", config.PhaseName, err)
    }

    // 创建图，带状态以累积消息历史
    g := compose.NewGraph[*schema.Message, *schema.Message](
        compose.WithGenLocalState(func(ctx context.Context) *phaseState {
            return &phaseState{Messages: make([]*schema.Message, 0, 20)}
        }),
    )

    // === ChatModel 节点 ===
    // 通过 StatePreHandler 注入系统提示词并累积消息历史
    modelPreHandle := func(ctx context.Context, input *schema.Message, ps *phaseState) ([]*schema.Message, error) {
        ps.Messages = append(ps.Messages, input)
        // 构建消息列表: 系统提示词 + 历史消息
        msgs := make([]*schema.Message, 0, len(ps.Messages)+1)
        msgs = append(msgs, schema.SystemMessage(config.SystemPrompt))
        msgs = append(msgs, ps.Messages...)
        return msgs, nil
    }
    g.AddChatModelNode("chat", chatModelWithTools,
        compose.WithStatePreHandler(modelPreHandle),
        compose.WithNodeName(config.PhaseName+"_chat"),
    )

    // === ToolsNode 节点 ===
    // 通过 StatePreHandler 将 assistant 的 tool_call 消息也累积到历史
    toolsPreHandle := func(ctx context.Context, input *schema.Message, ps *phaseState) (*schema.Message, error) {
        ps.Messages = append(ps.Messages, input) // assistant 的 tool_call 消息
        return input, nil
    }
    g.AddToolsNode("tools", toolsNode,
        compose.WithStatePreHandler(toolsPreHandle),
        compose.WithNodeName(config.PhaseName+"_tools"),
    )

    // === ChatModel → Branch（检测 ToolCalls） ===
    // 这是 ReAct 循环的核心：如果 LLM 输出包含 ToolCalls → 执行工具
    // 否则 → 结束当前阶段（返回最后的消息）
    g.AddBranch("chat", compose.NewStreamGraphBranch(
        func(ctx context.Context, sr *schema.StreamReader[*schema.Message]) (endNode string, err error) {
            defer sr.Close()
            for {
                msg, err := sr.Recv()
                if err == io.EOF {
                    return compose.END, nil
                }
                if err != nil {
                    return "", err
                }
                if len(msg.ToolCalls) > 0 {
                    return "tools", nil // ← 有工具调用，执行工具
                }
                if msg.Content != "" {
                    return compose.END, nil // ← 文本回复，阶段结束
                }
            }
        },
        map[string]bool{"tools": true, compose.END: true},
    ))

    // === ToolsNode → Branch（循环回 ChatModel） ===
    // 工具执行完成后，结果自动注入到消息历史，
    // 然后回到 ChatModel 继续推理
    g.AddBranch("tools", compose.NewStreamGraphBranch(
        func(ctx context.Context, sr *schema.StreamReader[[]*schema.Message]) (endNode string, err error) {
            sr.Close()
            return "chat", nil // ← 始终回到 ChatModel
        },
        map[string]bool{"chat": true},
    ))

    return g, nil
}

// NewReimbursementGraph 构建完整的三阶段报销子流程
// 图类型: [*schema.Message, *schema.Message]
// 父图类型: compose.Graph，共享 ReimbursementState
func NewReimbursementGraph(
    ctx context.Context,
    deps ReimbursementGraphDeps,
) (compose.Runnable[*schema.Message, *schema.Message], error) {

    // ── 构建三个阶段的 ReAct 子图 ──
    phase1Graph, err := buildReActPhase(ctx, deps.ChatModel, deps.Logger, PhaseConfig{
        PhaseName:    "phase1_collect",
        SystemPrompt: agent.BuildSystemPrompt("phase1_collect", nil),
        Tools:        deps.ToolSet.GetPhase1BaseTools(),
        MaxTurns:     deps.Config.MaxPhaseTurns,
    }, phase.Phase1Guard)
    if err != nil {
        return nil, err
    }

    phase2Graph, err := buildReActPhase(ctx, deps.ChatModel, deps.Logger, PhaseConfig{
        PhaseName:    "phase2_validate",
        SystemPrompt: agent.BuildSystemPrompt("phase2_validate", nil),
        Tools:        deps.ToolSet.GetPhase2BaseTools(),
        MaxTurns:     deps.Config.MaxPhaseTurns,
    }, phase.Phase2Guard)
    if err != nil {
        return nil, err
    }

    phase3Graph, err := buildReActPhase(ctx, deps.ChatModel, deps.Logger, PhaseConfig{
        PhaseName:    "phase3_execute",
        SystemPrompt: agent.BuildSystemPrompt("phase3_execute", nil),
        Tools:        deps.ToolSet.GetPhase3BaseTools(),
        MaxTurns:     5, // Phase3 不需要太多轮
    }, nil) // Phase3 无 Guard
    if err != nil {
        return nil, err
    }

    // ── 构建父图：串联三阶段 ──
    // 父图使用 WithGenLocalState 共享 ReimbursementState
    // 并注入用户上下文（SessionID/UserID/EmployeeID/Role）
    g := compose.NewGraph[*schema.Message, *schema.Message](
        compose.WithGenLocalState(func(ctx context.Context) *agent.ReimbursementState {
            return &agent.ReimbursementState{CurrentPhase: "phase1_collect"}
        }),
    )

    // 将三个子图添加为嵌套节点
    g.AddGraphNode("phase1", phase1Graph,
        compose.WithNodeName("phase1_collect"),
        // StatePreHandler: 将当前用户消息注入到子图
        compose.WithStatePreHandler(func(ctx context.Context, msg *schema.Message, rs *agent.ReimbursementState) (*schema.Message, error) {
            rs.CurrentPhase = "phase1_collect"
            return msg, nil
        }),
    )
    g.AddGraphNode("phase2", phase2Graph,
        compose.WithNodeName("phase2_validate"),
        compose.WithStatePreHandler(func(ctx context.Context, msg *schema.Message, rs *agent.ReimbursementState) (*schema.Message, error) {
            rs.CurrentPhase = "phase2_validate"
            return msg, nil
        }),
    )
    g.AddGraphNode("phase3", phase3Graph,
        compose.WithNodeName("phase3_execute"),
        compose.WithStatePreHandler(func(ctx context.Context, msg *schema.Message, rs *agent.ReimbursementState) (*schema.Message, error) {
            rs.CurrentPhase = "phase3_execute"
            return msg, nil
        }),
    )

    // Guard 节点：Phase1 → Phase2
    g.AddLambdaNode("phase1_guard", compose.InvokableLambda(
        func(ctx context.Context, msg *schema.Message) (*schema.Message, error) {
            var passed bool
            var guardMsg string
            _ = compose.ProcessState(ctx, func(ctx context.Context, rs *agent.ReimbursementState) error {
                rs.Phase1Turns++
                result := phase.Phase1Guard(rs)
                passed = result.Passed
                guardMsg = result.Message
                return nil
            })
            if passed {
                return msg, nil // 通过 → 进入 Phase2
            }
            return schema.UserMessage(guardMsg), nil // 未通过 → 回 Phase1
        },
    ), compose.WithNodeName("phase1_guard"))

    // Guard 节点：Phase2 → Phase3
    g.AddLambdaNode("phase2_guard", compose.InvokableLambda(
        func(ctx context.Context, msg *schema.Message) (*schema.Message, error) {
            var passed bool
            var guardMsg string
            _ = compose.ProcessState(ctx, func(ctx context.Context, rs *agent.ReimbursementState) error {
                rs.Phase2Turns++
                result := phase.Phase2Guard(rs)
                passed = result.Passed
                guardMsg = result.Message
                return nil
            })
            if passed {
                return msg, nil
            }
            return schema.UserMessage(guardMsg), nil
        },
    ), compose.WithNodeName("phase2_guard"))

    // ── 连线 ──
    // Phase 1: START → phase1 → phase1_guard → phase2 or loop to phase1
    g.AddEdge(compose.START, "phase1")
    g.AddEdge("phase1", "phase1_guard")

    // phase1_guard Branch
    g.AddBranch("phase1_guard", compose.NewGraphBranch(
        func(ctx context.Context, msg *schema.Message) (endNode string, err error) {
            var passed bool
            _ = compose.ProcessState(ctx, func(ctx context.Context, rs *agent.ReimbursementState) error {
                passed = phase.Phase1Guard(rs).Passed
                return nil
            })
            if passed {
                return "phase2", nil
            }
            return "phase1", nil // ← 不符合条件，重新执行 Phase1
        },
        map[string]bool{"phase1": true, "phase2": true},
    ))

    // Phase 2: phase2 → phase2_guard → phase3 or loop
    g.AddEdge("phase2", "phase2_guard")
    g.AddBranch("phase2_guard", compose.NewGraphBranch(
        func(ctx context.Context, msg *schema.Message) (endNode string, err error) {
            var passed bool
            _ = compose.ProcessState(ctx, func(ctx context.Context, rs *agent.ReimbursementState) error {
                passed = phase.Phase2Guard(rs).Passed
                return nil
            })
            if passed {
                return "phase3", nil
            }
            return "phase2", nil
        },
        map[string]bool{"phase2": true, "phase3": true},
    ))

    // Phase 3: phase3 → END
    g.AddEdge("phase3", compose.END)

    // ── 编译 ──
    runnable, err := g.Compile(ctx,
        compose.WithGraphName("reimbursement_workflow"),
        compose.WithMaxRunSteps(100), // 三阶段 × 每阶段最多10轮工具调用
    )
    if err != nil {
        return nil, fmt.Errorf("编译报销子流程Graph失败: %w", err)
    }

    deps.Logger.Info("报销子流程 Graph 编译成功（ReAct 模式）")
    return runnable, nil
}
```

### 5.4 与旧版的关键差异

| 特性 | 旧版 | 新版 |
|------|------|------|
| Phase 节点类型 | `InvokableLambda` | `AddGraphNode`（嵌套子图） |
| Phase 内部结构 | 手动 for 循环 | `ChatModelNode + ToolsNode + Branch` ReAct 循环 |
| 工具传递 | `einoModel.WithTools(toolInfos)` | `chatModel.WithTools()` + `AddToolsNode()` |
| ToolCall 执行 | ❌ 从未执行 | ✅ `ToolsNode` 自动执行 |
| 消息历史管理 | 手动拼接 | `WithStatePreHandler` 自动累积 |
| 防死循环 | `for turn < maxTurns` | `WithMaxRunSteps(100)` 全局限制 |
| Guard 位置 | `newPhaseWithGuard` 内联 | 独立 `AddLambdaNode` + `AddBranch` |

---

## 6. Phase III — 根路由适配

### 6.1 变更

**文件**: `internal/domain/agent/graph/root.go`

```go
// RootGraphDeps 增加 Config 字段（修复 B4: 硬编码阈值）
type RootGraphDeps struct {
    Logger    *log.Logger
    ChatModel model.ToolCallingChatModel
    Config    *agent.AgentConfig  // ← 新增：用于读取 IntentConfidenceThreshold

    ReimbursementRunnable compose.Runnable[*schema.Message, *schema.Message]
    ProgressRunnable      compose.Runnable[*schema.Message, *schema.Message]
    BudgetRunnable        compose.Runnable[*schema.Message, *schema.Message]
    PolicyRunnable        compose.Runnable[*schema.Message, *schema.Message]
    ModifyRunnable        compose.Runnable[*schema.Message, *schema.Message]
}

// classifyIntent —— 读取 Config 中的阈值（修复 B4）
func classifyIntent(ctx context.Context, input agent.AgentInput, deps RootGraphDeps) string {
    if deps.ChatModel != nil {
        // 使用配置中的阈值，默认 0.7
        threshold := 0.7
        if deps.Config != nil && deps.Config.IntentConfidenceThreshold > 0 {
            threshold = deps.Config.IntentConfidenceThreshold
        }

        prompt := agent.BuildIntentClassifyPrompt(input.Message)
        resp, err := deps.ChatModel.Generate(ctx, ...)
        if err == nil && resp != nil {
            var intent intentOutput
            if json.Unmarshal([]byte(resp.Content), &intent) == nil && intent.Confidence >= threshold {
                // ...
            }
        }
    }
    return classifyByKeywords(input.Message)
}

// dispatchToWorkflow —— 通过 State 传递用户上下文（修复 B1, B7）
func dispatchToWorkflow(ctx context.Context, input agent.AgentInput, deps RootGraphDeps) *schema.Message {
    // 将用户上下文注入到 context 中，供子图通过 ProcessState 读取
    ctx = context.WithValue(ctx, userContextKey{}, input)

    route := classifyIntent(ctx, input, deps)
    msg := schema.UserMessage(input.Message)

    switch route {
    case "new_reimbursement":
        if deps.ReimbursementRunnable != nil {
            resp, err := deps.ReimbursementRunnable.Invoke(ctx, msg)
            // ...
        }
    // ... 其他 case
    }
}
```

### 6.2 dispatcher 改为 StreamableLambda（修复 B6）

```go
// 旧版: InvokableLambda（不支持 Stream）
g.AddLambdaNode("dispatcher", compose.InvokableLambda(func(...) { ... }))

// 新版: StreamableLambda（支持真正流式）
g.AddLambdaNode("dispatcher", compose.StreamableLambda(
    func(ctx context.Context, input agent.AgentInput) (*schema.StreamReader[*schema.Message], error) {
        result := dispatchToWorkflow(ctx, input, deps)
        return schema.StreamReaderFromArray([]*schema.Message{result}), nil
    },
))
```

---

## 7. Phase IV — 简单子流程修复

### 7.1 agentInputAdapter 修复（修复 B1）

**文件**: `internal/domain/agent/graph/provider.go`

```go
// 旧版: 丢失全部元数据
func (a *agentInputAdapter) Invoke(ctx context.Context, input *schema.Message, ...) (*schema.Message, error) {
    ai := agent.AgentInput{Message: input.Content}
    return a.inner.Invoke(ctx, ai, opts...)
}

// 新版: 从 context 中恢复完整 AgentInput
type userContextKey struct{}

func (a *agentInputAdapter) Invoke(ctx context.Context, input *schema.Message, opts ...compose.Option) (*schema.Message, error) {
    // 从 context 恢复完整用户上下文（由 dispatchToWorkflow 注入）
    ai := agent.AgentInput{Message: input.Content}
    if uc, ok := ctx.Value(userContextKey{}).(agent.AgentInput); ok {
        ai = uc
        ai.Message = input.Content
    }
    return a.inner.Invoke(ctx, ai, opts...)
}
```

### 7.2 修复 schedule 图（progress/budget/policy/modify）

这些子图的 `build_prompt` Lambda 现在可以通过 `agent.AgentInput` 中的用户上下文来填充 EmployeeID、DepartmentID 等字段，从而使进度查询能自动按工号过滤。

---

## 8. Phase V — Runner 层流式适配

### 8.1 变更

**文件**: `internal/domain/agent/runner.go`

```go
// StreamChat —— 流式路径现在真正工作
func (r *AgentRunner) StreamChat(ctx context.Context, input agent.AgentInput, sseWriter SSEWriter) error {
    // ... thinking 事件 ...

    // 将用户上下文注入到 State（供子图访问）
    // 旧版：无此步骤
    // 新版：在调用前注入

    // 尝试流式执行（现在支持！因为 dispatcher 是 StreamableLambda）
    stream, err := r.rootGraph.Stream(ctx, input)
    if err != nil {
        // 降级路径保留（网络异常等极端情况）
        return r.invokeFallback(ctx, input, sseWriter)
    }

    // 流式循环（现在真正逐 chunk 输出！）
    var fullContent string
    for {
        chunk, recvErr := stream.Recv()
        if recvErr == io.EOF {
            break
        }
        if recvErr != nil {
            return fmt.Errorf("流式读取失败: %w", recvErr)
        }
        if chunk != nil && chunk.Content != "" {
            fullContent += chunk.Content
            _ = sseWriter.WriteEvent(NewMessageEvent(chunk.Content, true)) // delta=true
            _ = sseWriter.Flush()
        }
    }

    // 持久化 + done 事件 ...
}
```

---

## 9. 数据流全景

### 9.1 一次完整报销对话的端到端数据流

```
1. 用户发送 "我要报销差旅费 500 元"
   ↓
2. HTTP GET /api/chat/stream?session_id=X&message=我要报销差旅费 500 元
   ↓
3. JWT 中间件 → 注入 gin.Context: user_id=42, employee_id="EMP001", role="employee"
   ↓
4. AgentService.HandleChat()
   ├── 从 gin.Context 提取 userID, employeeID, role
   ├── BuildAgentInput(sessionID, message, employeeID, userID, role)
   └── runner.StreamChat(ctx, input, sseWriter)
   ↓
5. AgentRunner.StreamChat()
   ├── SSE: thinking 事件
   ├── SessionStore.GetHistory(sessionID) → [历史消息]
   ├── SessionStore.SaveMessages([userMsg])
   ├── rootGraph.Stream(ctx, AgentInput{...})  ← 流式执行
   │     ↓
   │   6. Root Graph (dispatcher)
   │      ├── classifyIntent("我要报销差旅费 500 元") → "new_reimbursement"
   │      └── dispatchToWorkflow()
   │            ├── 将 AgentInput 注入 context (userContextKey{})
   │            └── ReimbursementRunnable.Invoke(ctx, msg)
   │                  ↓
   │                7. Reimbursement Graph
   │                   State: ReimbursementState{
   │                     SessionID: "X",
   │                     UserID: 42,
   │                     EmployeeID: "EMP001",
   │                     Role: "employee",
   │                     CurrentPhase: "phase1_collect"
   │                   }
   │                   ↓
   │                  Phase 1 ReAct 循环:
   │                   ChatModel ←→ ToolsNode(OCR, Compliance)
   │                   [LLM: "请上传票据" → 用户上传 → OCR识别 → ]
   │                   [LLM: "识别到差旅-交通 ¥500.00" → 用户确认 → ]
   │                   [Guard: Phase1Guard 通过]
   │                   ↓
   │                  Phase 2 ReAct 循环:
   │                   ChatModel ←→ ToolsNode(Compliance, Budget)
   │                   [LLM: 调用 check_compliance → pass]
   │                   [LLM: 调用 check_budget → 余额充足]
   │                   [LLM: "合规检查通过，预算充足，是否确认提交？"]
   │                   [用户确认 → Guard: Phase2Guard 通过]
   │                   ↓
   │                  Phase 3 ReAct 循环:
   │                   ChatModel ←→ ToolsNode(PDF, Email, Progress)
   │                   [LLM: 调用 generate_pdf → 成功]
   │                   [LLM: 调用 send_email → 成功]
   │                   [LLM: "报销单已提交，单号 REIMB-2026-0001"]
   │                   ↓
   │                  END → 返回 *schema.Message
   │
   ├── 流式循环: chunk.Recv() → SSE message(delta=true)
   │     前端收到: "报销单" → "已提交" → "单号" → "REIMB-2026-0001"
   ├── SessionStore.SaveMessages([assistantMsg])
   └── SSE: done 事件
```

### 9.2 Guard 分支决策流程

```
Phase 1 → phase1_guard:
  ├── Phase1Guard(state):
  │     ├── Invoices.length == 0? → Passed=false → 回 Phase1 ("请上传票据")
  │     ├── Invoice.amount == 0? → Passed=false → 回 Phase1 ("请补充金额")
  │     ├── !UserConfirmed?     → Passed=false → 回 Phase1 ("请确认票据")
  │     └── 全部通过            → Passed=true  → 进入 Phase2

Phase 2 → phase2_guard:
  ├── Phase2Guard(state):
  │     ├── ComplianceResult == nil?  → 回 Phase2 ("合规检查未完成")
  │     ├── Result == "error"?        → 回 Phase2 ("不合规，无法提交")
  │     ├── Budget不足 && !Confirmed? → 回 Phase2 ("预算不足，确认提交？")
  │     ├── !FinalConfirmed?          → 回 Phase2 ("请最终确认")
  │     └── 全部通过                  → 进入 Phase3
```

---

## 10. 文件变更清单

### 新增文件

| 文件 | 说明 |
|------|------|
| `internal/domain/agent/graph/react_phase.go` | `buildReActPhase()` 通用 ReAct 阶段构建器 |

### 重写文件

| 文件 | 变更内容 |
|------|---------|
| `internal/domain/agent/graph/reimbursement.go` | 从手动 Lambda 循环 → ReAct 图模式（~300行 → ~200行） |
| `internal/domain/agent/graph/provider.go` | `RootGraphDeps` 增加 `Config`；`agentInputAdapter` 修复元数据丢失 |

### 修改文件

| 文件 | 变更内容 |
|------|---------|
| `internal/domain/agent/graph/root.go` | `classifyIntent` 读取 Config 阈值；`dispatcher` 改为 `StreamableLambda`；`dispatchToWorkflow` 注入用户上下文 |
| `internal/domain/agent/runner.go` | `StreamChat` 移除无效的 stream fallback 逻辑（现在真正流式） |
| `internal/domain/agent/tools/provider.go` | 新增 `GetPhaseXBaseTools()` 方法 |

### 不变文件

| 文件 | 原因 |
|------|------|
| `dto.go` | AgentInput/ReimbursementState 结构体不变 |
| `config.go` | AgentConfig 配置结构不变 |
| `prompt.go` | 系统提示词模板不变 |
| `llm.go` | ChatModel 工厂不变 |
| `checkpoint.go` | Checkpoint 持久化不变 |
| `sse.go` | SSE 事件类型不变 |
| `service.go` | HTTP 服务层接口不变 |
| `phase/guard.go` | Guard 逻辑可复用 |
| `phase/phase_config.go` | Phase 配置可复用 |
| 所有 `tools/*.go` | 工具实现不变（仅暴露方式变化） |
| `graph/progress.go` | 简单子流程保留现有模式，修复 adapter 即可 |
| `graph/budget.go` | 同上 |
| `graph/policy.go` | 同上 |
| `graph/modify.go` | 同上 |

### 删除文件

| 文件 | 原因 |
|------|------|
| （无） | 所有测试文件需同步更新，但文件结构保持 |

---

## 11. 迁移风险与缓解

| 风险 | 等级 | 缓解措施 |
|------|------|---------|
| `AddBranch` 在嵌套子图中与父图 Branch 冲突 | M | 每个 Phase 作为独立子图，Branch 作用域隔离 |
| `WithGenLocalState` 的 `ReimbursementState` 在嵌套图中不可见 | M | 使用 `ProcessState` 从父图访问；Eino 已支持 parent-linking |
| ChatModelNode 的 `WithTools()` 绑定与旧工具兼容 | L | 已确认 `tool.InvokableTool` 实现 `tool.BaseTool` 接口 |
| 流式输出在上游代理（Nginx）被缓冲 | L | 已有 `X-Accel-Buffering: no` 响应头 |
| 编译后图性能下降 | L | ReAct 模式与旧 Lambda 循环等价，无性能差异 |

---

## 附录 A: BUG 修复对照表

| Bug ID | 描述 | 严重级 | 修复位置 | 修复方式 |
|--------|------|--------|---------|---------|
| B1 | agentInputAdapter 丢失元数据 | 🔴 P0 | `graph/provider.go` | 从 context 恢复完整 AgentInput |
| B2 | ToolCalls 从不执行 | 🔴 P0 | `graph/reimbursement.go` | AddToolsNode + AddBranch ReAct 循环 |
| B3 | Phase1Turns 在所有 Phase 递增 | 🟡 P2 | `graph/reimbursement.go` | 各 Phase 递增各自的 Turns 字段 |
| B4 | classifyIntent 硬编码阈值 | 🟡 P2 | `graph/root.go` | 读取 `deps.Config.IntentConfidenceThreshold` |
| B5 | Phase3 无验证 | 🟡 P2 | `graph/reimbursement.go` | Phase3 使用 ReAct 循环（自然有循环验证） |
| B6 | 不支持真流式 | 🟡 P1 | `graph/root.go` | dispatcher 改为 StreamableLambda |
| B7 | dispatch 只传 msg.Content | 🟡 P2 | `graph/root.go` | Context 注入 + State 传递 |
| B8 | truncate 字节截断 | 🟢 P3 | `graph/root.go` | 替换为 `truncateStr`（rune 截断） |
| B9 | MessageID 错误格式化 | 🟢 P3 | `tools/email_tool.go` | 改为时间戳 |

---

## 附录 B: 参考资料

- [Eino ReAct Agent 源码](https://github.com/cloudwego/eino/blob/main/flow/agent/react/react.go)
- [Eino ADK ChatModelAgent 源码](https://github.com/cloudwego/eino/blob/main/adk/chatmodel.go)
- [Eino Graph/Chain Orchestration 文档](https://www.cloudwego.io/docs/eino/core_modules/chain_and_graph_orchestration/chain_graph_introduction/)
- [Eino ToolsNode 文档](https://www.cloudwego.io/docs/eino/core_modules/components/tools_node_guide/)
- [Eino ChatModelAgent 文档](https://www.cloudwego.io/docs/eino/core_modules/eino_adk/agent_implementation/chat_model/)
- [Eino 完整示例仓库](https://github.com/cloudwego/eino-examples)
