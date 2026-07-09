package reimbursement

import (
	"github.com/CycleZero/Reimbee/infra"
	"github.com/CycleZero/Reimbee/model"
	"gorm.io/gorm"
)

// ItemRepo 报销明细数据访问层
// 负责 ReimbursementItem 的 CRUD 操作
type ItemRepo struct {
	db *gorm.DB
}

// NewItemRepo 创建报销明细数据访问层实例
func NewItemRepo(data *infra.Data) *ItemRepo {
	return &ItemRepo{db: data.DB}
}

// Create 创建单条报销明细
func (r *ItemRepo) Create(item *model.ReimbursementItem) error {
	return r.db.Create(item).Error
}

// BatchCreate 批量创建报销明细
func (r *ItemRepo) BatchCreate(items []*model.ReimbursementItem) error {
	return r.db.Create(items).Error
}

// GetByReimbursementID 根据报销单 ID 查询所有明细，预加载票据
func (r *ItemRepo) GetByReimbursementID(reimbID uint) ([]*model.ReimbursementItem, error) {
	var items []*model.ReimbursementItem
	err := r.db.Where("reimbursement_id = ?", reimbID).
		Preload("Receipts").
		Order("id ASC").
		Find(&items).Error
	return items, err
}

// Update 更新报销明细
func (r *ItemRepo) Update(item *model.ReimbursementItem) error {
	return r.db.Save(item).Error
}

// DeleteByReimbursementID 删除报销单下所有明细（级联删除票据）
func (r *ItemRepo) DeleteByReimbursementID(reimbID uint) error {
	return r.db.Where("reimbursement_id = ?", reimbID).Delete(&model.ReimbursementItem{}).Error
}
