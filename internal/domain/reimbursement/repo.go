package reimbursement

import (
	"gorm.io/gorm"

	"github.com/CycleZero/Reimbee/infra"
	"github.com/CycleZero/Reimbee/model"
)

// ReimbursementRepo 报销单数据访问层
type ReimbursementRepo struct {
	db *gorm.DB
}

// NewReimbursementRepo 创建报销单数据访问层实例，自动迁移表结构
func NewReimbursementRepo(data *infra.Data) *ReimbursementRepo {
	if err := data.DB.AutoMigrate(&model.Reimbursement{}); err != nil {
		panic(err)
	}
	return &ReimbursementRepo{db: data.DB}
}

// Create 创建报销单
func (r *ReimbursementRepo) Create(rm *model.Reimbursement) error {
	return r.db.Create(rm).Error
}

// GetByID 根据主键 ID 查询报销单，预加载部门、票据明细、审批记录
func (r *ReimbursementRepo) GetByID(id uint) (*model.Reimbursement, error) {
	var rm model.Reimbursement
	if err := r.db.Preload("Department").
		Preload("Invoices").
		Preload("Approvals").
		First(&rm, id).Error; err != nil {
		return nil, err
	}
	return &rm, nil
}

// GetByNo 根据报销单号查询，预加载部门、票据明细、审批记录
func (r *ReimbursementRepo) GetByNo(no string) (*model.Reimbursement, error) {
	var rm model.Reimbursement
	if err := r.db.Where("reimbursement_no = ?", no).
		Preload("Department").
		Preload("Invoices").
		Preload("Approvals").
		First(&rm).Error; err != nil {
		return nil, err
	}
	return &rm, nil
}

// List 分页查询报销单列表，可按员工工号筛选
func (r *ReimbursementRepo) List(page, pageSize int, employeeID string) ([]*model.Reimbursement, int64, error) {
	var rms []*model.Reimbursement
	var total int64
	db := r.db.Model(&model.Reimbursement{})
	if employeeID != "" {
		db = db.Where("employee_id = ?", employeeID)
	}
	db.Count(&total)
	err := db.Offset((page - 1) * pageSize).Limit(pageSize).
		Preload("Department").Preload("Invoices").Preload("Approvals").
		Order("id DESC").Find(&rms).Error
	return rms, total, err
}

// ListByStatus 按状态查询报销单列表
func (r *ReimbursementRepo) ListByStatus(status string) ([]*model.Reimbursement, error) {
	var rms []*model.Reimbursement
	err := r.db.Where("status = ?", status).
		Preload("Department").Preload("Invoices").Preload("Approvals").
		Order("id ASC").Find(&rms).Error
	return rms, err
}

// Update 更新报销单
func (r *ReimbursementRepo) Update(rm *model.Reimbursement) error {
	return r.db.Save(rm).Error
}

// UpdateStatus 仅更新报销单状态
func (r *ReimbursementRepo) UpdateStatus(id uint, status string) error {
	return r.db.Model(&model.Reimbursement{}).Where("id = ?", id).
		Update("status", status).Error
}
