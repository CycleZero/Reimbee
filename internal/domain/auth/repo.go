package auth

import (
	"fmt"

	"gorm.io/gorm"

	"github.com/CycleZero/Reimbee/infra"
	"github.com/CycleZero/Reimbee/model"
)

// EmployeeRepo 认证模块员工数据访问层
type EmployeeRepo struct {
	db *gorm.DB
}

// NewEmployeeRepo 创建员工数据访问层
func NewEmployeeRepo(data *infra.Data) *EmployeeRepo {
	return &EmployeeRepo{db: data.DB}
}

// GetByEmployeeID 根据工号查询员工
func (r *EmployeeRepo) GetByEmployeeID(employeeID string) (*model.Employee, error) {
	var e model.Employee
	if err := r.db.Where("employee_id = ?", employeeID).First(&e).Error; err != nil {
		return nil, err
	}
	return &e, nil
}

// Create 创建员工
func (r *EmployeeRepo) Create(e *model.Employee) error {
	return r.db.Create(e).Error
}

// NextEmployeeID 生成下一个工号（EMP001, EMP002...）
func (r *EmployeeRepo) NextEmployeeID() (string, error) {
	var maxID string
	if err := r.db.Model(&model.Employee{}).Select("COALESCE(MAX(employee_id), '')").Scan(&maxID).Error; err != nil {
		return "", fmt.Errorf("查询最大工号失败: %w", err)
	}
	if maxID == "" {
		return "EMP001", nil
	}
	num := 0
	fmt.Sscanf(maxID, "EMP%d", &num)
	return fmt.Sprintf("EMP%03d", num+1), nil
}
