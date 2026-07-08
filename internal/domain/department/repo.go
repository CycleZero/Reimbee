package department

import (
	"errors"

	"gorm.io/gorm"

	"github.com/CycleZero/Reimbee/infra"
	"github.com/CycleZero/Reimbee/model"
)

var (
	ErrDeptHasEmployees = errors.New("该部门下仍有员工，无法删除")
	ErrDeptHasBudget    = errors.New("该部门下仍有预算记录，无法删除")
)

// DepartmentRepo 部门数据访问层
type DepartmentRepo struct {
	db *gorm.DB
}

// NewDepartmentRepo 创建部门数据访问层实例，自动迁移表结构
func NewDepartmentRepo(data *infra.Data) *DepartmentRepo {
	if err := data.DB.AutoMigrate(&model.Department{}); err != nil {
		panic(err)
	}
	return &DepartmentRepo{db: data.DB}
}

// Create 创建部门
func (r *DepartmentRepo) Create(d *model.Department) error {
	return r.db.Create(d).Error
}

// GetByID 根据主键 ID 查询部门，预加载部门主管信息
func (r *DepartmentRepo) GetByID(id uint) (*model.Department, error) {
	var d model.Department
	if err := r.db.Preload("Manager").First(&d, id).Error; err != nil {
		return nil, err
	}
	return &d, nil
}

// GetByName 根据部门名称查询部门
func (r *DepartmentRepo) GetByName(name string) (*model.Department, error) {
	var d model.Department
	if err := r.db.Where("name = ?", name).First(&d).Error; err != nil {
		return nil, err
	}
	return &d, nil
}

// SearchByName 模糊搜索部门（按名称，支持部分匹配）
func (r *DepartmentRepo) SearchByName(name string) ([]*model.Department, error) {
	var depts []*model.Department
	err := r.db.Where("name LIKE ?", "%"+name+"%").Find(&depts).Error
	return depts, err
}

// List 分页查询部门列表
func (r *DepartmentRepo) List(page, pageSize int) ([]*model.Department, int64, error) {
	var depts []*model.Department
	var total int64
	db := r.db.Model(&model.Department{})
	db.Count(&total)
	err := db.Offset((page - 1) * pageSize).Limit(pageSize).Order("id ASC").Find(&depts).Error
	return depts, total, err
}

// Update 更新部门信息
func (r *DepartmentRepo) Update(d *model.Department) error {
	return r.db.Save(d).Error
}

// Delete 删除部门，若部门下仍有员工或预算记录则拒绝删除
func (r *DepartmentRepo) Delete(id uint) error {
	var count int64
	r.db.Model(&model.Employee{}).Where("department_id = ?", id).Count(&count)
	if count > 0 {
		return ErrDeptHasEmployees
	}
	r.db.Model(&model.DepartmentBudget{}).Where("department_id = ?", id).Count(&count)
	if count > 0 {
		return ErrDeptHasBudget
	}
	return r.db.Delete(&model.Department{}, id).Error
}
