# Agent 层 — 纯 ReAct 模式设计方案

> **状态**: 设计中，待审核  
> **日期**: 2026-07-06  
> **目标**: 通过配置切换 Graph 编排模式 ↔ 纯 ReAct 自主模式

---

## 1. 两种模式的本质区别

### 模式 A: Graph 编排（当前实现）

```
┌──────────────────────────────────────────┐
│  确定性：Graph 控制阶段流转                │
│                                          │
│  Phase1 → Guard ─┬─pass→ Phase2           │
│          └─fail──┘    → Guard ─┬─pass→ Phase3 → END
│                        └─fail──┘           │
│  每个 Phase 内部是 ReAct (ChatModel↔Tools) │
└──────────────────────────────────────────┘

代码实现: compose.Graph + AddGraphNode + AddBranch + Guard Lambda
```

### 模式 B: 纯 ReAct（设计目标）

```
┌──────────────────────────────────────────┐
│  自主：LLM 决定一切                        │
│                                          │
│  单个 ChatModelAgent，持有全部工具         │
│  LLM 通过系统提示词理解三阶段逻辑           │
│  LLM 自主决定：                            │
│    - 当前处于哪个阶段                       │
│    - 应该调用哪个工具                       │
│    - 何时应该进入下一阶段                   │
│    - 信息不足时向用户提问                   │
└──────────────────────────────────────────┘

代码实现: adk.NewChatModelAgent(ctx, config)
```

| 维度 | 模式 A (Graph) | 模式 B (Pure ReAct) |
|------|---------------|---------------------|
| 阶段控制 | Graph Guard 分支 | LLM 自主判断 |
| 工具可见性 | 按阶段分组（Phase1 看不到 Phase3 工具） | 全部工具可见 |
| 防死循环 | MaxRunSteps + Guard 计数器 | MaxIterations |
| 可控性 | 高（精确控制每个阶段行为） | 低（依赖 LLM 遵循 prompt） |
| 灵活性 | 低（改流程需改代码） | 高（改 prompt 即可） |
| Token 消耗 | 低（工具集精简） | 高（全量工具 schema 注入） |
| 流式 | 支持 | 支持（ChatModelAgent 内置） |
| 中断/恢复 | 手动实现 | ChatModelAgent 内置 |

---

## 2. 配置切换设计

### 2.1 config.yaml

```yaml
agent:
  # 运行模式: "graph" (确定性编排) | "react" (LLM自主)
  mode: "graph"

  # 以下配置仅 mode=react 时生效
  react:
    # 最大 ReAct 循环迭代次数，默认 30
    max_iterations: 30
    # 是否允许 LLM 自主跳过阶段（如直接进入 Phase3）
    allow_phase_skip: false
    # 系统提示词模板（可用 {phase_rules} 占位符注入阶段规则）
    system_prompt_template: ""

  # 以下配置仅 mode=graph 时生效  
  max_phase_turns: 10
  checkpoint_cleanup_hours: 1
```

### 2.2 AgentConfig 扩展

```go
// internal/domain/agent/config.go

type AgentMode string

const (
    ModeGraph AgentMode = "graph"  // 图编排模式（当前）
    ModeReAct AgentMode = "react"  // 纯 ReAct 自主模式
)

type AgentConfig struct {
    // ... 现有字段保持不变 ...

    Mode AgentMode `json:"mode"` // 运行模式

    // ReAct 模式专用配置
    ReActMaxIterations  int    `json:"react_max_iterations"`
    ReActAllowPhaseSkip bool   `json:"react_allow_phase_skip"`
    ReActSystemPrompt   string `json:"react_system_prompt"`
}
```

---

## 3. 纯 ReAct 模式架构

### 3.1 整体结构

```
AgentRunner.StreamChat()
  │
  ├── if config.Mode == "graph":
  │     └── rootGraph.Stream(ctx, input)     ← 现有逻辑
  │
  └── if config.Mode == "react":
        └── reactAgent.Run(ctx, input)       ← 新增逻辑
              │
              ▼
        ┌─────────────────────────────────────┐
        │  adk.ChatModelAgent                 │
        │                                     │
        │  Instruction = BuildReActPrompt()   │
        │  Tools = ToolSet.GetAllBaseTools()  │ ← 全部 9 个工具
        │  MaxIterations = 30                 │
        │                                     │
        │  ChatModelAgent 内部自动:            │
        │    ReAct Loop (Reason→Act→Observe)  │
        │    Streaming output                 │
        │    Interrupt/Resume                 │
        └─────────────────────────────────────┘
```

### 3.2 系统提示词（纯 ReAct 版）

```go
// internal/domain/agent/prompt.go — 新增

func BuildPureReActPrompt() string {
    return `你是 Reimbee，企业财务报销智能助手。你需要自主引导用户完成报销全流程。

## 你的能力
你可以调用以下工具来帮助用户：
- recognize_invoice: OCR 识别票据图片
- check_compliance: 检查费用是否符合公司标准
- check_budget: 查询部门预算余额
- create_reimbursement: 创建报销单草稿
- submit_reimbursement: 提交报销单进入审批
- generate_pdf: 生成标准报销单 PDF
- send_email: 发送审批通知邮件
- query_progress: 查询报销审批进度
- query_reimbursements: 查询历史报销记录

## 报销流程指引

你需要引导用户完成三个阶段：

### 阶段 1: 信息收集
1. 请用户上传票据图片（必须——图片是法定审计凭证）
2. 用户上传后，自动调用 recognize_invoice 工具进行 OCR 识别
3. 将识别结果逐项展示给用户确认（金额、类别、日期）
4. OCR 失败时引导用户手动输入（不阻塞流程）
5. 用户可以继续添加更多票据
6. 调用 check_compliance 工具查询报销标准供用户参考
7. 所有票据确认后，汇总展示总金额，告知用户进入下一阶段

### 阶段 2: 校验确认
1. 调用 check_compliance 工具对每张票据执行合规检查
2. 调用 check_budget 工具检查部门预算余额
3. 汇总检查结果：
   - 合规通过 + 预算充足：展示摘要，请用户确认提交
   - 合规警告：展示超标项和标准值，询问是否继续
   - 合规不通过：告知无法提交，说明原因
   - 预算不足：告知将触发特殊审批，询问是否继续
4. 用户确认后，告知提交后不可撤销

### 阶段 3: 执行提交
1. 用户确认后，依次调用:
   - create_reimbursement（创建报销单）
   - submit_reimbursement（提交审批）
   - generate_pdf（生成 PDF）
   - send_email（发送通知）
2. 告知用户报销单号和后续步骤

## 行为规范
- 一次一步：每次只引导用户完成一个步骤
- 金额确认：涉及金额操作前必须让用户明确确认
- 专业简洁：使用中文，保持专业友好的语气
- 如果你不确定当前处于哪个阶段，根据对话上下文判断`
}
```

### 3.3 Runner 改动

```go
// internal/domain/agent/runner.go

type AgentRunner struct {
    // ... 现有字段 ...

    reactAgent *adk.ChatModelAgent  // 纯 ReAct 模式实例（惰性初始化）
}

func (r *AgentRunner) StreamChat(ctx context.Context, input AgentInput, sseWriter SSEWriter) error {
    // 根据配置选择模式
    if r.config.Mode == ModeReAct {
        return r.streamChatReAct(ctx, input, sseWriter)
    }
    return r.streamChatGraph(ctx, input, sseWriter) // 现有逻辑
}

func (r *AgentRunner) streamChatReAct(ctx context.Context, input AgentInput, sseWriter SSEWriter) error {
    // 1. 发送 thinking 事件
    _ = sseWriter.WriteEvent(NewThinkingEvent("正在处理..."))
    _ = sseWriter.Flush()

    // 2. 加载会话历史
    history, _ := r.sessionStore.GetHistory(ctx, input.SessionID, r.config.MaxHistoryTurns*2)

    // 3. 构建 AgentInput
    messages := make([]*schema.Message, 0, len(history)+1)
    for _, h := range history {
        messages = append(messages, h)
    }
    messages = append(messages, schema.UserMessage(input.Message))

    // 4. 执行 ChatModelAgent（内置 ReAct 循环）
    runner := adk.NewRunner(ctx, adk.RunnerConfig{Agent: r.reactAgent, EnableStreaming: true})
    iter := runner.Query(ctx, messages)

    // 5. 消费事件流 → SSE 输出
    for {
        event, ok := iter.Next()
        if !ok {
            break
        }
        if event.Err != nil {
            _ = sseWriter.WriteEvent(NewErrorEvent(event.Err.Error(), false, "agent_error"))
            _ = sseWriter.Flush()
            return event.Err
        }
        if event.Output != nil && event.Output.MessageOutput != nil {
            // 流式输出 model 文本
            // 工具调用事件同步
            // ... 
        }
    }

    // 6. 持久化 + done
    _ = sseWriter.WriteEvent(NewDoneEvent())
    _ = sseWriter.Flush()
    return nil
}
```

---

## 4. 需要修改的文件

### 4.1 新增文件

| 文件 | 说明 |
|------|------|
| `internal/domain/agent/react_agent.go` | `buildPureReActAgent()` — 创建 ChatModelAgent 实例 |
| `internal/domain/agent/react_stream.go` | `streamChatReAct()` — ReAct 模式 SSE 流式输出 |
| `internal/domain/agent/prompt_react.go` | `BuildPureReActPrompt()` — 纯 ReAct 系统提示词 |

### 4.2 修改文件

| 文件 | 变更 |
|------|------|
| `internal/domain/agent/config.go` | 增加 `Mode`、`ReActMaxIterations` 等字段 |
| `internal/domain/agent/runner.go` | `StreamChat` 增加模式分支；新增 `streamChatReAct` |
| `internal/domain/agent/provider.go` | Wire 注册 `buildPureReActAgent`（惰性初始化） |
| `config.yaml` | 增加 `agent.mode` 和 `agent.react.*` 配置段 |
| `config.yaml.example` | 同步更新 |

### 4.3 不变文件

| 文件 | 原因 |
|------|------|
| 所有 `graph/*.go` | 模式 A 代码完全保留 |
| 所有 `tools/*.go` | 工具被两种模式共用 |
| `sse.go` | SSE 事件格式不变 |
| `dto.go` | AgentInput 结构不变 |

---

## 5. 两种模式的工具可见性差异

### 模式 A (Graph): 按阶段隔离

```
Phase 1: OCR, Compliance
Phase 2: Compliance, Budget  
Phase 3: CreateReimb, SubmitReimb, PDF, Email, Progress
```

### 模式 B (Pure ReAct): 全部可见

```
全部阶段: OCR, Compliance, Budget, CreateReimb, SubmitReimb, PDF, Email, Progress, QueryRecords
```

**风险**: LLM 可能在 Phase 1 就调用 `submit_reimbursement`。缓解方式：
- 系统提示词中明确说明工具调用顺序
- `create_reimbursement` 和 `submit_reimbursement` 工具描述中添加"仅在用户确认后调用"
- 可选：通过 `ChatModelAgentMiddleware.BeforeModel` 动态过滤工具列表

---

## 6. 切换行为对照

| 场景 | 模式 A (Graph) | 模式 B (Pure ReAct) |
|------|---------------|---------------------|
| 用户说"报销" | 进入 Phase1 ReAct → Guard fail → 提示上传票据 | LLM 理解 prompt，引导用户上传票据 |
| 用户上传票据 | Phase1 ReAct: OCR → 确认 → Guard pass → Phase2 | LLM 调用 OCR → 展示结果 → 引导确认 |
| 用户说"确认提交" | Phase3 ReAct: 依次调用 create→submit→pdf→email | LLM 依次调用工具 |
| LLM 跳过 Phase1 | 不可能（Graph 强制顺序） | 可能（如果 prompt 约束不够强） |
| 工具滥用 | 不可能（工具按阶段隔离） | 可能（需要 prompt + middleware 防护） |

---

## 7. 执行计划

| Wave | 内容 | 预计 |
|------|------|------|
| 1 | `config.go` 增加 Mode 字段 + config.yaml 更新 | 30min |
| 2 | `react_agent.go` + `prompt_react.go` — 创建 ChatModelAgent + 提示词 | 1h |
| 3 | `react_stream.go` — SSE 事件桥接 | 1.5h |
| 4 | `runner.go` — StreamChat 模式分支 | 30min |
| 5 | 测试 + `config.mode=react` 端到端验证 | 1h |

**总计约 4.5 小时。**
