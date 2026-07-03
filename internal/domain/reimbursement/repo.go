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

func NewReimbursementRepo(data *infra.Data) *ReimbursementRepo {
	if err := data.DB.AutoMigrate(&model.Reimbursement{}); err != nil {
		panic(err)
	}
	return &ReimbursementRepo{db: data.DB}
}

func (r *ReimbursementRepo) Create(rm *model.Reimbursement) error {
	return r.db.Create(rm).Error
}

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

func (r *ReimbursementRepo) ListByStatus(status string) ([]*model.Reimbursement, error) {
	var rms []*model.Reimbursement
	err := r.db.Where("status = ?", status).
		Preload("Department").Preload("Invoices").Preload("Approvals").
		Order("id ASC").Find(&rms).Error
	return rms, err
}

func (r *ReimbursementRepo) Update(rm *model.Reimbursement) error {
	return r.db.Save(rm).Error
}

func (r *ReimbursementRepo) UpdateStatus(id uint, status string) error {
	return r.db.Model(&model.Reimbursement{}).Where("id = ?", id).
		Update("status", status).Error
}
