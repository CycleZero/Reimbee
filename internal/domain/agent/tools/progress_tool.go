package tools

import (
	"context"
	"fmt"

	"github.com/CycleZero/Reimbee/internal/domain/approval"
	"github.com/CycleZero/Reimbee/internal/domain/reimbursement"
	"github.com/CycleZero/Reimbee/log"
	"github.com/CycleZero/Reimbee/model"
	"github.com/CycleZero/blades/tools"
	"go.uber.org/zap"
)

// ProgressInput query_progress 工具的输入参数
type ProgressInput struct {
	ReimbursementNo string `json:"reimbursement_no"` // 报销单号（可选，为空时查最近5条）
	EmployeeID      string `json:"employee_id"`      // 员工工号（可选，从会话上下文自动填充）
}

// ProgressOutput query_progress 工具的输出结果
type ProgressOutput struct {
	Reimbursements []ProgressItem `json:"reimbursements"` // 报销单及审批进度列表
}

// ProgressItem 单条报销单的进度信息
type ProgressItem struct {
	No          string           `json:"no"`           // 报销单号
	Status      string           `json:"status"`       // 整体状态（pending/reviewing/approved/rejected）
	TotalAmount int64            `json:"total_amount"` // 报销总金额（分）
	SubmitNote  string           `json:"submit_note"`  // 报销事由
	Approvals   []ApprovalStatus `json:"approvals"`    // 各审批人的审批状态
	Comment     string           `json:"comment"`      // 金额单位提示
}

// ApprovalStatus 审批人的审批状态
type ApprovalStatus struct {
	ApproverName string `json:"approver_name"` // 审批人姓名
	Action       string `json:"action"`        // pending / approved / rejected
	Comment      string `json:"comment"`       // 审批意见
	ActionAt     string `json:"action_at"`     // 审批操作时间
}

// ProgressTool Wire 命名类型（Blades tools.Tool）
type ProgressTool struct{ tools.Tool }

// NewProgressTool 创建进度查询工具，封装 reimbursement.ReimbursementBiz + approval.ApprovalBiz
func NewProgressTool(reimbursementBiz *reimbursement.ReimbursementBiz, approvalBiz *approval.ApprovalBiz, logger *log.Logger) *ProgressTool {
	t, err := tools.NewFunc[ProgressInput, ProgressOutput](
		ToolQueryProgress,
		"查询报销单的审批进度。可按报销单号精确查询，或按工号查询该员工的最近5条报销记录。返回每个审批人的审批状态（待审批/已通过/已驳回）及审批意见",
		func(ctx context.Context, input ProgressInput) (ProgressOutput, error) {
			logger.Debug("进度查询工具开始执行",
				zap.String("报销单号", input.ReimbursementNo),
				zap.String("工号", input.EmployeeID))

			// 按报销单号精确查询
			if input.ReimbursementNo != "" {
				rm, err := reimbursementBiz.GetByNo(input.ReimbursementNo)
				if err != nil {
					logger.Warn("查询报销单失败", zap.String("报销单号", input.ReimbursementNo), zap.Error(err))
					return ProgressOutput{}, fmt.Errorf("报销单号'%s'不存在", input.ReimbursementNo)
				}

				// 查询该报销单的审批进度
				records, err := approvalBiz.GetProgress(rm.ID)
				if err != nil {
					logger.Error("查询审批进度失败", zap.Uint("报销单ID", rm.ID), zap.Error(err))
					return ProgressOutput{}, fmt.Errorf("查询审批进度失败: %w", err)
				}

				item := buildProgressItem(rm, records)
				logger.Info("进度查询完成（按单号）", zap.String("报销单号", input.ReimbursementNo), zap.String("状态", rm.Status))
				return ProgressOutput{Reimbursements: []ProgressItem{item}}, nil
			}

			// 按工号查询最近5条
			rms, _, err := reimbursementBiz.List(1, 5, input.EmployeeID)
			if err != nil {
				logger.Error("查询报销单列表失败", zap.Error(err))
				return ProgressOutput{}, fmt.Errorf("查询报销单失败: %w", err)
			}

			items := make([]ProgressItem, 0, len(rms))
			for _, rm := range rms {
				records, _ := approvalBiz.GetProgress(rm.ID)
				items = append(items, buildProgressItem(rm, records))
			}

			logger.Info("进度查询完成（按工号）", zap.String("工号", input.EmployeeID), zap.Int("数量", len(items)))
			return ProgressOutput{Reimbursements: items}, nil
		},
	)
	if err != nil {
		panic("创建进度查询工具失败: " + err.Error())
	}
	logger.Debug("进度查询工具初始化完成")
	return &ProgressTool{t}
}

// buildProgressItem 将报销单模型 + 审批记录转换为 ProgressItem
func buildProgressItem(rm *model.Reimbursement, records []*model.ApprovalRecord) ProgressItem {
	item := ProgressItem{
		No:          rm.ReimbursementNo,
		Status:      rm.Status,
		TotalAmount: rm.TotalAmount,
		SubmitNote:  rm.SubmitNote,
		Comment:     "TotalAmount金额单位为分，呈现时建议转换为元",
	}

	for _, r := range records {
		s := ApprovalStatus{
			ApproverName: r.ApproverName,
			Action:       r.Action,
			Comment:      r.Comment,
		}
		if r.ActionAt != nil {
			s.ActionAt = r.ActionAt.Format("2006-01-02 15:04:05")
		}
		item.Approvals = append(item.Approvals, s)
	}

	return item
}
