package reimbursement

import (
	"fmt"
	"strconv"
	"time"

	"github.com/CycleZero/Reimbee/internal/domain/approval"
	"github.com/CycleZero/Reimbee/internal/domain/budget"
	"github.com/CycleZero/Reimbee/internal/domain/employee"
	"github.com/CycleZero/Reimbee/log"
	"github.com/CycleZero/Reimbee/model"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// generateReimbursementNo 从数据库查询当前年度最大序号，原子递增生成报销单号
// 必须在事务内调用：唯一索引兜底防并发冲突
func (b *ReimbursementBiz) generateReimbursementNo(tx *gorm.DB) (string, error) {
	year := time.Now().Year()
	var lastNo string
	tx.Model(&model.Reimbursement{}).
		Select("reimbursement_no").
		Where("reimbursement_no LIKE ?", fmt.Sprintf("REIMB-%d-%%", year)).
		Order("reimbursement_no DESC").
		Limit(1).
		Scan(&lastNo)

	seq := uint64(1)
	if lastNo != "" && len(lastNo) >= 4 {
		if n, err := strconv.ParseUint(lastNo[len(lastNo)-4:], 10, 64); err == nil && n > 0 {
			seq = n + 1
		}
	}
	return fmt.Sprintf("REIMB-%d-%04d", year, seq), nil
}

// 报销单状态常量（引用 model 统一定义）
const (
	StatusDraft     = model.ReimbStatusDraft
	StatusPending   = model.ReimbStatusPending
	StatusReviewing = model.ReimbStatusReviewing
	StatusApproved  = model.ReimbStatusApproved
	StatusRejected  = model.ReimbStatusRejected
)

// ReimbursementBiz 报销单业务逻辑层
type ReimbursementBiz struct {
	logger      *log.Logger
	repo        *ReimbursementRepo
	itemBiz     *ItemBiz
	receiptRepo *ReceiptRepo
	budgetBiz   *budget.BudgetBiz
	approvalBiz *approval.ApprovalBiz
	employeeBiz *employee.EmployeeBiz
}

// NewReimbursementBiz 创建报销单业务逻辑层实例
func NewReimbursementBiz(
	logger *log.Logger,
	repo *ReimbursementRepo,
	itemBiz *ItemBiz,
	receiptRepo *ReceiptRepo,
	budgetBiz *budget.BudgetBiz,
	approvalBiz *approval.ApprovalBiz,
	employeeBiz *employee.EmployeeBiz,
) *ReimbursementBiz {
	logger.Debug("初始化报销单业务逻辑层（含明细+票据管理）")
	return &ReimbursementBiz{
		logger:      logger,
		repo:        repo,
		itemBiz:     itemBiz,
		receiptRepo: receiptRepo,
		budgetBiz:   budgetBiz,
		approvalBiz: approvalBiz,
		employeeBiz: employeeBiz,
	}
}

// Create 创建报销单（含明细和票据），使用事务保证原子性
func (b *ReimbursementBiz) Create(input *CreateReimbInput) (*model.Reimbursement, error) {
	b.logger.Debug("开始创建报销单",
		zap.String("工号", input.EmployeeID),
		zap.String("姓名", input.EmployeeName),
		zap.Uint("部门ID", input.DepartmentID),
		zap.Int("明细数", len(input.Items)))

	rm := &model.Reimbursement{
		EmployeeID:   input.EmployeeID,
		EmployeeName: input.EmployeeName,
		DepartmentID: input.DepartmentID,
		Status:       StatusDraft,
		SubmitNote:   input.SubmitNote,
	}

	// 事务：生成单号 → 创建报销单 → 创建明细 → 创建票据
	err := b.repo.db.Transaction(func(tx *gorm.DB) error {
		reimbNo, err := b.generateReimbursementNo(tx)
		if err != nil {
			return fmt.Errorf("生成报销单号失败: %w", err)
		}
		rm.ReimbursementNo = reimbNo
		if err := tx.Create(rm).Error; err != nil {
			return fmt.Errorf("创建报销单记录失败: %w", err)
		}

		for _, itemInput := range input.Items {
			item := &model.ReimbursementItem{
				ReimbursementID: rm.ID,
				Category:        itemInput.Category,
				Amount:          itemInput.Amount,
				Description:     itemInput.Description,
			}
			if err := tx.Create(item).Error; err != nil {
				return fmt.Errorf("创建报销明细失败: %w", err)
			}

			for _, rctInput := range itemInput.Receipts {
				receipt := &model.Receipt{
					ItemID:         item.ID,
					InvoiceCode:    rctInput.InvoiceCode,
					InvoiceNumber:  rctInput.InvoiceNumber,
					Amount:         rctInput.Amount,
					InvoiceDate:    rctInput.InvoiceDate,
					SellerName:     rctInput.SellerName,
					Category:       itemInput.Category,
					ImagePath:      rctInput.ImagePath,
					OCRRawAmount:   rctInput.OCRRawAmount,
					OCRRawDate:     rctInput.OCRRawDate,
					OCRRawCategory: rctInput.OCRRawCategory,
					OCRConfidence:  rctInput.OCRConfidence,
				}
				if err := tx.Create(receipt).Error; err != nil {
					return fmt.Errorf("创建票据记录失败: %w", err)
				}
			}
		}
		return nil
	})
	if err != nil {
		b.logger.Error("创建报销单失败", zap.Error(err))
		return nil, err
	}

	b.logger.Info("报销单创建成功（含明细和票据）",
		zap.String("报销单号", rm.ReimbursementNo),
		zap.Uint("ID", rm.ID),
		zap.Int("明细数", len(input.Items)))
	return rm, nil
}

// AddItems 向已有报销单追加报销明细和票据（用于 organize_items 工具）
func (b *ReimbursementBiz) AddItems(reimbID uint, inputs []ItemInput) error {
	rm, err := b.repo.GetByID(reimbID)
	if err != nil {
		return fmt.Errorf("报销单不存在")
	}
	if rm.Status != StatusDraft {
		return fmt.Errorf("只有草稿状态的报销单可以添加明细")
	}

	return b.repo.db.Transaction(func(tx *gorm.DB) error {
		for _, itemInput := range inputs {
			item := &model.ReimbursementItem{
				ReimbursementID: reimbID,
				Category:        itemInput.Category,
				Amount:          itemInput.Amount,
				Description:     itemInput.Description,
			}
			if err := tx.Create(item).Error; err != nil {
				return fmt.Errorf("创建明细失败: %w", err)
			}
			for _, rctInput := range itemInput.Receipts {
				receipt := &model.Receipt{
					ItemID:         item.ID,
					InvoiceCode:    rctInput.InvoiceCode,
					InvoiceNumber:  rctInput.InvoiceNumber,
					Amount:         rctInput.Amount,
					InvoiceDate:    rctInput.InvoiceDate,
					SellerName:     rctInput.SellerName,
					Category:       itemInput.Category,
					ImagePath:      rctInput.ImagePath,
					OCRRawAmount:   rctInput.OCRRawAmount,
					OCRRawDate:     rctInput.OCRRawDate,
					OCRRawCategory: rctInput.OCRRawCategory,
					OCRConfidence:  rctInput.OCRConfidence,
				}
				if err := tx.Create(receipt).Error; err != nil {
					return fmt.Errorf("创建票据失败: %w", err)
				}
			}
		}
		return nil
	})
}

// Submit 提交报销单（草稿 → 待审批）
// 总金额从 Items 汇总计算，不再由外部传入
func (b *ReimbursementBiz) Submit(id uint) (*model.Reimbursement, error) {
	// 获取报销单
	rm, err := b.repo.GetByID(id)
	if err != nil {
		b.logger.Warn("报销单不存在", zap.Uint("报销单ID", id))
		return nil, fmt.Errorf("报销单不存在")
	}

	// 状态校验
	if rm.Status != StatusDraft && rm.Status != StatusRejected {
		return nil, fmt.Errorf("当前状态为'%s'，只有草稿或已驳回的报销单可以提交", rm.Status)
	}

	// 从 Items 汇总总金额
	var totalAmount int64
	for _, item := range rm.Items {
		totalAmount += item.Amount
	}
	if totalAmount <= 0 {
		return nil, fmt.Errorf("报销金额必须大于零")
	}
	b.logger.Debug("汇总报销金额", zap.Int64("总金额(分)", totalAmount), zap.Int("明细数", len(rm.Items)))

	// 预算检查
	_, needSpecial, err := b.budgetBiz.CheckBudget(rm.DepartmentID, totalAmount)
	if err != nil {
		return nil, fmt.Errorf("预算检查失败: %w", err)
	}

	// 冻结预算
	if err := b.budgetBiz.Freeze(rm.DepartmentID, totalAmount); err != nil {
		return nil, fmt.Errorf("冻结预算失败: %w", err)
	}

	// 创建审批链
	approvers, err := b.employeeBiz.ListApprovers()
	if err != nil {
		b.budgetBiz.Unfreeze(rm.DepartmentID, totalAmount)
		return nil, fmt.Errorf("获取审批人列表失败: %w", err)
	}
	if len(approvers) == 0 {
		b.budgetBiz.Unfreeze(rm.DepartmentID, totalAmount)
		return nil, fmt.Errorf("系统中没有审批人，无法提交报销单")
	}
	if err := b.approvalBiz.CreateApprovalChain(rm.ID, approvers); err != nil {
		b.budgetBiz.Unfreeze(rm.DepartmentID, totalAmount)
		return nil, fmt.Errorf("创建审批链失败: %w", err)
	}

	// 更新报销单状态
	rm.TotalAmount = totalAmount
	rm.Status = StatusPending
	rm.NeedSpecialApproval = needSpecial
	if needSpecial {
		b.logger.Warn("报销单需要特殊审批（预算不足）", zap.Uint("报销单ID", id))
	}
	if err := b.repo.Update(rm); err != nil {
		b.budgetBiz.Unfreeze(rm.DepartmentID, totalAmount)
		return nil, fmt.Errorf("提交报销单失败: %w", err)
	}

	b.logger.Info("报销单提交成功",
		zap.String("报销单号", rm.ReimbursementNo),
		zap.Int64("金额(分)", totalAmount),
		zap.Int("审批人数", len(approvers)),
		zap.Bool("需要特殊审批", needSpecial))
	return rm, nil
}

// Approve 审批通过报销单（按当前审批人审批，非强制模式）
func (b *ReimbursementBiz) Approve(id uint, approverName string) (*model.Reimbursement, error) {
	rm, err := b.repo.GetByID(id)
	if err != nil {
		return nil, fmt.Errorf("报销单不存在")
	}
	if rm.Status != StatusPending && rm.Status != StatusReviewing {
		return nil, fmt.Errorf("当前状态为'%s'，不可审批", rm.Status)
	}

	// 只审批当前审批人的待审批记录
	var approved bool
	for _, a := range rm.Approvals {
		if a.ApproverName == approverName && a.Action == model.ApprovalActionPending {
			if err := b.approvalBiz.Approve(a.ID, "审批通过"); err != nil {
				return nil, fmt.Errorf("更新审批记录失败: %w", err)
			}
			approved = true
			break
		}
	}
	if !approved {
		return nil, fmt.Errorf("未找到待审批记录或无审批权限")
	}

	// 检查是否所有审批人都已通过
	allApproved, err := b.approvalBiz.IsAllApproved(rm.ID)
	if err != nil {
		return nil, fmt.Errorf("检查审批状态失败: %w", err)
	}

	if allApproved {
		if err := b.budgetBiz.Deduct(rm.DepartmentID, rm.TotalAmount); err != nil {
			return nil, fmt.Errorf("扣减预算失败: %w", err)
		}
		rm.Status = StatusApproved
	} else {
		rm.Status = StatusReviewing
	}

	if err := b.repo.Update(rm); err != nil {
		return nil, fmt.Errorf("审批操作失败: %w", err)
	}

	b.logger.Info("审批处理完成",
		zap.String("报销单号", rm.ReimbursementNo),
		zap.String("审批人", approverName),
		zap.Bool("全部通过", allApproved))
	return rm, nil
}

// Reject 驳回报销单（解冻预算）
func (b *ReimbursementBiz) Reject(id uint, reason string) (*model.Reimbursement, error) {
	rm, err := b.repo.GetByID(id)
	if err != nil {
		return nil, fmt.Errorf("报销单不存在")
	}
	if rm.Status != StatusPending && rm.Status != StatusReviewing {
		return nil, fmt.Errorf("当前状态为'%s'，不可驳回", rm.Status)
	}

	if err := b.budgetBiz.Unfreeze(rm.DepartmentID, rm.TotalAmount); err != nil {
		return nil, fmt.Errorf("解冻预算失败: %w", err)
	}

	rm.Status = StatusRejected
	if err := b.repo.Update(rm); err != nil {
		return nil, fmt.Errorf("驳回操作失败: %w", err)
	}

	b.logger.Info("报销单已驳回",
		zap.String("报销单号", rm.ReimbursementNo),
		zap.String("驳回原因", reason))
	return rm, nil
}

// Cancel 取消报销单草稿
func (b *ReimbursementBiz) Cancel(id uint) (*model.Reimbursement, error) {
	rm, err := b.repo.GetByID(id)
	if err != nil {
		return nil, fmt.Errorf("报销单不存在")
	}
	if rm.Status != StatusDraft {
		return nil, fmt.Errorf("当前状态为'%s'，只有草稿状态的报销单可以取消", rm.Status)
	}
	rm.Status = model.ReimbStatusCancelled
	if err := b.repo.Update(rm); err != nil {
		return nil, fmt.Errorf("取消报销单失败: %w", err)
	}
	b.logger.Info("报销单已取消", zap.String("报销单号", rm.ReimbursementNo))
	return rm, nil
}

// GetByID 根据 ID 查询报销单
func (b *ReimbursementBiz) GetByID(id uint) (*model.Reimbursement, error) {
	rm, err := b.repo.GetByID(id)
	if err != nil {
		return nil, fmt.Errorf("报销单不存在")
	}
	return rm, nil
}

// GetByNo 根据报销单号查询
func (b *ReimbursementBiz) GetByNo(no string) (*model.Reimbursement, error) {
	rm, err := b.repo.GetByNo(no)
	if err != nil {
		return nil, fmt.Errorf("报销单号'%s'不存在", no)
	}
	return rm, nil
}

// List 分页查询报销单列表
func (b *ReimbursementBiz) List(page, pageSize int, employeeID string) ([]*model.Reimbursement, int64, error) {
	return b.repo.List(page, pageSize, employeeID)
}

// ListPending 查询待审批的报销单
func (b *ReimbursementBiz) ListPending() ([]*model.Reimbursement, error) {
	return b.repo.ListByStatus(StatusPending)
}

// ListPendingByApprover 按审批人查询待审批报销单
func (b *ReimbursementBiz) ListPendingByApprover(approverName string) ([]*model.Reimbursement, error) {
	records, err := b.approvalBiz.ListPendingByApprover(approverName)
	if err != nil {
		return nil, fmt.Errorf("查询待审批报销单失败: %w", err)
	}
	if len(records) == 0 {
		return nil, nil
	}
	ids := make([]uint, 0, len(records))
	for _, r := range records {
		ids = append(ids, r.ReimbursementID)
	}
	return b.repo.ListByIDs(ids)
}
