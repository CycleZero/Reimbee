package tools

import (
	"context"
	"fmt"

	"github.com/CycleZero/Reimbee/infra"
	"github.com/CycleZero/Reimbee/internal/domain/agent/types"
	"github.com/CycleZero/Reimbee/internal/domain/compliance"
	"github.com/CycleZero/Reimbee/log"
	"github.com/cloudwego/eino/components/tool/utils"
	"go.uber.org/zap"
)

// ComplianceInput check_compliance 工具的输入参数
type ComplianceInput struct {
	Amount      int64  `json:"amount" jsonschema:"required" jsonschema_description:"票据金额（分）"`
	Category    string `json:"category" jsonschema:"required" jsonschema_description:"费用类别: 差旅-交通/差旅-住宿/招待费/办公用品/印刷费/其他"`
	InvoiceDate string `json:"invoice_date" jsonschema:"required" jsonschema_description:"开票日期 YYYY-MM-DD"`
}

// ComplianceOutput check_compliance 工具的输出结果
type ComplianceOutput struct {
	Result  string `json:"result"`  // pass / warning / error
	Level   string `json:"level"`   // 与 result 相同，兼容字段
	Message string `json:"message"` // 检查结果描述（中文）
	RuleID  string `json:"rule_id"` // 触发的规则ID（用于审计追溯）
}

// NewComplianceTool 创建合规检查工具，封装 compliance.ComplianceBiz
// store 参数为 v3.0 新增，后续版本工具将直接调用 store.SaveState 更新 ReimbursementState
func NewComplianceTool(complianceBiz *compliance.ComplianceBiz, store infra.SessionStore, logger *log.Logger) *ComplianceTool {
	t, err := utils.InferTool[ComplianceInput, ComplianceOutput](
		"check_compliance",
		"检查票据金额是否符合企业报销政策标准（差旅住宿、交通、招待费、办公用品等），并检查发票是否在有效期内。返回 pass（通过）、warning（接近上限需审批确认）或 error（严重超限不可提交）",
		func(ctx context.Context, input ComplianceInput) (ComplianceOutput, error) {
			logger.Debug("合规检查工具开始执行",
				zap.Int64("金额(分)", input.Amount),
				zap.String("类别", input.Category),
				zap.String("开票日期", input.InvoiceDate))

			// 调用合规检查引擎（RAG 知识库检索 + 阈值比对）
			result, err := complianceBiz.CheckCompliance(ctx, &compliance.ComplianceInput{
				Amount:      input.Amount,
				Category:    input.Category,
				InvoiceDate: input.InvoiceDate,
			})
			if err != nil {
				logger.Error("合规检查执行失败", zap.Error(err))
				return ComplianceOutput{}, fmt.Errorf("合规检查失败: %w", err)
			}

			logger.Info("合规检查完成",
				zap.String("结果", result.Result),
				zap.String("级别", result.Level),
				zap.String("消息", result.Message),
				zap.String("规则ID", result.RuleID),
			)

			// v3.0: 持久化合规检查结果到 ReimbursementState
			if sid := getSessionIDFromCtx(ctx); sid != "" {
				var state types.ReimbursementState
				store.GetState(ctx, sid, infra.StateKeyReimbursement, &state)
				state.ComplianceResult = &types.ComplianceCheckResult{
					Result:  result.Result,
					Level:   result.Level,
					Message: result.Message,
					RuleID:  result.RuleID,
				}
				store.SaveState(ctx, sid, infra.StateKeyReimbursement, &state)
			}

			return ComplianceOutput{
				Result:  result.Result,
				Level:   result.Level,
				Message: result.Message,
				RuleID:  result.RuleID,
			}, nil
		},
	)
	if err != nil {
		panic("创建合规检查工具失败: " + err.Error())
	}
	logger.Debug("合规检查工具初始化完成")
	return &ComplianceTool{t}
}
