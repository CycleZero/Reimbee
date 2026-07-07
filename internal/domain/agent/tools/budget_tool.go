package tools

import (
	"context"
	"fmt"
	"time"

	"github.com/CycleZero/Reimbee/infra"
	"github.com/CycleZero/Reimbee/internal/domain/agent/types"
	"github.com/CycleZero/Reimbee/internal/domain/budget"
	"github.com/CycleZero/Reimbee/log"
	"github.com/cloudwego/eino/components/tool/utils"
	"go.uber.org/zap"
)

// BudgetInput check_budget 工具的输入参数
type BudgetInput struct {
	DepartmentID uint  `json:"department_id" jsonschema:"required" jsonschema_description:"部门ID"`
	Amount       int64 `json:"amount" jsonschema:"required" jsonschema_description:"本次报销金额（分）"`
}

// BudgetOutput check_budget 工具的输出结果
type BudgetOutput struct {
	Remaining           int64   `json:"remaining"`            // 可用余额（分）
	NeedSpecialApproval bool    `json:"need_special_approval"` // 是否需要特殊审批（预算不足时触发）
	UsageRate           float64 `json:"usage_rate"`           // 部门预算使用率 0~1
}

// NewBudgetTool 创建预算检查工具，封装 budget.BudgetBiz
// store 参数为 v3.0 新增，后续版本工具将直接调用 store.SaveState 更新 ReimbursementState
func NewBudgetTool(budgetBiz *budget.BudgetBiz, store infra.SessionStore, logger *log.Logger) *BudgetTool {
	t, err := utils.InferTool[BudgetInput, BudgetOutput](
		"check_budget",
		"查询指定部门的当前财年预算余额。若本次报销金额超过可用余额，将触发特殊审批流程（need_special_approval=true）。返回可用余额、是否触发特殊审批、以及部门预算使用率",
		func(ctx context.Context, input BudgetInput) (BudgetOutput, error) {
			logger.Debug("预算检查工具开始执行",
				zap.Uint("部门ID", input.DepartmentID),
				zap.Int64("申请金额(分)", input.Amount))

			// 调用预算检查（Freeze/Deduct 由后来 Graph 节点执行，不在此工具内）
			remaining, needSpecial, err := budgetBiz.CheckBudget(input.DepartmentID, input.Amount)
			if err != nil {
				logger.Error("预算检查执行失败", zap.Uint("部门ID", input.DepartmentID), zap.Error(err))
				return BudgetOutput{}, fmt.Errorf("预算检查失败: %w", err)
			}

			// 计算预算使用率（需要从 Biz 获取年度总额，这里简化计算）
			// 实际使用率 = (已结算 + 已冻结) / 年度预算
			usageRate := calculateUsageRate(budgetBiz, input.DepartmentID)

			logger.Info("预算检查完成",
				zap.Int64("可用余额(分)", remaining),
				zap.Bool("需要特殊审批", needSpecial),
				zap.Float64("使用率", usageRate),
			)

			// v3.0: 持久化预算检查结果到 ReimbursementState
			if sid := getSessionIDFromCtx(ctx); sid != "" {
				var state types.ReimbursementState
				store.GetState(ctx, sid, infra.StateKeyReimbursement, &state)
				state.BudgetResult = &types.BudgetCheckResult{
					Remaining:           remaining,
					NeedSpecialApproval: needSpecial,
					UsageRate:           usageRate,
				}
				store.SaveState(ctx, sid, infra.StateKeyReimbursement, &state)
			}

			return BudgetOutput{
				Remaining:           remaining,
				NeedSpecialApproval: needSpecial,
				UsageRate:           usageRate,
			}, nil
		},
	)
	if err != nil {
		panic("创建预算检查工具失败: " + err.Error())
	}
	logger.Debug("预算检查工具初始化完成")
	return &BudgetTool{t}
}

// calculateUsageRate 计算部门预算使用率
// 公式：(已结算 + 已冻结) / 年度预算
func calculateUsageRate(biz *budget.BudgetBiz, deptID uint) float64 {
	// 通过获取看板数据来获取预算详情
	budgets, err := biz.GetDashboard(time.Now().Year()) // 当前财年
	if err != nil || len(budgets) == 0 {
		return 0
	}
	for _, b := range budgets {
		if b.DepartmentID == deptID && b.AnnualBudget > 0 {
			return float64(b.SpentAmount+b.FrozenAmount) / float64(b.AnnualBudget)
		}
	}
	return 0
}
