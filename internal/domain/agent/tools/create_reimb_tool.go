// Package tools 智能体工具层
// create_reimbursement 工具：在数据库中创建报销单草稿
package tools

import (
	"context"
	"fmt"

	"github.com/CycleZero/Reimbee/infra"
	"github.com/CycleZero/Reimbee/internal/domain/agent/types"
	"github.com/CycleZero/Reimbee/internal/domain/reimbursement"
	"github.com/CycleZero/Reimbee/log"
	"github.com/CycleZero/blades/tools"
	"go.uber.org/zap"
)

// CreateReimbInput 创建报销单草稿工具的输入参数
type CreateReimbInput struct {
	EmployeeID   string `json:"employee_id"`
	EmployeeName string `json:"employee_name"`
	DepartmentID uint   `json:"department_id"`
	SubmitNote   string `json:"submit_note"`
}

// CreateReimbOutput 创建报销单草稿工具的输出结果
type CreateReimbOutput struct {
	ReimbursementID uint   `json:"reimbursement_id"`
	ReimbursementNo string `json:"reimbursement_no"`
	Status          string `json:"status"`
}

type CreateReimbTool struct{ tools.Tool }

func NewCreateReimbTool(reimbursementBiz *reimbursement.ReimbursementBiz, store infra.StateStore, logger *log.Logger) *CreateReimbTool {
	t, err := tools.NewFunc[CreateReimbInput, CreateReimbOutput](
		ToolCreateReimb,
		"创建报销单草稿。用户表达报销意图时立即调用。返回草稿ID，后续工具需要此ID。",
		func(ctx context.Context, input CreateReimbInput) (CreateReimbOutput, error) {
			sid := getSessionID(ctx)
			logger.Debug("创建报销单草稿工具开始执行",
				zap.String("工号", input.EmployeeID),
				zap.String("姓名", input.EmployeeName),
				zap.Uint("部门ID", input.DepartmentID))

			// 创建报销单草稿（不含明细）
			rm, err := reimbursementBiz.Create(&reimbursement.CreateReimbInput{
				EmployeeID:   input.EmployeeID,
				EmployeeName: input.EmployeeName,
				DepartmentID: input.DepartmentID,
				SubmitNote:   input.SubmitNote,
			})
			if err != nil {
				logger.Error("创建报销单草稿失败", zap.Error(err))
				return CreateReimbOutput{}, fmt.Errorf("创建报销单草稿失败: %w", err)
			}

			// 更新 session state
			var state types.ReimbursementState
			store.GetState(ctx, sid, infra.StateKeyReimbursement, &state)
			state.ReimbursementID = rm.ID
			store.SaveState(ctx, sid, infra.StateKeyReimbursement, &state)

			logger.Info("报销单草稿创建成功",
				zap.Uint("ID", rm.ID),
				zap.String("单号", rm.ReimbursementNo),
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
	logger.Debug("创建报销单草稿工具初始化完成")
	return &CreateReimbTool{t}
}
