package budget

import (
	"fmt"

	"github.com/CycleZero/Reimbee/log"
	"github.com/CycleZero/Reimbee/model"
	"go.uber.org/zap"
)

const currentFiscalYear = 2026

// BudgetBiz 部门预算业务逻辑层
type BudgetBiz struct {
	logger *log.Logger
	repo   *BudgetRepo
}

// NewBudgetBiz 创建预算业务逻辑层实例
func NewBudgetBiz(logger *log.Logger, repo *BudgetRepo) *BudgetBiz {
	logger.Debug("初始化预算业务逻辑层")
	return &BudgetBiz{logger: logger, repo: repo}
}

// Create 创建部门年度预算，校验同一部门同年不可重复
func (b *BudgetBiz) Create(deptID uint, year int, annualBudget int64) (*model.DepartmentBudget, error) {
	b.logger.Debug("开始创建预算记录", zap.Uint("部门ID", deptID), zap.Int("财年", year), zap.Int64("年度预算(分)", annualBudget))

	// 检查同一部门同一年是否已有预算记录
	existing, _ := b.repo.GetByDepartmentAndYear(deptID, year)
	if existing != nil {
		b.logger.Warn("预算记录已存在，创建失败", zap.Uint("部门ID", deptID), zap.Int("财年", year))
		return nil, fmt.Errorf("该部门%d年度预算已存在", year)
	}

	budget := &model.DepartmentBudget{
		DepartmentID: deptID,
		FiscalYear:   year,
		AnnualBudget: annualBudget,
	}
	if err := b.repo.Create(budget); err != nil {
		b.logger.Error("创建预算记录失败", zap.Uint("部门ID", deptID), zap.Error(err))
		return nil, fmt.Errorf("创建预算记录失败: %w", err)
	}

	b.logger.Info("预算记录创建成功", zap.Uint("预算ID", budget.ID), zap.Uint("部门ID", deptID), zap.Int("财年", year))
	return budget, nil
}

// GetByID 根据 ID 查询预算
func (b *BudgetBiz) GetByID(id uint) (*model.DepartmentBudget, error) {
	b.logger.Debug("查询预算记录", zap.Uint("预算ID", id))
	budget, err := b.repo.GetByID(id)
	if err != nil {
		b.logger.Warn("预算记录不存在", zap.Uint("预算ID", id), zap.Error(err))
		return nil, fmt.Errorf("预算记录不存在")
	}
	return budget, nil
}

// GetDashboard 获取当前财年预算看板数据
func (b *BudgetBiz) GetDashboard(year int) ([]*model.DepartmentBudget, error) {
	b.logger.Debug("获取预算看板数据", zap.Int("财年", year))
	budgets, err := b.repo.ListByYear(year)
	if err != nil {
		b.logger.Error("获取预算看板数据失败", zap.Error(err))
		return nil, fmt.Errorf("获取预算看板数据失败: %w", err)
	}
	b.logger.Debug("获取预算看板数据成功", zap.Int("部门数量", len(budgets)))
	return budgets, nil
}

// CheckBudget 检查预算是否充足，返回可用余额和是否需要特殊审批
// 返回: remaining(分), needSpecialApproval
func (b *BudgetBiz) CheckBudget(deptID uint, amount int64) (remaining int64, needSpecialApproval bool, err error) {
	b.logger.Debug("检查预算可用性", zap.Uint("部门ID", deptID), zap.Int64("申请金额(分)", amount))

	budget, err := b.repo.GetByDepartmentAndYear(deptID, currentFiscalYear)
	if err != nil {
		b.logger.Warn("未找到部门预算记录", zap.Uint("部门ID", deptID), zap.Error(err))
		return 0, false, fmt.Errorf("该部门未设置%d年度预算", currentFiscalYear)
	}

	// 可用余额 = 年度预算 - 已结算 - 已冻结
	remaining = budget.AnnualBudget - budget.SpentAmount - budget.FrozenAmount

	b.logger.Debug("预算检查完成",
		zap.Int64("年度预算", budget.AnnualBudget),
		zap.Int64("已结算", budget.SpentAmount),
		zap.Int64("已冻结", budget.FrozenAmount),
		zap.Int64("可用余额", remaining),
	)

	// 超额标记特殊审批
	if amount > remaining {
		b.logger.Warn("预算不足，触发特殊审批", zap.Int64("申请金额", amount), zap.Int64("可用余额", remaining))
		needSpecialApproval = true
	}

	return remaining, needSpecialApproval, nil
}

// Freeze 冻结预算（提交报销时调用）
func (b *BudgetBiz) Freeze(deptID uint, amount int64) error {
	b.logger.Debug("冻结预算", zap.Uint("部门ID", deptID), zap.Int64("冻结金额(分)", amount))

	if amount <= 0 {
		b.logger.Warn("冻结金额无效", zap.Int64("金额", amount))
		return fmt.Errorf("冻结金额必须大于零")
	}

	if err := b.repo.Freeze(deptID, currentFiscalYear, amount); err != nil {
		b.logger.Error("冻结预算失败", zap.Uint("部门ID", deptID), zap.Error(err))
		return fmt.Errorf("冻结预算失败: %w", err)
	}

	b.logger.Info("预算冻结成功", zap.Uint("部门ID", deptID), zap.Int64("冻结金额(分)", amount))
	return nil
}

// Deduct 扣减预算（审批通过后调用）
func (b *BudgetBiz) Deduct(deptID uint, amount int64) error {
	b.logger.Debug("扣减预算", zap.Uint("部门ID", deptID), zap.Int64("扣减金额(分)", amount))

	if amount <= 0 {
		b.logger.Warn("扣减金额无效", zap.Int64("金额", amount))
		return fmt.Errorf("扣减金额必须大于零")
	}

	if err := b.repo.Deduct(deptID, currentFiscalYear, amount); err != nil {
		b.logger.Error("扣减预算失败", zap.Uint("部门ID", deptID), zap.Error(err))
		return fmt.Errorf("扣减预算失败: %w", err)
	}

	b.logger.Info("预算扣减成功", zap.Uint("部门ID", deptID), zap.Int64("扣减金额(分)", amount))
	return nil
}

// Unfreeze 解冻预算（报销被驳回时调用）
func (b *BudgetBiz) Unfreeze(deptID uint, amount int64) error {
	b.logger.Debug("解冻预算", zap.Uint("部门ID", deptID), zap.Int64("解冻金额(分)", amount))

	if amount <= 0 {
		b.logger.Warn("解冻金额无效", zap.Int64("金额", amount))
		return fmt.Errorf("解冻金额必须大于零")
	}

	if err := b.repo.Unfreeze(deptID, currentFiscalYear, amount); err != nil {
		b.logger.Error("解冻预算失败", zap.Uint("部门ID", deptID), zap.Error(err))
		return fmt.Errorf("解冻预算失败: %w", err)
	}

	b.logger.Info("预算解冻成功", zap.Uint("部门ID", deptID), zap.Int64("解冻金额(分)", amount))
	return nil
}

// Update 更新预算记录
func (b *BudgetBiz) Update(id uint, annualBudget int64) (*model.DepartmentBudget, error) {
	b.logger.Debug("开始更新预算记录", zap.Uint("预算ID", id), zap.Int64("新年预算(分)", annualBudget))

	budget, err := b.repo.GetByID(id)
	if err != nil {
		b.logger.Warn("要更新的预算记录不存在", zap.Uint("预算ID", id))
		return nil, fmt.Errorf("预算记录不存在")
	}

	budget.AnnualBudget = annualBudget
	if err := b.repo.Update(budget); err != nil {
		b.logger.Error("更新预算记录失败", zap.Uint("预算ID", id), zap.Error(err))
		return nil, fmt.Errorf("更新预算记录失败: %w", err)
	}

	b.logger.Info("预算记录更新成功", zap.Uint("预算ID", id))
	return budget, nil
}
