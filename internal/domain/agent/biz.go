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
	"fmt"

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
	agent    blades.Agent
	runner   *blades.Runner
	repo     *infra.SessionRepo
	tools    map[string]blades_tools.Tool // 全部工具实例（供 HandleApprove 重放）
	resolver *Resolver                    // 角色动态工具解析
	config   *Config
	logger   *log.Logger
}

// NewReimburseAgent 创建报销 Agent 实例（Wire 兼容，失败 panic）
func NewReimburseAgent(
	model blades.ModelProvider,
	toolSet *agenttools.ToolSet,
	repo *infra.SessionRepo,
	config *Config,
	logger *log.Logger,
) *ReimburseAgent {
	// 构建全量工具 map（供 HandleApprove 使用）
	toolList := collectTools(toolSet)
	toolMap := make(map[string]blades_tools.Tool, len(toolList))
	for _, t := range toolList {
		toolMap[t.Name()] = t
	}

	// 拆分工具：共用 + employee 专属 + approver 专属
	var shared, employee, approver []blades_tools.Tool
	for _, t := range toolList {
		switch t.Name() {
		case "recognize_invoice", "check_budget", "create_reimbursement",
			"submit_reimbursement", "cancel_reimbursement", "generate_pdf",
			"send_email", "get_department_id":
			employee = append(employee, t)
		case "approve_reimbursement", "reject_reimbursement", "list_pending":
			approver = append(approver, t)
		default:
			shared = append(shared, t) // search_policy, compliance, progress, query, detail
		}
	}

	resolver := NewResolver(shared, employee, approver)

	agent, err := blades.NewAgent("reimburse_agent",
		blades.WithModel(NewLoggingModelProvider(model, logger.Logger)),
		blades.WithInstructionProvider(BuildInstruction()),
		blades.WithDescription("企业报销全流程智能助手"),
		blades.WithToolsResolver(resolver),
		blades.WithMaxIterations(15),
		blades.WithContext(true),
		blades.WithMiddleware(MessageLoggingMiddleware(logger.Logger)),
	)
	if err != nil {
		panic("创建Agent失败: " + err.Error())
	}

	logger.Info("ReimburseAgent初始化完成",
		zap.Int("共享工具", len(shared)),
		zap.Int("员工工具", len(employee)),
		zap.Int("审批工具", len(approver)))

	return &ReimburseAgent{
		agent:    agent,
		runner:   blades.NewRunner(agent),
		repo:     repo,
		tools:    toolMap,
		resolver: resolver,
		config:   config,
		logger:   logger,
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
	Resume       bool // true=跳过 user msg 写入 session（仅 LLM 基于已有历史回复）
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
	if params.UserID != 0 {
		session.InjectUser(params.UserID, params.EmployeeID, params.EmployeeName, params.Role)
		// 首请求立即持久化用户身份，防止后续错误导致丢失
		if err := a.repo.Save(ctx, session.Snapshot()); err != nil {
			a.logger.Warn("保存用户身份失败", zap.Error(err))
		}
	}

	// 将审批状态注入 context，供工具读取
	ctx = agenttools.InjectApprovalState(ctx, session.State())
	// 将 sessionID 注入 context，供工具写 state
	ctx = agenttools.WithSessionID(ctx, params.SessionID)
	// 将角色元数据注入 context，供 Resolver + InstructionProvider 读取
	ctx = WithAgentMeta(ctx, &AgentMeta{Role: params.Role})

	writer.WriteEvent(NewThinkingEvent("正在处理..."))
	writer.Flush()

	stream := a.runner.RunStream(ctx,
		blades.UserMessage(params.Message),
		blades.WithSession(session),
		blades.WithResume(params.Resume),
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
				var toolName string
				for _, part := range msg.Parts {
					if tp, ok := any(part).(blades.ToolPart); ok {
						toolName = tp.Name
						session.SetState("pending_tool", map[string]any{
							"name":  tp.Name,
							"input": tp.Request,
						})
					}
				}

				// agent.handle 的 session.Append 在 yield 之后才执行，
				// 此时必须手动落库：克隆消息、剥离中断信号、存入 session
				clone := msg.Clone()
				delete(clone.Actions, "await_approval")
				delete(clone.Actions, "loop_exit")
				session.Append(ctx, clone)

				writer.WriteEvent(NewInterruptedEvent(toolName, reason.(string)))
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
						if tp.Name != "" {
							writer.WriteEvent(NewToolCallEvent(tp.Name, tp.Request))
						}
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

	// 读取中断工具信息
	pending, ok := session.State()["pending_tool"].(map[string]any)
	if !ok || pending["name"] == nil {
		writer.WriteEvent(NewErrorEvent("没有待审批的操作"))
		writer.Flush()
		return fmt.Errorf("会话 %s 没有待审批的操作", sessionID)
	}

	toolName, _ := pending["name"].(string)
	toolInput, _ := pending["input"].(string)

	// 先写入审批状态（工具重放前注入，Interruptable 检测到 Approved 才会真正执行）
	session.SetState("approval", &agenttools.ApprovalState{
		Approved: approved,
		Reason:   reason,
		Consumed: false,
	})
	ctx = agenttools.InjectApprovalState(ctx, session.State())

	// 重放被中断的工具：尝试更新旧 tool_msg 的 Response，找不到则追加新消息
	if t, ok := a.tools[toolName]; ok {
		result, err := t.Handle(ctx, toolInput)
		if err != nil {
			a.logger.Warn("重放工具失败", zap.String("tool", toolName), zap.Error(err))
		}
		msgs, _ := session.History(ctx)
		updated := false
		for _, msg := range msgs {
			if msg.Role == blades.RoleTool {
				for i, part := range msg.Parts {
					if tp, ok := any(part).(blades.ToolPart); ok && tp.Name == toolName && tp.Completed {
						tp.Response = result
						msg.Parts[i] = tp
						updated = true
						if err := a.repo.UpdateMessageBladesJSON(ctx, sessionID, toolName, msg); err != nil {
							a.logger.Warn("更新tool消息BladesJSON失败", zap.Error(err))
						}
					}
				}
			}
		}
		if !updated {
			// 旧消息被 Append 跳过（await_approval），追加新 tool result
			toolMsg := &blades.Message{
				Role: blades.RoleTool,
				Parts: []blades.Part{
					blades.ToolPart{
						Name: toolName, Request: toolInput,
						Response: result, Completed: true,
					},
				},
			}
			session.Append(ctx, toolMsg)
		}
	}

	if err := a.repo.Save(ctx, session.Snapshot()); err != nil {
		return err
	}

	return a.Run(ctx, RunParams{SessionID: sessionID, Resume: true}, writer)
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
			if tp, ok := any(part).(blades.ToolPart); ok {
				item.ToolName = tp.Name
				item.ToolCalls = append(item.ToolCalls, ToolCall{
					Name:      tp.Name,
					Arguments: tp.Request,
					Result:    tp.Response,
				})
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
	if ts.SearchDepartment != nil {
		list = append(list, ts.SearchDepartment)
	}
	if ts.SearchEmployee != nil {
		list = append(list, ts.SearchEmployee)
	}
	if ts.Compliance != nil {
		list = append(list, ts.Compliance)
	}
	if ts.CreateReimb != nil {
		list = append(list, ts.CreateReimb)
	}
	if ts.SubmitReimb != nil {
		list = append(list, ts.SubmitReimb)
	}
	if ts.ApproveReimb != nil {
		list = append(list, ts.ApproveReimb)
	}
	if ts.RejectReimb != nil {
		list = append(list, ts.RejectReimb)
	}
	if ts.PendingList != nil {
		list = append(list, ts.PendingList)
	}
	if ts.CancelReimb != nil {
		list = append(list, ts.CancelReimb)
	}
	if ts.DeptQuery != nil {
		list = append(list, ts.DeptQuery)
	}
	if ts.ReimbDetail != nil {
		list = append(list, ts.ReimbDetail)
	}
	if ts.OCR != nil {
		list = append(list, ts.OCR)
	}
	if ts.Budget != nil {
		list = append(list, ts.Budget)
	}
	if ts.TestInterrupt != nil {
		list = append(list, ts.TestInterrupt)
	}
	return list
}
