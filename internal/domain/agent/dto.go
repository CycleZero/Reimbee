// Package agent 智能体层，负责基于 Eino Graph 的对话式报销流程编排
// 采用 Graph 定义阶段边界 + Agent（LLM）决定阶段内执行的混合架构
package agent

// ============================================
// 意图分类
// ============================================

// IntentResult 意图分类节点的结构化输出
type IntentResult struct {
	Intent     string            `json:"intent"`     // 意图类别（new_reimbursement / query_progress / query_budget / policy_question / modify_reimbursement / general_chat）
	Entities   map[string]string `json:"entities"`   // 实体提取（amount, category, department, reimbursement_no）
	Confidence float64           `json:"confidence"` // 分类置信度 0~1
	Reason     string            `json:"reason"`     // 分类依据简述
}

// ============================================
// 工作流路由
// ============================================

// WorkflowRoute 工作流路由标识
type WorkflowRoute string

const (
	RouteNewReimbursement    WorkflowRoute = "new_reimbursement"    // 新建报销流程
	RouteQueryProgress       WorkflowRoute = "query_progress"       // 进度查询
	RouteQueryBudget         WorkflowRoute = "query_budget"         // 预算查询
	RoutePolicyQuestion      WorkflowRoute = "policy_question"      // 政策咨询（LLM 直接回复）
	RouteModifyReimbursement WorkflowRoute = "modify_reimbursement" // 修改已有报销单
	RouteGeneralChat         WorkflowRoute = "general_chat"         // 通用对话（问候、感谢等）
)

// ============================================
// 报销流程状态（Graph 共享状态，贯穿三阶段）
// ============================================

// ReimbursementState 报销流程的全局状态，通过 Eino compose.WithGenLocalState 管理
// 三个阶段共享同一份状态，Guard 节点据此判断能否进入下一阶段
type ReimbursementState struct {
	ReimbursementID     uint   `json:"reimbursement_id"`
	ReimbursementNo     string `json:"reimbursement_no"`
	DepartmentID        uint   `json:"department_id"`
	EmployeeID          string `json:"employee_id"`
	EmployeeName        string `json:"employee_name"`
	CurrentPhase        string `json:"current_phase"`

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

// ============================================
// Guard 护卫条件
// ============================================

// GuardResult 阶段护卫条件的检查结果
// 用于 Graph Branch 节点判断是否允许进入下一阶段
type GuardResult struct {
	Passed  bool   `json:"passed"`           // 是否通过护卫条件
	Reason  string `json:"reason"`           // 未通过时的原因（用于日志）
	Message string `json:"message,omitempty"` // 返回给用户的提示（未通过时）
}

// ============================================
// Graph 输入/输出
// ============================================

// AgentInput 顶层 Graph 的输入结构
// 由 HTTP Service 层从请求上下文构建，包含当前用户和会话的全部信息
type AgentInput struct {
	SessionID  string // 会话 ID（UUID v7，用于 Checkpoint + 消息历史）
	UserID     uint   // 当前登录用户 DB 主键
	EmployeeID string // 当前员工工号
	Role       string // 当前用户角色（employee / approver / admin）
	Message    string // 用户最新输入的自然语言消息
}

// AgentOutput 顶层 Graph 的输出结构
// Graph 执行完成后返回给 HTTP Service，由 Service 封装为 SSE 事件
type AgentOutput struct {
	Message  string         `json:"message"`           // 给用户的自然语言回复
	Metadata map[string]any `json:"metadata,omitempty"` // 附加元数据（阶段、进度等）
	Done     bool           `json:"done"`              // 流程是否已结束
}
