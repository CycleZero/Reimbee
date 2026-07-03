package department

import (
	"errors"
	"fmt"

	"github.com/CycleZero/Reimbee/log"
	"github.com/CycleZero/Reimbee/model"
	"go.uber.org/zap"
)

// DepartmentBiz 部门业务逻辑层
type DepartmentBiz struct {
	logger *log.Logger
	repo   *DepartmentRepo
}

// NewDepartmentBiz 创建部门业务逻辑层实例
func NewDepartmentBiz(logger *log.Logger, repo *DepartmentRepo) *DepartmentBiz {
	logger.Debug("初始化部门业务逻辑层")
	return &DepartmentBiz{logger: logger, repo: repo}
}

// Create 创建部门，校验部门名称唯一性
func (b *DepartmentBiz) Create(name string, managerID *uint) (*model.Department, error) {
	b.logger.Debug("开始创建部门", zap.String("部门名称", name))

	// 检查部门名称是否已存在
	existing, err := b.repo.GetByName(name)
	if err == nil && existing != nil {
		b.logger.Warn("部门名称已存在，创建失败", zap.String("部门名称", name))
		return nil, fmt.Errorf("部门名称'%s'已存在", name)
	}

	// 如果指定了主管，校验主管是否存在且是该部门成员
	if managerID != nil {
		b.logger.Debug("校验部门主管", zap.Uint("主管ID", *managerID))
	}

	dept := &model.Department{
		Name:      name,
		ManagerID: managerID,
	}
	if err := b.repo.Create(dept); err != nil {
		b.logger.Error("创建部门失败", zap.String("部门名称", name), zap.Error(err))
		return nil, fmt.Errorf("创建部门失败: %w", err)
	}

	b.logger.Info("部门创建成功", zap.Uint("部门ID", dept.ID), zap.String("部门名称", dept.Name))
	return dept, nil
}

// GetByID 根据 ID 查询部门
func (b *DepartmentBiz) GetByID(id uint) (*model.Department, error) {
	b.logger.Debug("查询部门", zap.Uint("部门ID", id))
	dept, err := b.repo.GetByID(id)
	if err != nil {
		b.logger.Warn("部门不存在", zap.Uint("部门ID", id), zap.Error(err))
		return nil, fmt.Errorf("部门不存在")
	}
	return dept, nil
}

// GetByName 根据名称查询部门
func (b *DepartmentBiz) GetByName(name string) (*model.Department, error) {
	b.logger.Debug("根据名称查询部门", zap.String("部门名称", name))
	dept, err := b.repo.GetByName(name)
	if err != nil {
		b.logger.Warn("部门不存在", zap.String("部门名称", name), zap.Error(err))
		return nil, fmt.Errorf("部门不存在")
	}
	return dept, nil
}

// List 分页查询部门列表
func (b *DepartmentBiz) List(page, pageSize int) ([]*model.Department, int64, error) {
	b.logger.Debug("查询部门列表", zap.Int("页码", page), zap.Int("每页数量", pageSize))
	depts, total, err := b.repo.List(page, pageSize)
	if err != nil {
		b.logger.Error("查询部门列表失败", zap.Error(err))
		return nil, 0, fmt.Errorf("查询部门列表失败: %w", err)
	}
	b.logger.Debug("查询部门列表成功", zap.Int64("总数", total), zap.Int("返回数量", len(depts)))
	return depts, total, nil
}

// Update 更新部门信息，校验名称唯一性
func (b *DepartmentBiz) Update(id uint, name string, managerID *uint) (*model.Department, error) {
	b.logger.Debug("开始更新部门", zap.Uint("部门ID", id), zap.String("新名称", name))

	dept, err := b.repo.GetByID(id)
	if err != nil {
		b.logger.Warn("要更新的部门不存在", zap.Uint("部门ID", id))
		return nil, fmt.Errorf("部门不存在")
	}

	// 校验名称唯一性（排除自身）
	if name != dept.Name {
		existing, _ := b.repo.GetByName(name)
		if existing != nil && existing.ID != id {
			b.logger.Warn("部门名称冲突", zap.String("部门名称", name))
			return nil, fmt.Errorf("部门名称'%s'已被其他部门使用", name)
		}
	}

	dept.Name = name
	if managerID != nil {
		dept.ManagerID = managerID
	}
	if err := b.repo.Update(dept); err != nil {
		b.logger.Error("更新部门失败", zap.Uint("部门ID", id), zap.Error(err))
		return nil, fmt.Errorf("更新部门失败: %w", err)
	}

	b.logger.Info("部门更新成功", zap.Uint("部门ID", dept.ID), zap.String("部门名称", dept.Name))
	return dept, nil
}

// Delete 删除部门，校验无下属员工和预算记录
func (b *DepartmentBiz) Delete(id uint) error {
	b.logger.Debug("开始删除部门", zap.Uint("部门ID", id))

	if err := b.repo.Delete(id); err != nil {
		if errors.Is(err, errors.New("该部门下仍有员工，无法删除")) ||
			errors.Is(err, errors.New("该部门下仍有预算记录，无法删除")) {
			b.logger.Warn("部门删除被拒绝", zap.Uint("部门ID", id), zap.Error(err))
			return err
		}
		b.logger.Error("删除部门失败", zap.Uint("部门ID", id), zap.Error(err))
		return fmt.Errorf("删除部门失败: %w", err)
	}

	b.logger.Info("部门删除成功", zap.Uint("部门ID", id))
	return nil
}
