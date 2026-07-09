package reimbursement

// ============================================
// CreateReimbInput — 创建报销单的内部输入（供 biz 层使用）
// ============================================

// CreateReimbInput 创建报销单的内部输入参数
type CreateReimbInput struct {
	EmployeeID   string      // 工号
	EmployeeName string      // 申请人姓名
	DepartmentID uint        // 部门ID
	SubmitNote   string      // 报销事由
	Items        []ItemInput // 报销明细列表
}

// ItemInput 报销明细输入
type ItemInput struct {
	Category    string         // 费用类别
	Amount      int64          // 申请报销金额(分)
	Description string         // 事由说明
	Receipts    []ReceiptInput // 关联票据列表
}

// ReceiptInput 票据输入
type ReceiptInput struct {
	ImagePath      string  // 票据图片路径
	Amount         int64   // 票面金额(分)
	InvoiceDate    string  // 开票日期 YYYY-MM-DD
	InvoiceCode    string  // 发票代码
	InvoiceNumber  string  // 发票号码
	SellerName     string  // 销售方名称
	OCRRawAmount   int64   // OCR原始金额(分)
	OCRRawDate     string  // OCR原始日期
	OCRRawCategory string  // OCR原始类别
	OCRConfidence  float64 // OCR置信度
}

// ============================================
// HTTP 请求/响应 DTO
// ============================================

// CreateReimbursementRequest 创建报销单 HTTP 请求
type CreateReimbursementRequest struct {
	EmployeeID   string `json:"employee_id" binding:"required"`
	EmployeeName string `json:"employee_name" binding:"required"`
	DepartmentID uint   `json:"department_id" binding:"required"`
	SubmitNote   string `json:"submit_note"`
}

// SubmitReimbursementRequest 提交报销单 HTTP 请求
type SubmitReimbursementRequest struct {
	TotalAmount int64 `json:"total_amount" binding:"required"` // 保留兼容旧 API
}

// ReimbursementResponse 报销单 HTTP 响应
type ReimbursementResponse struct {
	ID                  uint                     `json:"id"`
	ReimbursementNo     string                   `json:"reimbursement_no"`
	EmployeeID          string                   `json:"employee_id"`
	EmployeeName        string                   `json:"employee_name"`
	DepartmentID        uint                     `json:"department_id"`
	Department          string                   `json:"department,omitempty"`
	TotalAmount         int64                    `json:"total_amount"`
	Status              string                   `json:"status"`
	SubmitNote          string                   `json:"submit_note"`
	NeedSpecialApproval bool                     `json:"need_special_approval"`
	Items               []*ItemResponse          `json:"items,omitempty"`
	Approvals           []*ApprovalInfoResponse  `json:"approvals,omitempty"`
	CreatedAt           string                   `json:"created_at"`
	UpdatedAt           string                   `json:"updated_at"`
}

// ItemResponse 报销明细 HTTP 响应
type ItemResponse struct {
	ID          uint               `json:"id"`
	Category    string             `json:"category"`
	Amount      int64              `json:"amount"`
	Description string             `json:"description"`
	Receipts    []*ReceiptResponse `json:"receipts,omitempty"`
}

// ReceiptResponse 票据 HTTP 响应
type ReceiptResponse struct {
	ID            uint   `json:"id"`
	Amount        int64  `json:"amount"`
	InvoiceDate   string `json:"invoice_date"`
	InvoiceCode   string `json:"invoice_code,omitempty"`
	InvoiceNumber string `json:"invoice_number,omitempty"`
	ImagePath     string `json:"image_path,omitempty"`
	Category      string `json:"category"`
	CheckResult   string `json:"check_result"`
}

// ApprovalInfoResponse 审批信息响应
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
type UploadInvoiceResponse struct {
	FileID   string `json:"file_id"`
	FileName string `json:"file_name"`
	FilePath string `json:"file_path"`
	URL      string `json:"url"`
	Size     int64  `json:"size"`
}
