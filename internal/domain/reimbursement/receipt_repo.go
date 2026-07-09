package reimbursement

import (
	"github.com/CycleZero/Reimbee/infra"
	"github.com/CycleZero/Reimbee/model"
	"gorm.io/gorm"
)

// ReceiptRepo 票据数据访问层
// 负责 Receipt（原 InvoiceItem）的 CRUD 操作
type ReceiptRepo struct {
	db *gorm.DB
}

// NewReceiptRepo 创建票据数据访问层实例
func NewReceiptRepo(data *infra.Data) *ReceiptRepo {
	return &ReceiptRepo{db: data.DB}
}

// Create 创建单张票据记录
func (r *ReceiptRepo) Create(receipt *model.Receipt) error {
	return r.db.Create(receipt).Error
}

// BatchCreate 批量创建票据记录
func (r *ReceiptRepo) BatchCreate(receipts []*model.Receipt) error {
	return r.db.Create(receipts).Error
}

// GetByItemID 根据明细 ID 查询所有关联票据
func (r *ReceiptRepo) GetByItemID(itemID uint) ([]*model.Receipt, error) {
	var receipts []*model.Receipt
	err := r.db.Where("item_id = ?", itemID).
		Order("id ASC").
		Find(&receipts).Error
	return receipts, err
}

// Update 更新票据（OCR 确认、用户修正、审批裁决等场景）
func (r *ReceiptRepo) Update(receipt *model.Receipt) error {
	return r.db.Save(receipt).Error
}

// UpdateCheckResult 更新单张票据的合规检查结果
func (r *ReceiptRepo) UpdateCheckResult(id uint, result, message string) error {
	return r.db.Model(&model.Receipt{}).Where("id = ?", id).
		Updates(map[string]interface{}{
			"check_result":  result,
			"check_message": message,
		}).Error
}

// DeleteByItemID 删除某明细下的所有票据
func (r *ReceiptRepo) DeleteByItemID(itemID uint) error {
	return r.db.Where("item_id = ?", itemID).Delete(&model.Receipt{}).Error
}
