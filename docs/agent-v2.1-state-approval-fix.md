# Agent 层 v2.1 — State 持久化 & 审批链路修复方案

> **日期**: 2026-07-06  
> **前置文档**: `agent-redesign-v2.md`  
> **修复**: 跨请求 State 丢失 + 审批链路分离 + Phase3 写 DB

---

## 目录

1. [问题诊断](#1-问题诊断)
2. [目标架构](#2-目标架构)
3. [改动清单](#3-改动清单)
4. [详细设计](#4-详细设计)
5. [执行计划](#5-执行计划)

---

## 1. 问题诊断

### 1.1 State 跨请求丢失（P0 阻塞）

```
请求1: "我要报销"  → Graph执行 → ReimbursementState创建 → Phase1未完成 → SSE结束
请求2: "已上传发票" → Graph执行 → ReimbursementState ⚠️ 重新创建（空白），票据全丢！
```

**根因**：`ReimbursementState` 只存在于单次 Graph 执行期间（`WithGenLocalState`），下一个请求创建新的空白 State。

### 1.2 审批链路与 Agent 混淆

当前 Phase 2 的 `FinalConfirmed` 语义模糊——是员工确认还是审批人批准？实际上：
- **员工确认提交** → 应该在 Agent 对话内完成 → Phase 2 `FinalConfirmed`
- **审批人批准** → 独立的 REST API 流程 → `POST /api/approvals/:id/approve`

### 1.3 Phase 3 缺少 DB 写入

Phase 3 当前只生成 PDF + 发邮件，但**报销单从未写入数据库**。PDF 工具调用 `reimbursementBiz.GetByID()` 查询不存在的记录。

---

## 2. 目标架构

### 2.1 整体流程

```
┌─── 链 A: Agent (员工自助，SSE 多轮对话) ────────────────────────────────┐
│                                                                          │
│  请求1: "我要报销差旅费"                                                  │
│    → 从 SessionStore 恢复 ReimbursementState (首次为空)                   │
│    → Graph 执行，Phase1: LLM引导→上传票据→OCR                            │
│    → Phase1 Guard fail (无票据) → 保存 State → SSE: "请上传票据"          │
│                                                                          │
│  请求2: "已上传 /uploads/inv001.png"                                     │
│    → 从 SessionStore 恢复 ReimbursementState (含上一轮票据)               │
│    → Phase1: OCR → 识别到 ¥500 → "请确认"                                │
│    → Phase1 Guard fail (UserConfirmed=false) → 保存 State               │
│                                                                          │
│  请求3: "确认"                                                           │
│    → 从 SessionStore 恢复 State → LLM理解"确认" → UserConfirmed=true     │
│    → Phase1 Guard pass → Phase2                                          │
│    → 合规检查 → 预算检查 → "合规通过，预算充足，确认提交？"                │
│    → Phase2 Guard fail (FinalConfirmed=false) → 保存 State               │
│                                                                          │
│  请求4: "确认提交"                                                       │
│    → 从 SessionStore 恢复 State → FinalConfirmed=true                    │
│    → Phase2 Guard pass → Phase3                                          │
│    → LLM顺序调用:                                                        │
│       ① create_reimbursement → DB写入, 状态=draft                        │
│       ② submit_reimbursement → 冻结预算, 创建审批链, 状态=pending         │
│       ③ generate_pdf → 使用DB记录生成PDF                                 │
│       ④ send_email → 发送审批通知                                        │
│    → "报销单 REIMB-2026-0001 已提交，请等待审批人处理"                    │
│    → 保存 State → SSE 结束                                               │
└──────────────────────────────────────┬───────────────────────────────────┘
                                       │
                                   写入 DB
                                 status=pending
                                       │
┌─── 链 B: REST API (审批人，独立流程) ────────────────────────────────────┐
│                                                                          │
│  GET  /api/reimbursements/pending    审批人查看待审批列表                  │
│  POST /api/approvals/:id/approve     逐级审批通过                         │
│  POST /api/approvals/:id/reject      驳回（解冻预算，状态=rejected）       │
│                                                                          │
│  ReimbursementBiz.Approve():                                             │
│    → Mark all ApprovalRecords approved → Deduct budget → Status=approved  │
│                                                                          │
│  ReimbursementBiz.Reject():                                              │
│    → Unfreeze budget → Status=rejected                                   │
└──────────────────────────────────────────────────────────────────────────┘
```

### 2.2 关键设计原则

| 原则 | 说明 |
|------|------|
| **State 跨请求持久** | `ReimbursementState` 序列化为 JSON，通过 `SessionStore` 按 `sessionID` 持久化 |
| **Agent 只服务员工** | 报销三阶段全部是员工自助操作，审批人通过 REST API 审批 |
| **Phase3 写 DB** | 新增 `create_reimbursement` 工具，LLM 自主决定调用顺序 |
| **确认消息走 SSE** | 用户的所有确认（"确认票据"、"确认提交"）都通过 `GET /api/chat/stream?message=确认` |

---

## 3. 改动清单

### 3.1 新增文件

| 文件 | 说明 |
|------|------|
| `infra/state_store.go` | `StateStore` 接口定义（State 持久化） |
| `infra/state_mysql.go` | MySQL 实现（复用 GORM） |
| `internal/domain/agent/tools/create_reimb_tool.go` | `create_reimbursement` 工具 |
| `internal/domain/agent/tools/submit_reimb_tool.go` | `submit_reimbursement` 工具 |

### 3.2 修改文件

| 文件 | 变更内容 |
|------|---------|
| `infra/session.go` | `SessionStore` 接口增加 `SaveState`/`GetState`/`DeleteState` |
| `infra/session_mysql.go` | 实现 State 持久化（新建表 `session_states`） |
| `infra/provider.go` | Wire 注册 `StateStore` |
| `internal/domain/agent/runner.go` | `StreamChat` 增加 State 加载/保存逻辑 |
| `internal/domain/agent/graph/reimbursement.go` | `WithGenLocalState` 使用已保存的 State |
| `internal/domain/agent/graph/react_phase.go` | `buildReActPhase` 支持从 context 注入已保存状态 |
| `internal/domain/agent/tools/provider.go` | Phase 3 工具列表增加 `create_reimb`+`submit_reimb` |
| `internal/domain/agent/prompt.go` | Phase 3 提示词增加创建+提交工具说明 |
| `internal/domain/agent/dto.go` | `ReimbursementState` 增加 JSON tag 确保序列化 |

### 3.3 不变文件

| 文件 | 原因 |
|------|------|
| `phase/guard.go` | Guard 条件无需修改（本已是员工侧确认） |
| `graph/root.go` | 意图路由逻辑不变 |
| `graph/progress/budget/policy/modify.go` | 简单子流程不变 |
| 所有 `tools/*.go`（除新增和 provider.go） | 现有工具实现不变 |

---

## 4. 详细设计

### 4.1 State 持久化

#### SessionStore 接口扩展

```go
// infra/session.go
type SessionStore interface {
    SaveMessages(ctx context.Context, sessionID string, msgs []*schema.Message) error
    GetHistory(ctx context.Context, sessionID string, limit int) ([]*schema.Message, error)
    Clear(ctx context.Context, sessionID string) error

    // 🆕 State 持久化
    SaveState(ctx context.Context, sessionID string, key string, state any) error
    GetState(ctx context.Context, sessionID string, key string, target any) (bool, error)
    DeleteState(ctx context.Context, sessionID string, key string) error
}
```

#### State Key 约定

```go
const (
    StateKeyReimbursement = "reimbursement"  // → agent.ReimbursementState
)
```

#### MySQL 实现

```sql
CREATE TABLE session_states (
    id VARCHAR(256) PRIMARY KEY,     -- sessionID:stateKey
    session_id VARCHAR(128) NOT NULL,
    state_key VARCHAR(64) NOT NULL,
    data MEDIUMTEXT NOT NULL,        -- JSON serialized state
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    INDEX idx_session_key (session_id, state_key)
);
```

#### AgentRunner 改动

```go
func (r *AgentRunner) StreamChat(ctx context.Context, input AgentInput, sseWriter SSEWriter) error {
    // ... existing preamble (thinking event, history load) ...

    // 🆕 加载已保存的 ReimbursementState
    var savedState agent.ReimbursementState
    found, _ := r.sessionStore.GetState(ctx, input.SessionID, "reimbursement", &savedState)

    // 🆕 将 State 注入 context，Graph 通过它初始化 WithGenLocalState
    if found {
        ctx = context.WithValue(ctx, stateKey{}, &savedState)
        r.logger.Debug("恢复已保存的报销状态", zap.String("sessionID", input.SessionID),
            zap.String("当前阶段", savedState.CurrentPhase))
    }

    // ... Graph execution ...

    // 🆕 提取并保存最终 State
    _ = compose.ProcessState(ctx, func(ctx context.Context, rs *agent.ReimbursementState) error {
        if err := r.sessionStore.SaveState(ctx, input.SessionID, "reimbursement", rs); err != nil {
            r.logger.Warn("保存报销状态失败", zap.Error(err))
        }
        return nil
    })

    // ... SSE done event ...
}
```

#### Graph 初始化改动

```go
// reimbursement.go
g := compose.NewGraph[*schema.Message, *schema.Message](
    compose.WithGenLocalState(func(ctx context.Context) *agent.ReimbursementState {
        // 尝试从 context 恢复已保存的状态
        if saved, ok := ctx.Value(stateKey{}).(*agent.ReimbursementState); ok && saved != nil {
            return saved
        }
        // 首次请求：创建空白状态
        return &agent.ReimbursementState{CurrentPhase: "phase1_collect"}
    }),
)
```

### 4.2 新增工具：create_reimbursement

```go
// tools/create_reimb_tool.go

// CreateReimbInput 创建报销单工具的输入参数
type CreateReimbInput struct {
    EmployeeID   string `json:"employee_id" jsonschema:"required" jsonschema_description:"申请人工号"`
    EmployeeName string `json:"employee_name" jsonschema:"required" jsonschema_description:"申请人姓名"`
    DepartmentID uint   `json:"department_id" jsonschema:"required" jsonschema_description:"部门ID"`
    SubmitNote   string `json:"submit_note" jsonschema_description:"报销事由"`
}

// CreateReimbOutput 创建报销单工具的输出结果
type CreateReimbOutput struct {
    ReimbursementID uint   `json:"reimbursement_id"`  // 报销单 DB 主键
    ReimbursementNo string `json:"reimbursement_no"`  // 报销单号
    Status          string `json:"status"`            // 当前状态（draft）
}

func NewCreateReimbTool(reimbursementBiz *reimbursement.ReimbursementBiz, logger *log.Logger) *CreateReimbTool {
    t, err := utils.InferTool[CreateReimbInput, CreateReimbOutput](
        "create_reimbursement",
        "在系统中创建报销单草稿。调用此工具后，报销单将获得唯一单号并保存到数据库。后续可通过 submit_reimbursement 工具提交审批，或通过 generate_pdf 工具生成 PDF。",
        func(ctx context.Context, input CreateReimbInput) (CreateReimbOutput, error) {
            rm, err := reimbursementBiz.Create(input.EmployeeID, input.EmployeeName, input.DepartmentID, input.SubmitNote)
            if err != nil {
                return CreateReimbOutput{}, fmt.Errorf("创建报销单失败: %w", err)
            }
            return CreateReimbOutput{
                ReimbursementID: rm.ID,
                ReimbursementNo: rm.ReimbursementNo,
                Status:          rm.Status,
            }, nil
        },
    )
    // ...
}
```

### 4.3 新增工具：submit_reimbursement

```go
// tools/submit_reimb_tool.go

type SubmitReimbInput struct {
    ReimbursementID uint  `json:"reimbursement_id" jsonschema:"required" jsonschema_description:"报销单ID（由 create_reimbursement 返回）"`
    TotalAmount     int64 `json:"total_amount" jsonschema:"required" jsonschema_description:"报销总金额（分）"`
}

type SubmitReimbOutput struct {
    ReimbursementNo     string `json:"reimbursement_no"`
    Status              string `json:"status"`               // pending
    NeedSpecialApproval bool   `json:"need_special_approval"` // 是否触发特殊审批
}

func NewSubmitReimbTool(reimbursementBiz *reimbursement.ReimbursementBiz, logger *log.Logger) *SubmitReimbTool {
    t, err := utils.InferTool[SubmitReimbInput, SubmitReimbOutput](
        "submit_reimbursement",
        "提交报销单进入审批流程。此操作不可撤销，将冻结部门预算并创建审批链。调用前请确保用户已确认所有信息。需要先调用 create_reimbursement 获得报销单ID。",
        func(ctx context.Context, input SubmitReimbInput) (SubmitReimbOutput, error) {
            rm, err := reimbursementBiz.Submit(input.ReimbursementID, input.TotalAmount)
            if err != nil {
                return SubmitReimbOutput{}, fmt.Errorf("提交报销单失败: %w", err)
            }
            return SubmitReimbOutput{
                ReimbursementNo:     rm.ReimbursementNo,
                Status:              rm.Status,
                NeedSpecialApproval: rm.NeedSpecialApproval,
            }, nil
        },
    )
    // ...
}
```

### 4.4 Phase 3 提示词更新

```go
// prompt.go — phase3_execute 指令更新

case "phase3_execute":
    return strings.Join([]string{
        "## 执行提交阶段",
        "1. 首先调用 create_reimbursement 工具，传入员工信息和报销事由，创建报销单草稿",
        "2. 创建成功后，调用 submit_reimbursement 工具，传入报销单ID和总金额，提交审批",
        "3. 提交成功后，调用 generate_pdf 工具生成标准格式的报销单 PDF",
        "4. PDF 生成成功后，调用 send_email 工具发送审批通知",
        "5. 最后告知用户报销单号和后续步骤",
        "",
        "⚠️ 重要：必须按顺序调用这些工具。金额以分为单位传递给工具。",
        "⚠️ 提交后不可撤销，请在调用 submit_reimbursement 前确保用户已确认。",
    }, "\n")
```

### 4.5 Phase 3 工具列表更新

```go
// tools/provider.go
func (ts *ToolSet) GetPhase3Tools() []tool.InvokableTool {
    return []tool.InvokableTool{ts.CreateReimb, ts.SubmitReimb, ts.PDF, ts.Email, ts.Progress}
}
```

### 4.6 移除的工具

| 工具 | 原因 |
|------|------|
| `pdf_tool.go` 中直接查 DB 的逻辑 | 改为先 create_reimbursement 后 PDF 查询 |
| `email_tool.go` 中构造的假收件人列表 | 改为依赖 submit_reimbursement 创建的审批链 |

### 4.7 ReimbursementState 序列化

```go
// dto.go — 确保全部字段可序列化
type ReimbursementState struct {
    ReimbursementID     uint                 `json:"reimbursement_id"`
    ReimbursementNo     string               `json:"reimbursement_no"`
    DepartmentID        uint                 `json:"department_id"`
    EmployeeID          string               `json:"employee_id"`
    EmployeeName        string               `json:"employee_name"`
    CurrentPhase        string               `json:"current_phase"`
    Invoices            []InvoiceState       `json:"invoices"`
    TotalAmount         int64                `json:"total_amount"`
    UserConfirmed       bool                 `json:"user_confirmed"`
    ComplianceResult    *ComplianceCheckResult `json:"compliance_result,omitempty"`
    BudgetResult        *BudgetCheckResult   `json:"budget_result,omitempty"`
    FinalConfirmed      bool                 `json:"final_confirmed"`
    NeedSpecialApproval bool                 `json:"need_special_approval"`
    Phase1Turns         int                  `json:"phase1_turns"`
    Phase2Turns         int                  `json:"phase2_turns"`
    Phase3Turns         int                  `json:"phase3_turns"`
}
```

---

## 5. 执行计划

### Wave 1 — State 持久化（基础能力）

| # | 任务 | 文件 |
|---|------|------|
| 1.1 | `SessionStore` 接口增加 `SaveState`/`GetState`/`DeleteState` | `infra/session.go` |
| 1.2 | MySQL 实现：新建 `session_states` 表 + GORM 操作 | `infra/session_mysql.go` |
| 1.3 | State key 常量定义 | `internal/domain/agent/runner.go` |
| 1.4 | `AgentRunner.StreamChat` 增加 State 加载/保存 | `internal/domain/agent/runner.go` |
| 1.5 | `ReimbursementState` 增加 json tag | `internal/domain/agent/dto.go` |
| 1.6 | `WithGenLocalState` 从 context 恢复已保存 State | `internal/domain/agent/graph/reimbursement.go` |
| 1.7 | Wire 注册 `StateStore` 绑定 | `infra/provider.go` |

### Wave 2 — 新增工具 + Prompt 更新

| # | 任务 | 文件 |
|---|------|------|
| 2.1 | `create_reimbursement` 工具 | `tools/create_reimb_tool.go` |
| 2.2 | `submit_reimbursement` 工具 | `tools/submit_reimb_tool.go` |
| 2.3 | ToolSet 增加新工具字段 + GetPhase3Tools 更新 | `tools/provider.go` |
| 2.4 | Wire 注册新工具 | `tools/provider.go` |
| 2.5 | Phase 3 提示词更新 | `agent/prompt.go` |

### Wave 3 — PDF/Email 工具适配

| # | 任务 | 文件 |
|---|------|------|
| 3.1 | PDF 工具改为使用 `ReimbursementBiz.GetByID()`（已有工具链） | 不变 |
| 3.2 | Email 工具增加获取审批人收件人列表 | `tools/email_tool.go` |

### Wave 4 — 测试

| # | 任务 |
|---|------|
| 4.1 | `StateStore` 单元测试（Set/Get/Delete/覆盖） |
| 4.2 | `AgentRunner` State 持久化集成测试 |
| 4.3 | 新增工具单元测试 |
| 4.4 | 全量回归 `go test ./... -race` |

---

## 附录 A：多轮对话示例

```
[用户] GET /api/chat/stream?session_id=abc&message=我要报销差旅费
[Agent SSE]
  event: thinking → "正在理解您的需求..."
  event: message → "您好！请上传差旅票据图片，我来帮您处理。"
  event: done
  (State 保存: Phase1, 0条票据)

[用户] GET /api/chat/stream?session_id=abc&message=已上传 /uploads/ticket.png
[Agent SSE]
  event: thinking → "正在识别票据..."
  event: message → "识别到票据：差旅-交通 ¥500.00，置信度95%。请确认信息无误。"
  event: done
  (State 保存: Phase1, 1条票据, UserConfirmed=false)

[用户] GET /api/chat/stream?session_id=abc&message=确认
[Agent SSE]
  event: thinking → "正在校验..."
  event: message → "合规检查通过。预算余额 ¥5,000.00。是否确认提交报销？"
  event: done
  (State 保存: Phase2, UserConfirmed=true, ComplianceResult=pass, FinalConfirmed=false)

[用户] GET /api/chat/stream?session_id=abc&message=确认提交
[Agent SSE]
  event: thinking → "正在创建报销单..."
  event: tool_call → create_reimbursement
  event: tool_result → "REIMB-2026-0001"
  event: tool_call → submit_reimbursement
  event: tool_result → "提交成功，状态 pending"
  event: tool_call → generate_pdf
  event: tool_result → "/exports/REIMB-2026-0001.pdf"
  event: tool_call → send_email
  event: tool_result → "邮件已发送"
  event: message → "报销单 REIMB-2026-0001 已提交，¥500.00。审批人已收到通知。您可随时查询进度。"
  event: done
  (State 保存: Phase3, ReimbursementNo 已填充)

[审批人] 登录系统 → 看到待审批列表
[审批人] POST /api/approvals/42/approve
[REST] → 200 OK, status=approved
```

## 附录 B：与修复前的对比

| 维度 | 修复前 | 修复后 |
|------|--------|--------|
| State 生命周期 | 单次 Graph 执行 | Session 级别（跨请求） |
| 用户确认方式 | 不明确 | `GET /api/chat/stream?message=确认` |
| 审批方式 | Agent 内（不可能） | REST API（独立） |
| DB 写入 | Phase3 缺失 | `create_reimbursement` 工具 |
| 提交审批 | Phase3 缺失 | `submit_reimbursement` 工具 |
| 两条链路连接 | 无 | `reimbursementBiz.Create()+Submit()` |
