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
	// 记录业务层初始化，便于追踪模块加载顺序
	logger.Debug("初始化部门业务逻辑层")
	// 将日志器和数据仓库注入到业务实例中，供后续所有方法使用
	return &DepartmentBiz{logger: logger, repo: repo}
}

// Create 创建部门，校验部门名称唯一性
func (b *DepartmentBiz) Create(name string, managerID *uint) (*model.Department, error) {
	// 记录入口参数，方便追踪完整的创建链路
	b.logger.Debug("开始创建部门", zap.String("部门名称", name))

	// 步骤1：校验部门名称唯一性 —— 防止重名导致后续业务混淆
	existing, err := b.repo.GetByName(name)
	// 如果查询没有报错且确实查到了记录，说明名称已被占用
	if err == nil && existing != nil {
		b.logger.Warn("部门名称已存在，创建失败", zap.String("部门名称", name))
		// 返回明确的业务错误，方便上层展示给用户
		return nil, fmt.Errorf("部门名称'%s'已存在", name)
	}
	// 如果 err != nil 且 existing == nil，说明是"记录不存在"的正常情况（GORM 在找不到记录时返回 error）
	// 原逻辑对此不作处理，直接进入后续写入流程
	// 名称唯一性校验通过（未发现同名记录），记录调试日志
	b.logger.Debug("部门名称唯一性校验通过", zap.String("部门名称", name))

	// 步骤2：如果指定了主管，校验主管是否存在且是该部门成员
	if managerID != nil {
		// 记录主管校验开始，便于定位主管相关业务的审批链问题
		b.logger.Debug("校验部门主管", zap.Uint("主管ID", *managerID))
		// TODO: 此处应补充对 managerID 对应员工的校验逻辑：
		//       1. 查询 managerID 是否存在
		//       2. 校验该员工是否已经属于本部门或尚未分配部门
		// 当前暂不校验，仅记录日志，后续需完善
		b.logger.Debug("主管校验已跳过（待实现完整校验逻辑）", zap.Uint("主管ID", *managerID))
	}

	// 步骤3：构建部门模型并写入数据库
	dept := &model.Department{
		Name:      name,
		ManagerID: managerID,
	}
	b.logger.Debug("准备写入部门记录", zap.String("部门名称", name))
	// 调用数据仓储层执行实际的 INSERT 操作
	if err := b.repo.Create(dept); err != nil {
		b.logger.Error("创建部门失败", zap.String("部门名称", name), zap.Error(err))
		// 使用 %w 包装错误，保留原始错误链供上层判断
		return nil, fmt.Errorf("创建部门失败: %w", err)
	}
	// 写入成功后，dept.ID 已被 GORM 回填为自增主键值
	b.logger.Debug("部门记录已写入数据库", zap.Uint("部门ID", dept.ID))

	// 记录最终成功日志，包含关键标识字段
	b.logger.Info("部门创建成功", zap.Uint("部门ID", dept.ID), zap.String("部门名称", dept.Name))
	return dept, nil
}

// GetByID 根据 ID 查询部门
func (b *DepartmentBiz) GetByID(id uint) (*model.Department, error) {
	// 记录入口参数，方便追踪查询请求
	b.logger.Debug("查询部门", zap.Uint("部门ID", id))

	// 调用仓储层执行主键查询 —— GORM First 方法在记录不存在时会返回 error
	dept, err := b.repo.GetByID(id)
	if err != nil {
		// 记录不存在属于业务层的 Warn 级别，不需要 Error 级别
		b.logger.Warn("部门不存在", zap.Uint("部门ID", id), zap.Error(err))
		// 返回统一的中文错误信息，隐藏底层实现细节
		return nil, fmt.Errorf("部门不存在")
	}
	// 查询成功，记录确认日志
	b.logger.Debug("查询部门成功", zap.Uint("部门ID", id), zap.String("部门名称", dept.Name))
	return dept, nil
}

// GetByName 根据名称查询部门
func (b *DepartmentBiz) GetByName(name string) (*model.Department, error) {
	// 记录入口参数，名称是唯一索引，查询效率高
	b.logger.Debug("根据名称查询部门", zap.String("部门名称", name))

	// 调用仓储层执行名称精确匹配查询
	dept, err := b.repo.GetByName(name)
	if err != nil {
		// 未找到记录时返回 Warn 级别日志
		b.logger.Warn("部门不存在", zap.String("部门名称", name), zap.Error(err))
		return nil, fmt.Errorf("部门不存在")
	}
	// 查询成功，记录确认日志
	b.logger.Debug("根据名称查询部门成功", zap.String("部门名称", name), zap.Uint("部门ID", dept.ID))
	return dept, nil
}

// SearchByName 模糊搜索部门（按名称，支持部分匹配如输入"计算机"可匹配"计算机科学与技术学院"）
func (b *DepartmentBiz) SearchByName(name string) ([]*model.Department, error) {
	b.logger.Debug("模糊搜索部门", zap.String("关键词", name))
	depts, err := b.repo.SearchByName(name)
	if err != nil {
		b.logger.Error("模糊搜索部门失败", zap.String("关键词", name), zap.Error(err))
		return nil, fmt.Errorf("搜索部门失败: %w", err)
	}
	b.logger.Info("模糊搜索部门成功", zap.String("关键词", name), zap.Int("匹配数", len(depts)))
	return depts, nil
}

// List 分页查询部门列表
func (b *DepartmentBiz) List(page, pageSize int) ([]*model.Department, int64, error) {
	// 记录分页参数，便于追踪性能（大页码可能导致慢查询）
	b.logger.Debug("查询部门列表", zap.Int("页码", page), zap.Int("每页数量", pageSize))

	// 调用仓储层执行分页查询 —— 一次 SQL 返回 COUNT + 分页数据
	depts, total, err := b.repo.List(page, pageSize)
	if err != nil {
		// 列表查询失败属于 Error 级别，可能表示数据库连接异常
		b.logger.Error("查询部门列表失败", zap.Error(err))
		// 返回零值和错误，上层根据 err 判断是否展示空列表或报错
		return nil, 0, fmt.Errorf("查询部门列表失败: %w", err)
	}
	// 查询成功，记录结果统计信息，方便监控数据量
	b.logger.Debug("查询部门列表成功", zap.Int64("总数", total), zap.Int("返回数量", len(depts)))
	return depts, total, nil
}

// Update 更新部门信息，校验名称唯一性
func (b *DepartmentBiz) Update(id uint, name string, managerID *uint) (*model.Department, error) {
	// 记录所有入参，包括可选参数，方便追踪更新操作的完整上下文
	b.logger.Debug("开始更新部门", zap.Uint("部门ID", id), zap.String("新名称", name))

	// 步骤1：先查询目标部门是否存在 —— 如果不存在，拒绝更新
	dept, err := b.repo.GetByID(id)
	if err != nil {
		// 更新不存在的部门属于业务逻辑错误，使用 Warn 级别
		b.logger.Warn("要更新的部门不存在", zap.Uint("部门ID", id))
		return nil, fmt.Errorf("部门不存在")
	}
	b.logger.Debug("目标部门已找到", zap.Uint("部门ID", id), zap.String("当前名称", dept.Name))

	// 步骤2：校验名称唯一性（排除自身）
	// 只有在用户尝试更换部门名称时才需要检查，不改名则跳过
	if name != dept.Name {
		b.logger.Debug("检测到部门名称变更，开始唯一性校验",
			zap.String("旧名称", dept.Name), zap.String("新名称", name))
		// 查询新名称是否已被其他部门占用
		existing, _ := b.repo.GetByName(name)
		// 如果查询到了记录，且该记录的 ID 与当前部门不同，说明名称冲突
		if existing != nil && existing.ID != id {
			b.logger.Warn("部门名称冲突", zap.String("部门名称", name))
			return nil, fmt.Errorf("部门名称'%s'已被其他部门使用", name)
		}
		// 名称唯一且未与其他部门冲突
		b.logger.Debug("部门名称唯一性校验通过", zap.String("部门名称", name))
	} else {
		b.logger.Debug("部门名称未变更，跳过唯一性校验", zap.String("部门名称", name))
	}

	// 步骤3：更新字段值
	dept.Name = name
	// managerID 为 nil 表示不修改主管字段；非 nil 时才更新
	if managerID != nil {
		b.logger.Debug("更新部门主管信息", zap.Uint("部门ID", id), zap.Uint("新主管ID", *managerID))
		dept.ManagerID = managerID
	} else {
		b.logger.Debug("未提供新主管信息，保留原值", zap.Uint("部门ID", id))
	}
	// 执行数据库 UPDATE 操作
	b.logger.Debug("准备更新部门记录", zap.Uint("部门ID", id))
	if err := b.repo.Update(dept); err != nil {
		b.logger.Error("更新部门失败", zap.Uint("部门ID", id), zap.Error(err))
		return nil, fmt.Errorf("更新部门失败: %w", err)
	}
	b.logger.Debug("部门记录已更新到数据库", zap.Uint("部门ID", id))

	// 记录最终成功日志
	b.logger.Info("部门更新成功", zap.Uint("部门ID", dept.ID), zap.String("部门名称", dept.Name))
	return dept, nil
}

// Delete 删除部门，校验无下属员工和预算记录
func (b *DepartmentBiz) Delete(id uint) error {
	// 记录删除操作的入口参数
	b.logger.Debug("开始删除部门", zap.Uint("部门ID", id))

	// 步骤1：先确认目标部门存在（在 repo.Delete 内部会处理）
	// 步骤2：调用仓储层执行删除 —— 仓储层负责检查外键约束（员工和预算）
	b.logger.Debug("准备调用仓储层删除部门", zap.Uint("部门ID", id))
	if err := b.repo.Delete(id); err != nil {
		// 业务约束拒绝 —— 部门下仍有员工或预算记录时，阻止删除
		if errors.Is(err, ErrDeptHasEmployees) || errors.Is(err, ErrDeptHasBudget) {
			// 业务约束属于正常流程拒绝，使用 Warn 级别
			b.logger.Warn("部门删除被拒绝", zap.Uint("部门ID", id), zap.Error(err))
			return err
		}
		// 其他数据库层面的异常属于 Error 级别
		b.logger.Error("删除部门失败", zap.Uint("部门ID", id), zap.Error(err))
		return fmt.Errorf("删除部门失败: %w", err)
	}
	// 删除成功 —— 记录确认日志
	b.logger.Debug("部门记录已从数据库删除", zap.Uint("部门ID", id))

	// 记录最终成功日志，辅助审计追踪
	b.logger.Info("部门删除成功", zap.Uint("部门ID", id))
	return nil
}
