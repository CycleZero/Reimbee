package reimbursement

// CreateReimbursementRequest 创建报销单请求
type CreateReimbursementRequest struct {
	EmployeeID   string `json:"employee_id" binding:"required"`   // 工号
	EmployeeName string `json:"employee_name" binding:"required"`  // 申请人姓名
	DepartmentID uint   `json:"department_id" binding:"required"`  // 部门ID
	SubmitNote   string `json:"submit_note"`                       // 报销事由
}

// SubmitReimbursementRequest 提交报销单请求
type SubmitReimbursementRequest struct {
	TotalAmount int64 `json:"total_amount" binding:"required"` // 报销总金额(分)
}

// ReimbursementResponse 报销单响应
type ReimbursementResponse struct {
	ID                  uint                        `json:"id"`
	ReimbursementNo     string                      `json:"reimbursement_no"`
	EmployeeID          string                      `json:"employee_id"`
	EmployeeName        string                      `json:"employee_name"`
	DepartmentID        uint                        `json:"department_id"`
	Department          string                      `json:"department,omitempty"`
	TotalAmount         int64                       `json:"total_amount"`
	Status              string                      `json:"status"`
	SubmitNote          string                      `json:"submit_note"`
	NeedSpecialApproval bool                        `json:"need_special_approval"`
	Invoices            []*InvoiceItemResponse      `json:"invoices,omitempty"`
	Approvals           []*ApprovalInfoResponse     `json:"approvals,omitempty"`
	CreatedAt           string                      `json:"created_at"`
	UpdatedAt           string                      `json:"updated_at"`
}

// InvoiceItemResponse 票据明细响应
type InvoiceItemResponse struct {
	ID         uint   `json:"id"`
	Amount     int64  `json:"amount"`
	InvoiceDate string `json:"invoice_date"`
	Category   string `json:"category"`
	CheckResult string `json:"check_result"`
}

// ApprovalInfoResponse 审批信息响应（报销单详情中展示）
type ApprovalInfoResponse struct {
	ID           uint   `json:"id"`
	ApproverName string `json:"approver_name"`
	Action       string `json:"action"`
	Comment      string `json:"comment"`
	ActionAt     string `json:"action_at,omitempty"`
}

// ListReimbursementResponse 报销单列表响应
type ListReimbursementResponse struct {
	List  []*ReimbursementResponse `json:"list"`
	Total int64                    `json:"total"`
	Page  int                      `json:"page"`
}

// UploadInvoiceResponse 票据上传响应
// FilePath 是关键字段——前端将此值传给 Agent 对话，LLM 调用 recognize_invoice 工具时作为 image_path 参数
type UploadInvoiceResponse struct {
	FileID   string `json:"file_id"`   // 文件唯一标识（UUID）
	FileName string `json:"file_name"` // 原始文件名
	FilePath string `json:"file_path"` // 存储路径（供 Agent OCR 工具使用）
	URL      string `json:"url"`       // 可访问 URL（用于前端预览）
	Size     int64  `json:"size"`      // 文件大小（字节）
}
