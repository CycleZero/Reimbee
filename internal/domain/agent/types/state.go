package types

import "encoding/json"

type ReimbursementState struct {
	Invoices         []InvoiceState      `json:"invoices"`
	TotalAmount      int64               `json:"total_amount"`
	CurrentPhase     string              `json:"current_phase"`
	BudgetResult     *BudgetCheckResult  `json:"budget_result,omitempty"`
	ComplianceResult json.RawMessage     `json:"compliance_result,omitempty"`
	ReimbursementID  uint                `json:"reimbursement_id"`
	EmployeeID       string              `json:"employee_id"`
}

type InvoiceState struct {
	ImagePath string `json:"image_path"`
	Amount    int64  `json:"amount"`
	Category  string `json:"category"`
}

type BudgetCheckResult struct {
	Remaining           int64   `json:"remaining"`
	NeedSpecialApproval bool    `json:"need_special_approval"`
	UsageRate           float64 `json:"usage_rate"`
}
