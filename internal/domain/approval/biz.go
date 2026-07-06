package approval

import (
	"fmt"
	"time"

	"github.com/CycleZero/Reimbee/log"
	"github.com/CycleZero/Reimbee/model"
	"go.uber.org/zap"
)

// ApprovalBiz 审批业务逻辑层
type ApprovalBiz struct {
	logger *log.Logger
	repo   *ApprovalRepo
}

// NewApprovalBiz 创建审批业务逻辑层实例
func NewApprovalBiz(logger *log.Logger, repo *ApprovalRepo) *ApprovalBiz {
	logger.Debug("初始化审批业务逻辑层")
	return &ApprovalBiz{logger: logger, repo: repo}
}

// CreateApprovalChain 为一笔报销单创建审批链（多位审批人）
func (b *ApprovalBiz) CreateApprovalChain(reimbursementID uint, approvers []*model.Employee) error {
	b.logger.Debug("开始创建审批链", zap.Uint("报销单ID", reimbursementID), zap.Int("审批人数", len(approvers)))

	if len(approvers) == 0 {
		b.logger.Warn("审批人列表为空，无法创建审批链")
		return fmt.Errorf("至少需要指定一位审批人")
	}

	for _, approver := range approvers {
		record := &model.ApprovalRecord{
			ReimbursementID: reimbursementID,
			ApproverName:    approver.Name,
			ApproverEmail:   approver.Email,
			Action:          model.ApprovalActionPending,
		}
		if err := b.repo.Create(record); err != nil {
			b.logger.Error("创建审批记录失败", zap.String("审批人", approver.Name), zap.Error(err))
			return fmt.Errorf("创建审批记录失败: %w", err)
		}
		b.logger.Debug("审批记录创建成功", zap.String("审批人", approver.Name))
	}

	b.logger.Info("审批链创建完成", zap.Uint("报销单ID", reimbursementID), zap.Int("审批人数", len(approvers)))
	return nil
}

// Approve 审批通过
func (b *ApprovalBiz) Approve(recordID uint, comment string) error {
	b.logger.Debug("审批通过", zap.Uint("审批记录ID", recordID))

	record, err := b.repo.GetByID(recordID)
	if err != nil {
		b.logger.Warn("审批记录不存在", zap.Uint("审批记录ID", recordID))
		return fmt.Errorf("审批记录不存在")
	}

	if record.Action != model.ApprovalActionPending {
		b.logger.Warn("审批记录已处理，不可重复操作", zap.Uint("审批记录ID", recordID), zap.String("当前状态", record.Action))
		return fmt.Errorf("该审批已处理（当前状态: %s），不可重复操作", record.Action)
	}

	now := time.Now()
	record.Action = model.ApprovalActionApproved
	record.Comment = comment
	record.ActionAt = &now

	if err := b.repo.Update(record); err != nil {
		b.logger.Error("审批通过操作失败", zap.Uint("审批记录ID", recordID), zap.Error(err))
		return fmt.Errorf("审批操作失败: %w", err)
	}

	b.logger.Info("审批已通过", zap.Uint("审批记录ID", recordID), zap.String("审批人", record.ApproverName))
	return nil
}

// Reject 驳回审批
func (b *ApprovalBiz) Reject(recordID uint, reason string) error {
	b.logger.Debug("驳回审批", zap.Uint("审批记录ID", recordID))

	if reason == "" {
		b.logger.Warn("驳回原因不能为空")
		return fmt.Errorf("驳回时必须填写驳回原因")
	}

	record, err := b.repo.GetByID(recordID)
	if err != nil {
		b.logger.Warn("审批记录不存在", zap.Uint("审批记录ID", recordID))
		return fmt.Errorf("审批记录不存在")
	}

	if record.Action != model.ApprovalActionPending {
		b.logger.Warn("审批记录已处理，不可重复操作", zap.Uint("审批记录ID", recordID), zap.String("当前状态", record.Action))
		return fmt.Errorf("该审批已处理（当前状态: %s），不可重复操作", record.Action)
	}

	now := time.Now()
	record.Action = model.ApprovalActionRejected
	record.Comment = reason
	record.ActionAt = &now

	if err := b.repo.Update(record); err != nil {
		b.logger.Error("驳回审批操作失败", zap.Uint("审批记录ID", recordID), zap.Error(err))
		return fmt.Errorf("驳回操作失败: %w", err)
	}

	b.logger.Info("审批已驳回", zap.Uint("审批记录ID", recordID), zap.String("审批人", record.ApproverName), zap.String("原因", reason))
	return nil
}

// IsAllApproved 检查报销单的所有审批人是否都已完成审批
func (b *ApprovalBiz) IsAllApproved(reimbursementID uint) (bool, error) {
	b.logger.Debug("检查审批是否全部完成", zap.Uint("报销单ID", reimbursementID))

	records, err := b.repo.ListByReimbursement(reimbursementID)
	if err != nil {
		b.logger.Error("查询审批记录失败", zap.Uint("报销单ID", reimbursementID), zap.Error(err))
		return false, fmt.Errorf("查询审批记录失败: %w", err)
	}

	for _, r := range records {
		if r.Action == model.ApprovalActionPending {
			b.logger.Debug("仍有审批人未处理", zap.String("审批人", r.ApproverName))
			return false, nil
		}
		if r.Action == model.ApprovalActionRejected {
			b.logger.Debug("有审批人已驳回", zap.String("审批人", r.ApproverName))
			return false, nil
		}
	}

	b.logger.Debug("所有审批人均已通过", zap.Uint("报销单ID", reimbursementID), zap.Int("审批人数", len(records)))
	return true, nil
}

// IsAnyRejected 检查是否有审批人驳回了报销
func (b *ApprovalBiz) IsAnyRejected(reimbursementID uint) (bool, string, error) {
	b.logger.Debug("检查是否存在驳回", zap.Uint("报销单ID", reimbursementID))

	records, err := b.repo.ListByReimbursement(reimbursementID)
	if err != nil {
		b.logger.Error("查询审批记录失败", zap.Uint("报销单ID", reimbursementID), zap.Error(err))
		return false, "", fmt.Errorf("查询审批记录失败: %w", err)
	}

	for _, r := range records {
		if r.Action == "rejected" {
			b.logger.Debug("发现驳回记录", zap.String("审批人", r.ApproverName), zap.String("原因", r.Comment))
			return true, r.Comment, nil
		}
	}

	return false, "", nil
}

// GetProgress 获取报销单的审批进度
func (b *ApprovalBiz) GetProgress(reimbursementID uint) ([]*model.ApprovalRecord, error) {
	b.logger.Debug("查询审批进度", zap.Uint("报销单ID", reimbursementID))

	records, err := b.repo.ListByReimbursement(reimbursementID)
	if err != nil {
		b.logger.Error("查询审批进度失败", zap.Uint("报销单ID", reimbursementID), zap.Error(err))
		return nil, fmt.Errorf("查询审批进度失败: %w", err)
	}

	b.logger.Debug("查询审批进度成功", zap.Uint("报销单ID", reimbursementID), zap.Int("审批记录数", len(records)))
	return records, nil
}
