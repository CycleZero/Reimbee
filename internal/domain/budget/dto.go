package budget

// CreateBudgetRequest 创建预算请求
type CreateBudgetRequest struct {
	DepartmentID uint  `json:"department_id" binding:"required"` // 部门ID
	FiscalYear   int   `json:"fiscal_year" binding:"required"`   // 财年
	AnnualBudget int64 `json:"annual_budget" binding:"required"` // 年度预算(分)
}

// UpdateBudgetRequest 更新预算请求
type UpdateBudgetRequest struct {
	AnnualBudget int64 `json:"annual_budget" binding:"required"` // 年度预算(分)
}

// BudgetResponse 预算响应
type BudgetResponse struct {
	ID           uint   `json:"id"`
	DepartmentID uint   `json:"department_id"`
	Department   string `json:"department,omitempty"`
	FiscalYear   int    `json:"fiscal_year"`
	AnnualBudget int64  `json:"annual_budget"` // 年度预算(分)
	SpentAmount  int64  `json:"spent_amount"`  // 已结算(分)
	FrozenAmount int64  `json:"frozen_amount"` // 已冻结(分)
	Remaining    int64  `json:"remaining"`     // 可用余额(分)
	UsageRate    float64 `json:"usage_rate"`   // 使用率百分比
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

// DashboardResponse 预算看板响应
type DashboardResponse struct {
	Departments []*BudgetResponse `json:"departments"`
	Summary     DashboardSummary  `json:"summary"`
}

// DashboardSummary 预算看板汇总
type DashboardSummary struct {
	TotalBudget    int64   `json:"total_budget"`    // 总预算(分)
	TotalSpent     int64   `json:"total_spent"`     // 总已结算(分)
	TotalRemaining int64   `json:"total_remaining"` // 总剩余(分)
	OverallUsage   float64 `json:"overall_usage"`   // 总体使用率
}
