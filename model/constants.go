package model

// ============================================
// 角色常量
// ============================================
const (
	RoleEmployee = "employee" // 普通员工
	RoleApprover = "approver" // 审批人
	RoleAdmin    = "admin"    // 管理员
)

// IsApproverRole 判断角色是否具有审批权限
func IsApproverRole(role string) bool {
	return role == RoleApprover || role == RoleAdmin
}

// ============================================
// 报销单状态
// ============================================
const (
	ReimbStatusDraft     = "draft"     // 草稿
	ReimbStatusPending   = "pending"   // 待审批
	ReimbStatusReviewing = "reviewing" // 审批中
	ReimbStatusApproved  = "approved"  // 已通过
	ReimbStatusRejected  = "rejected"  // 已驳回
)

// ============================================
// 审批动作
// ============================================
const (
	ApprovalActionPending  = "pending"  // 待审批
	ApprovalActionApproved = "approved" // 已通过
	ApprovalActionRejected = "rejected" // 已驳回
)

// ============================================
// 合规检查结果
// ============================================
const (
	CheckResultPass    = "pass"    // 通过
	CheckResultWarning = "warning" // 警告（可继续但需审批人确认）
	CheckResultError   = "error"   // 严重违规（不可提交）
	CheckResultPending = "pending" // 待检查
)

// ============================================
// 审批人裁决
// ============================================
const (
	ApproverChoiceOCR  = "ocr"  // 采纳 OCR 原始值
	ApproverChoiceUser = "user" // 采纳用户修正值
)

// ============================================
// 费用类别
// ============================================
const (
	CategoryTravel        = "差旅-交通"
	CategoryAccommodation = "差旅-住宿"
	CategorySubsidy       = "差旅-补助"
	CategoryEntertainment = "招待费"
	CategoryOffice        = "办公用品"
	CategoryPrinting      = "印刷费"
	CategoryOther         = "其他"
)
