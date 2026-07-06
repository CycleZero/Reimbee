package employee

import (
	"fmt"

	"github.com/CycleZero/Reimbee/log"
	"github.com/CycleZero/Reimbee/model"
	"go.uber.org/zap"
)

// EmployeeBiz 员工业务逻辑层
type EmployeeBiz struct {
	logger *log.Logger
	repo   *EmployeeRepo
}

// NewEmployeeBiz 创建员工业务逻辑层实例
func NewEmployeeBiz(logger *log.Logger, repo *EmployeeRepo) *EmployeeBiz {
	logger.Debug("初始化员工业务逻辑层")
	return &EmployeeBiz{logger: logger, repo: repo}
}

// Create 创建员工，校验工号唯一性
func (b *EmployeeBiz) Create(employeeID, name, email string, deptID uint, role string) (*model.Employee, error) {
	b.logger.Debug("开始创建员工", zap.String("工号", employeeID), zap.String("姓名", name))

	// 检查工号是否已存在
	existing, err := b.repo.GetByEmployeeID(employeeID)
	if err == nil && existing != nil {
		b.logger.Warn("工号已存在，创建失败", zap.String("工号", employeeID))
		return nil, fmt.Errorf("工号'%s'已被使用", employeeID)
	}

	emp := &model.Employee{
		EmployeeID:   employeeID,
		Name:         name,
		DepartmentID: deptID,
		Email:        email,
		Role:         role,
		IsApprover:   model.IsApproverRole(role),
	}
	if err := b.repo.Create(emp); err != nil {
		b.logger.Error("创建员工失败", zap.String("工号", employeeID), zap.Error(err))
		return nil, fmt.Errorf("创建员工失败: %w", err)
	}

	b.logger.Info("员工创建成功", zap.Uint("员工ID", emp.ID), zap.String("工号", emp.EmployeeID), zap.String("姓名", emp.Name))
	return emp, nil
}

// GetByID 根据主键 ID 查询员工
func (b *EmployeeBiz) GetByID(id uint) (*model.Employee, error) {
	b.logger.Debug("查询员工", zap.Uint("员工ID", id))
	emp, err := b.repo.GetByID(id)
	if err != nil {
		b.logger.Warn("员工不存在", zap.Uint("员工ID", id), zap.Error(err))
		return nil, fmt.Errorf("员工不存在")
	}
	return emp, nil
}

// GetByEmployeeID 根据工号查询员工
func (b *EmployeeBiz) GetByEmployeeID(employeeID string) (*model.Employee, error) {
	b.logger.Debug("根据工号查询员工", zap.String("工号", employeeID))
	emp, err := b.repo.GetByEmployeeID(employeeID)
	if err != nil {
		b.logger.Warn("员工不存在", zap.String("工号", employeeID), zap.Error(err))
		return nil, fmt.Errorf("工号为'%s'的员工不存在", employeeID)
	}
	return emp, nil
}

// List 分页查询员工列表
func (b *EmployeeBiz) List(page, pageSize int) ([]*model.Employee, int64, error) {
	b.logger.Debug("查询员工列表", zap.Int("页码", page), zap.Int("每页数量", pageSize))
	emps, total, err := b.repo.List(page, pageSize)
	if err != nil {
		b.logger.Error("查询员工列表失败", zap.Error(err))
		return nil, 0, fmt.Errorf("查询员工列表失败: %w", err)
	}
	b.logger.Debug("查询员工列表成功", zap.Int64("总数", total), zap.Int("返回数量", len(emps)))
	return emps, total, nil
}

// ListByDepartment 查询指定部门下的员工
func (b *EmployeeBiz) ListByDepartment(deptID uint) ([]*model.Employee, error) {
	b.logger.Debug("查询部门员工", zap.Uint("部门ID", deptID))
	emps, err := b.repo.ListByDepartment(deptID)
	if err != nil {
		b.logger.Error("查询部门员工失败", zap.Uint("部门ID", deptID), zap.Error(err))
		return nil, fmt.Errorf("查询部门员工失败: %w", err)
	}
	b.logger.Debug("查询部门员工成功", zap.Int("员工数量", len(emps)))
	return emps, nil
}

// ListApprovers 查询所有审批人
func (b *EmployeeBiz) ListApprovers() ([]*model.Employee, error) {
	b.logger.Debug("查询审批人列表")
	approvers, err := b.repo.ListApprovers()
	if err != nil {
		b.logger.Error("查询审批人列表失败", zap.Error(err))
		return nil, fmt.Errorf("查询审批人列表失败: %w", err)
	}
	b.logger.Debug("查询审批人列表成功", zap.Int("审批人数量", len(approvers)))
	return approvers, nil
}

// Update 更新员工信息
func (b *EmployeeBiz) Update(id uint, name, email string, deptID uint, role string) (*model.Employee, error) {
	b.logger.Debug("开始更新员工", zap.Uint("员工ID", id))

	emp, err := b.repo.GetByID(id)
	if err != nil {
		b.logger.Warn("要更新的员工不存在", zap.Uint("员工ID", id))
		return nil, fmt.Errorf("员工不存在")
	}

	emp.Name = name
	emp.Email = email
	emp.DepartmentID = deptID
	emp.Role = role
	emp.IsApprover = role == "approver" || role == "admin"

	if err := b.repo.Update(emp); err != nil {
		b.logger.Error("更新员工失败", zap.Uint("员工ID", id), zap.Error(err))
		return nil, fmt.Errorf("更新员工失败: %w", err)
	}

	b.logger.Info("员工更新成功", zap.Uint("员工ID", emp.ID), zap.String("工号", emp.EmployeeID))
	return emp, nil
}

// Delete 删除员工
func (b *EmployeeBiz) Delete(id uint) error {
	b.logger.Debug("开始删除员工", zap.Uint("员工ID", id))

	// 检查员工是否存在
	if _, err := b.repo.GetByID(id); err != nil {
		b.logger.Warn("要删除的员工不存在", zap.Uint("员工ID", id))
		return fmt.Errorf("员工不存在")
	}

	if err := b.repo.Delete(id); err != nil {
		b.logger.Error("删除员工失败", zap.Uint("员工ID", id), zap.Error(err))
		return fmt.Errorf("删除员工失败: %w", err)
	}

	b.logger.Info("员工删除成功", zap.Uint("员工ID", id))
	return nil
}
