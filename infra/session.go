// Package infra 基础设施层常量与公共类型
package infra

// StateKey 业务状态标识
const (
	StateKeyReimbursement = "reimbursement" // 报销流程状态
	StateKeyUserIdentity  = "user_identity"  // 用户身份信息
)

// 会话状态常量
const (
	SessionStatusActive    = "active"
	SessionStatusCompleted = "completed"
	SessionStatusExpired   = "expired"
)
