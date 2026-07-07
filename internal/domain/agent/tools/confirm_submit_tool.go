package tools

import (
	"context"

	"github.com/CycleZero/Reimbee/infra"
	"github.com/CycleZero/Reimbee/internal/domain/agent/types"
	"github.com/CycleZero/Reimbee/log"
	"github.com/cloudwego/eino/components/tool/utils"
	"go.uber.org/zap"
)

// ConfirmSubmitInput confirm_submit 工具的输入参数
type ConfirmSubmitInput struct {
	Confirmed bool `json:"confirmed" jsonschema:"required" jsonschema_description:"用户是否最终确认提交报销单（提交后不可撤销）"`
}

// ConfirmSubmitOutput confirm_submit 工具的输出结果
type ConfirmSubmitOutput struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

// NewConfirmSubmitTool 创建最终确认提交工具，Phase 2 结束时调用
// 当 LLM 调用此工具且 Confirmed=true 时，设置 FinalConfirmed=true 并进入 Phase 3
func NewConfirmSubmitTool(store infra.SessionStore, logger *log.Logger) *ConfirmSubmitTool {
	t, err := utils.InferTool[ConfirmSubmitInput, ConfirmSubmitOutput](
		"confirm_submit",
		"用户最终确认提交报销单后调用此工具。此操作不可撤销——调用后进入Phase3执行提交阶段（创建报销单→提交审批→生成PDF→发送邮件）。仅在用户明确表示'确认提交'或类似表述时调用。",
		func(ctx context.Context, input ConfirmSubmitInput) (ConfirmSubmitOutput, error) {
			sessionID := getSessionIDFromCtx(ctx)
			logger.Debug("确认提交工具执行", zap.String("sessionID", sessionID), zap.Bool("确认", input.Confirmed))

			if sessionID == "" {
				logger.Warn("会话ID为空，跳过状态持久化")
			}

			var state types.ReimbursementState
			if sessionID != "" {
				if _, err := store.GetState(ctx, sessionID, infra.StateKeyReimbursement, &state); err != nil {
					logger.Warn("读取报销状态失败", zap.Error(err))
				}
			}

			if input.Confirmed {
				state.FinalConfirmed = true
				state.CurrentPhase = "phase3_execute"
				if sessionID != "" {
					if err := store.SaveState(ctx, sessionID, infra.StateKeyReimbursement, &state); err != nil {
						logger.Warn("保存报销状态失败", zap.Error(err))
					}
				}
				logger.Info("用户已最终确认，进入Phase3", zap.String("sessionID", sessionID))
				return ConfirmSubmitOutput{
					Status:  "confirmed",
					Message: "报销单已确认提交，正在执行创建和提交流程。",
				}, nil
			}
			return ConfirmSubmitOutput{
				Status:  "pending",
				Message: "请确认报销信息无误后再次确认提交。",
			}, nil
		},
	)
	if err != nil {
		panic("创建confirm_submit工具失败: " + err.Error())
	}
	logger.Debug("确认提交工具初始化完成")
	return &ConfirmSubmitTool{t}
}
