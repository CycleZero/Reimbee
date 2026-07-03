package model

import "gorm.io/gorm"

// InvoiceItem 票据明细
type InvoiceItem struct {
	gorm.Model
	ReimbursementID uint   `gorm:"index;not null;comment:关联报销单ID" json:"reimbursement_id"`
	InvoiceCode     string `gorm:"type:varchar(50);comment:发票代码" json:"invoice_code"`
	InvoiceNumber   string `gorm:"type:varchar(50);comment:发票号码" json:"invoice_number"`
	Amount          int64  `gorm:"not null;default:0;comment:金额(分)" json:"amount"`
	InvoiceDate     string `gorm:"type:varchar(20);comment:开票日期" json:"invoice_date"`
	SellerName      string `gorm:"type:varchar(200);comment:销售方名称" json:"seller_name"`
	Category        string `gorm:"type:varchar(50);not null;index;comment:费用类别" json:"category"`
	ImagePath       string `gorm:"type:varchar(500);comment:原始票据图片路径" json:"image_path"`
	OCRRawData      string `gorm:"type:text;comment:OCR识别原始JSON" json:"ocr_raw_data"`
	CheckResult     string `gorm:"type:varchar(20);default:pending;comment:pass/warning/error" json:"check_result"`
	CheckMessage    string `gorm:"type:text;comment:合规检查说明" json:"check_message"`
}
