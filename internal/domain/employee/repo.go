package employee

import (
	"gorm.io/gorm"

	"github.com/CycleZero/Reimbee/infra"
	"github.com/CycleZero/Reimbee/model"
)

// EmployeeRepo 员工数据访问层
type EmployeeRepo struct {
	db *gorm.DB
}

func NewEmployeeRepo(data *infra.Data) *EmployeeRepo {
	if err := data.DB.AutoMigrate(&model.Employee{}); err != nil {
		panic(err)
	}
	return &EmployeeRepo{db: data.DB}
}

func (r *EmployeeRepo) Create(e *model.Employee) error {
	return r.db.Create(e).Error
}

func (r *EmployeeRepo) GetByID(id uint) (*model.Employee, error) {
	var e model.Employee
	if err := r.db.Preload("Department").First(&e, id).Error; err != nil {
		return nil, err
	}
	return &e, nil
}

func (r *EmployeeRepo) GetByEmployeeID(employeeID string) (*model.Employee, error) {
	var e model.Employee
	if err := r.db.Where("employee_id = ?", employeeID).Preload("Department").First(&e).Error; err != nil {
		return nil, err
	}
	return &e, nil
}

func (r *EmployeeRepo) List(page, pageSize int) ([]*model.Employee, int64, error) {
	var emps []*model.Employee
	var total int64
	db := r.db.Model(&model.Employee{})
	db.Count(&total)
	err := db.Offset((page - 1) * pageSize).Limit(pageSize).Preload("Department").Order("id ASC").Find(&emps).Error
	return emps, total, err
}

func (r *EmployeeRepo) ListByDepartment(deptID uint) ([]*model.Employee, error) {
	var emps []*model.Employee
	err := r.db.Where("department_id = ?", deptID).Preload("Department").Find(&emps).Error
	return emps, err
}

func (r *EmployeeRepo) ListApprovers() ([]*model.Employee, error) {
	var emps []*model.Employee
	err := r.db.Where("is_approver = ?", true).Find(&emps).Error
	return emps, err
}

func (r *EmployeeRepo) Update(e *model.Employee) error {
	return r.db.Save(e).Error
}

func (r *EmployeeRepo) Delete(id uint) error {
	return r.db.Delete(&model.Employee{}, id).Error
}
