package budget

import (
	"gorm.io/gorm"

	"github.com/CycleZero/Reimbee/infra"
	"github.com/CycleZero/Reimbee/model"
)

// BudgetRepo 部门预算数据访问层
type BudgetRepo struct {
	db *gorm.DB
}

func NewBudgetRepo(data *infra.Data) *BudgetRepo {
	if err := data.DB.AutoMigrate(&model.DepartmentBudget{}); err != nil {
		panic(err)
	}
	return &BudgetRepo{db: data.DB}
}

func (r *BudgetRepo) Create(b *model.DepartmentBudget) error {
	return r.db.Create(b).Error
}

func (r *BudgetRepo) GetByID(id uint) (*model.DepartmentBudget, error) {
	var b model.DepartmentBudget
	if err := r.db.Preload("Department").First(&b, id).Error; err != nil {
		return nil, err
	}
	return &b, nil
}

func (r *BudgetRepo) GetByDepartmentAndYear(deptID uint, year int) (*model.DepartmentBudget, error) {
	var b model.DepartmentBudget
	err := r.db.Where("department_id = ? AND fiscal_year = ?", deptID, year).First(&b).Error
	return &b, err
}

func (r *BudgetRepo) ListByYear(year int) ([]*model.DepartmentBudget, error) {
	var budgets []*model.DepartmentBudget
	err := r.db.Where("fiscal_year = ?", year).Preload("Department").Find(&budgets).Error
	return budgets, err
}

func (r *BudgetRepo) Update(b *model.DepartmentBudget) error {
	return r.db.Save(b).Error
}

// Deduct 扣减预算（审批通过时调用）
func (r *BudgetRepo) Deduct(deptID uint, year int, amount int64) error {
	return r.db.Model(&model.DepartmentBudget{}).
		Where("department_id = ? AND fiscal_year = ?", deptID, year).
		Updates(map[string]interface{}{
			"spent_amount":  gorm.Expr("spent_amount + ?", amount),
			"frozen_amount": gorm.Expr("GREATEST(frozen_amount - ?, 0)", amount),
		}).Error
}

// Freeze 冻结预算（提交报销时调用）
func (r *BudgetRepo) Freeze(deptID uint, year int, amount int64) error {
	return r.db.Model(&model.DepartmentBudget{}).
		Where("department_id = ? AND fiscal_year = ?", deptID, year).
		Update("frozen_amount", gorm.Expr("frozen_amount + ?", amount)).Error
}

// Unfreeze 解冻预算（驳回时调用）
func (r *BudgetRepo) Unfreeze(deptID uint, year int, amount int64) error {
	return r.db.Model(&model.DepartmentBudget{}).
		Where("department_id = ? AND fiscal_year = ?", deptID, year).
		Update("frozen_amount", gorm.Expr("GREATEST(frozen_amount - ?, 0)", amount)).Error
}

func (r *BudgetRepo) Delete(id uint) error {
	return r.db.Delete(&model.DepartmentBudget{}, id).Error
}
