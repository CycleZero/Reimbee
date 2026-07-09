// Package model 数据模型层
// ReimbursementItem 报销明细 — 报销单中的一条费用申请
// 一条明细可关联多张票据（Receipt）作为证明材料
//
// 实体关系：
//
//	Reimbursement (1) ── (N) ReimbursementItem (1) ── (N) Receipt
//
// 业务语义：
//   - Reimbursement: "我报销参加 IJCAI 2026 的差旅费，总共 ¥2400"
//   - ReimbursementItem: "差旅-交通 ¥1500，北京→上海往返机票"
//   - Receipt: "XX航空电子行程单，票面 ¥1500"（证明材料）
package model

import "gorm.io/gorm"

// ReimbursementItem 报销明细
// 报销单中的一条费用申请，包含费用类别、申请金额和事由说明
// 每条明细可附带多张票据（Receipt）作为报销凭证
type ReimbursementItem struct {
	gorm.Model
	// ReimbursementID 关联的报销单ID（外键）
	ReimbursementID uint `gorm:"index;not null;comment:关联报销单ID" json:"reimbursement_id"`
	// Category 费用类别，如"差旅-交通"、"办公用品"等
	Category string `gorm:"type:varchar(50);not null;index;comment:费用类别" json:"category"`
	// Amount 申请报销金额（分），可能与票据票面金额不同（如部分报销）
	Amount int64 `gorm:"not null;default:0;comment:申请报销金额(分)" json:"amount"`
	// Description 事由说明，如"北京→上海往返机票"
	Description string `gorm:"type:varchar(500);comment:事由说明" json:"description"`
	// Receipts 该明细关联的票据列表（1:N）
	Receipts []Receipt `gorm:"foreignKey:ItemID;constraint:OnDelete:CASCADE" json:"receipts,omitempty"`
}
