package compliance

// ComplianceInput 合规检查输入参数
type ComplianceInput struct {
	Amount      int64  `json:"amount" jsonschema:"required" jsonschema_description:"票据金额(分)"`
	Category    string `json:"category" jsonschema:"required" jsonschema_description:"费用类别"`
	InvoiceDate string `json:"invoice_date" jsonschema:"required" jsonschema_description:"开票日期 YYYY-MM-DD"`
}

// ComplianceOutput 合规检查输出结果
type ComplianceOutput struct {
	Result  string `json:"result"`  // pass/warning/error
	Level   string `json:"level"`
	Message string `json:"message"`
	RuleID  string `json:"rule_id"`
}
