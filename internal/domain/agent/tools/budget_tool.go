package tools

import (
	"context"
	"fmt"
	"time"

	"github.com/CycleZero/Reimbee/infra"
	"github.com/CycleZero/Reimbee/internal/domain/agent/types"
	"github.com/CycleZero/Reimbee/internal/domain/budget"
	"github.com/CycleZero/Reimbee/log"
	"github.com/CycleZero/blades/tools"
	"go.uber.org/zap"
)

type BudgetInput struct {
	DepartmentID uint  `json:"department_id"`
	Amount       int64 `json:"amount"`
}

type BudgetOutput struct {
	Remaining           int64   `json:"remaining"`
	NeedSpecialApproval bool    `json:"need_special_approval"`
	UsageRate           float64 `json:"usage_rate"`
}

type BudgetTool struct{ tools.Tool }

func NewBudgetTool(budgetBiz *budget.BudgetBiz, store infra.StateStore, logger *log.Logger) *BudgetTool {
	t, err := tools.NewFunc[BudgetInput, BudgetOutput](
		ToolCheckBudget,
		"查询指定部门的当前财年预算余额。若本次报销金额超过可用余额，将触发特殊审批流程（need_special_approval=true）。返回可用余额、是否触发特殊审批、以及部门预算使用率。",
		func(ctx context.Context, input BudgetInput) (BudgetOutput, error) {
			sid := getSessionID(ctx)
			logger.Debug("预算检查开始", zap.Uint("部门ID", input.DepartmentID), zap.Int64("金额(分)", input.Amount))

			remaining, needSpecial, err := budgetBiz.CheckBudget(input.DepartmentID, input.Amount)
			if err != nil {
				logger.Error("预算检查失败", zap.Error(err))
				return BudgetOutput{}, fmt.Errorf("预算检查失败: %w", err)
			}

			usageRate := calculateUsageRate(budgetBiz, input.DepartmentID)

			logger.Info("预算检查完成", zap.Int64("可用余额(分)", remaining), zap.Bool("特殊审批", needSpecial))

			// 持久化预算检查结果到 ReimbursementState
			var state types.ReimbursementState
			store.GetState(ctx, sid, infra.StateKeyReimbursement, &state)
			state.BudgetResult = &types.BudgetCheckResult{
				Remaining:           remaining,
				NeedSpecialApproval: needSpecial,
				UsageRate:           usageRate,
			}
			store.SaveState(ctx, sid, infra.StateKeyReimbursement, &state)

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

func calculateUsageRate(biz *budget.BudgetBiz, deptID uint) float64 {
	budgets, err := biz.GetDashboard(time.Now().Year())
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
