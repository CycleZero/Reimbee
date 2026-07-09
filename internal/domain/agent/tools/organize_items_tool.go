// Package tools 票据归类工具（organize_items）
// 将待归类的 OCR 票据分配到报销明细中，持久化到数据库并更新会话状态
package tools

import (
	"context"
	"fmt"

	"github.com/CycleZero/Reimbee/infra"
	"github.com/CycleZero/Reimbee/internal/domain/agent/types"
	"github.com/CycleZero/Reimbee/internal/domain/reimbursement"
	"github.com/CycleZero/Reimbee/log"
	"github.com/CycleZero/Reimbee/model"
	"github.com/CycleZero/blades/tools"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// OrganizeItemsInput 归类票据的输入参数
type OrganizeItemsInput struct {
	ReimbursementID uint                 `json:"reimbursement_id"`
	Items           []OrganizeItemInput  `json:"items"`
}

// OrganizeItemInput 单条归类的明细输入
type OrganizeItemInput struct {
	Category       string `json:"category"`
	Description    string `json:"description"`
	ReceiptIndices []int  `json:"receipt_indices"` // 1-based 索引，对应 PendingReceipts 列表
}

// OrganizeItemsOutput 归类票据的输出结果
type OrganizeItemsOutput struct {
	ItemsAdded      int    `json:"items_added"`
	ReceiptsGrouped int    `json:"receipts_grouped"`
	PendingLeft     int    `json:"pending_left"`
	Summary         string `json:"summary"`
}

type OrganizeItemsTool struct{ tools.Tool }

func NewOrganizeItemsTool(
	itemRepo *reimbursement.ItemRepo,
	receiptRepo *reimbursement.ReceiptRepo,
	store infra.StateStore,
	logger *log.Logger,
) *OrganizeItemsTool {
	t, err := tools.NewFunc[OrganizeItemsInput, OrganizeItemsOutput](
		ToolOrganizeItems,
		"将待归类的OCR票据分配到报销明细中。接收明细类别、事由和票据索引列表（1-based），创建报销明细并更新票据归属。应在用户确认票据归类后、创建报销单前调用。",
		func(ctx context.Context, input OrganizeItemsInput) (OrganizeItemsOutput, error) {
			sid := getSessionID(ctx)

			var state types.ReimbursementState
			found, err := store.GetState(ctx, sid, infra.StateKeyReimbursement, &state)
			if err != nil {
				logger.Error("读取票据状态失败", zap.String("会话ID", sid), zap.Error(err))
				return OrganizeItemsOutput{}, fmt.Errorf("读取票据状态失败: %w", err)
			}
			if !found || len(state.PendingReceipts) == 0 {
				return OrganizeItemsOutput{
					Summary: "没有待归类的票据，请先使用 recognize_invoice 上传并识别票据。",
				}, nil
			}

			if input.ReimbursementID == 0 {
				return OrganizeItemsOutput{}, fmt.Errorf("报销单ID不能为空，请先创建报销单")
			}

			// 构建索引集合，检测重复归属
			usedIndices := make(map[int]bool)
			totalGrouped := 0

			for i, item := range input.Items {
				if len(item.ReceiptIndices) == 0 {
					logger.Warn("明细缺少票据索引", zap.Int("明细序号", i+1))
					continue
				}

				// 校验索引范围并收集票据
				var itemReceipts []types.ReceiptState
				var itemAmount int64
				for _, idx := range item.ReceiptIndices {
					if idx < 1 || idx > len(state.PendingReceipts) {
						return OrganizeItemsOutput{}, fmt.Errorf(
							"第%d条明细的票据索引%d超出范围（当前待归类票据共%d张）",
							i+1, idx, len(state.PendingReceipts))
					}
					if usedIndices[idx-1] {
						return OrganizeItemsOutput{}, fmt.Errorf(
							"票据索引%d被多条明细重复引用", idx)
					}
					usedIndices[idx-1] = true
					rct := state.PendingReceipts[idx-1]
					itemReceipts = append(itemReceipts, rct)
					itemAmount += rct.Amount
				}

				// 创建报销明细 DB 记录
				dbItem := &model.ReimbursementItem{
					ReimbursementID: input.ReimbursementID,
					Category:        item.Category,
					Amount:          itemAmount,
					Description:     item.Description,
				}
				if err := itemRepo.Create(dbItem); err != nil {
					logger.Error("创建报销明细失败", zap.Error(err))
					return OrganizeItemsOutput{}, fmt.Errorf("创建报销明细失败: %w", err)
				}

				// 更新对应票据的 ItemID
				for _, rct := range itemReceipts {
					if rct.DBID != 0 {
						if err := receiptRepo.Update(&model.Receipt{
							Model:  gormModel(rct.DBID),
							ItemID: dbItem.ID,
						}); err != nil {
							logger.Error("更新票据归属失败",
								zap.Uint("票据ID", rct.DBID),
								zap.Uint("明细ID", dbItem.ID),
								zap.Error(err))
							return OrganizeItemsOutput{}, fmt.Errorf("更新票据归属失败: %w", err)
						}
					}
				}

				// 写入会话状态
				state.Items = append(state.Items, types.ItemState{
					Category:    item.Category,
					Amount:      itemAmount,
					Description: item.Description,
					Receipts:    itemReceipts,
				})

				totalGrouped += len(itemReceipts)
				logger.Info("报销明细归类完成",
					zap.Int("明细序号", i+1),
					zap.String("类别", item.Category),
					zap.Int64("金额(分)", itemAmount),
					zap.Int("票据数", len(itemReceipts)))
			}

			// 移除已归类的票据
			newPending := make([]types.ReceiptState, 0, len(state.PendingReceipts))
			for i, rct := range state.PendingReceipts {
				if !usedIndices[i] {
					newPending = append(newPending, rct)
				}
			}
			state.PendingReceipts = newPending

			if err := store.SaveState(ctx, sid, infra.StateKeyReimbursement, &state); err != nil {
				logger.Error("保存票据状态失败", zap.String("会话ID", sid), zap.Error(err))
				return OrganizeItemsOutput{}, fmt.Errorf("保存票据状态失败: %w", err)
			}

			summary := fmt.Sprintf("成功归类%d张票据到%d条报销明细，剩余%d张票据待归类",
				totalGrouped, len(input.Items), len(state.PendingReceipts))

			logger.Info("票据归类完成",
				zap.Int("新增明细", len(input.Items)),
				zap.Int("归入票据", totalGrouped),
				zap.Int("剩余待归类", len(state.PendingReceipts)))

			return OrganizeItemsOutput{
				ItemsAdded:      len(input.Items),
				ReceiptsGrouped: totalGrouped,
				PendingLeft:     len(state.PendingReceipts),
				Summary:         summary,
			}, nil
		},
	)
	if err != nil {
		panic("创建票据归类工具失败: " + err.Error())
	}
	logger.Debug("票据归类工具初始化完成")
	return &OrganizeItemsTool{t}
}

// gormModel 根据 ID 构造 gorm.Model（用于 Update 时指定主键）
func gormModel(id uint) gorm.Model {
	return gorm.Model{ID: id}
}
