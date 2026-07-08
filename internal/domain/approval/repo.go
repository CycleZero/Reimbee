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

// NewApprovalRepo 创建审批记录数据访问层实例，自动迁移表结构
func NewApprovalRepo(data *infra.Data) *ApprovalRepo {
	if err := data.DB.AutoMigrate(&model.ApprovalRecord{}); err != nil {
		panic(err)
	}
	return &ApprovalRepo{db: data.DB}
}

// Create 创建审批记录
func (r *ApprovalRepo) Create(a *model.ApprovalRecord) error {
	return r.db.Create(a).Error
}

// GetByID 根据主键 ID 查询审批记录
func (r *ApprovalRepo) GetByID(id uint) (*model.ApprovalRecord, error) {
	var a model.ApprovalRecord
	if err := r.db.First(&a, id).Error; err != nil {
		return nil, err
	}
	return &a, nil
}

// ListByReimbursement 查询指定报销单的所有审批记录，按创建时间升序
func (r *ApprovalRepo) ListByReimbursement(reimbursementID uint) ([]*model.ApprovalRecord, error) {
	var records []*model.ApprovalRecord
	err := r.db.Where("reimbursement_id = ?", reimbursementID).
		Order("id ASC").Find(&records).Error
	return records, err
}

// ListPendingByApprover 根据审批人姓名查询所有待审批记录，按创建时间倒序排列
func (r *ApprovalRepo) ListPendingByApprover(approverName string) ([]*model.ApprovalRecord, error) {
	var records []*model.ApprovalRecord
	err := r.db.Where("approver_name = ? AND action = ?", approverName, "pending").Order("created_at DESC").Find(&records).Error
	return records, err
}

// Update 更新审批记录
func (r *ApprovalRepo) Update(a *model.ApprovalRecord) error {
	return r.db.Save(a).Error
}
