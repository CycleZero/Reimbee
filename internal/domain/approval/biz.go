package approval

import (
	"fmt"
	"time"

	"github.com/CycleZero/Reimbee/log"
	"github.com/CycleZero/Reimbee/model"
	"go.uber.org/zap"
)

// ApprovalBiz 审批业务逻辑层
// 负责审批链的创建、审批通过/驳回操作、审批进度查询等核心业务逻辑
// 所有审批状态变更均通过本层处理，确保业务规则一致性
type ApprovalBiz struct {
	logger *log.Logger // 结构化日志记录器，用于记录审批操作的审计轨迹
	repo   *ApprovalRepo // 审批数据访问层，封装所有数据库 CRUD 操作
}

// NewApprovalBiz 创建审批业务逻辑层实例
// 通过依赖注入接收日志器和数据访问层，确保各层之间松耦合
func NewApprovalBiz(logger *log.Logger, repo *ApprovalRepo) *ApprovalBiz {
	// 记录初始化事件，便于追踪业务模块的启动时机
	logger.Debug("初始化审批业务逻辑层")
	// 将依赖注入到结构体中——日志器用于全链路追踪，repo 用于数据持久化
	return &ApprovalBiz{logger: logger, repo: repo}
}

// CreateApprovalChain 为一笔报销单创建审批链（多位审批人）
// 遍历审批人列表，为每位审批人生成一条状态为"待审批"的记录
// 所有记录关联到同一报销单ID，形成完整的多级审批链
func (b *ApprovalBiz) CreateApprovalChain(reimbursementID uint, approvers []*model.Employee) error {
	b.logger.Debug("开始创建审批链", zap.Uint("报销单ID", reimbursementID), zap.Int("审批人数", len(approvers)))

	// 防御性校验：审批链至少需要一位审批人，否则报销流程无法推进
	if len(approvers) == 0 {
		b.logger.Warn("审批人列表为空，无法创建审批链", zap.Uint("报销单ID", reimbursementID))
		return fmt.Errorf("至少需要指定一位审批人")
	}

	// 遍历审批人列表，逐条创建审批记录——每条记录代表审批链中的一个审批节点
	for _, approver := range approvers {
		// 调试日志：记录当前正在为哪位审批人创建记录，便于排查个别节点失败问题
		b.logger.Debug("正在为审批人创建审批记录",
			zap.Uint("报销单ID", reimbursementID),
			zap.String("审批人姓名", approver.Name),
			zap.String("审批人邮箱", approver.Email))

		// 构造审批记录模型：初始化状态为"待审批"，后续由 Approve/Reject 方法更新状态
		record := &model.ApprovalRecord{
			ReimbursementID: reimbursementID, // 关联到报销单，通过外键维护审批关系
			ApproverName:    approver.Name,   // 审批人姓名，用于审批进度展示
			ApproverEmail:   approver.Email,  // 审批人邮箱，用于发送审批通知
			Action:          model.ApprovalActionPending, // 初始状态：等待审批人操作
		}

		// 调用数据访问层将审批记录写入数据库
		if err := b.repo.Create(record); err != nil {
			// 创建失败意味着该审批节点的数据未能持久化，审批链不完整，必须终止并返回错误
			b.logger.Error("创建审批记录失败",
				zap.Uint("报销单ID", reimbursementID),
				zap.String("审批人", approver.Name),
				zap.String("审批人邮箱", approver.Email),
				zap.Error(err))
			return fmt.Errorf("创建审批记录失败: %w", err)
		}

		// 单条记录创建成功——继续处理下一位审批人
		b.logger.Debug("审批记录创建成功",
			zap.Uint("报销单ID", reimbursementID),
			zap.String("审批人", approver.Name),
			zap.Uint("审批记录ID", record.ID))
	}

	// 所有审批人记录均已成功创建，审批链完整可用
	b.logger.Info("审批链创建完成",
		zap.Uint("报销单ID", reimbursementID),
		zap.Int("审批人数", len(approvers)))
	return nil
}

// Approve 审批通过
// 将指定审批记录的状态从"待审批"更新为"已通过"，并记录审批时间和备注
// 前置条件：审批记录必须存在且状态为"待审批"，否则拒绝操作（幂等性保护）
func (b *ApprovalBiz) Approve(recordID uint, comment string) error {
	b.logger.Debug("开始执行审批通过操作", zap.Uint("审批记录ID", recordID), zap.String("审批备注", comment))

	// 第一步：根据审批记录ID从数据库查询当前记录，验证记录是否存在
	record, err := b.repo.GetByID(recordID)
	if err != nil {
		// 记录不存在无法进行审批，可能原因：ID 错误或记录已被删除
		b.logger.Warn("审批记录不存在，无法审批通过",
			zap.Uint("审批记录ID", recordID),
			zap.Error(err))
		return fmt.Errorf("审批记录不存在")
	}
	// 调试日志：确认记录已成功加载，输出当前状态用于后续状态校验的上下文
	b.logger.Debug("审批记录加载成功",
		zap.Uint("审批记录ID", recordID),
		zap.String("当前状态", record.Action),
		zap.String("审批人", record.ApproverName))

	// 第二步：状态校验——只有"待审批"状态的记录才能被审批通过
	// 这是幂等性保护：防止同一记录被重复审批，确保状态机的正确性
	if record.Action != model.ApprovalActionPending {
		b.logger.Warn("审批记录已处理，不可重复审批通过",
			zap.Uint("审批记录ID", recordID),
			zap.String("当前状态", record.Action),
			zap.String("审批人", record.ApproverName))
		return fmt.Errorf("该审批已处理（当前状态: %s），不可重复操作", record.Action)
	}

	// 第三步：更新记录字段——状态改为"已通过"，记录审批时间和备注
	now := time.Now()
	record.Action = model.ApprovalActionApproved // 状态变更：待审批 → 已通过
	record.Comment = comment                       // 记录审批人的备注意见，用于审计追溯
	record.ActionAt = &now                         // 记录审批操作时间，用于计算审批时效

	// 调试日志：输出即将持久化的关键字段，便于问题排查
	b.logger.Debug("准备更新审批记录为已通过状态",
		zap.Uint("审批记录ID", recordID),
		zap.String("新状态", model.ApprovalActionApproved),
		zap.Time("审批时间", now))

	// 第四步：将更新后的记录写回数据库，完成状态持久化
	if err := b.repo.Update(record); err != nil {
		// 数据库写入失败——审批操作实际未生效，需要返回错误让调用方重试或告警
		b.logger.Error("审批通过操作——数据库更新失败",
			zap.Uint("审批记录ID", recordID),
			zap.String("审批人", record.ApproverName),
			zap.Error(err))
		return fmt.Errorf("审批操作失败: %w", err)
	}

	// 审批通过操作完整成功——记录审计日志，便于后续追溯审批轨迹
	b.logger.Info("审批已通过，记录已更新",
		zap.Uint("审批记录ID", recordID),
		zap.String("审批人", record.ApproverName),
		zap.Time("审批时间", now))
	return nil
}

// Reject 驳回审批
// 将指定审批记录的状态从"待审批"更新为"已驳回"，强制要求提供驳回原因
// 驳回会中断报销流程，因此必须留下明确的驳回理由供报销人修改后重新提交
func (b *ApprovalBiz) Reject(recordID uint, reason string) error {
	b.logger.Debug("开始执行驳回审批操作", zap.Uint("审批记录ID", recordID), zap.String("驳回原因", reason))

	// 第一步：业务规则校验——驳回必须附带原因，否则报销人无法知道需要修改什么
	// 这个校验放在数据库查询之前，可以提前拦截无效请求，减少不必要的 IO
	if reason == "" {
		b.logger.Warn("驳回操作被拒绝：驳回原因不能为空",
			zap.Uint("审批记录ID", recordID))
		return fmt.Errorf("驳回时必须填写驳回原因")
	}
	b.logger.Debug("驳回原因校验通过", zap.Uint("审批记录ID", recordID))

	// 第二步：根据审批记录ID从数据库查询当前记录，验证记录是否存在
	record, err := b.repo.GetByID(recordID)
	if err != nil {
		// 记录不存在无法进行驳回操作，可能原因：ID 错误或记录已被删除
		b.logger.Warn("审批记录不存在，无法驳回",
			zap.Uint("审批记录ID", recordID),
			zap.Error(err))
		return fmt.Errorf("审批记录不存在")
	}
	// 调试日志：确认记录已成功加载，输出当前状态用于后续状态校验
	b.logger.Debug("审批记录加载成功",
		zap.Uint("审批记录ID", recordID),
		zap.String("当前状态", record.Action),
		zap.String("审批人", record.ApproverName))

	// 第三步：状态校验——只有"待审批"状态的记录才能被驳回
	// 这是与 Approve 方法一致的幂等性保护，防止重复驳回同一记录
	if record.Action != model.ApprovalActionPending {
		b.logger.Warn("审批记录已处理，不可重复驳回",
			zap.Uint("审批记录ID", recordID),
			zap.String("当前状态", record.Action),
			zap.String("审批人", record.ApproverName))
		return fmt.Errorf("该审批已处理（当前状态: %s），不可重复操作", record.Action)
	}

	// 第四步：更新记录字段——状态改为"已驳回"，驳回原因写入备注字段
	now := time.Now()
	record.Action = model.ApprovalActionRejected // 状态变更：待审批 → 已驳回
	record.Comment = reason                        // 驳回原因存入备注——报销人需要据此修改报销内容
	record.ActionAt = &now                         // 记录驳回操作时间，用于计算审批处理时长

	// 调试日志：输出即将持久化的关键字段
	b.logger.Debug("准备更新审批记录为已驳回状态",
		zap.Uint("审批记录ID", recordID),
		zap.String("新状态", model.ApprovalActionRejected),
		zap.String("驳回原因", reason),
		zap.Time("驳回时间", now))

	// 第五步：将更新后的记录写回数据库，完成驳回状态的持久化
	if err := b.repo.Update(record); err != nil {
		// 数据库写入失败——驳回操作实际未生效，需要返回错误让调用方重试或告警
		b.logger.Error("驳回审批操作——数据库更新失败",
			zap.Uint("审批记录ID", recordID),
			zap.String("审批人", record.ApproverName),
			zap.String("驳回原因", reason),
			zap.Error(err))
		return fmt.Errorf("驳回操作失败: %w", err)
	}

	// 驳回操作完整成功——记录审计日志，便于追溯驳回原因和处理时间
	b.logger.Info("审批已驳回，记录已更新",
		zap.Uint("审批记录ID", recordID),
		zap.String("审批人", record.ApproverName),
		zap.String("驳回原因", reason),
		zap.Time("驳回时间", now))
	return nil
}

// IsAllApproved 检查报销单的所有审批人是否都已完成审批（全部为"已通过"状态）
// 这是报销单进入下一流程（如财务打款）的前置条件
// 返回值：true 表示全部通过可继续流转，false 表示仍有待审批或已驳回的记录
func (b *ApprovalBiz) IsAllApproved(reimbursementID uint) (bool, error) {
	b.logger.Debug("开始检查审批是否全部通过", zap.Uint("报销单ID", reimbursementID))

	// 第一步：从数据库获取该报销单下的全部审批记录
	records, err := b.repo.ListByReimbursement(reimbursementID)
	if err != nil {
		// 查询失败意味着无法判断审批状态——保守处理返回 false 并上报错误
		b.logger.Error("查询审批记录失败，无法判断是否全部通过",
			zap.Uint("报销单ID", reimbursementID),
			zap.Error(err))
		return false, fmt.Errorf("查询审批记录失败: %w", err)
	}

	// 第二步：遍历每条审批记录，检查是否存在非"已通过"状态
	// 逐条遍历而非使用数据库聚合，是为了能精确记录是哪位审批人导致了未通过
	for _, r := range records {
		// 检查是否有审批人仍处于"待审批"状态——这意味着审批链尚未走完
		if r.Action == model.ApprovalActionPending {
			b.logger.Debug("仍有审批人未处理，审批未全部通过",
				zap.Uint("报销单ID", reimbursementID),
				zap.String("审批人", r.ApproverName),
				zap.Uint("审批记录ID", r.ID))
			return false, nil // 发现未处理记录，直接返回 false——尽早退出，无需继续遍历
		}
		// 检查是否有审批人已驳回——驳回意味着报销被拒绝，同样不是"全部通过"
		if r.Action == model.ApprovalActionRejected {
			b.logger.Debug("有审批人已驳回，审批未全部通过",
				zap.Uint("报销单ID", reimbursementID),
				zap.String("审批人", r.ApproverName),
				zap.Uint("审批记录ID", r.ID))
			return false, nil // 发现驳回记录，直接返回 false——驳回优先级高于待审批
		}
	}

	// 第三步：所有记录遍历完毕，未发现待审批或驳回——全部审批人均已通过
	b.logger.Info("所有审批人均已通过，审批链完整",
		zap.Uint("报销单ID", reimbursementID),
		zap.Int("审批记录总数", len(records)))
	return true, nil
}

// IsAnyRejected 检查是否有审批人驳回了报销
// 返回驳回状态、驳回原因（第一个被驳回的记录的原因）以及可能的错误
// 当报销单被驳回时，调用方可以取出驳回原因展示给报销人
func (b *ApprovalBiz) IsAnyRejected(reimbursementID uint) (bool, string, error) {
	b.logger.Debug("开始检查是否存在驳回记录", zap.Uint("报销单ID", reimbursementID))

	// 第一步：从数据库获取该报销单下的全部审批记录
	records, err := b.repo.ListByReimbursement(reimbursementID)
	if err != nil {
		// 查询失败意味着无法判断驳回状态——保守处理返回 false 并上报错误
		b.logger.Error("查询审批记录失败，无法判断是否存在驳回",
			zap.Uint("报销单ID", reimbursementID),
			zap.Error(err))
		return false, "", fmt.Errorf("查询审批记录失败: %w", err)
	}

	// 调试日志：输出查询到的记录总数，辅助判断空结果场景
	b.logger.Debug("审批记录查询完成",
		zap.Uint("报销单ID", reimbursementID),
		zap.Int("审批记录数", len(records)))

	// 第二步：遍历每条审批记录，查找是否存在"已驳回"状态的记录
	// 返回第一个被驳回的记录的原因——通常一个报销单只需一个驳回原因
	for _, r := range records {
		// 注意：这里使用字符串常量 "rejected" 与 model.ApprovalActionRejected 对应
		// 判断当前记录是否为驳回状态
		if r.Action == "rejected" {
			b.logger.Debug("发现驳回记录",
				zap.Uint("报销单ID", reimbursementID),
				zap.String("审批人", r.ApproverName),
				zap.String("驳回原因", r.Comment),
				zap.Uint("审批记录ID", r.ID))
			// 找到驳回记录后立即返回——取第一个驳回记录的原因作为整体驳回原因
			return true, r.Comment, nil
		}
	}

	// 第三步：遍历完毕未发现驳回记录——该报销单的审批链中尚无驳回
	b.logger.Debug("未发现驳回记录，报销单可继续审批流程",
		zap.Uint("报销单ID", reimbursementID))
	return false, "", nil
}

// GetProgress 获取报销单的审批进度
// 返回该报销单下所有审批记录的列表，包含每位审批人的状态、时间和备注
// 调用方（通常是 service 层）可根据此列表构建审批进度视图展示给用户
func (b *ApprovalBiz) GetProgress(reimbursementID uint) ([]*model.ApprovalRecord, error) {
	b.logger.Debug("开始查询审批进度", zap.Uint("报销单ID", reimbursementID))

	// 第一步：通过数据访问层查询该报销单关联的所有审批记录
	// 按创建时间排序，确保审批链的顺序正确展示
	records, err := b.repo.ListByReimbursement(reimbursementID)
	if err != nil {
		// 查询失败——可能是数据库连接问题或报销单ID无效
		b.logger.Error("查询审批进度失败",
			zap.Uint("报销单ID", reimbursementID),
			zap.Error(err))
		return nil, fmt.Errorf("查询审批进度失败: %w", err)
	}

	// 第二步：查询成功，返回审批记录列表
	// 即使 records 为空切片（该报销单没有审批记录），也正常返回——调用方自行处理空列表场景
	b.logger.Info("查询审批进度成功",
		zap.Uint("报销单ID", reimbursementID),
		zap.Int("审批记录数", len(records)))
	return records, nil
}

// ListPendingByApprover 根据审批人姓名查询其所有待审批记录
func (b *ApprovalBiz) ListPendingByApprover(approverName string) ([]*model.ApprovalRecord, error) {
	b.logger.Debug("查询审批人待审批列表", zap.String("审批人", approverName))
	records, err := b.repo.ListPendingByApprover(approverName)
	if err != nil {
		b.logger.Error("查询待审批列表失败", zap.String("审批人", approverName), zap.Error(err))
		return nil, fmt.Errorf("查询待审批列表失败: %w", err)
	}
	b.logger.Info("查询审批人待审批列表成功", zap.String("审批人", approverName), zap.Int("数量", len(records)))
	return records, nil
}
