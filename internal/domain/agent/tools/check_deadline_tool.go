// Package tools 票据截止日期校验工具
package tools

import (
	"context"
	"fmt"
	"time"

	"github.com/CycleZero/Reimbee/infra"
	"github.com/CycleZero/Reimbee/internal/domain/agent/types"
	"github.com/CycleZero/Reimbee/log"
	"github.com/CycleZero/blades/tools"
	"go.uber.org/zap"
)

// CheckDeadlineInput 校验截止日期的输入参数
type CheckDeadlineInput struct {
	ValidityDays int `json:"validity_days"` // 有效期天数，0或未设置时默认为90天
}

// DeadlineResult 单张票据的截止日期校验结果
type DeadlineResult struct {
	Index         int     `json:"index"`          // 票据序号
	Category      string  `json:"category"`       // 费用类别
	Amount        float64 `json:"amount"`         // 金额（元）
	Date          string  `json:"date"`           // 开票日期
	DaysRemaining int     `json:"days_remaining"` // 剩余有效天数
	Status        string  `json:"status"`         // 状态：valid/approaching/expired/unknown
}

// DeadlineSummary 校验汇总信息
type DeadlineSummary struct {
	HasExpired    bool `json:"has_expired"`    // 是否存在已过期票据
	HasApproaching bool `json:"has_approaching"` // 是否存在即将过期票据
	HasUnknown    bool `json:"has_unknown"`    // 是否存在日期未知票据
	TotalCount    int  `json:"total_count"`    // 票据总数
}

// CheckDeadlineOutput 截止日期校验输出
type CheckDeadlineOutput struct {
	Results []DeadlineResult `json:"results"` // 逐票据校验结果
	Summary DeadlineSummary  `json:"summary"` // 汇总信息
}

// CheckDeadlineTool 校验已收集票据的开票日期是否在有效期内。
// 默认有效期为开票日期起90天内。
// Now 字段可在测试中替换为固定时间。
type CheckDeadlineTool struct {
	tools.Tool
	Now func() time.Time
}

// NewCheckDeadlineTool 创建截止日期校验工具实例
func NewCheckDeadlineTool(store infra.StateStore, logger *log.Logger) *CheckDeadlineTool {
	t := &CheckDeadlineTool{Now: time.Now}

	base, err := tools.NewFunc[CheckDeadlineInput, CheckDeadlineOutput](
		"check_deadline",
		"校验已收集票据的开票日期是否在有效期内。默认有效期为开票日期起90天内。返回每张票据的剩余天数、状态（valid有效/approaching即将过期/expired已过期/unknown未知）和汇总信息。金额以人民币元为单位展示。",
		func(ctx context.Context, input CheckDeadlineInput) (CheckDeadlineOutput, error) {
			sid := getSessionID(ctx)
			logger.Debug("截止日期校验开始", zap.String("sessionID", sid))

			validityDays := input.ValidityDays
			if validityDays <= 0 {
				validityDays = 90
			}

			// 读取报销状态
			var state types.ReimbursementState
			store.GetState(ctx, sid, infra.StateKeyReimbursement, &state)

			results := make([]DeadlineResult, 0, len(state.Invoices))
			summary := DeadlineSummary{TotalCount: len(state.Invoices)}

			today := t.Now()

			for i, inv := range state.Invoices {
				result := DeadlineResult{
					Index:    i,
					Category: inv.Category,
					Amount:   float64(inv.Amount) / 100.0, // 分→元
					Date:     inv.Date,
				}

				// 解析日期：尝试两种格式
				issueDate, err := time.Parse("2006-01-02", inv.Date)
				if err != nil {
					issueDate, err = time.Parse("2006/01/02", inv.Date)
				}
				if err != nil || inv.Date == "" {
					result.Status = "unknown"
					result.DaysRemaining = 0
					summary.HasUnknown = true
					results = append(results, result)
					logger.Warn("票据日期无法解析或为空", zap.Int("序号", i), zap.String("原始值", inv.Date))
					continue
				}

				// 未来日期视为未知
				if issueDate.After(today) {
					result.Status = "unknown"
					result.DaysRemaining = 0
					summary.HasUnknown = true
					results = append(results, result)
					logger.Warn("票据日期为未来日期", zap.Int("序号", i), zap.String("日期", inv.Date))
					continue
				}

				// 计算剩余天数
				daysSinceIssue := int(today.Sub(issueDate).Hours() / 24)
				daysRemaining := validityDays - daysSinceIssue

				result.DaysRemaining = daysRemaining
				switch {
				case daysRemaining < 0:
					result.Status = "expired"
					summary.HasExpired = true
				case daysRemaining <= 7:
					result.Status = "approaching"
					summary.HasApproaching = true
				default:
					result.Status = "valid"
				}

				results = append(results, result)
			}

			logger.Info("截止日期校验完成",
				zap.Int("票据总数", summary.TotalCount),
				zap.Bool("有过期", summary.HasExpired),
				zap.Bool("有即将过期", summary.HasApproaching),
				zap.Bool("有未知日期", summary.HasUnknown))

			return CheckDeadlineOutput{
				Results: results,
				Summary: summary,
			}, nil
		},
	)
	if err != nil {
		panic(fmt.Sprintf("创建check_deadline工具失败: %v", err))
	}
	t.Tool = base
	logger.Debug("截止日期校验工具初始化完成")
	return t
}
