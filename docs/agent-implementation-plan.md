# Reimbee Agent Layer — 详细实施计划

> 基于 `agent-design.md` v2.2 + `agent-design-supplement.md` v1.0 + 现有代码基线
> 编制日期：2026-07-06 | 状态：待评审

---

## 零、现有基础设施盘点

### ✅ 已完成（无需重新实现）

| 组件 | 文件 | 状态 |
|------|------|:--:|
| Session 持久化（MySQL+Redis双层） | `infra/session*.go` | ✅ 完成，已注册 Wire |
| SessionMessage 模型 | `model/session_message.go` | ✅ 完成 |
| Eino 依赖（v0.9.12） | `go.mod` | ✅ 已安装 |
| OCR 多模态识别器 | `infra/ocr_multimodal.go` | ✅ 完成 |
| 合规检查引擎 | `internal/domain/compliance/biz.go` | ✅ 完成 |
| 知识库（RAG政策检索） | `internal/domain/compliance/knowledge.go` | ✅ 完成 |
| 预算检查逻辑 | `internal/domain/budget/biz.go` | ✅ 完成 |
| 审批进度查询 | `internal/domain/approval/biz.go` | ✅ 完成 |
| 报销单查询 | `internal/domain/reimbursement/biz.go` | ✅ 完成 |
| PDF/Email Mock | `infra/pdf_mock.go`, `infra/smtp_mock.go` | ⚠️ Mock，需真实实现 |
| OCR审计链字段 | `model/invoice_item.go` | ✅ 完成 |
| Agent 配置段 | `config.yaml` | ✅ 已有字段配置 |
| SSE 端点占位 | `router/root.go:114` | ⚠️ 501 stub，需替换 |

### ❌ 缺失（需要新建）

**`internal/domain/agent/` 目录完全不存在。** 需从零构建约 25 个文件。

---

## 一、实施阶段总览

```
Phase A: 基础设施 ──→ Phase B: 工具层 ──→ Phase C: Graph 层 ──→ Phase D: 集成层
    4 文件              8 文件              10 文件               6 文件
    (DTO + Config       (Tool 定义包装        (Root Graph +        (Runner + SSE +
     + Wire + 测试)      现有 infra)           Sub-Workflows        Checkpoint +
                                               + Phase Agents)      测试完善)
```

| 阶段 | 产出 | 文件数 | 依赖 | 可验证 |
|:--:|------|:--:|------|:--:|
| **A** | DTO、Config、Provider、测试基座 | 4 | 无 | `make wire` 成功 |
| **B** | 7 个 Eino Tool 定义 | 8 | Phase A | 每个 Tool 单元测试 |
| **C** | 5 个子流程 Graph + Root Graph + 3 Phase Agent + Prompt | 10 | Phase B | Graph 编译 + 流程测试 |
| **D** | AgentRunner、SSE、Checkpoint、HTTP Service | 6 | Phase C | `curl /api/chat/stream` |

---

## 二、Phase A — 基础设施层（DTO + Config + Wire + 测试）

### A1. `internal/domain/agent/dto.go` — 共享类型定义

**新建**

定义所有 Agent 层共享的数据结构：

```go
package agent

// ========== 意图分类 ==========
type IntentResult struct {
    Intent     string            `json:"intent"`
    Entities   map[string]string `json:"entities"`
    Confidence float64           `json:"confidence"`
    Reason     string            `json:"reason"`
}

// ========== 工作流路由 ==========
type WorkflowRoute string

const (
    RouteNewReimbursement    WorkflowRoute = "new_reimbursement"
    RouteQueryProgress       WorkflowRoute = "query_progress"
    RouteQueryBudget         WorkflowRoute = "query_budget"
    RoutePolicyQuestion      WorkflowRoute = "policy_question"
    RouteModifyReimbursement WorkflowRoute = "modify_reimbursement"
    RouteGeneralChat         WorkflowRoute = "general_chat"
)

// ========== 报销流程状态（Graph State） ==========
type ReimbursementState struct {
    ReimbursementID      uint
    ReimbursementNo      string
    DepartmentID         uint
    EmployeeID           string
    EmployeeName         string
    CurrentPhase         string
    Invoices             []InvoiceState
    TotalAmount          int64
    ComplianceResult     *ComplianceCheckResult
    BudgetResult         *BudgetCheckResult
    UserConfirmed        bool
    FinalConfirmed       bool
    NeedSpecialApproval  bool
    Phase1Turns          int
    Phase2Turns          int
    Phase3Turns          int
}

type InvoiceState struct {
    Index         int
    ImagePath     string
    OCRRawAmount  int64
    OCRRawCategory string
    OCRRawDate    string
    OCRConfidence float64
    Amount        int64
    Category      string
    InvoiceDate   string
    IsModified    bool
    ModifyReason  string
    UserConfirmed bool
}

type ComplianceCheckResult struct {
    Result  string `json:"result"`
    Level   string `json:"level"`
    Message string `json:"message"`
    RuleID  string `json:"rule_id"`
}

type BudgetCheckResult struct {
    Remaining           int64   `json:"remaining"`
    NeedSpecialApproval bool    `json:"need_special_approval"`
    UsageRate           float64 `json:"usage_rate"`
}

// ========== Guard 检查结果 ==========
type GuardResult struct {
    Passed  bool   `json:"passed"`
    Reason  string `json:"reason"`
    Message string `json:"message,omitempty"`
}

// ========== Graph 输入/输出 ==========
type AgentInput struct {
    SessionID  string
    UserID     uint
    EmployeeID string
    Role       string
    Message    string
}

type AgentOutput struct {
    Message  string         `json:"message"`
    Metadata map[string]any `json:"metadata,omitempty"`
    Done     bool           `json:"done"`
}
```

### A2. `internal/domain/agent/config.go` — Agent 配置加载

**新建**

从 Viper 加载 Agent 配置（对应 `config.yaml` 的 `agent:` 段）：

```go
package agent

import "github.com/spf13/viper"

type AgentConfig struct {
    SessionTTLMinutes        int     // 会话超时（分钟）
    MaxHistoryTurns          int     // 最大历史保留轮数
    MaxPhaseTurns            int     // 每个 Phase 最大轮次（防死循环）
    CheckpointCleanupHours   int     // 孤儿 Checkpoint 清理时间
    LLMMaxRetries            int     // LLM 最大重试次数
    LLMRetryBackoffSeconds   int     // 重试退避时间（秒）
    ToolTimeoutSeconds       int     // 工具调用超时（秒）
    IntentConfidenceThreshold float64 // 意图分类置信度阈值
}

func LoadAgentConfig(vc *viper.Viper) *AgentConfig {
    return &AgentConfig{
        SessionTTLMinutes:         vc.GetInt("agent.session_ttl_minutes"),
        MaxHistoryTurns:           vc.GetInt("agent.max_history_turns"),
        MaxPhaseTurns:             vc.GetInt("agent.max_phase_turns"),
        CheckpointCleanupHours:    vc.GetInt("agent.checkpoint_cleanup_hours"),
        LLMMaxRetries:             vc.GetInt("agent.llm_max_retries"),
        LLMRetryBackoffSeconds:    vc.GetInt("agent.llm_retry_backoff_seconds"),
        ToolTimeoutSeconds:        vc.GetInt("agent.tool_timeout_seconds"),
        IntentConfidenceThreshold: vc.GetFloat64("agent.intent_confidence_threshold"),
    }
}
```

### A3. `internal/domain/agent/provider.go` — Wire ProviderSet

**新建**

```go
package agent

import "github.com/google/wire"

var ProviderSet = wire.NewSet(
    // 配置
    LoadAgentConfig,

    // Tool 聚合
    tools.NewToolSet,

    // Graph 编译
    NewRootGraph,
    NewReimbursementGraph,
    NewProgressGraph,
    NewBudgetGraph,
    NewPolicyHandler,
    NewModifyGraph,

    // AgentRunner
    NewAgentRunner,

    // Checkpoint 存储
    NewMySQLCheckpointStore,
    wire.Bind(new(CheckpointStore), new(*MySQLCheckpointStore)),

    // HTTP Service
    NewAgentService,
)
```

### A4. `internal/domain/provider.go` — 引入 Agent ProviderSet

**修改现有文件**

```go
import "github.com/CycleZero/Reimbee/internal/domain/agent"

var ProviderSet = wire.NewSet(
    NewServiceHub,
    approval.ProviderSet,
    budget.ProviderSet,
    compliance.ProviderSet,
    department.ProviderSet,
    employee.ProviderSet,
    reimbursement.ProviderSet,
    agent.ProviderSet, // ← 新增
)
```

### A5. `internal/testutil/db.go` — 补充缺失的自动迁移

**修改现有文件**

在 `NewTestData()` 的 `AutoMigrate` 列表中增加：
- `model.SessionMessage{}`
- `model.PolicyDocument{}`
- `model.PolicyChunk{}`

新增 Seed 辅助函数：
- `SeedPolicyDocument(db, title, content)` — 合规知识库种子数据
- `SeedSessionMessages(db, sessionID, messages)` — 会话消息种子数据
- `CreateMockLLM(responses)` — 可编程 Mock LLM

### A6. `config.yaml` + `config.yaml.example` — 补充 Agent 配置项

**修改现有文件**

在 `agent:` 段增加：
```yaml
agent:
  session_ttl_minutes: 30
  max_history_turns: 20
  max_phase_turns: 10               # 新增
  checkpoint_cleanup_hours: 1
  llm_max_retries: 3                # 新增
  llm_retry_backoff_seconds: 2      # 新增
  tool_timeout_seconds: 30          # 新增
  intent_confidence_threshold: 0.7  # 新增
```

---

## 三、Phase B — 工具层（7 个 Eino Tool）

每个 Tool 文件遵循统一模式：
1. 定义输入/输出结构体（带 `jsonschema` 标签）
2. 实现 Eino `tool.InvokableTool` 接口
3. 在 `tools/provider.go` 中聚合为 `ToolSet`

### B1. `internal/domain/agent/tools/ocr_tool.go` — recognize_invoice

| 属性 | 值 |
|------|-----|
| **工具名** | `recognize_invoice` |
| **描述** | 识别票据图片，提取金额、类别、日期、销售方等信息 |
| **输入** | `{"image_path": "string"}` |
| **输出** | `{invoice: {amount, date, category, seller_name, confidence, raw_text, error, retry}}` |
| **后端** | `infra.OCRRecognizer.Recognize()` |
| **错误处理** | OCR 失败返回 `{error: "...", retry: true}`，不抛异常 |
| **依赖注入** | 需注入 `infra.OCRRecognizer` 接口 |

### B2. `internal/domain/agent/tools/compliance_tool.go` — check_compliance

| 属性 | 值 |
|------|-----|
| **工具名** | `check_compliance` |
| **描述** | 检查票据是否符合企业报销政策（金额标准、发票有效期等） |
| **输入** | `{"amount": int64, "category": string, "invoice_date": string}` |
| **输出** | `{result, level, message, rule_id}` |
| **后端** | `compliance.ComplianceBiz.CheckCompliance()` |
| **依赖注入** | 需注入 `*compliance.ComplianceBiz` |

### B3. `internal/domain/agent/tools/budget_tool.go` — check_budget

| 属性 | 值 |
|------|-----|
| **工具名** | `check_budget` |
| **描述** | 检查指定部门的预算是否充足，返回可用余额和是否需要特殊审批 |
| **输入** | `{"department_id": uint, "amount": int64}` |
| **输出** | `{remaining, need_special_approval, usage_rate}` |
| **后端** | `budget.BudgetBiz.CheckBudget()` |
| **依赖注入** | 需注入 `*budget.BudgetBiz` |

### B4. `internal/domain/agent/tools/pdf_tool.go` — generate_pdf

| 属性 | 值 |
|------|-----|
| **工具名** | `generate_pdf` |
| **描述** | 生成标准格式的报销单 PDF 文件 |
| **输入** | `{"reimbursement_id": uint}` |
| **输出** | `{pdf_path, reimbursement_no}` |
| **后端** | `infra.PDFGenerator.GenerateReimbursementPDF()` |
| **依赖注入** | 需注入 `infra.PDFGenerator` 接口 |

### B5. `internal/domain/agent/tools/email_tool.go` — send_email

| 属性 | 值 |
|------|-----|
| **工具名** | `send_email` |
| **描述** | 将报销单 PDF 通过邮件发送给审批人 |
| **输入** | `{"reimbursement_id": uint, "pdf_path": string}` |
| **输出** | `{success: bool, message_id, error}` |
| **后端** | `infra.EmailSender.SendReimbursementNotification()` |
| **依赖注入** | 需注入 `infra.EmailSender` 接口 |
| **错误处理** | 发送失败返回 `{success: false, error: "..."}`，不阻断流程 |

### B6. `internal/domain/agent/tools/progress_tool.go` — query_progress

| 属性 | 值 |
|------|-----|
| **工具名** | `query_progress` |
| **描述** | 查询指定报销单的审批进度（各审批人的审批状态） |
| **输入** | `{"reimbursement_no": string, "employee_id": string}` |
| **输出** | `{reimbursements: [{no, status, total_amount, submit_note, approvals}]}` |
| **后端** | `reimbursement.ReimbursementBiz.GetByNo()` + `approval.ApprovalBiz.GetProgress()` |
| **依赖注入** | 需注入 `*reimbursement.ReimbursementBiz` + `*approval.ApprovalBiz` |

### B7. `internal/domain/agent/tools/query_tool.go` — query_reimbursements

| 属性 | 值 |
|------|-----|
| **工具名** | `query_reimbursements` |
| **描述** | 查询用户的报销记录列表（支持分页） |
| **输入** | `{"employee_id": string, "page": int, "page_size": int}` |
| **输出** | `{list: [{no, status, total_amount, created_at}], total}` |
| **后端** | `reimbursement.ReimbursementBiz.List()` |
| **依赖注入** | 需注入 `*reimbursement.ReimbursementBiz` |

### B8. `internal/domain/agent/tools/provider.go` — ToolSet 聚合

**新建**

```go
package tools

import "github.com/cloudwego/eino/components/tool"

// ToolSet 聚合所有 Agent 可用工具，按阶段分组提供
type ToolSet struct {
    RecognizeInvoice     tool.InvokableTool
    CheckCompliance      tool.InvokableTool
    CheckBudget          tool.InvokableTool
    GeneratePDF          tool.InvokableTool
    SendEmail            tool.InvokableTool
    QueryProgress        tool.InvokableTool
    QueryReimbursements  tool.InvokableTool
}

// GetPhase1Tools 返回 Phase 1（信息收集）可用的工具
func (ts *ToolSet) GetPhase1Tools() []tool.InvokableTool { ... }

// GetPhase2Tools 返回 Phase 2（校验确认）可用的工具
func (ts *ToolSet) GetPhase2Tools() []tool.InvokableTool { ... }

// GetPhase3Tools 返回 Phase 3（执行提交）可用的工具
func (ts *ToolSet) GetPhase3Tools() []tool.InvokableTool { ... }
```

| Phase | 可用工具 |
|-------|---------|
| Phase 1 | `recognize_invoice`, `check_compliance`（仅规则查询） |
| Phase 2 | `check_compliance`（校验模式）, `check_budget` |
| Phase 3 | `generate_pdf`, `send_email`, `query_progress` |

---

## 四、Phase C — Graph 层

### C1. Graph 状态管理 — `internal/domain/agent/phase/guard.go`

**新建**

两个核心护卫条件：

```go
// Phase1Guard Phase 1 → Phase 2 的护卫条件
// 要求：≥1张票据已上传 AND 每张票据有金额 AND 用户确认
func Phase1Guard(ctx context.Context, state *ReimbursementState) *GuardResult

// Phase2Guard Phase 2 → Phase 3 的护卫条件
// 要求：合规通过 AND 预算可接受 AND FinalConfirmed=true
func Phase2Guard(ctx context.Context, state *ReimbursementState) *GuardResult

// Phase3Complete Phase 3 结束条件
func Phase3Complete(ctx context.Context, state *ReimbursementState) bool
```

### C2. `internal/domain/agent/phase/phase1_collect.go` — 信息收集 Agent

| 属性 | 值 |
|------|-----|
| **节点类型** | ChatModelAgent（Eino） |
| **可用工具** | `recognize_invoice`, `check_compliance`（仅规则查询，不做校验） |
| **系统 Prompt** | Phase 1 模板（来自 `prompt.go`） |
| **退出条件** | ≥1 张票据已上传 AND 每张票据有金额 AND 用户确认 |
| **防死循环** | 超过 `MaxPhaseTurns` 轮 → 强制退出并提示 |
| **多票据策略** | Agent 自主决定串行确认 / 批量预览 / 快速模式 |

**StatePreHandler**：每轮更新 `Phase1Turns++`，注入当前票据列表摘要到 Prompt。

### C3. `internal/domain/agent/phase/phase2_validate.go` — 校验确认 Agent

| 属性 | 值 |
|------|-----|
| **节点类型** | ChatModelAgent |
| **可用工具** | `check_compliance`（校验模式）, `check_budget` |
| **系统 Prompt** | Phase 2 模板（含修正感知逻辑） |
| **退出条件** | 合规 pass 且 预算可接受 且 `FinalConfirmed = true` |
| **修正感知** | 若 `IsUserModified=true`，Agent 额外标注并提醒审批人将复核 |
| **Warning 处理** | Agent 展示超标项 → 询问用户 → 用户确认后设置 `FinalConfirmed` |

### C4. `internal/domain/agent/phase/phase3_execute.go` — 执行提交 Agent

| 属性 | 值 |
|------|-----|
| **节点类型** | ChatModelAgent |
| **可用工具** | `generate_pdf`, `send_email`, `query_progress` |
| **系统 Prompt** | Phase 3 模板 |
| **退出条件** | PDF 已生成（邮件可失败） |
| **错误处理** | 邮件失败仅提醒，不阻塞完成 |

### C5. `internal/domain/agent/graph/reimbursement.go` — 报销子流程 Graph

**Graph 拓扑**：

```
START
  │
  ▼
Phase1Agent (ChatModelAgent: OCR + 规则查询)
  │
  ▼
Phase1Guard (Lambda: 检查票据完整性 + 用户确认)
  ├── PASS ──→ Phase2Agent
  └── FAIL ──→ 返回提示 ──→ 回到 Phase1Agent
                │
                ▼
          Phase2Agent (ChatModelAgent: 合规 + 预算)
                │
                ▼
          Phase2Guard (Lambda: 合规通过 + 预算可接受 + 最终确认)
                ├── PASS ──→ Phase3Agent
                └── FAIL ──→ 返回提示 ──→ 回到 Phase2Agent
                              │
                              ▼
                        Phase3Agent (ChatModelAgent: PDF + 邮件)
                              │
                              ▼
                             END
```

关键设计：
- 使用 `compose.WithGenLocalState` 创建 `*ReimbursementState`
- 使用 `compose.WithInterruptBeforeNodes("Phase1Guard", "Phase2Guard")` 在护卫节点前暂停
- 使用 `compose.WithMaxRunSteps(50)` 防止无限循环
- Branch 节点根据 Guard 结果决定继续前进还是回退

### C6. `internal/domain/agent/graph/progress.go` — 进度查询子流程

```
START
  │
  ▼
QueryAgent (ChatModelAgent: query_progress + query_reimbursements)
  │
  ▼
END
```

无 Guard，Agent 自主对话。

### C7. `internal/domain/agent/graph/budget.go` — 预算查询子流程

```
START
  │
  ▼
BudgetQuery (Lambda: 从 Session 上下文获取部门ID)
  │
  ▼
BudgetAgent (ChatModelAgent: check_budget)
  │
  ▼
END
```

### C8. `internal/domain/agent/graph/policy.go` — 政策咨询

```
START
  │
  ▼
PolicyAgent (ChatModelAgent: 无工具，纯 LLM 回复)
  │
  ▼
END
```

简单 ChatModel + ChatTemplate 链，Prompt 锚定政策知识库。

### C9. `internal/domain/agent/graph/modify.go` — 修改报销子流程

```
START
  │
  ▼
ModifyAgent (ChatModelAgent: query_progress)
  │
  ▼
END
```

适用场景：报销被驳回后修改金额/类别重新提交。

### C10. `internal/domain/agent/graph/root.go` — 顶层 Root Graph

```
START
  │
  ▼
IntentClassifier (ChatModel) ──→ 输出 JSON: {intent, entities, confidence}
  │
  ▼
IntentRouter (Lambda: 根据 intent 路由)
  ├── new_reimbursement    → ReimbursementSubGraph
  ├── query_progress        → ProgressSubGraph
  ├── query_budget          → BudgetSubGraph
  ├── policy_question       → PolicyHandler
  ├── modify_reimbursement  → ModifySubGraph
  └── general_chat          → GeneralChatHandler (LLM 直接回复)
  │
  ▼
END
```

关键设计：
- `IntentClassifier` 为 ChatModel 节点，Prompt 使用 intent classification 模板
- `IntentRouter` 为 Lambda 节点，解析 LLM 输出 JSON → 返回路由目标
- 使用 `compose.NewGraphBranch` 实现条件路由
- Sub-Graph 使用 `g.AddGraphNode("reimburse_workflow", subGraph)` 嵌套
- 置信度 < 阈值时在 Router 中暂停并询问用户确认

### C11. `internal/domain/agent/prompt.go` — Prompt 模板库

**新建** — 包含以下模板函数：

| 函数 | 用途 |
|------|------|
| `BuildSystemPrompt(phase, state)` | 系统级 Agent Prompt |
| `BuildIntentClassifyPrompt(userMsg, history)` | 意图分类 Prompt |
| `BuildPhase1Prompt(state)` | Phase 1 收集指令 |
| `BuildPhase2Prompt(state)` | Phase 2 校验指令 |
| `BuildPhase3Prompt(state)` | Phase 3 提交指令 |
| `BuildGeneralChatPrompt()` | 通用对话 Prompt |
| `BuildStateSummary(state)` | 状态摘要（注入到 Prompt） |
| `BuildModifiedInvoicesWarning(state)` | 修正票据风险提示 |

---

## 五、Phase D — 集成层

### D1. `internal/domain/agent/checkpoint.go` — MySQL Checkpoint 持久化

**新建**

```go
// CheckpointStore Eino Checkpoint 持久化接口
type CheckpointStore interface {
    Get(ctx context.Context, key string) ([]byte, bool, error)
    Set(ctx context.Context, key string, value []byte) error
    Delete(ctx context.Context, key string) error
}

// MySQLCheckpointStore 基于 MySQL 的 Checkpoint 实现
type MySQLCheckpointStore struct {
    db *gorm.DB
}

// CheckpointRecord 对应的 GORM 模型
type CheckpointRecord struct {
    ID        string    `gorm:"primaryKey;type:varchar(128)"`
    Data      string    `gorm:"type:mediumtext;not null"`
    UpdatedAt time.Time
}
```

**Checkpoint ID 格式**：`GraphName:SessionID`（如 `reimbursement_workflow:550e8400-...`）

**清理策略**：定时任务每小时清理 `updated_at < NOW() - INTERVAL 1 HOUR` 的孤儿记录。

### D2. `internal/domain/agent/runner.go` — AgentRunner

**新建**

```go
type AgentRunner struct {
    rootGraph       compose.Runnable[AgentInput, *schema.Message]
    sessionStore    infra.SessionStore
    checkpointStore CheckpointStore
    config          *AgentConfig
    logger          *log.Logger
}

// StreamChat 执行一次对话交互，通过 SSE Writer 流式返回
func (r *AgentRunner) StreamChat(ctx context.Context, input AgentInput, w SSEWriter) error
```

**StreamChat 核心流程**：
1. 从 `SessionStore.GetHistory()` 获取历史消息 → 传入 Graph Input
2. 持久化用户消息到 SessionStore
3. `rootGraph.Stream(ctx, input)` → 获取 `StreamReader[*schema.Message]`
4. 循环读取流 → 转换为 SSE 事件 → 写入 `sseWriter`
5. 收集完整的 assistant 回复 → 持久化到 SessionStore
6. 检查流程是否结束 → 清理 Checkpoint / Session 缓存

### D3. `internal/domain/agent/sse.go` — SSE 事件定义与写入

**新建**

```go
// SSEWriter SSE 事件写入接口
type SSEWriter interface {
    WriteEvent(event SSEEvent) error
    Flush() error
}

// SSEEvent SSE 事件（对应 agent-design-supplement.md §4 的 8 种类型）
type SSEEvent struct {
    Type string `json:"type"`
    Data any    `json:"data"`
}
```

**8 种事件类型**：

| 事件类型 | 来源节点 | 携带数据 | 前端渲染 |
|---------|---------|------|------|
| `thinking` | LLM 节点 | `{message: "..."}` | 思考动画 + 文字 |
| `tool_call` | ToolNode | `{tool: "...", input: {...}}` | 工具调用卡片 |
| `tool_result` | ToolNode | `{tool: "...", output: {...}}` | 卡片状态更新 |
| `message` | LLM 节点 | `{content: "...", delta: true}` | 打字机追加文本 |
| `phase_change` | Guard | `{from, to, summary}` | 阶段过渡提示 |
| `confirm_required` | Guard | `{prompt: "..."}` | 确认按钮组 |
| `error` | 任意节点 | `{message: "...", retry: bool}` | 错误提示 + 重试 |
| `done` | END | `{}` | 停止加载 |

### D4. `internal/domain/agent/service.go` — HTTP Service 层

**新建**

```go
type AgentService struct {
    runner *AgentRunner
    logger *log.Logger
}

// HandleChat 处理 GET /api/chat/stream 请求（SSE 流式响应）
func (s *AgentService) HandleChat(c *gin.Context) {
    // 1. 解析 session_id（query param）和 message（query param）
    // 2. 从 JWT claims 获取 user_id, employee_id, role
    // 3. 设置 SSE 响应头
    // 4. 获取 gin.ResponseWriter + http.Flusher
    // 5. 构建 AgentInput
    // 6. 调用 runner.StreamChat(ctx, input, sseWriter)
    // 7. 异常捕获 → 返回 error SSE 事件
}
```

### D5. `internal/router/root.go` — 替换 SSE 端点

**修改现有文件**（约第 113-116 行）

```go
// 替换前：
api.GET("/chat/stream", func(c *gin.Context) {
    c.JSON(501, gin.H{"message": "Agent 对话接口待实现"})
})

// 替换后：
api.GET("/chat/stream", hub.AgentService.HandleChat)
```

### D6. `internal/domain/hub.go` — ServiceHub 扩展

**修改现有文件**

```go
type ServiceHub struct {
    // ... 现有 Service ...
    AgentService *agent.AgentService  // ← 新增
}

func NewServiceHub(
    // ... 现有参数 ...
    agentSvc *agent.AgentService,  // ← 新增
) *ServiceHub { ... }
```

---

## 六、实施顺序与依赖图

```
                    ┌─────────────────────────────────────┐
                    │         Phase A: 基础设施             │
                    │  dto.go + config.go + provider.go    │
                    │  + testutil/db.go 修复               │
                    │  + config.yaml 补充                  │
                    └──────────────┬──────────────────────┘
                                   │
                    ┌──────────────▼──────────────────────┐
                    │          Phase B: 工具层              │
                    │  tools/provider.go                   │
                    │  tools/ocr_tool.go                   │
                    │  tools/compliance_tool.go            │
                    │  tools/budget_tool.go                │
                    │  tools/pdf_tool.go                   │
                    │  tools/email_tool.go                 │
                    │  tools/progress_tool.go              │
                    │  tools/query_tool.go                 │
                    └──────────────┬──────────────────────┘
                                   │
              ┌────────────────────┼────────────────────┐
              ▼                    ▼                    ▼
    ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐
    │ phase/guard.go   │  │ prompt.go       │  │ checkpoint.go   │
    │ phase/phase1.go  │  │ (Prompt 模板库)  │  │ (Checkpoint存储) │
    │ phase/phase2.go  │  └────────┬────────┘  └────────┬────────┘
    │ phase/phase3.go  │           │                    │
    └────────┬─────────┘           │                    │
             │                     │                    │
             └─────────┬───────────┘                    │
                       │                                │
              ┌────────▼────────────────────────────────▼──┐
              │           Phase C: Graph 层                 │
              │  graph/root.go          (核心：编译Root)    │
              │  graph/reimbursement.go (核心：报销子流程)   │
              │  graph/progress.go                          │
              │  graph/budget.go                            │
              │  graph/policy.go                            │
              │  graph/modify.go                            │
              └──────────────┬──────────────────────────────┘
                             │
              ┌──────────────▼──────────────────────────────┐
              │           Phase D: 集成层                    │
              │  runner.go  (AgentRunner)                    │
              │  sse.go     (SSE事件定义)                    │
              │  service.go (HTTP Service)                   │
              │  hub.go 修改                                 │
              │  router/root.go 替换                         │
              │  testutil/db.go 最终完善                     │
              └──────────────┬──────────────────────────────┘
                             │
              ┌──────────────▼──────────────────────────────┐
              │          验证与联调                          │
              │  make wire → make build → 集成测试           │
              │  curl /api/chat/stream 端到端验证            │
              └─────────────────────────────────────────────┘
```

---

## 七、关键依赖注入链

Wire 需要新增的注入链（数据流向）：

```
config.yaml (viper.Viper)
    │
    ├── agent: {...}  ──────────→  AgentConfig
    │
    ├── openai: {...}  ──────────→  ChatModel (Eino OpenAI)
    │                                │
    │                                ├──→ IntentClassifier
    │                                ├──→ Phase1Agent
    │                                ├──→ Phase2Agent
    │                                └──→ Phase3Agent
    │
    └── compliance: {...}  ─────→  KnowledgeBase → ComplianceBiz
                                                      │
                                                      └──→ check_compliance Tool

infra.Data (MySQL + Redis)
    ├──→ ReimbursementRepo → ReimbursementBiz
    │                              ├──→ query_progress Tool
    │                              └──→ query_reimbursements Tool
    ├──→ ApprovalRepo → ApprovalBiz
    ├──→ BudgetRepo → BudgetBiz ──→ check_budget Tool
    ├──→ MySQLSessionStore (SessionStore)
    │                              └──→ AgentRunner
    └──→ MySQLCheckpointStore (CheckpointStore)
                                   └──→ AgentRunner → Root Graph

infra.OCRRecognizer  ────→ recognize_invoice Tool
infra.PDFGenerator   ────→ generate_pdf Tool
infra.EmailSender    ────→ send_email Tool
```

---

## 八、测试策略（四层金字塔）

### 第 1 层：Tool 单元测试（每个 Tool 3-5 个用例）

| Tool | 测试用例 |
|------|---------|
| `ocr_tool` | 成功识别 / OCR 失败降级 / 空图片输入 |
| `compliance_tool` | pass / warning / error / 未知类别 |
| `budget_tool` | 预算充足 / 预算不足触发特殊审批 / 未设置预算 |
| `pdf_tool` | 正常生成 / 报销单不存在 |
| `email_tool` | 发送成功 / 发送失败不阻断 |
| `progress_tool` | 查特定单号 / 查最近5条 / 无记录 |
| `query_tool` | 分页查询 / 空结果 |

### 第 2 层：Graph 流程测试（Mock LLM，每个子流程 3-5 个）

| 子流程 | 测试用例 |
|--------|---------|
| `reimbursement` | 完整三阶段流程 / Phase1 Guard 阻止未确认 / Phase2 error 阻止 / warning→用户确认继续 |
| `progress` | 查特定单号 / 模糊查询 |
| `budget` | 查当前财年预算 |
| `policy` | 简单政策咨询 |
| `modify` | 修改金额后重新提交 |

### 第 3 层：意图分类测试（真实 LLM）

| 场景 | 验证标准 |
|------|---------|
| 意图分类准确性 | 10 个标注样本准确率 > 90% |

```go
var intentTestCases = []struct{ input, intent string }{
    {"我要报销一张发票", "new_reimbursement"},
    {"REIMB-2026-0001 的状态", "query_progress"},
    {"上周的报销批了吗", "query_progress"},
    {"还剩多少预算", "query_budget"},
    {"住宿标准是多少", "policy_question"},
    {"驳回的帮我改金额重提", "modify_reimbursement"},
    {"你好", "general_chat"},
    {"谢谢", "general_chat"},
    {"报销差旅费 1500", "new_reimbursement"},
    {"我提交的报销到哪了", "query_progress"},
}
```

### 第 4 层：端到端测试（真实 LLM + 完整对话）

| 场景 | 验证点 |
|------|--------|
| 完整报销流程 | 创建 → OCR → 合规 → 预算 → 确认 → 提交 |
| 合规 warning 处理 | 超标展示 + 用户确认继续 |
| 驳回重提 | 修改金额 → 重新走三阶段 |
| SSE 流式输出 | `curl` 验证所有 8 种事件类型 |

---

## 九、文件清单（按实施顺序）

| # | 文件路径 | 阶段 | 操作 | 行数估算 |
|:--:|------|:--:|------|:--:|
| 1 | `internal/domain/agent/dto.go` | A | 新建 | ~150 |
| 2 | `internal/domain/agent/config.go` | A | 新建 | ~60 |
| 3 | `internal/domain/agent/provider.go` | A | 新建 | ~40 |
| 4 | `internal/domain/provider.go` | A | 修改 | +2 |
| 5 | `internal/testutil/db.go` | A | 修改 | +30 |
| 6 | `config.yaml` | A | 修改 | +5 |
| 7 | `config.yaml.example` | A | 修改 | +5 |
| 8 | `internal/domain/agent/tools/provider.go` | B | 新建 | ~80 |
| 9 | `internal/domain/agent/tools/ocr_tool.go` | B | 新建 | ~100 |
| 10 | `internal/domain/agent/tools/compliance_tool.go` | B | 新建 | ~90 |
| 11 | `internal/domain/agent/tools/budget_tool.go` | B | 新建 | ~80 |
| 12 | `internal/domain/agent/tools/pdf_tool.go` | B | 新建 | ~80 |
| 13 | `internal/domain/agent/tools/email_tool.go` | B | 新建 | ~80 |
| 14 | `internal/domain/agent/tools/progress_tool.go` | B | 新建 | ~90 |
| 15 | `internal/domain/agent/tools/query_tool.go` | B | 新建 | ~80 |
| 16 | `internal/domain/agent/prompt.go` | C | 新建 | ~200 |
| 17 | `internal/domain/agent/phase/guard.go` | C | 新建 | ~80 |
| 18 | `internal/domain/agent/phase/phase1_collect.go` | C | 新建 | ~150 |
| 19 | `internal/domain/agent/phase/phase2_validate.go` | C | 新建 | ~150 |
| 20 | `internal/domain/agent/phase/phase3_execute.go` | C | 新建 | ~120 |
| 21 | `internal/domain/agent/graph/root.go` | C | 新建 | ~150 |
| 22 | `internal/domain/agent/graph/reimbursement.go` | C | 新建 | ~180 |
| 23 | `internal/domain/agent/graph/progress.go` | C | 新建 | ~60 |
| 24 | `internal/domain/agent/graph/budget.go` | C | 新建 | ~60 |
| 25 | `internal/domain/agent/graph/policy.go` | C | 新建 | ~40 |
| 26 | `internal/domain/agent/graph/modify.go` | C | 新建 | ~60 |
| 27 | `internal/domain/agent/checkpoint.go` | D | 新建 | ~80 |
| 28 | `internal/domain/agent/runner.go` | D | 新建 | ~200 |
| 29 | `internal/domain/agent/sse.go` | D | 新建 | ~100 |
| 30 | `internal/domain/agent/service.go` | D | 新建 | ~100 |
| 31 | `internal/domain/hub.go` | D | 修改 | +3 |
| 32 | `internal/router/root.go` | D | 修改 | -3 +10 |
| **合计** | **~32 文件** | | **~2,600 行** | |

---

## 十、风险与注意事项

| 风险 | 影响 | 缓解措施 |
|------|------|---------|
| Eino 版本 API 差异 | ChatModelAgent 接口与设计不匹配 | 优先使用 `compose.Graph` 底层 API，非 ADK |
| LLM 意图分类不准 | 路由到错误子流程 | 设置 `confidence_threshold`，低置信度询问用户 |
| Phase 内 Agent 死循环 | 无限制多轮交互 | `max_phase_turns: 10`，超限强制退出 |
| Checkpoint 孤儿数据 | MySQL 堆积无用记录 | 定时任务每小时清理 `updated_at < 1h` 的记录 |
| OCR 多模态 LLM 不稳定 | 识别失败率高 | OCR 失败 → 引导用户手动输入，不阻塞流程 |
| Mock PDF/Email 不满足生产 | 审批人收不到真实通知 | 标注 TODO，后续替换为真实 SMTP/PDF 实现 |
| Graph 编译复杂度 | 循环依赖、类型不匹配 | 每阶段完成后 `go build` 验证 |
| 新依赖未安装 | 编译失败 | 实施前 `go mod tidy` 确认所有 Eino 子包可用 |

---

## 十一、验收标准

完成时需满足以下条件：

- [ ] `make wire` 通过（Wire DI 所有依赖正确注入）
- [ ] `make build` 通过（所有 Go 文件编译无错误）
- [ ] `lsp_diagnostics` 全线绿色（0 错误 0 警告）
- [ ] 所有 Tool 单元测试通过（7 个 tool × 3-5 用例）
- [ ] 所有 Graph 流程测试通过（Mock LLM，每个子流程 3-5 场景）
- [ ] `curl -N "http://localhost:8080/api/chat/stream?session_id=xxx&message=我要报销"` 返回 SSE 流
- [ ] 意图分类准确率 > 90%（基于 10 个标注样本）
- [ ] Checkpoint 持久化读写在 MySQL 可验证（查询 `checkpoint_records` 表）
- [ ] Session 消息在 MySQL 可查询（查询 `session_messages` 表）
- [ ] Redis 缓存未命中时能正确回源 MySQL
- [ ] 所有现有测试仍通过（不引入回归）

---

## 附录 A：Eino 关键 API 速查

### Graph 构建

```go
g := compose.NewGraph[Input, Output](
    compose.WithGenLocalState(func(ctx context.Context) *MyState { return &MyState{} }),
)

g.AddChatModelNode("name", chatModel, compose.WithStatePreHandler(fn))
g.AddToolsNode("name", toolsNode)
g.AddLambdaNode("name", compose.InvokableLambda(fn))
g.AddGraphNode("name", subGraph)  // 嵌套子图

g.AddEdge(compose.START, "firstNode")
g.AddEdge("lastNode", compose.END)

g.AddBranch("node", compose.NewGraphBranch(routingFn, destinations))

runnable, _ := g.Compile(ctx,
    compose.WithCheckPointStore(store),
    compose.WithInterruptBeforeNodes([]string{"guard"}),
    compose.WithMaxRunSteps(50),
)
```

### 执行

```go
// 非流式
result, _ := runnable.Invoke(ctx, input)

// 流式
stream, _ := runnable.Stream(ctx, input)
for {
    chunk, err := stream.Recv()
    if errors.Is(err, io.EOF) { break }
    // process chunk
}
```

### 状态操作

```go
// 在 Lambda/Branch 内读写 State
err := compose.ProcessState[*MyState](ctx, func(ctx context.Context, s *MyState) error {
    s.Counter++
    return nil
})

// 注册类型以支持序列化
func init() {
    schema.RegisterName[*MyState]("my_state_v1")
}
```

### Dynamic Interrupt（v0.7.0+）

```go
// 在节点内暂停等待外部输入
return nil, compose.StatefulInterrupt(ctx, &ApprovalInfo{...}, localState)

// 恢复时注入数据
ctx = compose.ResumeWithData(ctx, interruptID, &ApprovalResult{Approved: true})
```

---

*计划结束。*
