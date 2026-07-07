// Package tools 智能体工具层
// submit_reimbursement 工具：提交报销单进入审批流程
// 此操作不可撤销——冻结预算、创建审批链、状态变更为 pending
package tools

import (
	"context"
	"fmt"

	"github.com/CycleZero/Reimbee/internal/domain/reimbursement"
	"github.com/CycleZero/Reimbee/log"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"go.uber.org/zap"
)

// SubmitReimbInput 提交报销单工具的输入参数
type SubmitReimbInput struct {
	ReimbursementID uint  `json:"reimbursement_id" jsonschema:"required" jsonschema_description:"报销单ID（由create_reimbursement工具返回）"`
	TotalAmount     int64 `json:"total_amount" jsonschema:"required" jsonschema_description:"报销总金额（分）"`
}

// SubmitReimbOutput 提交报销单工具的输出结果
type SubmitReimbOutput struct {
	ReimbursementNo     string `json:"reimbursement_no"`
	Status              string `json:"status"`
	NeedSpecialApproval bool   `json:"need_special_approval"`
}

// SubmitReimbTool Wire 命名类型
type SubmitReimbTool struct{ tool.InvokableTool }

func NewSubmitReimbTool(reimbursementBiz *reimbursement.ReimbursementBiz, logger *log.Logger) *SubmitReimbTool {
	t, err := utils.InferTool[SubmitReimbInput, SubmitReimbOutput](
		"submit_reimbursement",
		"提交报销单进入审批流程。此操作不可撤销——将冻结部门预算、创建审批链并通知审批人。调用前必须确保用户已确认所有信息（FinalConfirmed=true）。需要先调用create_reimbursement获得报销单ID。",
		func(ctx context.Context, input SubmitReimbInput) (SubmitReimbOutput, error) {
			logger.Debug("提交报销单工具开始执行",
				zap.Uint("报销单ID", input.ReimbursementID),
				zap.Int64("总金额(分)", input.TotalAmount))

			rm, err := reimbursementBiz.Submit(input.ReimbursementID, input.TotalAmount)
			if err != nil {
				logger.Error("提交报销单失败", zap.Uint("报销单ID", input.ReimbursementID), zap.Error(err))
				return SubmitReimbOutput{}, fmt.Errorf("提交报销单失败: %w", err)
			}

			logger.Info("报销单提交成功",
				zap.String("报销单号", rm.ReimbursementNo),
				zap.String("状态", rm.Status),
				zap.Bool("需要特殊审批", rm.NeedSpecialApproval))

			return SubmitReimbOutput{
				ReimbursementNo:     rm.ReimbursementNo,
				Status:              rm.Status,
				NeedSpecialApproval: rm.NeedSpecialApproval,
			}, nil
		},
	)
	if err != nil {
		panic("创建submit_reimbursement工具失败: " + err.Error())
	}
	logger.Debug("提交报销单工具初始化完成")
	return &SubmitReimbTool{t}
}
