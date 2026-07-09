// Package types Agent 会话状态定义
//
// ReimbursementState 贯穿 Agent 对话全流程，存储在 Session.state 中。
// v2: 三层结构 — Items（已确认明细）+ PendingReceipts（待归类票据）
package types

import "encoding/json"

// ReimbursementState 报销对话的会话状态
type ReimbursementState struct {
	// Items 已确认的报销明细列表（含费用类别、申请金额、事由、关联票据）
	Items []ItemState `json:"items"`
	// PendingReceipts OCR 识别后尚未归入任何明细的票据
	PendingReceipts []ReceiptState `json:"pending_receipts"`
	// TotalAmount 报销总金额（分），由 Items 汇总计算
	TotalAmount int64 `json:"total_amount"`
	// CurrentPhase 当前对话阶段标识
	CurrentPhase string `json:"current_phase"`
	// BudgetResult 预算检查结果（check_budget 写入）
	BudgetResult *BudgetCheckResult `json:"budget_result,omitempty"`
	// ComplianceResult 合规检查结果（check_compliance 写入）
	ComplianceResult json.RawMessage `json:"compliance_result,omitempty"`
	// ReimbursementID 创建的报销单 ID（create_reimbursement 写入）
	ReimbursementID uint `json:"reimbursement_id"`
	// EmployeeID 操作员工工号
	EmployeeID string `json:"employee_id"`
}

// ItemState 一条报销明细的会话状态
type ItemState struct {
	Category    string         `json:"category"`
	Amount      int64          `json:"amount"`
	Description string         `json:"description"`
	Receipts    []ReceiptState `json:"receipts"`
}

// ReceiptState 一张票据的会话状态（替代旧 InvoiceState）
type ReceiptState struct {
	DBID         uint    `json:"db_id,omitempty"` // 对应 DB Receipt 记录的主键ID
	ImagePath    string  `json:"image_path"`
	Amount       int64   `json:"amount"`
	Category     string  `json:"category"`
	Date         string  `json:"date"`
	InvoiceCode  string  `json:"invoice_code,omitempty"`
	InvoiceNo    string  `json:"invoice_no,omitempty"`
	SellerName   string  `json:"seller_name,omitempty"`
	OCRRawAmount int64   `json:"ocr_raw_amount,omitempty"`
	OCRConfidence float64 `json:"ocr_confidence,omitempty"`
}

// BudgetCheckResult 预算检查结果
type BudgetCheckResult struct {
	Remaining           int64   `json:"remaining"`
	NeedSpecialApproval bool    `json:"need_special_approval"`
	UsageRate           float64 `json:"usage_rate"`
}
