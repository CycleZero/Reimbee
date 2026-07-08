package reimbursement

import (
	"fmt"
	"sync/atomic"
	"time"

	"github.com/CycleZero/Reimbee/internal/domain/approval"
	"github.com/CycleZero/Reimbee/internal/domain/budget"
	"github.com/CycleZero/Reimbee/internal/domain/employee"
	"github.com/CycleZero/Reimbee/log"
	"github.com/CycleZero/Reimbee/model"
	"go.uber.org/zap"
)

// 报销单流水号计数器（全局原子递增，保证并发安全）
// 原子操作避免多协程同时创建报销单时的竞态条件，确保流水号唯一
var (
	reimbursementSeq uint64
	currentYear      = time.Now().Year()
)

// generateReimbursementNo 生成报销单号 REIMB-YYYY-NNNN
// 使用原子递增保证并发场景下流水号不重复
func generateReimbursementNo() string {
	// 原子自增流水号 —— 多个请求同时创建报销单时不会出现编号冲突
	seq := atomic.AddUint64(&reimbursementSeq, 1)
	// 组装报销单号：固定前缀 + 年份 + 四位流水号，便于人工识别和检索
	reimbNo := fmt.Sprintf("REIMB-%d-%04d", currentYear, seq)
	return reimbNo
}

// 报销单状态常量（引用 model 统一定义）
// 不直接使用字符串，避免拼写错误；统一引用 model 包保证全局一致性
const (
	StatusDraft     = model.ReimbStatusDraft     // 草稿：仅创建者可见，可编辑
	StatusPending   = model.ReimbStatusPending   // 待审批：已提交，等待审批人处理
	StatusReviewing = model.ReimbStatusReviewing // 审批中：至少一位审批人已处理
	StatusApproved  = model.ReimbStatusApproved  // 已通过：所有审批人通过，预算已扣减
	StatusRejected  = model.ReimbStatusRejected  // 已驳回：审批人驳回，预算已解冻
)

// ReimbursementBiz 报销单业务逻辑层，负责报销单生命周期管理和跨域编排
// 协调 repo（数据持久化）、budgetBiz（预算冻结/扣减/解冻）、approvalBiz（审批链创建/审批）、
// employeeBiz（审批人列表）四个领域模块，实现完整的报销单状态机
type ReimbursementBiz struct {
	logger      *log.Logger          // 结构化日志器，支持 Debug/Info/Warn/Error 级别
	repo        *ReimbursementRepo   // 报销单数据访问层
	budgetBiz   *budget.BudgetBiz    // 预算业务层，负责预算检查、冻结、扣减、解冻的原子操作
	approvalBiz *approval.ApprovalBiz // 审批业务层，负责审批链创建和逐人审批
	employeeBiz *employee.EmployeeBiz // 员工业务层，负责获取审批人列表等员工相关操作
}

// NewReimbursementBiz 创建报销单业务逻辑层实例
// 通过依赖注入接收所有协作模块，遵循 DDD 分层架构，biz 层不直接依赖 HTTP 或 DB
func NewReimbursementBiz(
	logger *log.Logger,
	repo *ReimbursementRepo,
	budgetBiz *budget.BudgetBiz,
	approvalBiz *approval.ApprovalBiz,
	employeeBiz *employee.EmployeeBiz,
) *ReimbursementBiz {
	// 记录初始化日志 —— 便于排查启动时 Wire 注入是否正确
	logger.Debug("初始化报销单业务逻辑层（含审批链编排）")
	// 组装结构体：注入下层依赖，后续方法通过 b.xxx 调用各模块能力
	return &ReimbursementBiz{
		logger:      logger,
		repo:        repo,
		budgetBiz:   budgetBiz,
		approvalBiz: approvalBiz,
		employeeBiz: employeeBiz,
	}
}

// Create 创建报销单（草稿状态）
// 只做基础数据落库，不涉及预算检查或审批流；创建后状态为 Draft，可继续编辑
func (b *ReimbursementBiz) Create(employeeID, employeeName string, deptID uint, submitNote string) (*model.Reimbursement, error) {
	b.logger.Debug("开始创建报销单", zap.String("工号", employeeID), zap.String("姓名", employeeName), zap.Uint("部门ID", deptID))

	// 构建报销单模型 —— 填充业务字段，状态初始化为草稿
	rm := &model.Reimbursement{
		ReimbursementNo: generateReimbursementNo(), // 生成全局唯一报销单号，便于后续检索和展示
		EmployeeID:      employeeID,                // 关联员工工号，用于按员工筛选报销单
		EmployeeName:    employeeName,              // 冗余存储姓名，避免每次查询关联员工表
		DepartmentID:    deptID,                    // 关联部门，预算检查时需要根据部门查询剩余预算
		Status:          StatusDraft,               // 初始状态：草稿，后续通过 Submit 流转到待审批
		SubmitNote:      submitNote,                // 提交备注，记录报销事由等补充信息
	}
	b.logger.Debug("报销单模型构建完成，准备持久化", zap.String("报销单号", rm.ReimbursementNo))

	// 持久化到数据库 —— repo 层负责 GORM 的 INSERT 操作
	if err := b.repo.Create(rm); err != nil {
		b.logger.Error("创建报销单失败", zap.String("工号", employeeID), zap.String("报销单号", rm.ReimbursementNo), zap.Error(err))
		return nil, fmt.Errorf("创建报销单失败: %w", err)
	}
	// 持久化成功后，GORM 会回填 rm.ID，后续可用 ID 操作报销单

	b.logger.Info("报销单创建成功（草稿）", zap.String("报销单号", rm.ReimbursementNo), zap.Uint("ID", rm.ID))
	return rm, nil
}

// Submit 提交报销单（草稿 → 待审批）
// 执行顺序：校验状态 → 校验金额 → 预算检查 → 冻结预算 → 创建审批链 → 更新状态
// 若中间步骤失败，已执行的前置操作（预算冻结）将被回滚解冻
func (b *ReimbursementBiz) Submit(id uint, totalAmount int64) (*model.Reimbursement, error) {
	b.logger.Debug("开始提交报销单", zap.Uint("报销单ID", id), zap.Int64("总金额(分)", totalAmount))

	// ===== 步骤 1：获取报销单并校验状态 =====
	// 必须先从数据库查出完整报销单对象，后续操作依赖 rm 的部门ID、当前状态等字段
	rm, err := b.repo.GetByID(id)
	if err != nil {
		// 数据库中不存在该报销单 —— 可能是非法 ID 或已被物理删除
		b.logger.Warn("报销单不存在", zap.Uint("报销单ID", id))
		return nil, fmt.Errorf("报销单不存在")
	}
	b.logger.Debug("报销单已查询到", zap.Uint("报销单ID", id), zap.String("当前状态", rm.Status), zap.String("报销单号", rm.ReimbursementNo))

	// 状态守卫：只有草稿或已驳回的报销单可以提交
	// 已通过的报销单不允许重复提交；审批中的不允许直接提交（应走审批流程）
	if rm.Status != StatusDraft && rm.Status != StatusRejected {
		b.logger.Warn("报销单状态不允许提交", zap.Uint("报销单ID", id), zap.String("当前状态", rm.Status))
		return nil, fmt.Errorf("当前状态为'%s'，只有草稿或已驳回的报销单可以提交", rm.Status)
	}
	b.logger.Debug("报销单状态校验通过", zap.Uint("报销单ID", id), zap.String("当前状态", rm.Status))

	// ===== 步骤 2：校验金额 =====
	// 金额必须大于零，防止无效或恶意提交零元/负元报销单
	if totalAmount <= 0 {
		b.logger.Warn("报销金额无效", zap.Int64("金额", totalAmount))
		return nil, fmt.Errorf("报销金额必须大于零")
	}
	b.logger.Debug("报销金额校验通过", zap.Int64("金额(分)", totalAmount))

	// ===== 步骤 3：预算检查 =====
	// 调用预算模块检查部门剩余预算是否充足，同时返回是否需要特殊审批（预算不足时标记）
	b.logger.Debug("开始预算检查", zap.Uint("部门ID", rm.DepartmentID), zap.Int64("金额(分)", totalAmount))
	_, needSpecial, err := b.budgetBiz.CheckBudget(rm.DepartmentID, totalAmount)
	if err != nil {
		// 预算检查失败 —— 可能是预算模块异常或数据不一致
		b.logger.Error("预算检查失败", zap.Uint("部门ID", rm.DepartmentID), zap.Int64("金额(分)", totalAmount), zap.Error(err))
		return nil, fmt.Errorf("预算检查失败: %w", err)
	}
	b.logger.Debug("预算检查完成", zap.Uint("部门ID", rm.DepartmentID), zap.Bool("需要特殊审批", needSpecial))

	// ===== 步骤 4：冻结预算 =====
	// 预算冻结是原子操作，占住额度防止超支；若后续步骤失败需要手工解冻（Unfreeze）
	b.logger.Debug("开始冻结预算", zap.Uint("部门ID", rm.DepartmentID), zap.Int64("金额(分)", totalAmount))
	if err := b.budgetBiz.Freeze(rm.DepartmentID, totalAmount); err != nil {
		// 冻结失败 —— 可能并发导致预算不足或 budget 模块异常
		b.logger.Error("冻结预算失败", zap.Uint("部门ID", rm.DepartmentID), zap.Int64("金额(分)", totalAmount), zap.Error(err))
		return nil, fmt.Errorf("冻结预算失败: %w", err)
	}
	b.logger.Debug("预算冻结成功", zap.Uint("部门ID", rm.DepartmentID), zap.Int64("金额(分)", totalAmount))

	// ===== 步骤 5：创建审批链 =====
	// 先获取系统中所有可审批人员列表，再为每个审批人创建一条待审批记录
	b.logger.Debug("开始获取审批人列表")
	approvers, err := b.employeeBiz.ListApprovers()
	if err != nil {
		// 获取审批人失败 —— 回滚已冻结的预算，保证数据一致性
		b.logger.Error("获取审批人列表失败，回滚预算冻结", zap.Error(err))
		b.budgetBiz.Unfreeze(rm.DepartmentID, totalAmount) // 回滚冻结的预算额度
		return nil, fmt.Errorf("获取审批人列表失败: %w", err)
	}
	b.logger.Debug("审批人列表获取成功", zap.Int("审批人数", len(approvers)))

	// 如果没有审批人，无法完成审批流程，需解冻预算并返回错误
	if len(approvers) == 0 {
		b.logger.Warn("系统中没有审批人，回滚预算冻结", zap.Uint("报销单ID", id))
		b.budgetBiz.Unfreeze(rm.DepartmentID, totalAmount) // 回滚冻结的预算额度
		return nil, fmt.Errorf("系统中没有审批人，无法提交报销单")
	}

	// 创建审批链 —— 为每位审批人在 approval 表中插入一条待审批记录
	b.logger.Debug("开始创建审批链", zap.Uint("报销单ID", id), zap.Int("审批人数", len(approvers)))
	if err := b.approvalBiz.CreateApprovalChain(rm.ID, approvers); err != nil {
		// 审批链创建失败 —— 回滚预算冻结，审批记录不存在则无需清理
		b.logger.Error("创建审批链失败，回滚预算冻结", zap.Uint("报销单ID", id), zap.Error(err))
		b.budgetBiz.Unfreeze(rm.DepartmentID, totalAmount) // 回滚冻结的预算额度
		return nil, fmt.Errorf("创建审批链失败: %w", err)
	}
	b.logger.Debug("审批链创建成功", zap.Uint("报销单ID", id))

	// ===== 步骤 6：更新报销单状态为待审批 =====
	// 所有前置操作成功后，才将报销单状态改为 Pending，正式进入审批流程
	rm.TotalAmount = totalAmount     // 记录报销总金额（数据库存储分为单位）
	rm.Status = StatusPending         // 状态流转：草稿/已驳回 → 待审批
	rm.NeedSpecialApproval = needSpecial // 标记是否需要特殊审批（预算不足时触发）

	// 若预算不足，需要特殊审批流程 —— 记录警告日志便于追踪
	if needSpecial {
		b.logger.Warn("报销单需要特殊审批（预算不足）", zap.Uint("报销单ID", id), zap.Uint("部门ID", rm.DepartmentID))
	}

	b.logger.Debug("开始更新报销单状态为待审批", zap.Uint("报销单ID", id), zap.String("目标状态", StatusPending))
	if err := b.repo.Update(rm); err != nil {
		// 状态更新失败 —— 回滚预算冻结，审批链记录保留（下次提交时可复用或人工清理）
		b.logger.Error("更新报销单状态失败，回滚预算冻结", zap.Uint("报销单ID", id), zap.Error(err))
		b.budgetBiz.Unfreeze(rm.DepartmentID, totalAmount)
		return nil, fmt.Errorf("提交报销单失败: %w", err)
	}

	// 提交成功 —— 记录完整上下文信息，便于审计和问题排查
	b.logger.Info("报销单提交成功，审批链已创建",
		zap.String("报销单号", rm.ReimbursementNo),
		zap.Int64("金额(分)", totalAmount),
		zap.Int("审批人数", len(approvers)),
		zap.Bool("需要特殊审批", needSpecial),
	)
	return rm, nil
}

// Approve 审批通过报销单（强制模式，跳过逐人审批）
// 适用于模拟审批场景；生产环境应使用 ApprovalBiz.Approve() 逐人审批以保证审批链完整性
func (b *ReimbursementBiz) Approve(id uint) (*model.Reimbursement, error) {
	b.logger.Debug("审批通过报销单（强制模式）", zap.Uint("报销单ID", id))

	// 获取报销单 —— 需要当前状态来判断能否审批，以及获取部门ID用于预算扣减
	rm, err := b.repo.GetByID(id)
	if err != nil {
		b.logger.Warn("报销单不存在，无法审批", zap.Uint("报销单ID", id))
		return nil, fmt.Errorf("报销单不存在")
	}
	b.logger.Debug("报销单已查询到", zap.Uint("报销单ID", id), zap.String("当前状态", rm.Status), zap.String("报销单号", rm.ReimbursementNo))

	// 状态守卫：只有待审批或审批中的报销单可以审批通过
	// 草稿状态应先提交再审批；已通过的不可重复审批；已驳回的需重新提交
	if rm.Status != StatusPending && rm.Status != StatusReviewing {
		b.logger.Warn("报销单状态不允许审批", zap.Uint("报销单ID", id), zap.String("当前状态", rm.Status))
		return nil, fmt.Errorf("当前状态为'%s'，不可审批", rm.Status)
	}
	b.logger.Debug("报销单状态校验通过，准备执行审批操作", zap.Uint("报销单ID", id))

	// 遍历所有审批记录，将状态为"待审批"的逐一标记为通过
	// 强制模式下不区分审批顺序，一次性全部通过
	b.logger.Debug("开始逐条更新审批记录", zap.Uint("报销单ID", id), zap.Int("审批记录总数", len(rm.Approvals)))
	for _, a := range rm.Approvals {
		// 只处理尚未处理的审批记录 —— 已通过/已驳回的跳过
		if a.Action == model.ApprovalActionPending {
			b.logger.Debug("更新审批记录为通过", zap.Uint("审批记录ID", a.ID))
			if err := b.approvalBiz.Approve(a.ID, "系统自动审批"); err != nil {
				b.logger.Error("更新审批记录失败", zap.Uint("审批记录ID", a.ID), zap.Uint("报销单ID", id), zap.Error(err))
				return nil, fmt.Errorf("更新审批记录失败: %w", err)
			}
		}
	}
	b.logger.Debug("所有审批记录处理完成", zap.Uint("报销单ID", id))

	// 扣减预算 —— 将冻结的预算转为实际支出，完成预算的最终扣减
	b.logger.Debug("开始扣减预算", zap.Uint("部门ID", rm.DepartmentID), zap.Int64("金额(分)", rm.TotalAmount))
	if err := b.budgetBiz.Deduct(rm.DepartmentID, rm.TotalAmount); err != nil {
		// 扣减失败 —— 预算模块异常，此时审批记录已更新，需人工介入处理
		b.logger.Error("扣减预算失败", zap.Uint("部门ID", rm.DepartmentID), zap.Int64("金额(分)", rm.TotalAmount), zap.Error(err))
		return nil, fmt.Errorf("扣减预算失败: %w", err)
	}
	b.logger.Debug("预算扣减成功", zap.Uint("部门ID", rm.DepartmentID), zap.Int64("金额(分)", rm.TotalAmount))

	// 更新报销单状态为已通过 —— 最后一步，标志着报销流程完结
	rm.Status = StatusApproved
	b.logger.Debug("更新报销单状态为已通过", zap.Uint("报销单ID", id))
	if err := b.repo.Update(rm); err != nil {
		// 状态更新失败 —— 预算已扣减，审批记录已更新，需人工核对数据一致性
		b.logger.Error("更新报销单状态失败", zap.Uint("报销单ID", id), zap.Error(err))
		return nil, fmt.Errorf("审批通过操作失败: %w", err)
	}

	b.logger.Info("报销单已通过", zap.String("报销单号", rm.ReimbursementNo), zap.Int64("金额(分)", rm.TotalAmount))
	return rm, nil
}

// Reject 驳回报销单（强制驳回，解冻预算）
// 驳回后预算自动释放，报销单状态回到 Rejected，申请人可修改后重新提交
func (b *ReimbursementBiz) Reject(id uint, reason string) (*model.Reimbursement, error) {
	b.logger.Debug("驳回报销单", zap.Uint("报销单ID", id), zap.String("驳回原因", reason))

	// 获取报销单 —— 需要校验当前状态并获取部门ID、金额用于解冻预算
	rm, err := b.repo.GetByID(id)
	if err != nil {
		b.logger.Warn("报销单不存在，无法驳回", zap.Uint("报销单ID", id))
		return nil, fmt.Errorf("报销单不存在")
	}
	b.logger.Debug("报销单已查询到，准备执行驳回", zap.Uint("报销单ID", id), zap.String("当前状态", rm.Status), zap.String("报销单号", rm.ReimbursementNo))

	// 状态守卫：只有待审批或审批中的报销单可以驳回
	// 草稿状态的无需驳回；已通过的不应驳回（应走退款流程）；已驳回的不可重复驳回
	if rm.Status != StatusPending && rm.Status != StatusReviewing {
		b.logger.Warn("报销单状态不允许驳回", zap.Uint("报销单ID", id), zap.String("当前状态", rm.Status))
		return nil, fmt.Errorf("当前状态为'%s'，不可驳回", rm.Status)
	}
	b.logger.Debug("报销单状态校验通过", zap.Uint("报销单ID", id))

	// 解冻预算 —— 释放之前冻结的额度，让部门预算恢复可用
	b.logger.Debug("开始解冻预算", zap.Uint("部门ID", rm.DepartmentID), zap.Int64("金额(分)", rm.TotalAmount))
	if err := b.budgetBiz.Unfreeze(rm.DepartmentID, rm.TotalAmount); err != nil {
		// 解冻失败 —— 预算状态不一致，可能需人工介入
		b.logger.Error("解冻预算失败", zap.Uint("部门ID", rm.DepartmentID), zap.Int64("金额(分)", rm.TotalAmount), zap.Error(err))
		return nil, fmt.Errorf("解冻预算失败: %w", err)
	}
	b.logger.Debug("预算解冻成功", zap.Uint("部门ID", rm.DepartmentID), zap.Int64("金额(分)", rm.TotalAmount))

	// 更新报销单状态为已驳回 —— 驳回后申请人可查看原因并修改后重新提交
	rm.Status = StatusRejected
	b.logger.Debug("更新报销单状态为已驳回", zap.Uint("报销单ID", id))
	if err := b.repo.Update(rm); err != nil {
		// 状态更新失败 —— 预算已解冻，状态未变更，需人工核对
		b.logger.Error("更新报销单状态失败", zap.Uint("报销单ID", id), zap.Error(err))
		return nil, fmt.Errorf("驳回操作失败: %w", err)
	}

	b.logger.Info("报销单已驳回", zap.String("报销单号", rm.ReimbursementNo), zap.String("驳回原因", reason))
	return rm, nil
}

// Cancel 取消报销单草稿（仅 draft 状态可取消）
// 取消后状态变为 cancelled，不可恢复；不涉及预算操作（草稿阶段预算尚未冻结）
func (b *ReimbursementBiz) Cancel(id uint) (*model.Reimbursement, error) {
	b.logger.Debug("取消报销单草稿", zap.Uint("报销单ID", id))

	// 获取报销单 —— 需要校验当前状态
	rm, err := b.repo.GetByID(id)
	if err != nil {
		b.logger.Warn("报销单不存在，无法取消", zap.Uint("报销单ID", id))
		return nil, fmt.Errorf("报销单不存在")
	}
	b.logger.Debug("报销单已查询到", zap.Uint("报销单ID", id), zap.String("当前状态", rm.Status), zap.String("报销单号", rm.ReimbursementNo))

	// 状态守卫：只有草稿状态的报销单可以取消
	if rm.Status != StatusDraft {
		b.logger.Warn("报销单状态不允许取消", zap.Uint("报销单ID", id), zap.String("当前状态", rm.Status))
		return nil, fmt.Errorf("当前状态为'%s'，只有草稿状态的报销单可以取消", rm.Status)
	}
	b.logger.Debug("报销单状态校验通过", zap.Uint("报销单ID", id))

	// 更新状态为已取消 —— 草稿阶段无预算操作，仅变更状态
	rm.Status = model.ReimbStatusCancelled
	b.logger.Debug("更新报销单状态为已取消", zap.Uint("报销单ID", id))
	if err := b.repo.Update(rm); err != nil {
		b.logger.Error("更新报销单状态失败", zap.Uint("报销单ID", id), zap.Error(err))
		return nil, fmt.Errorf("取消报销单失败: %w", err)
	}

	b.logger.Info("报销单已取消", zap.String("报销单号", rm.ReimbursementNo))
	return rm, nil
}

// GetByID 根据 ID 查询报销单
// 直接委托 repo 层查询数据库，biz 层仅做简单透传
func (b *ReimbursementBiz) GetByID(id uint) (*model.Reimbursement, error) {
	b.logger.Debug("根据ID查询报销单", zap.Uint("报销单ID", id))

	// 调用 repo 层查询 —— repo 层会处理 GORM 的 Preload 等关联查询
	rm, err := b.repo.GetByID(id)
	if err != nil {
		// 查询不到记录时 repo 返回错误，biz 层封装为业务友好的中文错误
		b.logger.Warn("报销单不存在", zap.Uint("报销单ID", id), zap.Error(err))
		return nil, fmt.Errorf("报销单不存在")
	}
	b.logger.Debug("报销单查询成功", zap.Uint("报销单ID", id), zap.String("报销单号", rm.ReimbursementNo), zap.String("状态", rm.Status))
	return rm, nil
}

// GetByNo 根据报销单号查询
// 报销单号是业务唯一标识，前端可能通过单号而不是数据库 ID 来检索
func (b *ReimbursementBiz) GetByNo(no string) (*model.Reimbursement, error) {
	b.logger.Debug("根据单号查询报销单", zap.String("报销单号", no))

	// 按业务单号查询 —— 报销单号格式为 REIMB-YYYY-NNNN，易读且唯一
	rm, err := b.repo.GetByNo(no)
	if err != nil {
		// 单号不存在 —— 可能输入错误或已被删除
		b.logger.Warn("报销单不存在", zap.String("报销单号", no), zap.Error(err))
		return nil, fmt.Errorf("报销单号'%s'不存在", no)
	}
	b.logger.Debug("报销单查询成功", zap.String("报销单号", no), zap.Uint("ID", rm.ID), zap.String("状态", rm.Status))
	return rm, nil
}

// List 分页查询报销单列表，可按员工工号筛选
// 支持两种模式：不传 employeeID 查全部，传 employeeID 则按工号过滤
func (b *ReimbursementBiz) List(page, pageSize int, employeeID string) ([]*model.Reimbursement, int64, error) {
	b.logger.Debug("查询报销单列表", zap.Int("页码", page), zap.Int("每页数量", pageSize), zap.String("工号", employeeID))

	// 委托 repo 层执行分页查询 —— repo 负责 GORM 的 Offset/Limit/Where 构建
	rms, total, err := b.repo.List(page, pageSize, employeeID)
	if err != nil {
		// 数据库查询失败 —— 可能是连接异常或 SQL 错误
		b.logger.Error("查询报销单列表失败", zap.Int("页码", page), zap.Error(err))
		return nil, 0, fmt.Errorf("查询报销单列表失败: %w", err)
	}
	// 查询成功 —— 记录返回数量和总数，便于监控查询性能
	b.logger.Debug("查询报销单列表成功", zap.Int64("总数", total), zap.Int("当前页返回数量", len(rms)))
	return rms, total, nil
}

// ListPending 查询待审批的报销单
// 用于审批人查看待办列表，只返回状态为 Pending 的报销单
func (b *ReimbursementBiz) ListPending() ([]*model.Reimbursement, error) {
	b.logger.Debug("查询待审批报销单")

	// 按状态过滤查询 —— 待审批状态是报销单进入审批流程的入口状态
	rms, err := b.repo.ListByStatus(StatusPending)
	if err != nil {
		// 查询失败 —— 数据库层面问题
		b.logger.Error("查询待审批报销单失败", zap.Error(err))
		return nil, fmt.Errorf("查询待审批报销单失败: %w", err)
	}
	// 记录返回数量 —— 为 0 时表示当前没有待审批的报销单
	b.logger.Debug("查询待审批报销单成功", zap.Int("数量", len(rms)))
	return rms, nil
}

// ListPendingByApprover 按审批人姓名查询其待审批的报销单
// 通过审批记录表过滤，只返回该审批人负责的待审批报销单
func (b *ReimbursementBiz) ListPendingByApprover(approverName string) ([]*model.Reimbursement, error) {
	b.logger.Debug("按审批人查询待审批报销单", zap.String("审批人", approverName))

	records, err := b.approvalBiz.ListPendingByApprover(approverName)
	if err != nil {
		b.logger.Error("查询审批人待审批记录失败", zap.String("审批人", approverName), zap.Error(err))
		return nil, fmt.Errorf("查询待审批报销单失败: %w", err)
	}
	if len(records) == 0 {
		return nil, nil
	}
	ids := make([]uint, 0, len(records))
	for _, r := range records {
		ids = append(ids, r.ReimbursementID)
	}
	rms, err := b.repo.ListByIDs(ids)
	if err != nil {
		b.logger.Error("查询报销单失败", zap.Error(err))
		return nil, fmt.Errorf("查询待审批报销单失败: %w", err)
	}
	b.logger.Info("按审批人查询待审批报销单成功", zap.String("审批人", approverName), zap.Int("数量", len(rms)))
	return rms, nil
}
