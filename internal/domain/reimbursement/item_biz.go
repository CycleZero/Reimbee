package reimbursement

import (
	"github.com/CycleZero/Reimbee/log"
	"github.com/CycleZero/Reimbee/model"
)

// ItemBiz 报销明细业务逻辑层
// 负责明细级别的业务规则校验（如明细金额不能超过票据金额之和等）
type ItemBiz struct {
	logger *log.Logger
	repo   *ItemRepo
}

// NewItemBiz 创建报销明细业务逻辑层实例
func NewItemBiz(logger *log.Logger, repo *ItemRepo) *ItemBiz {
	logger.Debug("初始化报销明细业务逻辑层")
	return &ItemBiz{logger: logger, repo: repo}
}

// CreateFromInputs 根据输入参数批量创建报销明细
func (b *ItemBiz) CreateFromInputs(inputs []ItemInput, reimbID uint) ([]*model.ReimbursementItem, error) {
	items := make([]*model.ReimbursementItem, 0, len(inputs))
	for _, in := range inputs {
		item := &model.ReimbursementItem{
			ReimbursementID: reimbID,
			Category:        in.Category,
			Amount:          in.Amount,
			Description:     in.Description,
		}
		items = append(items, item)
	}
	if err := b.repo.BatchCreate(items); err != nil {
		return nil, err
	}
	return items, nil
}

// GetByReimbursementID 根据报销单 ID 查询所有明细
func (b *ItemBiz) GetByReimbursementID(reimbID uint) ([]*model.ReimbursementItem, error) {
	return b.repo.GetByReimbursementID(reimbID)
}
