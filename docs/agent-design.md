# Agent 层详细设计 v2.0 — 流程编排架构

> 版本: v2.0 | 日期: 2026-07-04 | 状态: 设计评审
>
> v1.0（ChatModelAgent + ReAct）已废弃。v2.0 采用 **compose.Graph 流程编排**。

---

## 一、设计理念

### 1.1 为什么不能只用 ReAct

报销是**强流程约束**的业务场景。每一步有明确的先后依赖关系：

```
OCR 识别 → 用户确认 → 合规检查 → 预算检查 → 最终确认 → 生成 PDF → 发送邮件
   │          │          │          │          │
   不能跳过   不能跳过    不能跳过    不能跳过    不能跳过
```

纯 ReAct Agent（"给 LLM 一堆工具，让它自己决定调用顺序"）的问题：

| 问题 | ReAct 表现 | 后果 |
|------|-----------|------|
| 跳过合规检查 | LLM 可能"忘记"调用 check_compliance | 超标报销直接提交 |
| 不确认就提交 | LLM 直接调 generate_pdf | 金额/类别可能错误 |
| 工具顺序错 | 先发邮件再生成 PDF | 附件缺失 |
| 用户意图纠缠 | 报销中途用户问"预算还剩多少"，ReAct 可能丢失上下文 | 报销中断、状态混乱 |
| 不可审计 | 每次 LLM 决策路径不同 | 无法追踪为什么某步被跳过 |

### 1.2 正确方案：Graph 编排 + LLM 作为节点

```
┌─────────────────────────────────────────────────┐
│              业务流程（确定性）                     │
│                                                  │
│  意图分类 ──▶ 工作流路由 ──▶ 子流程执行             │
│                │                                 │
│     ┌──────────┼──────────┐                      │
│     ▼          ▼          ▼                      │
│  报销流程   进度查询   预算查询                     │
│  (9 步确定性  (取数据    (取数据                    │
│   顺序)      + 格式化)   + 格式化)                  │
│                                                  │
│  每个节点内部：LLM 负责自然语言理解/生成             │
│  节点之间：Graph 保证执行顺序                       │
└─────────────────────────────────────────────────┘
```

**核心原则**：LLM 是节点内的决策引擎，Graph 是节点间的流程路由器。

---

## 二、顶层架构

### 2.1 Graph 拓扑

```
                    ┌──────────────┐
                    │   入口节点     │
                    │ IntentClassify│  ← LLM：分类用户意图
                    └──────┬───────┘
                           │
                    ┌──────▼───────┐
                    │  WorkflowRoute│  ← Lambda：根据意图路由
                    └──┬──┬──┬──┬──┘
           ┌───────────┘  │  │  └───────────┐
           ▼              ▼  ▼              ▼
    ┌────────────┐  ┌───────────┐  ┌──────────────┐
    │ 报销子流程   │  │ 进度查询   │  │ 预算查询      │
    │ (9 nodes)  │  │ (2 nodes) │  │ (2 nodes)    │
    └────────────┘  └───────────┘  └──────────────┘
```

### 2.2 子流程与意图映射

| 用户意图 | 子流程 | 确定性步骤 |
|---------|--------|:--:|
| 新建报销 / 我要报销 | `NewReimbursementWorkflow` | 7 步（见 §3） |
| 查询进度 / 我的报销到哪了 | `QueryProgressWorkflow` | 2 步（查 + 格式化） |
| 查询预算 / 还剩多少钱 | `QueryBudgetWorkflow` | 2 步（查 + 格式化） |
| 政策咨询 / 标准是多少 | `PolicyQuestionHandler` | 1 步（LLM 直接回复） |
| 修改报销 | `ModifyReimbursementWorkflow` | 3 步（查 + LLM 对话 + 更新） |

---

## 三、报销子流程（核心工作流）

### 3.1 状态机

```
START
  │
  ▼
┌───────────────┐
│ CollectReceipt │  LLM 节点：引导用户上传票据，等待用户操作
│  收集票据      │  用户："这是发票" / 上传图片
└───────┬───────┘
        │ 用户已上传票据
        ▼
┌───────────────┐
│   OCRNode     │  Tool 节点：调用 recognize_invoice
│  OCR 识别     │  输入：图片路径  输出：结构化 InvoiceResult
└───────┬───────┘
        │
        ▼
┌───────────────┐
│ ConfirmInvoice │  LLM 节点：展示 OCR 结果，询问"正确吗？"
│  确认票据      │  "对" → 继续  /  "不对，金额改为2000" → 修改
└───────┬───────┘
        │ 用户确认
        ▼
┌───────────────┐
│ ComplianceNode │  Tool 节点：调用 check_compliance
│  合规检查      │  返回 pass / warning / error
└───────┬───────┘
        │
        ▼
┌───────────────┐
│ ComplianceReview│ LLM 节点：展示合规结果
│  合规审查      │  pass → 继续  /  warning → 告知风险，确认是否继续
└───────┬───────┘  error → 拒绝，引导修改
        │ 合规通过或用户确认警告
        ▼
┌───────────────┐
│  BudgetNode   │  Tool 节点：调用 check_budget
│  预算检查      │  返回 余额 + need_special_approval
└───────┬───────┘
        │
        ▼
┌───────────────┐
│ BudgetReview  │  LLM 节点：展示预算状态
│  预算确认      │  预算充足 → 继续  /  预算不足 → 告知并确认
└───────┬───────┘
        │ 用户确认提交
        ▼
┌───────────────┐
│ FinalConfirm  │  LLM 节点：最终确认——汇总票据金额、合规结果、预算状态
│  最终确认      │  "确认提交？"  →  "确认"  →  不可逆操作开始
└───────┬───────┘
        │ 用户最终确认
        ▼
┌───────────────┐
│ GeneratePDF   │  Tool 节点：生成 PDF 报销单
│  生成 PDF     │  调用 generate_pdf
└───────┬───────┘
        │
        ▼
┌───────────────┐
│  SendEmail    │  Tool 节点：发送审批邮件
│  发送邮件      │  调用 send_email
└───────┬───────┘
        │
        ▼
┌───────────────┐
│ SubmitComplete│  LLM 节点：成功消息
│  提交完成      │  "报销单 REIMB-2026-0001 已生成并发送给审批人"
└───────────────┘
        │
        ▼
       END
```

### 3.2 节点类型分类

| 节点类型 | 数量 | 引擎 | 职责 |
|---------|:--:|------|------|
| **LLM 节点**（ChatModelNode） | 6 | LLM | 自然语言理解/生成、用户交互 |
| **Tool 节点**（ToolsNode） | 4 | 确定性代码 | OCR、合规、预算、PDF、邮件 |
| **结束节点** | 1 | — | 流程终止 |

**7 步确定性流程，每步之间的走向由 Graph 保证，LLM 只负责每步内部的"说什么"。**

### 3.3 为什么用 Graph 而不是 ChatModelAgent

| 维度 | ChatModelAgent (ReAct) | compose.Graph |
|------|:---:|:---:|
| 流程确定性 | ❌ LLM 决定下一步 | ✅ Graph 边定义 |
| 合规检查 | ❌ 可能被跳过 | ✅ 必经过 ComplianceNode |
| 用户确认 | ❌ 可能忘记确认 | ✅ ConfirmInvoice → FinalConfirm 两次确认 |
| 可审计性 | ❌ 每次路径不同 | ✅ 固定路径，可日志 |
| 异常处理 | ❌ LLM 自行判断 | ✅ 每个节点有明确错误分支 |
| 多意图并发 | ❌ 报销中途用户问进度，上下文丢失 | ✅ 子流程隔离，不互相干扰 |

---

## 四、意图分类设计

### 4.1 意图分类节点（IntentClassify）

**节点类型**: ChatModelNode（LLM）

**输入**: 用户的自然语言消息 + 当前 Graph 状态

**输出**: 结构化 JSON（通过 Eino 的 formatted output 或 prompt 约束）

```json
{
  "intent": "new_reimbursement",
  "entities": {
    "amount": null,
    "category": null,
    "department": "计算机学院"
  },
  "confidence": 0.95
}
```

**意图分类 Prompt**：

```
分析用户意图，从以下类别中选择最匹配的一个：

- new_reimbursement: 用户想发起新的报销
- query_progress: 用户想查询报销进度
- query_budget: 用户想查询部门预算
- policy_question: 用户想了解报销政策/标准
- modify_reimbursement: 用户想修改已有报销单
- general_chat: 问候、感谢等非业务对话

返回 JSON：
{"intent": "<类别>", "entities": {"amount": null, "category": null, "department": null}, "confidence": 0.95}

用户输入：{user_message}
```

### 4.2 工作流路由节点（WorkflowRoute）

**节点类型**: LambdaNode（确定性 Go 函数）

**输入**: 意图分类结果

**输出**: 路由到的子流程标识

```go
func workflowRouter(ctx context.Context, intent *IntentResult) (string, error) {
    switch intent.Intent {
    case "new_reimbursement":
        return "workflow_new_reimbursement", nil
    case "query_progress":
        return "workflow_query_progress", nil
    case "query_budget":
        return "workflow_query_budget", nil
    case "policy_question":
        return "workflow_policy_question", nil
    case "modify_reimbursement":
        return "workflow_modify", nil
    default:
        return "workflow_general_chat", nil
    }
}
```

---

## 五、会话状态管理

### 5.1 状态模型

报销流程是**多轮对话**——用户不会一次性提供所有信息。需要在 Redis 中持久化 Graph 的中间状态。

```go
// SessionState 会话状态——由 Graph 的 Checkpoint 机制自动管理
type SessionState struct {
    SessionID        string              `json:"session_id"`
    CurrentWorkflow  string              `json:"current_workflow"`  // 当前子流程标识
    CurrentNode      string              `json:"current_node"`      // 子流程中的当前节点
    
    // 报销流程上下文
    InvoiceResult    *infra.InvoiceResult `json:"invoice_result,omitempty"`
    ComplianceResult *ComplianceOutput    `json:"compliance_result,omitempty"`
    BudgetResult     *BudgetOutput        `json:"budget_result,omitempty"`
    ReimbursementID  uint                 `json:"reimbursement_id,omitempty"`
    PDFPath          string               `json:"pdf_path,omitempty"`
    
    // 用户确认历史
    Confirmations    []ConfirmationRecord `json:"confirmations"`
    
    // 消息历史（用户可见的对话）
    Messages         []Message            `json:"messages"`
    CreatedAt        time.Time            `json:"created_at"`
    UpdatedAt        time.Time            `json:"updated_at"`
}
```

### 5.2 Eino Checkpoint 机制

Eino 的 `compose.Graph` 原生支持 Checkpoint / Interrupt：

```
Graph.Compile(ctx, compose.WithCheckPointStore(redisStore))
```

每次 LLM 节点等待用户输入时，Graph 自动保存 Checkpoint（包含当前节点、状态变量、消息历史）到 Redis。用户回复后，Graph 从 Checkpoint 恢复并继续执行。

**优势**：不需要手动管理状态机——Graph 的 `Interrupt` 机制天然支持"等待用户输入→恢复执行"。

---

## 六、LLM 节点内的 Prompt 设计

LLM 节点不再需要"工具调用决策"。它只需做一件事：**基于当前节点上下文，生成合适的自然语言回复**。

### 6.1 各类 LLM 节点的 Prompt 模板

**CollectReceipt（收集票据）**：
```
你是 Reimbee，帮助用户完成报销。当前步骤：收集票据。

用户意图是发起报销。请友好地引导用户上传票据图片。
告诉用户可以拍照或选择文件，支持 JPG/PNG/PDF 格式。

历史对话：{history}
用户最新消息：{user_message}
```

**ConfirmInvoice（确认票据）**：
```
你是 Reimbee。OCR 识别完成，请向用户确认以下信息：

识别结果：{invoice_result}
- 金额：¥{amount}
- 类别：{category}
- 日期：{date}
- 销售方：{seller}

询问用户信息是否正确。如果用户要求修改，更新相应字段。
```

**ComplianceReview（合规审查）**：
```
你是 Reimbee。合规检查结果：

{compliance_result}

- 如果 pass → 告知用户"合规检查通过"，自然过渡到预算检查
- 如果 warning → 展示超标项和标准，询问是否继续
- 如果 error → 告知用户无法提交，说明原因和建议
```

**FinalConfirm（最终确认）**：
```
你是 Reimbee。这是最终确认步骤，之后将不可逆地提交报销。

汇总信息：
- 票据：{invoice_summary}
- 合规：{compliance_summary}
- 预算：{budget_summary}
- 总金额：¥{total}

请明确要求用户确认："确认提交报销吗？提交后将无法修改。"
```

---

## 七、文件结构

```
internal/domain/agent/
├── provider.go              # Wire ProviderSet
├── runner.go                # AgentRunner（顶层 Graph 管理）
├── prompt.go                # 所有 Prompt 模板
├── session.go               # SessionState + Redis 持久化
├── dto.go                   # IntentResult, Message 等 DTO
│
├── graph/                   # Graph 定义
│   ├── root.go              # 顶层 Graph（意图分类 → 路由 → 子流程）
│   ├── reimbursement.go     # NewReimbursementWorkflow（7 步）
│   ├── progress.go          # QueryProgressWorkflow
│   ├── budget.go            # QueryBudgetWorkflow
│   ├── policy.go            # PolicyQuestionHandler
│   └── modify.go            # ModifyReimbursementWorkflow
│
├── nodes/                   # 自定义节点（LambdaNode / ToolsNode 封装）
│   ├── intent.go            # IntentClassify：LLM 节点（意图分类 Prompt）
│   ├── router.go            # WorkflowRoute：Lambda 节点（路由函数）
│   ├── ocr.go               # OCRNode：ToolsNode（调用 OCR 工具）
│   ├── compliance.go        # ComplianceNode + ComplianceReview
│   ├── budget.go            # BudgetNode + BudgetReview
│   ├── confirm.go           # ConfirmInvoice + FinalConfirm
│   └── finish.go            # GeneratePDF + SendEmail + SubmitComplete
│
└── tools/                   # 7 个 Tool 定义（不变）
    ├── provider.go
    ├── ocr_tool.go
    ├── compliance_tool.go
    ├── budget_tool.go
    ├── pdf_tool.go
    ├── email_tool.go
    ├── progress_tool.go
    └── query_tool.go
```

---

## 八、与 v1.0（ChatModelAgent）的关键差异

| 维度 | v1.0 ChatModelAgent + ReAct | v2.0 compose.Graph 编排 |
|------|:---:|:---:|
| **架构模式** | Agent 自主决策 | Graph 确定性编排 |
| **流程保证** | LLM 决定顺序，可能出错 | Graph 边保证顺序 |
| **工具调用** | LLM 决定何时调哪个 | Graph 在指定节点调用 |
| **用户确认** | 依赖 Prompt 约束（不可靠） | Graph 节点强制执行 |
| **合规检查** | LLM 可能跳过 | 必经 ComplianceNode |
| **状态管理** | 手动管理 Session | Eino Checkpoint 自动 |
| **代码量** | 少（一个 Agent + 工具） | 多（多个 Graph 子流程） |
| **可测试性** | 难（LLM 行为不确定） | 易（子流程可独立测试） |
| **生产可靠性** | 低（黑盒决策） | 高（白盒流程） |

---

## 九、实施计划

| 阶段 | 内容 | 产出 |
|:--:|------|------|
| **P1** | 顶层 Graph（意图分类 + 路由） | 能识别意图并路由到子流程 |
| **P2** | 报销子流程（7 步） | 核心功能可用 |
| **P3** | 进度查询 + 预算查询子流程 | 辅助功能 |
| **P4** | SSE 流式端点 + 前端联调 | 完整端到端 |

---

*文档结束*
