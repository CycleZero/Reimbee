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

// NewEmployeeRepo 创建员工数据访问层实例，自动迁移表结构
func NewEmployeeRepo(data *infra.Data) *EmployeeRepo {
	if err := data.DB.AutoMigrate(&model.Employee{}); err != nil {
		panic(err)
	}
	return &EmployeeRepo{db: data.DB}
}

// Create 创建员工
func (r *EmployeeRepo) Create(e *model.Employee) error {
	return r.db.Create(e).Error
}

// GetByID 根据主键 ID 查询员工，预加载部门信息
func (r *EmployeeRepo) GetByID(id uint) (*model.Employee, error) {
	var e model.Employee
	if err := r.db.Preload("Department").First(&e, id).Error; err != nil {
		return nil, err
	}
	return &e, nil
}

// GetByEmployeeID 根据工号查询员工，预加载部门信息
func (r *EmployeeRepo) GetByEmployeeID(employeeID string) (*model.Employee, error) {
	var e model.Employee
	if err := r.db.Where("employee_id = ?", employeeID).Preload("Department").First(&e).Error; err != nil {
		return nil, err
	}
	return &e, nil
}

// List 分页查询员工列表，预加载部门信息
func (r *EmployeeRepo) List(page, pageSize int) ([]*model.Employee, int64, error) {
	var emps []*model.Employee
	var total int64
	db := r.db.Model(&model.Employee{})
	db.Count(&total)
	err := db.Offset((page - 1) * pageSize).Limit(pageSize).Preload("Department").Order("id ASC").Find(&emps).Error
	return emps, total, err
}

// ListByDepartment 查询指定部门下的所有员工
func (r *EmployeeRepo) ListByDepartment(deptID uint) ([]*model.Employee, error) {
	var emps []*model.Employee
	err := r.db.Where("department_id = ?", deptID).Preload("Department").Find(&emps).Error
	return emps, err
}

// ListApprovers 查询所有具有审批权限的员工
func (r *EmployeeRepo) ListApprovers() ([]*model.Employee, error) {
	var emps []*model.Employee
	err := r.db.Where("is_approver = ?", true).Find(&emps).Error
	return emps, err
}

// Update 更新员工信息
func (r *EmployeeRepo) Update(e *model.Employee) error {
	return r.db.Save(e).Error
}

// Delete 删除员工
func (r *EmployeeRepo) Delete(id uint) error {
	return r.db.Delete(&model.Employee{}, id).Error
}
