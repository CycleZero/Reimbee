package compliance

// ComplianceReceiptItem 单张票据的合规检查输入（原 ComplianceInvoiceItem）
type ComplianceReceiptItem struct {
	Amount      int64  `json:"amount" jsonschema:"required" jsonschema_description:"票据票面金额(分)"`
	Category    string `json:"category" jsonschema:"required" jsonschema_description:"费用类别"`
	InvoiceDate string `json:"invoice_date" jsonschema:"required" jsonschema_description:"开票日期 YYYY-MM-DD"`
	Description string `json:"description,omitempty" jsonschema_description:"票据描述（可选）"`
}

// ComplianceInput 合规检查输入参数（支持多明细多票据）
type ComplianceInput struct {
	// 单张模式（兼容旧调用）— 转为 Items[0] 含单张票据
	Amount      int64  `json:"amount,omitempty"`
	Category    string `json:"category,omitempty"`
	InvoiceDate string `json:"invoice_date,omitempty"`

	// 多明细模式 — 每条明细含多张票据
	Items []ComplianceItemInput `json:"items,omitempty" jsonschema_description:"待审核的报销明细列表"`
}

// ComplianceItemInput 单条报销明细的合规检查输入
type ComplianceItemInput struct {
	Category string                   `json:"category" jsonschema:"required" jsonschema_description:"费用类别"`
	Amount   int64                    `json:"amount" jsonschema:"required" jsonschema_description:"申请报销金额(分)"`
	Receipts []ComplianceReceiptItem  `json:"receipts" jsonschema_description:"该明细关联的票据列表"`
}

// HasItems 判断是否有待审核明细
func (in *ComplianceInput) HasItems() bool {
	return len(in.Items) > 0
}

// GetItems 获取明细列表（单张模式自动转为列表）
func (in *ComplianceInput) GetItems() []ComplianceItemInput {
	if len(in.Items) > 0 {
		return in.Items
	}
	if in.Amount > 0 && in.Category != "" {
		return []ComplianceItemInput{{
			Category: in.Category,
			Amount:   in.Amount,
			Receipts: []ComplianceReceiptItem{{
				Amount:      in.Amount,
				Category:    in.Category,
				InvoiceDate: in.InvoiceDate,
			}},
		}}
	}
	return nil
}

// ComplianceItemResult 单张票据的合规审核结果
type ComplianceItemResult struct {
	Result   string `json:"result"`   // pass/warning/error
	Message  string `json:"message"`  // 检查结果描述
	RuleID   string `json:"rule_id"`  // 触发的规则ID
	Amount   int64  `json:"amount"`   // 票据金额(分)
	Category string `json:"category"` // 费用类别
}

// ComplianceOutput 合规检查输出结果
type ComplianceOutput struct {
	// 聚合结果（取最严重的）
	Result  string `json:"result"`  // pass/warning/error
	Message string `json:"message"` // 聚合描述
	RuleID  string `json:"rule_id"` // 触发的规则ID

	// 逐票据结果（多张模式时填充）
	Items []ComplianceItemResult `json:"items,omitempty"`
}
