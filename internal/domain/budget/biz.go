package budget

import (
	"fmt"
	"time"

	"github.com/CycleZero/Reimbee/log"
	"github.com/CycleZero/Reimbee/model"
	"go.uber.org/zap"
)

// currentFiscalYear 返回当前自然年作为财年标识
// 业务约定：财年与日历年对齐，每年 1 月 1 日至 12 月 31 日
func currentFiscalYear() int {
	// 获取当前系统时间对应的年份作为财年
	return time.Now().Year()
}

// BudgetBiz 部门预算业务逻辑层
// 职责：封装预算的创建、查询、冻结、扣减、解冻等核心业务规则
type BudgetBiz struct {
	logger *log.Logger // 结构化日志记录器，用于记录业务操作轨迹
	repo   *BudgetRepo // 数据访问层，封装所有预算相关的数据库操作
}

// NewBudgetBiz 创建预算业务逻辑层实例
// 通过依赖注入接收 logger 和 repo，确保可测试性和松耦合
func NewBudgetBiz(logger *log.Logger, repo *BudgetRepo) *BudgetBiz {
	// 日志：记录业务层初始化事件，便于追踪服务启动流程
	logger.Debug("初始化预算业务逻辑层")
	// 构造 BudgetBiz 实例并返回，所有依赖由外部注入
	return &BudgetBiz{logger: logger, repo: repo}
}

// Create 创建部门年度预算，校验同一部门同年不可重复
// 参数：deptID 部门ID，year 财年，annualBudget 年度预算总额（单位：分）
// 返回：创建成功的预算记录，或业务错误
func (b *BudgetBiz) Create(deptID uint, year int, annualBudget int64) (*model.DepartmentBudget, error) {
	// 日志：记录创建预算的入参，便于问题追溯和审计
	b.logger.Debug("开始创建预算记录",
		zap.Uint("部门ID", deptID),
		zap.Int("财年", year),
		zap.Int64("年度预算(分)", annualBudget),
	)

	// 业务校验1：同一部门同一财年不允许存在多条预算记录
	// 这是核心业务约束，防止预算数据重复导致资金管理混乱
	existing, _ := b.repo.GetByDepartmentAndYear(deptID, year)
	// 日志：查询已有记录结果
	b.logger.Debug("查询同部门同年预算记录完成",
		zap.Bool("是否存在", existing != nil),
		zap.Uint("部门ID", deptID),
		zap.Int("财年", year),
	)
	if existing != nil {
		// 若已存在记录，拒绝创建并返回明确的业务错误信息
		b.logger.Warn("预算记录已存在，创建失败",
			zap.Uint("部门ID", deptID),
			zap.Int("财年", year),
			zap.Uint("已有预算ID", existing.ID),
		)
		return nil, fmt.Errorf("该部门%d年度预算已存在", year)
	}

	// 校验通过后，构建新的预算数据模型
	// DepartmentBudget 的 AnnualBudget 字段以"分"为单位存储，避免浮点精度问题
	b.logger.Debug("开始构建预算数据模型",
		zap.Uint("部门ID", deptID),
		zap.Int("财年", year),
	)
	budget := &model.DepartmentBudget{
		DepartmentID: deptID,   // 所属部门
		FiscalYear:   year,     // 业务财年
		AnnualBudget: annualBudget, // 年度预算总额（单位：分）
	}

	// 将预算记录持久化到数据库
	// repo.Create 会在 GORM 写入后将自增 ID 回填到 budget 对象中
	b.logger.Debug("调用 repo 写入预算记录到数据库", zap.Any("预算对象", budget))
	if err := b.repo.Create(budget); err != nil {
		// 数据库写入失败，记录完整错误上下文并返回包装后的错误
		b.logger.Error("创建预算记录失败",
			zap.Uint("部门ID", deptID),
			zap.Int("财年", year),
			zap.Int64("年度预算(分)", annualBudget),
			zap.Error(err),
		)
		return nil, fmt.Errorf("创建预算记录失败: %w", err)
	}

	// 日志：记录创建成功事件，包含数据库生成的 ID 用于后续操作引用
	b.logger.Info("预算记录创建成功",
		zap.Uint("预算ID", budget.ID),
		zap.Uint("部门ID", deptID),
		zap.Int("财年", year),
	)
	return budget, nil
}

// GetByID 根据主键 ID 查询单条预算记录
// 参数：id 预算记录主键
// 返回：预算记录，或"记录不存在"错误
func (b *BudgetBiz) GetByID(id uint) (*model.DepartmentBudget, error) {
	// 日志：记录查询请求，记录目标 ID 用于调用链路追踪
	b.logger.Debug("查询预算记录", zap.Uint("预算ID", id))

	// 调用 repo 层查询数据库
	// repo.GetByID 底层使用 GORM First 方法，记录不存在时返回 gorm.ErrRecordNotFound
	b.logger.Debug("调用 repo 查询预算记录", zap.Uint("预算ID", id))
	budget, err := b.repo.GetByID(id)
	if err != nil {
		// 记录不存在时使用 Warn 级别（业务预期内的异常），而非 Error（系统异常）
		b.logger.Warn("预算记录不存在",
			zap.Uint("预算ID", id),
			zap.Error(err),
		)
		return nil, fmt.Errorf("预算记录不存在")
	}

	// 日志：记录查询成功，包含找到的预算基本信息便于调试
	b.logger.Debug("预算记录查询成功",
		zap.Uint("预算ID", budget.ID),
		zap.Uint("部门ID", budget.DepartmentID),
		zap.Int("财年", budget.FiscalYear),
	)
	return budget, nil
}

// GetDashboard 获取指定财年所有部门的预算看板数据
// 看板用于前端展示各部门预算使用概览：年度预算、已结算、已冻结、可用余额
// 参数：year 财年
// 返回：该财年所有部门的预算记录列表
func (b *BudgetBiz) GetDashboard(year int) ([]*model.DepartmentBudget, error) {
	// 日志：记录看板查询请求，财年是看板数据的关键筛选维度
	b.logger.Debug("获取预算看板数据", zap.Int("财年", year))

	// 按财年维度批量查询所有部门的预算记录
	// 不使用分页是因为一个公司的部门数量通常在百级以内，一次性加载性能可接受
	b.logger.Debug("调用 repo 按财年查询所有部门预算", zap.Int("财年", year))
	budgets, err := b.repo.ListByYear(year)
	if err != nil {
		// 数据库查询异常，记录完整错误信息用于排障
		b.logger.Error("获取预算看板数据失败",
			zap.Int("财年", year),
			zap.Error(err),
		)
		return nil, fmt.Errorf("获取预算看板数据失败: %w", err)
	}

	// 日志：记录查询成功，输出部门数量用于确认数据完整性
	b.logger.Info("预算看板数据获取成功",
		zap.Int("财年", year),
		zap.Int("部门数量", len(budgets)),
	)
	return budgets, nil
}

// CheckBudget 检查部门在当前财年的预算是否充足
// 核心业务逻辑：计算可用余额并判断是否需要触发特殊审批流程
//
// 参数：deptID 部门ID，amount 申请报销金额（单位：分）
// 返回：remaining 可用余额（分），needSpecialApproval 是否需要特殊审批，err 错误信息
func (b *BudgetBiz) CheckBudget(deptID uint, amount int64) (remaining int64, needSpecialApproval bool, err error) {
	// 日志：记录预算检查请求，包含申请金额用于后续审计比对
	b.logger.Debug("检查预算可用性",
		zap.Uint("部门ID", deptID),
		zap.Int64("申请金额(分)", amount),
	)

	// 步骤1：获取当前财年的部门预算记录
	// 使用 currentFiscalYear() 确保始终基于当前年份做预算检查
	year := currentFiscalYear()
	b.logger.Debug("获取当前财年预算记录",
		zap.Uint("部门ID", deptID),
		zap.Int("财年", year),
	)
	budget, err := b.repo.GetByDepartmentAndYear(deptID, year)
	if err != nil {
		// 若该部门在当前财年未设置预算，则无法进行任何报销操作
		b.logger.Warn("未找到部门预算记录",
			zap.Uint("部门ID", deptID),
			zap.Int("财年", year),
			zap.Error(err),
		)
		return 0, false, fmt.Errorf("该部门未设置%d年度预算", year)
	}

	// 步骤2：计算可用余额
	// 公式：可用余额 = 年度预算总额 - 已结算金额 - 已冻结金额
	// - SpentAmount：已审批通过并完成扣减的金额
	// - FrozenAmount：已提交申请但尚未审批完成的金额（预占）
	b.logger.Debug("开始计算可用余额",
		zap.Int64("年度预算", budget.AnnualBudget),
		zap.Int64("已结算", budget.SpentAmount),
		zap.Int64("已冻结", budget.FrozenAmount),
	)
	remaining = budget.AnnualBudget - budget.SpentAmount - budget.FrozenAmount

	// 日志：输出计算结果，所有金额字段统一使用分为单位
	b.logger.Debug("预算检查计算完成",
		zap.Int64("年度预算", budget.AnnualBudget),
		zap.Int64("已结算", budget.SpentAmount),
		zap.Int64("已冻结", budget.FrozenAmount),
		zap.Int64("可用余额", remaining),
		zap.Int64("申请金额", amount),
	)

	// 步骤3：判断是否需要触发特殊审批
	// 当申请金额超过可用余额时，超出预算部分需要走特殊审批流程
	// 这不是硬性拒绝，而是标记给后续审批环节做决策依据
	if amount > remaining {
		// 日志：超额申请记录 warn 级别，用于异常检测和审计
		b.logger.Warn("预算不足，触发特殊审批标记",
			zap.Int64("申请金额", amount),
			zap.Int64("可用余额", remaining),
			zap.Int64("差额", amount-remaining),
			zap.Uint("部门ID", deptID),
		)
		needSpecialApproval = true
	} else {
		// 预算充足，无需特殊审批
		b.logger.Debug("预算充足，无需特殊审批",
			zap.Int64("申请金额", amount),
			zap.Int64("可用余额", remaining),
		)
	}

	return remaining, needSpecialApproval, nil
}

// Freeze 冻结预算金额（提交报销申请时调用）
// 冻结是一种预占机制：报销提交后先将金额标记为冻结，防止其他报销申请占用同一笔预算
// 后续审批通过时调用 Deduct 转为正式扣减，驳回时调用 Unfreeze 释放冻结金额
//
// 参数：deptID 部门ID，amount 冻结金额（单位：分）
// 返回：操作错误（成功时为 nil）
func (b *BudgetBiz) Freeze(deptID uint, amount int64) error {
	// 日志：记录冻结请求入参
	b.logger.Debug("开始冻结预算操作",
		zap.Uint("部门ID", deptID),
		zap.Int64("冻结金额(分)", amount),
	)

	// 业务校验1：冻结金额必须为正数
	// 负数或零冻结没有业务意义，零冻结可能由调用方逻辑错误导致
	if amount <= 0 {
		b.logger.Warn("冻结金额无效，拒绝冻结操作",
			zap.Int64("金额", amount),
			zap.Uint("部门ID", deptID),
		)
		return fmt.Errorf("冻结金额必须大于零")
	}

	// 获取当前财年用于定位预算记录
	// 预算冻结始终作用在当前财年，不支持跨财年操作
	year := currentFiscalYear()
	b.logger.Debug("定位当前财年预算记录",
		zap.Uint("部门ID", deptID),
		zap.Int("财年", year),
	)

	// 调用 repo 层执行原子冻结操作
	// repo.Freeze 内部使用数据库原子更新（UPDATE SET frozen_amount = frozen_amount + ?）
	// 确保并发安全：多个报销申请同时提交时不会出现冻结金额覆盖问题
	b.logger.Debug("调用 repo 执行原子冻结",
		zap.Uint("部门ID", deptID),
		zap.Int("财年", year),
		zap.Int64("冻结金额(分)", amount),
	)
	if err := b.repo.Freeze(deptID, year, amount); err != nil {
		// 冻结失败可能是数据库连接问题或行锁竞争超时
		b.logger.Error("冻结预算失败",
			zap.Uint("部门ID", deptID),
			zap.Int("财年", year),
			zap.Int64("冻结金额(分)", amount),
			zap.Error(err),
		)
		return fmt.Errorf("冻结预算失败: %w", err)
	}

	// 日志：记录冻结成功事件，Info 级别用于业务操作审计
	b.logger.Info("预算冻结成功",
		zap.Uint("部门ID", deptID),
		zap.Int("财年", year),
		zap.Int64("冻结金额(分)", amount),
	)
	return nil
}

// Deduct 扣减预算（审批通过后调用）
// 将冻结金额正式转为已结算，完成预算的实际消耗
// 调用前提：对应的金额必须先经过 Freeze 冻结，否则会导致 froze_amount 与 spent_amount 不一致
//
// 参数：deptID 部门ID，amount 扣减金额（单位：分）
// 返回：操作错误（成功时为 nil）
func (b *BudgetBiz) Deduct(deptID uint, amount int64) error {
	// 日志：记录扣减请求入参
	b.logger.Debug("开始扣减预算操作",
		zap.Uint("部门ID", deptID),
		zap.Int64("扣减金额(分)", amount),
	)

	// 业务校验1：扣减金额必须为正数
	// 负数扣减无业务含义，应在前端/调用方确保传入正值
	if amount <= 0 {
		b.logger.Warn("扣减金额无效，拒绝扣减操作",
			zap.Int64("金额", amount),
			zap.Uint("部门ID", deptID),
		)
		return fmt.Errorf("扣减金额必须大于零")
	}

	// 获取当前财年用于定位预算记录
	// Deduct 操作仅对当前财年有效，确保预算时效性
	year := currentFiscalYear()
	b.logger.Debug("定位当前财年预算记录",
		zap.Uint("部门ID", deptID),
		zap.Int("财年", year),
	)

	// 调用 repo 层执行原子扣减操作
	// repo.Deduct 内部同时更新 frozen_amount（减少）和 spent_amount（增加）
	// 使用数据库原子操作保证冻结→结算的状态转换一致性
	b.logger.Debug("调用 repo 执行原子扣减（冻结→结算）",
		zap.Uint("部门ID", deptID),
		zap.Int("财年", year),
		zap.Int64("扣减金额(分)", amount),
	)
	if err := b.repo.Deduct(deptID, year, amount); err != nil {
		// 扣减失败需保留冻结状态，由调用方决定重试或告警
		b.logger.Error("扣减预算失败",
			zap.Uint("部门ID", deptID),
			zap.Int("财年", year),
			zap.Int64("扣减金额(分)", amount),
			zap.Error(err),
		)
		return fmt.Errorf("扣减预算失败: %w", err)
	}

	// 日志：记录扣减成功事件，Info 级别用于业务操作审计
	b.logger.Info("预算扣减成功",
		zap.Uint("部门ID", deptID),
		zap.Int("财年", year),
		zap.Int64("扣减金额(分)", amount),
	)
	return nil
}

// Unfreeze 解冻预算（报销申请被驳回或撤销时调用）
// 将之前冻结的金额释放回可用余额，恢复预算的可用性
// 调用前提：对应的金额必须先经过 Freeze 冻结，否则会导致 frozen_amount 变成负值
//
// 参数：deptID 部门ID，amount 解冻金额（单位：分）
// 返回：操作错误（成功时为 nil）
func (b *BudgetBiz) Unfreeze(deptID uint, amount int64) error {
	// 日志：记录解冻请求入参
	b.logger.Debug("开始解冻预算操作",
		zap.Uint("部门ID", deptID),
		zap.Int64("解冻金额(分)", amount),
	)

	// 业务校验1：解冻金额必须为正数
	// 解冻负数意味着冻结更多金额，这在业务上没有意义
	if amount <= 0 {
		b.logger.Warn("解冻金额无效，拒绝解冻操作",
			zap.Int64("金额", amount),
			zap.Uint("部门ID", deptID),
		)
		return fmt.Errorf("解冻金额必须大于零")
	}

	// 获取当前财年用于定位预算记录
	// 解冻仅对当前财年的预算有效，跨年预算不可追溯解冻
	year := currentFiscalYear()
	b.logger.Debug("定位当前财年预算记录",
		zap.Uint("部门ID", deptID),
		zap.Int("财年", year),
	)

	// 调用 repo 层执行原子解冻操作
	// repo.Unfreeze 内部通过 UPDATE SET frozen_amount = frozen_amount - ? WHERE ... AND frozen_amount >= ?
	// 同时校验 frozen_amount 不能小于解冻金额，防止解冻超过冻结量的异常情况
	b.logger.Debug("调用 repo 执行原子解冻",
		zap.Uint("部门ID", deptID),
		zap.Int("财年", year),
		zap.Int64("解冻金额(分)", amount),
	)
	if err := b.repo.Unfreeze(deptID, year, amount); err != nil {
		// 解冻失败可能原因：数据库异常，或解冻金额超出已冻结金额
		b.logger.Error("解冻预算失败",
			zap.Uint("部门ID", deptID),
			zap.Int("财年", year),
			zap.Int64("解冻金额(分)", amount),
			zap.Error(err),
		)
		return fmt.Errorf("解冻预算失败: %w", err)
	}

	// 日志：记录解冻成功事件，Info 级别用于业务操作审计
	b.logger.Info("预算解冻成功",
		zap.Uint("部门ID", deptID),
		zap.Int("财年", year),
		zap.Int64("解冻金额(分)", amount),
	)
	return nil
}

// Update 更新预算记录的年度预算金额
// 仅允许修改 AnnualBudget 字段，其他字段（已结算/已冻结）由 Freeze/Deduct/Unfreeze 操作自动维护
// 这是一种部分更新策略：先获取完整记录，再修改目标字段，最后全量写回
//
// 参数：id 预算记录主键，annualBudget 新的年度预算总额（单位：分）
// 返回：更新后的完整预算记录，或错误信息
func (b *BudgetBiz) Update(id uint, annualBudget int64) (*model.DepartmentBudget, error) {
	// 日志：记录更新请求入参
	b.logger.Debug("开始更新预算记录",
		zap.Uint("预算ID", id),
		zap.Int64("新年预算(分)", annualBudget),
	)

	// 步骤1：先查询现有记录以确认记录存在
	// 先查后改的模式确保不会对不存在的记录执行无意义的 UPDATE
	b.logger.Debug("查询现有预算记录",
		zap.Uint("预算ID", id),
	)
	budget, err := b.repo.GetByID(id)
	if err != nil {
		// 目标记录不存在，无法更新
		b.logger.Warn("要更新的预算记录不存在",
			zap.Uint("预算ID", id),
			zap.Error(err),
		)
		return nil, fmt.Errorf("预算记录不存在")
	}
	b.logger.Debug("现有预算记录查询成功",
		zap.Uint("预算ID", budget.ID),
		zap.Int64("原年度预算(分)", budget.AnnualBudget),
		zap.Int64("已结算(分)", budget.SpentAmount),
		zap.Int64("已冻结(分)", budget.FrozenAmount),
	)

	// 步骤2：修改目标字段
	// 仅更新 AnnualBudget，保留已结算和已冻结金额不变
	b.logger.Debug("修改年度预算金额",
		zap.Int64("旧预算(分)", budget.AnnualBudget),
		zap.Int64("新预算(分)", annualBudget),
	)
	budget.AnnualBudget = annualBudget

	// 步骤3：将修改后的对象全量写回数据库
	// repo.Update 使用 GORM Save 方法，按主键匹配进行更新
	b.logger.Debug("调用 repo 持久化更新", zap.Any("更新后预算对象", budget))
	if err := b.repo.Update(budget); err != nil {
		// 数据库更新失败，记录完整错误上下文
		b.logger.Error("更新预算记录失败",
			zap.Uint("预算ID", id),
			zap.Int64("新年预算(分)", annualBudget),
			zap.Error(err),
		)
		return nil, fmt.Errorf("更新预算记录失败: %w", err)
	}

	// 日志：记录更新成功事件
	b.logger.Info("预算记录更新成功",
		zap.Uint("预算ID", id),
		zap.Int64("旧年度预算(分)", budget.AnnualBudget),
		zap.Int64("新年度预算(分)", annualBudget),
	)
	return budget, nil
}
