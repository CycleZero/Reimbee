package approval

import (
	"gorm.io/gorm"

	"github.com/CycleZero/Reimbee/infra"
	"github.com/CycleZero/Reimbee/model"
)

// ApprovalRepo 审批记录数据访问层
type ApprovalRepo struct {
	db *gorm.DB
}

func NewApprovalRepo(data *infra.Data) *ApprovalRepo {
	if err := data.DB.AutoMigrate(&model.ApprovalRecord{}); err != nil {
		panic(err)
	}
	return &ApprovalRepo{db: data.DB}
}

func (r *ApprovalRepo) Create(a *model.ApprovalRecord) error {
	return r.db.Create(a).Error
}

func (r *ApprovalRepo) GetByID(id uint) (*model.ApprovalRecord, error) {
	var a model.ApprovalRecord
	if err := r.db.First(&a, id).Error; err != nil {
		return nil, err
	}
	return &a, nil
}

func (r *ApprovalRepo) ListByReimbursement(reimbursementID uint) ([]*model.ApprovalRecord, error) {
	var records []*model.ApprovalRecord
	err := r.db.Where("reimbursement_id = ?", reimbursementID).
		Order("id ASC").Find(&records).Error
	return records, err
}

func (r *ApprovalRepo) Update(a *model.ApprovalRecord) error {
	return r.db.Save(a).Error
}
