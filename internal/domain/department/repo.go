package department

import (
	"errors"

	"gorm.io/gorm"

	"github.com/CycleZero/Reimbee/infra"
	"github.com/CycleZero/Reimbee/model"
)

// DepartmentRepo 部门数据访问层
type DepartmentRepo struct {
	db *gorm.DB
}

func NewDepartmentRepo(data *infra.Data) *DepartmentRepo {
	if err := data.DB.AutoMigrate(&model.Department{}); err != nil {
		panic(err)
	}
	return &DepartmentRepo{db: data.DB}
}

func (r *DepartmentRepo) Create(d *model.Department) error {
	return r.db.Create(d).Error
}

func (r *DepartmentRepo) GetByID(id uint) (*model.Department, error) {
	var d model.Department
	if err := r.db.Preload("Manager").First(&d, id).Error; err != nil {
		return nil, err
	}
	return &d, nil
}

func (r *DepartmentRepo) GetByName(name string) (*model.Department, error) {
	var d model.Department
	if err := r.db.Where("name = ?", name).First(&d).Error; err != nil {
		return nil, err
	}
	return &d, nil
}

func (r *DepartmentRepo) List(page, pageSize int) ([]*model.Department, int64, error) {
	var depts []*model.Department
	var total int64
	db := r.db.Model(&model.Department{})
	db.Count(&total)
	err := db.Offset((page - 1) * pageSize).Limit(pageSize).Order("id ASC").Find(&depts).Error
	return depts, total, err
}

func (r *DepartmentRepo) Update(d *model.Department) error {
	return r.db.Save(d).Error
}

func (r *DepartmentRepo) Delete(id uint) error {
	var count int64
	r.db.Model(&model.Employee{}).Where("department_id = ?", id).Count(&count)
	if count > 0 {
		return errors.New("该部门仍有员工，无法删除")
	}
	r.db.Model(&model.DepartmentBudget{}).Where("department_id = ?", id).Count(&count)
	if count > 0 {
		return errors.New("该部门仍有预算记录，无法删除")
	}
	return r.db.Delete(&model.Department{}, id).Error
}
