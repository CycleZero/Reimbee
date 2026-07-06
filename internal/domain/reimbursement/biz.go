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
var (
	reimbursementSeq uint64
	currentYear      = time.Now().Year()
)

// generateReimbursementNo 生成报销单号 REIMB-YYYY-NNNN
func generateReimbursementNo() string {
	seq := atomic.AddUint64(&reimbursementSeq, 1)
	return fmt.Sprintf("REIMB-%d-%04d", currentYear, seq)
}

// 报销单状态常量（引用 model 统一定义）
const (
	StatusDraft     = model.ReimbStatusDraft
	StatusPending   = model.ReimbStatusPending
	StatusReviewing = model.ReimbStatusReviewing
	StatusApproved  = model.ReimbStatusApproved
	StatusRejected  = model.ReimbStatusRejected
)

// ReimbursementBiz 报销单业务逻辑层，负责报销单生命周期管理和跨域编排
type ReimbursementBiz struct {
	logger       *log.Logger
	repo         *ReimbursementRepo
	budgetBiz    *budget.BudgetBiz
	approvalBiz  *approval.ApprovalBiz
	employeeBiz  *employee.EmployeeBiz
}

// NewReimbursementBiz 创建报销单业务逻辑层实例
func NewReimbursementBiz(
	logger *log.Logger,
	repo *ReimbursementRepo,
	budgetBiz *budget.BudgetBiz,
	approvalBiz *approval.ApprovalBiz,
	employeeBiz *employee.EmployeeBiz,
) *ReimbursementBiz {
	logger.Debug("初始化报销单业务逻辑层（含审批链编排）")
	return &ReimbursementBiz{
		logger:      logger,
		repo:        repo,
		budgetBiz:   budgetBiz,
		approvalBiz: approvalBiz,
		employeeBiz: employeeBiz,
	}
}

// Create 创建报销单（草稿状态）
func (b *ReimbursementBiz) Create(employeeID, employeeName string, deptID uint, submitNote string) (*model.Reimbursement, error) {
	b.logger.Debug("开始创建报销单", zap.String("工号", employeeID), zap.String("姓名", employeeName))

	rm := &model.Reimbursement{
		ReimbursementNo: generateReimbursementNo(),
		EmployeeID:      employeeID,
		EmployeeName:    employeeName,
		DepartmentID:    deptID,
		Status:          StatusDraft,
		SubmitNote:      submitNote,
	}
	if err := b.repo.Create(rm); err != nil {
		b.logger.Error("创建报销单失败", zap.String("工号", employeeID), zap.Error(err))
		return nil, fmt.Errorf("创建报销单失败: %w", err)
	}

	b.logger.Info("报销单创建成功（草稿）", zap.String("报销单号", rm.ReimbursementNo), zap.Uint("ID", rm.ID))
	return rm, nil
}

// Submit 提交报销单（草稿 → 待审批）
// 执行顺序：校验 → 预算检查 → 冻结预算 → 创建审批链 → 更新状态
// 若中间步骤失败，已执行的前置操作将被回滚
func (b *ReimbursementBiz) Submit(id uint, totalAmount int64) (*model.Reimbursement, error) {
	b.logger.Debug("开始提交报销单", zap.Uint("报销单ID", id), zap.Int64("总金额(分)", totalAmount))

	// 1. 获取报销单并校验状态
	rm, err := b.repo.GetByID(id)
	if err != nil {
		b.logger.Warn("报销单不存在", zap.Uint("报销单ID", id))
		return nil, fmt.Errorf("报销单不存在")
	}
	if rm.Status != StatusDraft && rm.Status != StatusRejected {
		b.logger.Warn("报销单状态不允许提交", zap.Uint("报销单ID", id), zap.String("当前状态", rm.Status))
		return nil, fmt.Errorf("当前状态为'%s'，只有草稿或已驳回的报销单可以提交", rm.Status)
	}

	// 2. 校验金额
	if totalAmount <= 0 {
		b.logger.Warn("报销金额无效", zap.Int64("金额", totalAmount))
		return nil, fmt.Errorf("报销金额必须大于零")
	}

	// 3. 检查预算
	_, needSpecial, err := b.budgetBiz.CheckBudget(rm.DepartmentID, totalAmount)
	if err != nil {
		b.logger.Error("预算检查失败", zap.Uint("部门ID", rm.DepartmentID), zap.Error(err))
		return nil, fmt.Errorf("预算检查失败: %w", err)
	}

	// 4. 冻结预算（原子操作，不可回滚需手工解冻）
	if err := b.budgetBiz.Freeze(rm.DepartmentID, totalAmount); err != nil {
		b.logger.Error("冻结预算失败", zap.Error(err))
		return nil, fmt.Errorf("冻结预算失败: %w", err)
	}

	// 5. 创建审批链——获取所有审批人，为每人创建一条待审批记录
	approvers, err := b.employeeBiz.ListApprovers()
	if err != nil {
		b.logger.Error("获取审批人列表失败，回滚预算冻结", zap.Error(err))
		b.budgetBiz.Unfreeze(rm.DepartmentID, totalAmount) // 回滚
		return nil, fmt.Errorf("获取审批人列表失败: %w", err)
	}
	if len(approvers) == 0 {
		b.logger.Warn("系统中没有审批人，回滚预算冻结", zap.Uint("报销单ID", id))
		b.budgetBiz.Unfreeze(rm.DepartmentID, totalAmount) // 回滚
		return nil, fmt.Errorf("系统中没有审批人，无法提交报销单")
	}
	if err := b.approvalBiz.CreateApprovalChain(rm.ID, approvers); err != nil {
		b.logger.Error("创建审批链失败，回滚预算冻结", zap.Uint("报销单ID", id), zap.Error(err))
		b.budgetBiz.Unfreeze(rm.DepartmentID, totalAmount) // 回滚
		return nil, fmt.Errorf("创建审批链失败: %w", err)
	}

	// 6. 更新报销单状态为待审批
	rm.TotalAmount = totalAmount
	rm.Status = StatusPending
	rm.NeedSpecialApproval = needSpecial

	if needSpecial {
		b.logger.Warn("报销单需要特殊审批（预算不足）", zap.Uint("报销单ID", id))
	}

	if err := b.repo.Update(rm); err != nil {
		// 更新失败，回滚冻结预算（审批记录保留，下次提交时清理或复用）
		b.logger.Error("更新报销单状态失败，回滚预算冻结", zap.Uint("报销单ID", id), zap.Error(err))
		b.budgetBiz.Unfreeze(rm.DepartmentID, totalAmount)
		return nil, fmt.Errorf("提交报销单失败: %w", err)
	}

	b.logger.Info("报销单提交成功，审批链已创建",
		zap.String("报销单号", rm.ReimbursementNo),
		zap.Int64("金额(分)", totalAmount),
		zap.Int("审批人数", len(approvers)),
		zap.Bool("需要特殊审批", needSpecial),
	)
	return rm, nil
}

// Approve 审批通过报销单（强制通过，跳过审批链逐人审批）
// 适用于模拟审批场景，生产环境应使用 ApprovalBiz.Approve() 逐人审批
func (b *ReimbursementBiz) Approve(id uint) (*model.Reimbursement, error) {
	b.logger.Debug("审批通过报销单（强制模式）", zap.Uint("报销单ID", id))

	rm, err := b.repo.GetByID(id)
	if err != nil {
		b.logger.Warn("报销单不存在", zap.Uint("报销单ID", id))
		return nil, fmt.Errorf("报销单不存在")
	}

	if rm.Status != StatusPending && rm.Status != StatusReviewing {
		b.logger.Warn("报销单状态不允许审批", zap.Uint("报销单ID", id), zap.String("当前状态", rm.Status))
		return nil, fmt.Errorf("当前状态为'%s'，不可审批", rm.Status)
	}

	// 将所有审批记录标记为通过
	for _, a := range rm.Approvals {
		if a.Action == model.ApprovalActionPending {
			if err := b.approvalBiz.Approve(a.ID, "系统自动审批"); err != nil {
				b.logger.Error("更新审批记录失败", zap.Uint("审批记录ID", a.ID), zap.Error(err))
				return nil, fmt.Errorf("更新审批记录失败: %w", err)
			}
		}
	}

	// 扣减预算（冻结 → 实际支出）
	if err := b.budgetBiz.Deduct(rm.DepartmentID, rm.TotalAmount); err != nil {
		b.logger.Error("扣减预算失败", zap.Error(err))
		return nil, fmt.Errorf("扣减预算失败: %w", err)
	}

	rm.Status = StatusApproved
	if err := b.repo.Update(rm); err != nil {
		b.logger.Error("更新报销单状态失败", zap.Uint("报销单ID", id), zap.Error(err))
		return nil, fmt.Errorf("审批通过操作失败: %w", err)
	}

	b.logger.Info("报销单已通过", zap.String("报销单号", rm.ReimbursementNo), zap.Int64("金额(分)", rm.TotalAmount))
	return rm, nil
}

// Reject 驳回报销单（强制驳回，解冻预算）
func (b *ReimbursementBiz) Reject(id uint) (*model.Reimbursement, error) {
	b.logger.Debug("驳回报销单", zap.Uint("报销单ID", id))

	rm, err := b.repo.GetByID(id)
	if err != nil {
		b.logger.Warn("报销单不存在", zap.Uint("报销单ID", id))
		return nil, fmt.Errorf("报销单不存在")
	}

	if rm.Status != StatusPending && rm.Status != StatusReviewing {
		b.logger.Warn("报销单状态不允许驳回", zap.Uint("报销单ID", id), zap.String("当前状态", rm.Status))
		return nil, fmt.Errorf("当前状态为'%s'，不可驳回", rm.Status)
	}

	// 解冻预算
	if err := b.budgetBiz.Unfreeze(rm.DepartmentID, rm.TotalAmount); err != nil {
		b.logger.Error("解冻预算失败", zap.Error(err))
		return nil, fmt.Errorf("解冻预算失败: %w", err)
	}

	rm.Status = StatusRejected
	if err := b.repo.Update(rm); err != nil {
		b.logger.Error("更新报销单状态失败", zap.Uint("报销单ID", id), zap.Error(err))
		return nil, fmt.Errorf("驳回操作失败: %w", err)
	}

	b.logger.Info("报销单已驳回", zap.String("报销单号", rm.ReimbursementNo))
	return rm, nil
}

// GetByID 根据 ID 查询报销单
func (b *ReimbursementBiz) GetByID(id uint) (*model.Reimbursement, error) {
	b.logger.Debug("查询报销单", zap.Uint("报销单ID", id))
	rm, err := b.repo.GetByID(id)
	if err != nil {
		b.logger.Warn("报销单不存在", zap.Uint("报销单ID", id), zap.Error(err))
		return nil, fmt.Errorf("报销单不存在")
	}
	return rm, nil
}

// GetByNo 根据报销单号查询
func (b *ReimbursementBiz) GetByNo(no string) (*model.Reimbursement, error) {
	b.logger.Debug("根据单号查询报销单", zap.String("报销单号", no))
	rm, err := b.repo.GetByNo(no)
	if err != nil {
		b.logger.Warn("报销单不存在", zap.String("报销单号", no), zap.Error(err))
		return nil, fmt.Errorf("报销单号'%s'不存在", no)
	}
	return rm, nil
}

// List 分页查询报销单列表，可按员工工号筛选
func (b *ReimbursementBiz) List(page, pageSize int, employeeID string) ([]*model.Reimbursement, int64, error) {
	b.logger.Debug("查询报销单列表", zap.Int("页码", page), zap.Int("每页数量", pageSize), zap.String("工号", employeeID))
	rms, total, err := b.repo.List(page, pageSize, employeeID)
	if err != nil {
		b.logger.Error("查询报销单列表失败", zap.Error(err))
		return nil, 0, fmt.Errorf("查询报销单列表失败: %w", err)
	}
	b.logger.Debug("查询报销单列表成功", zap.Int64("总数", total), zap.Int("返回数量", len(rms)))
	return rms, total, nil
}

// ListPending 查询待审批的报销单
func (b *ReimbursementBiz) ListPending() ([]*model.Reimbursement, error) {
	b.logger.Debug("查询待审批报销单")
	rms, err := b.repo.ListByStatus(StatusPending)
	if err != nil {
		b.logger.Error("查询待审批报销单失败", zap.Error(err))
		return nil, fmt.Errorf("查询待审批报销单失败: %w", err)
	}
	b.logger.Debug("查询待审批报销单成功", zap.Int("数量", len(rms)))
	return rms, nil
}
