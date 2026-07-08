// Package tools 智能体工具层
// create_reimbursement 工具：在数据库中创建报销单草稿
// Phase 3（执行提交）阶段的第一步——LLM 调用此工具后获得报销单 ID，
// 随后可调用 submit_reimbursement → generate_pdf → send_email
package tools

import (
	"context"
	"fmt"

	"github.com/CycleZero/Reimbee/internal/domain/reimbursement"
	"github.com/CycleZero/Reimbee/log"
	"github.com/CycleZero/blades/tools"
	"go.uber.org/zap"
)

// CreateReimbInput 创建报销单工具的输入参数
type CreateReimbInput struct {
	EmployeeID   string `json:"employee_id"`   // 申请人工号
	EmployeeName string `json:"employee_name"` // 申请人姓名
	DepartmentID uint   `json:"department_id"` // 申请部门ID
	SubmitNote   string `json:"submit_note"`   // 报销事由简述
}

// CreateReimbOutput 创建报销单工具的输出结果
type CreateReimbOutput struct {
	ReimbursementID uint   `json:"reimbursement_id"`
	ReimbursementNo string `json:"reimbursement_no"`
	Status          string `json:"status"`
}

// CreateReimbTool Wire 命名类型（Blades tools.Tool）
type CreateReimbTool struct{ tools.Tool }

// NewCreateReimbTool 创建报销单工具，封装 reimbursement.ReimbursementBiz
func NewCreateReimbTool(reimbursementBiz *reimbursement.ReimbursementBiz, logger *log.Logger) *CreateReimbTool {
	t, err := tools.NewFunc[CreateReimbInput, CreateReimbOutput](
		"create_reimbursement",
		"在系统中创建报销单草稿（状态=draft）。返回报销单ID和单号，后续工具（submit_reimbursement、generate_pdf、send_email）需要此ID。每个报销流程只需调用一次。",
		func(ctx context.Context, input CreateReimbInput) (CreateReimbOutput, error) {
			logger.Debug("创建报销单工具开始执行",
				zap.String("工号", input.EmployeeID),
				zap.String("姓名", input.EmployeeName),
				zap.Uint("部门ID", input.DepartmentID))

			rm, err := reimbursementBiz.Create(input.EmployeeID, input.EmployeeName, input.DepartmentID, input.SubmitNote)
			if err != nil {
				logger.Error("创建报销单失败", zap.Error(err))
				return CreateReimbOutput{}, fmt.Errorf("创建报销单失败: %w", err)
			}

			logger.Info("报销单创建成功",
				zap.Uint("报销单ID", rm.ID),
				zap.String("报销单号", rm.ReimbursementNo),
				zap.String("状态", rm.Status))

			return CreateReimbOutput{
				ReimbursementID: rm.ID,
				ReimbursementNo: rm.ReimbursementNo,
				Status:          rm.Status,
			}, nil
		},
	)
	if err != nil {
		panic("创建reimbursement工具失败: " + err.Error())
	}
	logger.Debug("创建报销单工具初始化完成")
	return &CreateReimbTool{t}
}
