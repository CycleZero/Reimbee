package compliance

// ComplianceInvoiceItem 单张票据的合规检查项
type ComplianceInvoiceItem struct {
	Amount      int64  `json:"amount" jsonschema:"required" jsonschema_description:"票据金额(分)"`
	Category    string `json:"category" jsonschema:"required" jsonschema_description:"费用类别"`
	InvoiceDate string `json:"invoice_date" jsonschema:"required" jsonschema_description:"开票日期 YYYY-MM-DD"`
	Description string `json:"description,omitempty" jsonschema_description:"票据描述（可选）"`
}

// ComplianceInput 合规检查输入参数（支持多张票据）
type ComplianceInput struct {
	// 单张模式（兼容旧调用）
	Amount      int64  `json:"amount,omitempty"`
	Category    string `json:"category,omitempty"`
	InvoiceDate string `json:"invoice_date,omitempty"`

	// 多张模式
	Invoices []ComplianceInvoiceItem `json:"invoices,omitempty" jsonschema_description:"待审核的票据列表"`
}

// HasInvoices 判断是否有待审核票据
func (in *ComplianceInput) HasInvoices() bool {
	return len(in.Invoices) > 0
}

// GetInvoices 获取票据列表（单张模式自动转为列表）
func (in *ComplianceInput) GetInvoices() []ComplianceInvoiceItem {
	if len(in.Invoices) > 0 {
		return in.Invoices
	}
	if in.Amount > 0 && in.Category != "" {
		return []ComplianceInvoiceItem{{
			Amount:      in.Amount,
			Category:    in.Category,
			InvoiceDate: in.InvoiceDate,
		}}
	}
	return nil
}

// ComplianceItemResult 单张票据的合规审核结果
type ComplianceItemResult struct {
	Result     string `json:"result"`     // pass/warning/error
	Level      string `json:"level"`      // pass/warning/error
	Message    string `json:"message"`    // 检查结果描述
	RuleID     string `json:"rule_id"`    // 触发的规则ID
	Amount     int64  `json:"amount"`     // 票据金额(分)
	Category   string `json:"category"`   // 费用类别
}

// ComplianceOutput 合规检查输出结果
type ComplianceOutput struct {
	// 聚合结果（取最严重的）
	Result  string `json:"result"`  // pass/warning/error
	Level   string `json:"level"`
	Message string `json:"message"`
	RuleID  string `json:"rule_id"`

	// 逐票据结果（多张模式时填充）
	Items []ComplianceItemResult `json:"items,omitempty"`
}
