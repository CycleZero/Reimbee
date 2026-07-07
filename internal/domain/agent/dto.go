// Package agent 智能体层，负责基于 Eino Graph 的对话式报销流程编排
// 采用 Graph 定义阶段边界 + Agent（LLM）决定阶段内执行的混合架构
package agent

import (
	"github.com/CycleZero/Reimbee/internal/domain/agent/types"
)

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
// 报销流程状态（来自 types 包，避免 tools → agent 循环导入）
// ============================================

// ReimbursementState 报销流程的全局状态，通过 Eino compose.WithGenLocalState 管理
type ReimbursementState = types.ReimbursementState

// InvoiceState 单张票据的处理状态
type InvoiceState = types.InvoiceState

// ============================================
// 校验结果
// ============================================

// ComplianceCheckResult 合规检查结果
type ComplianceCheckResult = types.ComplianceCheckResult

// BudgetCheckResult 预算检查结果
type BudgetCheckResult = types.BudgetCheckResult

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
