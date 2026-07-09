// Package model 数据模型层
// Receipt 票据/原始凭证 — 报销明细的证明材料
//
// 实体关系：
//
//	ReimbursementItem (1) ── (N) Receipt
//
// 业务语义：
//   - 票据是"证明"：票面金额（Amount）可能大于或等于申请报销金额（ReimbursementItem.Amount）
//   - 审批人可以看到票面金额与申请金额的差异，做出知情决策
//   - 一张票据属于一条报销明细（ItemID），不能跨明细共享
package model

import "gorm.io/gorm"

// Receipt 票据/原始凭证
// 记录发票的原始信息，作为报销明细的证明依据
// 保留 OCR 原始值、用户修正记录、审批人裁决，形成完整审计链
type Receipt struct {
	gorm.Model
	// ItemID 关联的报销明细ID（外键），替代原 ReimbursementID；0 表示尚未归类
	ItemID uint `gorm:"index;default:0;comment:关联报销明细ID(0=未归类)" json:"item_id"`
	// InvoiceCode 发票代码，如"031002200111"
	InvoiceCode string `gorm:"type:varchar(50);comment:发票代码" json:"invoice_code"`
	// InvoiceNumber 发票号码，如"98765432"
	InvoiceNumber string `gorm:"type:varchar(50);comment:发票号码" json:"invoice_number"`
	// Amount 票面金额（分）— 票据上印的原始金额，可能与申请报销金额不同
	Amount int64 `gorm:"not null;default:0;comment:票面金额(分)" json:"amount"`
	// InvoiceDate 开票日期，格式 YYYY-MM-DD
	InvoiceDate string `gorm:"type:varchar(20);comment:开票日期" json:"invoice_date"`
	// SellerName 销售方名称，如"中国国际航空股份有限公司"
	SellerName string `gorm:"type:varchar(200);comment:销售方名称" json:"seller_name"`
	// Category 费用类别，冗余存储便于独立查询
	Category string `gorm:"type:varchar(50);not null;index;comment:费用类别" json:"category"`
	// ImagePath 票据图片存储路径（MinIO或本地）
	ImagePath string `gorm:"type:varchar(500);comment:票据图片路径" json:"image_path"`

	// ── OCR 原始值（客观基准，不可被用户覆盖）──
	// OCR 识别的原始数据，用于与用户确认值对比，形成审计链

	// OCRRawData OCR 识别返回的原始 JSON 字符串，用于问题排查
	OCRRawData string `gorm:"type:text;comment:OCR识别原始JSON" json:"ocr_raw_data"`
	// OCRRawAmount OCR 原始识别的金额（分）
	OCRRawAmount int64 `gorm:"not null;default:0;comment:OCR原始识别金额(分)" json:"ocr_raw_amount"`
	// OCRRawDate OCR 原始识别的开票日期
	OCRRawDate string `gorm:"type:varchar(20);comment:OCR原始识别日期" json:"ocr_raw_date"`
	// OCRRawCategory OCR 原始推断的费用类别
	OCRRawCategory string `gorm:"type:varchar(50);comment:OCR原始推断类别" json:"ocr_raw_category"`
	// OCRConfidence OCR 识别置信度（0~1），低于阈值时建议用户手动核实
	OCRConfidence float64 `gorm:"default:0;comment:OCR识别置信度0~1" json:"ocr_confidence"`

	// ── 用户修正记录（审计链）──

	// IsUserModified 用户是否修改了 OCR 识别结果
	IsUserModified bool `gorm:"default:false;comment:用户是否修改了OCR结果" json:"is_user_modified"`
	// ModificationNote 用户修正的原因说明
	ModificationNote string `gorm:"type:text;comment:用户修正说明" json:"modification_note"`

	// ── 审批人裁决 ──

	// ApproverChoice 审批人选择的金额来源："ocr" 使用 OCR 原始值，"user" 使用用户修正值
	ApproverChoice string `gorm:"type:varchar(10);comment:审批人裁决 ocr/user" json:"approver_choice"`

	// ── 合规检查结果 ──

	// CheckResult 合规检查结果："pass" 通过 / "warning" 警告 / "error" 违规
	CheckResult string `gorm:"type:varchar(20);default:pending;comment:合规检查结果 pass/warning/error" json:"check_result"`
	// CheckMessage 合规检查的详细说明
	CheckMessage string `gorm:"type:text;comment:合规检查说明" json:"check_message"`
}
