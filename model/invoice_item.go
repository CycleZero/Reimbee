package model

import "gorm.io/gorm"

// InvoiceItem 票据明细
type InvoiceItem struct {
	gorm.Model
	ReimbursementID uint   `gorm:"index;not null;comment:关联报销单ID" json:"reimbursement_id"`
	InvoiceCode     string `gorm:"type:varchar(50);comment:发票代码" json:"invoice_code"`
	InvoiceNumber   string `gorm:"type:varchar(50);comment:发票号码" json:"invoice_number"`
	Amount          int64  `gorm:"not null;default:0;comment:用户确认后的金额(分)" json:"amount"`
	InvoiceDate     string `gorm:"type:varchar(20);comment:开票日期" json:"invoice_date"`
	SellerName      string `gorm:"type:varchar(200);comment:销售方名称" json:"seller_name"`
	Category        string `gorm:"type:varchar(50);not null;index;comment:费用类别" json:"category"`
	ImagePath       string `gorm:"type:varchar(500);comment:原始票据图片路径" json:"image_path"`
	OCRRawData      string `gorm:"type:text;comment:OCR识别原始JSON" json:"ocr_raw_data"`

	// OCR 原始值（客观基准——机器从票据上读到的值，不可被用户覆盖）
	OCRRawAmount    int64   `gorm:"not null;default:0;comment:OCR原始识别金额(分)" json:"ocr_raw_amount"`
	OCRRawDate      string  `gorm:"type:varchar(20);comment:OCR原始识别日期" json:"ocr_raw_date"`
	OCRRawCategory  string  `gorm:"type:varchar(50);comment:OCR原始推断类别" json:"ocr_raw_category"`
	OCRConfidence   float64 `gorm:"default:0;comment:OCR识别置信度0~1" json:"ocr_confidence"`

	// 用户修正标记（审计链）
	IsUserModified   bool   `gorm:"default:false;comment:用户是否修改了OCR结果" json:"is_user_modified"`
	ModificationNote string `gorm:"type:text;comment:用户修正原因说明" json:"modification_note"`

	// 审批人裁决（审批时填写）
	ApproverChoice string `gorm:"type:varchar(10);comment:审批人选择的金额来源 ocr/user" json:"approver_choice"`

	CheckResult  string `gorm:"type:varchar(20);default:pending;comment:pass/warning/error" json:"check_result"`
	CheckMessage string `gorm:"type:text;comment:合规检查说明" json:"check_message"`
}
