package employee

import (
	"fmt"

	"github.com/CycleZero/Reimbee/log"
	"github.com/CycleZero/Reimbee/model"
	"go.uber.org/zap"
)

// EmployeeBiz 员工业务逻辑层
// 负责员工领域的核心业务规则：工号唯一性校验、角色-审批人映射、CRUD 编排
type EmployeeBiz struct {
	logger *log.Logger // 结构化日志器，用于记录业务操作的关键路径
	repo   *EmployeeRepo // 数据访问层，封装所有 DB 操作
}

// NewEmployeeBiz 创建员工业务逻辑层实例
func NewEmployeeBiz(logger *log.Logger, repo *EmployeeRepo) *EmployeeBiz {
	logger.Debug("初始化员工业务逻辑层")
	return &EmployeeBiz{logger: logger, repo: repo}
}

// Create 创建员工，校验工号唯一性
// 业务流程：1) 检查工号唯一性 2) 自动推断审批人标志 3) 持久化员工
func (b *EmployeeBiz) Create(employeeID, name, email string, deptID uint, role string) (*model.Employee, error) {
	b.logger.Debug("开始创建员工", zap.String("工号", employeeID), zap.String("姓名", name))

	// 第一步：检查工号唯一性——工号是员工的业务主键，必须全局唯一
	// 先根据工号查询是否已存在，避免唯一索引冲突
	existing, err := b.repo.GetByEmployeeID(employeeID)
	// 如果查询成功（err == nil）且返回值非空，说明该工号已被占用
	if err == nil && existing != nil {
		b.logger.Warn("工号已存在，创建失败", zap.String("工号", employeeID))
		return nil, fmt.Errorf("工号'%s'已被使用", employeeID)
	}
	// 工号唯一性校验通过，可以继续创建
	b.logger.Debug("工号唯一性校验通过", zap.String("工号", employeeID))

	// 第二步：构造 Employee 模型，填充所有业务字段
	// 根据角色自动计算 IsApprover 标志——审批人和管理员需要审批权限
	emp := &model.Employee{
		EmployeeID:   employeeID,                                         // 工号，业务主键
		Name:         name,                                               // 员工姓名
		DepartmentID: deptID,                                             // 所属部门外键
		Email:        email,                                              // 工作邮箱
		Role:         role,                                               // 角色：employee/approver/admin
		IsApprover:   model.IsApproverRole(role),                         // 根据角色自动推断审批人标志
	}
	// 第三步：通过 repo 将员工持久化到数据库
	// GORM 会自动填充 ID、CreatedAt、UpdatedAt 等字段
	if err := b.repo.Create(emp); err != nil {
		b.logger.Error("创建员工失败", zap.String("工号", employeeID), zap.Error(err))
		return nil, fmt.Errorf("创建员工失败: %w", err)
	}
	// 持久化成功，emp.ID 已被 GORM 回填
	b.logger.Debug("员工持久化成功，ID已回填", zap.Uint("员工ID", emp.ID), zap.String("工号", emp.EmployeeID))

	b.logger.Info("员工创建成功", zap.Uint("员工ID", emp.ID), zap.String("工号", emp.EmployeeID), zap.String("姓名", emp.Name))
	return emp, nil
}

// GetByID 根据主键 ID 查询员工
// 从 repo 层获取员工信息（含预加载的部门关联数据）
func (b *EmployeeBiz) GetByID(id uint) (*model.Employee, error) {
	b.logger.Debug("查询员工", zap.Uint("员工ID", id))
	// repo.GetByID 使用 Preload("Department") 自动加载部门信息，避免 N+1 查询
	emp, err := b.repo.GetByID(id)
	if err != nil {
		// GORM First 未找到记录时返回 gorm.ErrRecordNotFound，统一转换为业务语义
		b.logger.Warn("员工不存在", zap.Uint("员工ID", id), zap.Error(err))
		return nil, fmt.Errorf("员工不存在")
	}
	// 查询成功，返回完整的员工信息（含部门关联）
	b.logger.Debug("员工查询成功", zap.Uint("员工ID", id), zap.String("姓名", emp.Name))
	return emp, nil
}

// GetByEmployeeID 根据工号查询员工
// 工号是业务主键，上层通过工号定位员工
func (b *EmployeeBiz) GetByEmployeeID(employeeID string) (*model.Employee, error) {
	b.logger.Debug("根据工号查询员工", zap.String("工号", employeeID))
	// repo 层使用 WHERE employee_id = ? 进行精确匹配查询
	emp, err := b.repo.GetByEmployeeID(employeeID)
	if err != nil {
		// 工号不存在是常见业务场景（如新建时查重），用 Warn 级别记录
		b.logger.Warn("员工不存在", zap.String("工号", employeeID), zap.Error(err))
		return nil, fmt.Errorf("工号为'%s'的员工不存在", employeeID)
	}
	b.logger.Debug("根据工号查询员工成功", zap.String("工号", employeeID), zap.String("姓名", emp.Name))
	return emp, nil
}

// List 分页查询员工列表
// 返回员工列表和总数，支持前端分页展示
func (b *EmployeeBiz) List(page, pageSize int) ([]*model.Employee, int64, error) {
	b.logger.Debug("查询员工列表", zap.Int("页码", page), zap.Int("每页数量", pageSize))
	// repo.List 内部先 COUNT 获取总数，再用 OFFSET/LIMIT 分页，同时 Preload 部门
	emps, total, err := b.repo.List(page, pageSize)
	if err != nil {
		b.logger.Error("查询员工列表失败", zap.Error(err))
		return nil, 0, fmt.Errorf("查询员工列表失败: %w", err)
	}
	// 分页查询成功——即使结果为空列表也是正常情况，不报错
	b.logger.Debug("查询员工列表成功", zap.Int64("总数", total), zap.Int("返回数量", len(emps)))
	return emps, total, nil
}

// ListByDepartment 查询指定部门下的员工
// 用于组织架构展示、部门人员管理等场景
func (b *EmployeeBiz) ListByDepartment(deptID uint) ([]*model.Employee, error) {
	b.logger.Debug("查询部门员工", zap.Uint("部门ID", deptID))
	// repo 层使用 WHERE department_id = ? 筛选，同时 Preload 部门详细信息
	emps, err := b.repo.ListByDepartment(deptID)
	if err != nil {
		b.logger.Error("查询部门员工失败", zap.Uint("部门ID", deptID), zap.Error(err))
		return nil, fmt.Errorf("查询部门员工失败: %w", err)
	}
	// 查询成功——即使部门下无员工也返回空切片而非 nil
	b.logger.Debug("查询部门员工成功", zap.Int("员工数量", len(emps)))
	return emps, nil
}

// ListApprovers 查询所有审批人
// 审批人是 is_approver = true 的员工，用于报销审批流程中的审批人选择
func (b *EmployeeBiz) ListApprovers() ([]*model.Employee, error) {
	b.logger.Debug("查询审批人列表")
	// repo 层使用 WHERE is_approver = ? 筛选出所有具有审批权限的员工
	approvers, err := b.repo.ListApprovers()
	if err != nil {
		b.logger.Error("查询审批人列表失败", zap.Error(err))
		return nil, fmt.Errorf("查询审批人列表失败: %w", err)
	}
	b.logger.Debug("查询审批人列表成功", zap.Int("审批人数量", len(approvers)))
	return approvers, nil
}

// SearchByName 模糊搜索员工（按姓名，支持部分匹配如输入"张"可匹配"张三"）
func (b *EmployeeBiz) SearchByName(name string) ([]*model.Employee, error) {
	b.logger.Debug("模糊搜索员工", zap.String("关键词", name))
	emps, err := b.repo.SearchByName(name)
	if err != nil {
		b.logger.Error("模糊搜索员工失败", zap.String("关键词", name), zap.Error(err))
		return nil, fmt.Errorf("搜索员工失败: %w", err)
	}
	b.logger.Info("模糊搜索员工成功", zap.String("关键词", name), zap.Int("匹配数", len(emps)))
	return emps, nil
}

// Update 更新员工信息
// 业务流程：1) 查询现有员工 2) 按需更新字段 3) 自动同步审批人标志 4) 持久化
func (b *EmployeeBiz) Update(id uint, name, email string, deptID uint, role string) (*model.Employee, error) {
	b.logger.Debug("开始更新员工", zap.Uint("员工ID", id))

	// 第一步：先查询员工是否存在——不允许更新不存在的员工
	emp, err := b.repo.GetByID(id)
	if err != nil {
		// GetByID 失败说明该 ID 的员工不存在，直接终止更新流程
		b.logger.Warn("要更新的员工不存在", zap.Uint("员工ID", id))
		return nil, fmt.Errorf("员工不存在")
	}
	// 员工存在，可以安全更新
	b.logger.Debug("已确认员工存在，准备更新字段", zap.Uint("员工ID", id), zap.String("当前姓名", emp.Name))

	// 第二步：逐一覆盖需要更新的业务字段
	// 这里使用全量覆盖策略——调用方传入的值直接替换，不做增量判断
	emp.Name = name                 // 更新姓名
	emp.Email = email               // 更新工作邮箱
	emp.DepartmentID = deptID       // 更新所属部门（外键）
	emp.Role = role                 // 更新角色
	emp.IsApprover = model.IsApproverRole(role) // 根据新角色重新计算审批人标志

	// 第三步：将更新后的员工持久化到数据库
	// repo.Update 使用 Save 方法，GORM 会根据 ID 判断是 UPDATE 操作
	b.logger.Debug("准备持久化员工更新", zap.Uint("员工ID", id), zap.String("新角色", role), zap.Bool("是否为审批人", emp.IsApprover))
	if err := b.repo.Update(emp); err != nil {
		b.logger.Error("更新员工失败", zap.Uint("员工ID", id), zap.Error(err))
		return nil, fmt.Errorf("更新员工失败: %w", err)
	}
	// 更新成功，GORM 已同步数据库
	b.logger.Debug("员工更新持久化成功", zap.Uint("员工ID", id))

	b.logger.Info("员工更新成功", zap.Uint("员工ID", emp.ID), zap.String("工号", emp.EmployeeID))
	return emp, nil
}

// Delete 删除员工
// 业务流程：1) 校验存在性 2) 执行软删除
func (b *EmployeeBiz) Delete(id uint) error {
	b.logger.Debug("开始删除员工", zap.Uint("员工ID", id))

	// 第一步：检查员工是否存在——不允许删除不存在的员工（幂等性考量）
	// 使用 GetByID 确认目标记录确实存在于数据库中
	if _, err := b.repo.GetByID(id); err != nil {
		b.logger.Warn("要删除的员工不存在", zap.Uint("员工ID", id))
		return fmt.Errorf("员工不存在")
	}
	// 员工存在性校验通过，可以安全删除
	b.logger.Debug("已确认员工存在，准备执行删除", zap.Uint("员工ID", id))

	// 第二步：通过 repo 执行删除操作
	// 注意：GORM 的 Delete 方法在 gorm.Model 嵌入时执行的是软删除（设置 deleted_at），而非物理删除
	if err := b.repo.Delete(id); err != nil {
		b.logger.Error("删除员工失败", zap.Uint("员工ID", id), zap.Error(err))
		return fmt.Errorf("删除员工失败: %w", err)
	}
	// 删除成功（软删除，deleted_at 字段被设置为当前时间戳）
	b.logger.Debug("员工删除操作已提交", zap.Uint("员工ID", id))

	b.logger.Info("员工删除成功", zap.Uint("员工ID", id))
	return nil
}
