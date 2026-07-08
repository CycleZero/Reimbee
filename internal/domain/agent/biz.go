// Package agent 业务逻辑层
//
// ReimburseAgent 基于 Blades Runner 封装报销对话流程：
//
//	Run() — 创建/加载 Session → Runner.RunStream() → 直接写 SSE 事件
//
// TODO: 审批中断恢复（待中断机制设计完成后实现）
package agent

import (
	"context"

	"github.com/CycleZero/Reimbee/infra"
	agenttools "github.com/CycleZero/Reimbee/internal/domain/agent/tools"
	"github.com/CycleZero/Reimbee/log"
	"github.com/CycleZero/blades"
	blades_tools "github.com/CycleZero/blades/tools"
	"go.uber.org/zap"
)

// ============================================
// ReimburseAgent
// ============================================

// ReimburseAgent 报销 Agent 核心业务逻辑
type ReimburseAgent struct {
	agent  blades.Agent
	runner *blades.Runner
	repo   *infra.SessionRepo
	config *Config
	logger *log.Logger
}

// NewReimburseAgent 创建报销 Agent 实例（Wire 兼容，失败 panic）
func NewReimburseAgent(
	model blades.ModelProvider,
	toolSet *agenttools.ToolSet,
	repo *infra.SessionRepo,
	config *Config,
	logger *log.Logger,
) *ReimburseAgent {
	toolList := collectTools(toolSet)

	agent, err := blades.NewAgent("reimburse_agent",
		blades.WithModel(NewLoggingModelProvider(model, logger.Logger)),
		blades.WithInstruction(buildSystemPrompt()),
		blades.WithDescription("企业报销全流程智能助手"),
		blades.WithTools(toolList...),
		blades.WithMaxIterations(15),
		blades.WithContext(true),
		blades.WithMiddleware(MessageLoggingMiddleware(logger.Logger)),
	)
	if err != nil {
		panic("创建Agent失败: " + err.Error())
	}

	logger.Info("ReimburseAgent初始化完成", zap.Int("工具数", len(toolList)))

	return &ReimburseAgent{
		agent:  agent,
		runner: blades.NewRunner(agent),
		repo:   repo,
		config: config,
		logger: logger,
	}
}

// ============================================
// RunParams
// ============================================

// RunParams 运行参数
type RunParams struct {
	SessionID    string
	Message      string
	UserID       uint
	EmployeeID   string
	EmployeeName string
	Role         string
}

// ============================================
// Run — 执行对话，直接写 SSE 事件
// ============================================

// Run 执行一轮对话，直接写 SSE 事件到 writer，并持久化会话
func (a *ReimburseAgent) Run(ctx context.Context, params RunParams, writer *GinSSEWriter) error {
	session, err := GetOrCreate(ctx, params.SessionID, a.repo)
	if err != nil {
		writer.WriteEvent(NewErrorEvent("加载会话失败: " + err.Error()))
		writer.Flush()
		return err
	}
	session.InjectUser(params.UserID, params.EmployeeID, params.EmployeeName, params.Role)

	// 将审批状态注入 context，供工具读取
	ctx = agenttools.InjectApprovalState(ctx, session.State())

	writer.WriteEvent(NewThinkingEvent("正在处理..."))
	writer.Flush()

	stream := a.runner.RunStream(ctx,
		blades.UserMessage(params.Message),
		blades.WithSession(session),
	)

	for msg, err := range stream {
		if err != nil {
			writer.WriteEvent(NewErrorEvent(err.Error()))
			writer.Flush()
			return err
		}

		switch msg.Role {
		case blades.RoleAssistant:
			for _, part := range msg.Parts {
				if rp, ok := any(part).(blades.ReasoningPart); ok && rp.Text != "" {
					writer.WriteEvent(NewReasoningEvent(rp.Text, msg.Status != blades.StatusCompleted))
					writer.Flush()
				}
			}
			if msg.Status == blades.StatusInProgress || msg.Status == blades.StatusIncomplete {
				if text := msg.Text(); text != "" {
					writer.WriteEvent(NewMessageEvent(text, true))
					writer.Flush()
				}
			} else if msg.Status == blades.StatusCompleted {
				writer.WriteEvent(NewMessageEvent(msg.Text(), false))
				writer.Flush()
			}

		case blades.RoleTool:
			// 检测中断信号
			if reason, ok := msg.Actions["await_approval"]; ok {
				writer.WriteEvent(NewInterruptedEvent(reason.(string)))
				writer.Flush()
				if err := a.repo.Save(ctx, session.Snapshot()); err != nil {
					a.logger.Warn("保存中断状态失败", zap.Error(err))
				}
				return nil
			}

			for _, part := range msg.Parts {
				if tp, ok := any(part).(blades.ToolPart); ok {
					if tp.Completed {
						writer.WriteEvent(NewToolResultEvent(tp.Name, tp.Response))
					} else {
						writer.WriteEvent(NewToolCallEvent(tp.Name, tp.Request))
					}
					writer.Flush()
				}
			}
		}
	}

	writer.WriteEvent(NewDoneEvent())
	writer.Flush()

	if err := a.repo.Save(ctx, session.Snapshot()); err != nil {
		a.logger.Warn("持久化会话失败", zap.Error(err))
	}

	return nil
}

// ============================================
// HandleApprove — 审批恢复
// ============================================

func (a *ReimburseAgent) HandleApprove(ctx context.Context, sessionID string, approved bool, reason string, writer *GinSSEWriter) error {
	session, err := GetOrCreate(ctx, sessionID, a.repo)
	if err != nil {
		return err
	}

	// 写入审批状态（Consumed=false，等待工具消费）
	session.SetState("approval", &agenttools.ApprovalState{
		Approved: approved,
		Reason:   reason,
		Consumed: false,
	})

	if err := a.repo.Save(ctx, session.Snapshot()); err != nil {
		return err
	}

	return a.Run(ctx, RunParams{SessionID: sessionID, Message: "继续"}, writer)
}

// ============================================
// 查询接口
// ============================================

func (a *ReimburseAgent) ListSessions(ctx context.Context, userID uint, cursor string, limit int) (*ListSessionsResponse, error) {
	result, err := a.repo.ListCursor(ctx, userID, cursor, limit)
	if err != nil {
		return nil, err
	}
	items := make([]SessionItem, 0, len(result.Sessions))
	for _, m := range result.Sessions {
		items = append(items, SessionItem{
			SessionID:    m.SessionID,
			Status:       m.Status,
			Summary:      m.Summary,
			MessageCount: m.MessageCount,
			CreatedAt:    m.CreatedAt,
			UpdatedAt:    m.UpdatedAt,
		})
	}
	return &ListSessionsResponse{
		Sessions:   items,
		NextCursor: result.NextCursor,
		HasMore:    result.HasMore,
	}, nil
}

func (a *ReimburseAgent) GetHistory(ctx context.Context, sessionID string) (*GetMessagesResponse, error) {
	session, err := GetOrCreate(ctx, sessionID, a.repo)
	if err != nil {
		return nil, err
	}
	msgs, _ := session.History(ctx)
	items := make([]MessageItem, 0, len(msgs))
	for _, msg := range msgs {
		item := MessageItem{Role: string(msg.Role)}
		item.Reasoning = msg.Reasoning()
		for _, part := range msg.Parts {
			if tp, ok := any(part).(blades.TextPart); ok {
				item.Content += tp.Text
			}
		}
		items = append(items, item)
	}
	return &GetMessagesResponse{Messages: items}, nil
}

// ============================================
// 工具聚合
// ============================================

func collectTools(ts *agenttools.ToolSet) []blades_tools.Tool {
	var list []blades_tools.Tool
	if ts.PDF != nil {
		list = append(list, ts.PDF)
	}
	if ts.Email != nil {
		list = append(list, ts.Email)
	}
	if ts.Progress != nil {
		list = append(list, ts.Progress)
	}
	if ts.QueryRecords != nil {
		list = append(list, ts.QueryRecords)
	}
	if ts.SearchPolicy != nil {
		list = append(list, ts.SearchPolicy)
	}
	if ts.CreateReimb != nil {
		list = append(list, ts.CreateReimb)
	}
	if ts.TestInterrupt != nil {
		list = append(list, ts.TestInterrupt)
	}
	return list
}
