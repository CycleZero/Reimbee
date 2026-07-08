package tools

import (
	"context"
	"fmt"

	"github.com/CycleZero/Reimbee/internal/domain/reimbursement"
	"github.com/CycleZero/Reimbee/log"
	"github.com/CycleZero/blades/tools"
	"go.uber.org/zap"
)

// QueryInput query_reimbursements 工具的输入参数
type QueryInput struct {
	EmployeeID string `json:"employee_id"` // 员工工号（可选，为空时查全员）
	Page       int    `json:"page"`        // 页码，从1开始
	PageSize   int    `json:"page_size"`   // 每页数量，默认5条
}

// QueryOutput query_reimbursements 工具的输出结果
type QueryOutput struct {
	List  []ReimbursementSummary `json:"list"`  // 报销单摘要列表
	Total int64                  `json:"total"` // 总记录数
}

// ReimbursementSummary 报销单摘要（精简字段，供 Agent 格式化展示）
type ReimbursementSummary struct {
	No          string `json:"no"`           // 报销单号
	Status      string `json:"status"`       // 状态
	TotalAmount int64  `json:"total_amount"`  // 报销总金额（分）
	CreatedAt   string `json:"created_at"`   // 创建时间
}

// QueryTool Wire 命名类型（Blades tools.Tool）
type QueryTool struct{ tools.Tool }

// NewQueryTool 创建报销记录查询工具，封装 reimbursement.ReimbursementBiz
func NewQueryTool(reimbursementBiz *reimbursement.ReimbursementBiz, logger *log.Logger) *QueryTool {
	t, err := tools.NewFunc[QueryInput, QueryOutput](
		"query_reimbursements",
		"查询用户的报销记录列表，支持分页。可用于查看历史报销单、检查重复提交等场景。返回报销单号、状态、金额和创建时间等摘要信息",
		func(ctx context.Context, input QueryInput) (QueryOutput, error) {
			logger.Debug("报销记录查询工具开始执行",
				zap.String("工号", input.EmployeeID),
				zap.Int("页码", input.Page),
				zap.Int("每页数量", input.PageSize))

			// 默认值处理
			if input.Page <= 0 {
				input.Page = 1
			}
			if input.PageSize <= 0 {
				input.PageSize = 5
			}

			rms, total, err := reimbursementBiz.List(input.Page, input.PageSize, input.EmployeeID)
			if err != nil {
				logger.Error("查询报销单列表失败", zap.Error(err))
				return QueryOutput{}, fmt.Errorf("查询报销单失败: %w", err)
			}

			// 转换为摘要结构
			items := make([]ReimbursementSummary, 0, len(rms))
			for _, rm := range rms {
				items = append(items, ReimbursementSummary{
					No:          rm.ReimbursementNo,
					Status:      rm.Status,
					TotalAmount: rm.TotalAmount,
					CreatedAt:   rm.CreatedAt.Format("2006-01-02 15:04:05"),
				})
			}

			logger.Info("报销记录查询完成",
				zap.Int64("总数", total),
				zap.Int("返回数", len(items)),
			)

			return QueryOutput{List: items, Total: total}, nil
		},
	)
	if err != nil {
		panic("创建报销记录查询工具失败: " + err.Error())
	}
	logger.Debug("报销记录查询工具初始化完成")
	return &QueryTool{t}
}
