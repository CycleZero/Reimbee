package tools

import (
	"context"

	"github.com/CycleZero/Reimbee/infra"
	"github.com/CycleZero/Reimbee/internal/domain/agent/types"
	"github.com/CycleZero/Reimbee/log"
	"github.com/cloudwego/eino/components/tool/utils"
	"go.uber.org/zap"
)

// ConfirmInvoiceInput confirm_invoice 工具的输入参数
type ConfirmInvoiceInput struct {
	Confirmed bool `json:"confirmed" jsonschema:"required" jsonschema_description:"用户是否确认票据信息无误"`
}

// ConfirmInvoiceOutput confirm_invoice 工具的输出结果
type ConfirmInvoiceOutput struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

// NewConfirmInvoiceTool 创建票据确认工具，Phase 1 结束时调用
// 当 LLM 调用此工具且 Confirmed=true 时，设置 UserConfirmed=true 并进入 Phase 2
func NewConfirmInvoiceTool(store infra.SessionStore, logger *log.Logger) *ConfirmInvoiceTool {
	t, err := utils.InferTool[ConfirmInvoiceInput, ConfirmInvoiceOutput](
		"confirm_invoice",
		"用户确认票据信息无误后调用此工具。调用后Phase1结束，进入Phase2合规与预算校验阶段。仅在用户明确表示确认票据信息正确时调用。",
		func(ctx context.Context, input ConfirmInvoiceInput) (ConfirmInvoiceOutput, error) {
			sessionID := getSessionIDFromCtx(ctx)
			logger.Debug("确认票据工具执行", zap.String("sessionID", sessionID), zap.Bool("确认", input.Confirmed))

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
				state.UserConfirmed = true
				state.CurrentPhase = "phase2_validate"
				if sessionID != "" {
					if err := store.SaveState(ctx, sessionID, infra.StateKeyReimbursement, &state); err != nil {
						logger.Warn("保存报销状态失败", zap.Error(err))
					}
				}
				logger.Info("用户已确认票据，进入Phase2", zap.String("sessionID", sessionID))
				return ConfirmInvoiceOutput{
					Status:  "confirmed",
					Message: "票据信息已确认，进入合规与预算校验阶段。",
				}, nil
			}
			return ConfirmInvoiceOutput{
				Status:  "pending",
				Message: "请核对票据信息后再次确认。",
			}, nil
		},
	)
	if err != nil {
		panic("创建confirm_invoice工具失败: " + err.Error())
	}
	logger.Debug("确认票据工具初始化完成")
	return &ConfirmInvoiceTool{t}
}
