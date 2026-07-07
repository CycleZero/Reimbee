// Package types 智能体领域共享类型定义
// 提取 agent 与 tools 的共享类型，避免循环导入
package types

// ============================================
// Context Key 类型
// ============================================

// StateContextKey 用于在 context 中传递 ReimbursementState
// v3.0 中由 GenInput 在加载业务状态后注入到 context，
// 工具通过 ctx.Value(StateContextKey{}) 读取当前报销状态
type StateContextKey struct{}

// SessionIDContextKey 用于在 context 中传递 sessionID
// 工具通过 ctx.Value(SessionIDContextKey{}) 获取当前会话 ID，
// 进而调用 store.SaveState(sessionID, ...) 更新业务状态
type SessionIDContextKey struct{}

// ============================================
// 报销流程状态
// ============================================

// ReimbursementState 报销流程的全局状态，通过 Eino compose.WithGenLocalState 管理
// 三个阶段共享同一份状态，Guard 节点据此判断能否进入下一阶段
type ReimbursementState struct {
	ReimbursementID uint   `json:"reimbursement_id"`
	ReimbursementNo string `json:"reimbursement_no"`
	DepartmentID    uint   `json:"department_id"`
	EmployeeID      string `json:"employee_id"`
	EmployeeName    string `json:"employee_name"`
	CurrentPhase    string `json:"current_phase"`

	Invoices      []InvoiceState `json:"invoices"`
	TotalAmount   int64          `json:"total_amount"`
	UserConfirmed bool           `json:"user_confirmed"`

	ComplianceResult    *ComplianceCheckResult `json:"compliance_result,omitempty"`
	BudgetResult        *BudgetCheckResult     `json:"budget_result,omitempty"`
	FinalConfirmed      bool                   `json:"final_confirmed"`
	NeedSpecialApproval bool                   `json:"need_special_approval"`

	Phase1Turns int `json:"phase1_turns"`
	Phase2Turns int `json:"phase2_turns"`
	Phase3Turns int `json:"phase3_turns"`
}

// InvoiceState 单张票据的处理状态（Phase 1 收集阶段追踪）
type InvoiceState struct {
	Index          int     `json:"index"`
	ImagePath      string  `json:"image_path"`
	OCRRawAmount   int64   `json:"ocr_raw_amount"`
	OCRRawCategory string  `json:"ocr_raw_category"`
	OCRRawDate     string  `json:"ocr_raw_date"`
	OCRConfidence  float64 `json:"ocr_confidence"`
	Amount         int64   `json:"amount"`
	Category       string  `json:"category"`
	InvoiceDate    string  `json:"invoice_date"`
	IsModified     bool    `json:"is_modified"`
	ModifyReason   string  `json:"modify_reason"`
	UserConfirmed  bool    `json:"user_confirmed"`
}

// ============================================
// 校验结果
// ============================================

// ComplianceCheckResult 合规检查结果
type ComplianceCheckResult struct {
	Result  string `json:"result"`  // pass / warning / error
	Level   string `json:"level"`   // 与 Result 相同（兼容字段）
	Message string `json:"message"` // 检查结果描述
	RuleID  string `json:"rule_id"` // 触发的规则 ID（用于审计追溯）
}

// BudgetCheckResult 预算检查结果
type BudgetCheckResult struct {
	Remaining           int64   `json:"remaining"`             // 部门可用预算余额（分）
	NeedSpecialApproval bool    `json:"need_special_approval"` // 是否触发特殊审批
	UsageRate           float64 `json:"usage_rate"`            // 部门预算使用率 0~1
}
