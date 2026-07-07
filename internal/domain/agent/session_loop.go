// Package agent 智能体层 — SessionLoop 实现
// SessionLoop 封装单个会话的 adk.TurnLoop 生命周期，
// 通过三个核心回调（GenInput / PrepareAgent / OnAgentEvents）驱动对话流程
package agent

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/CycleZero/Reimbee/infra"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
	"go.uber.org/zap"
)

// ============================================
// SessionLoop 结构体
// ============================================

// SessionLoop 封装单个会话的 TurnLoop 实例及运行时元数据
// 每个 Session 拥有独立的 TurnLoop 后台 goroutine，通过 Push/Pull 模式与 HTTP handler 协作
type SessionLoop struct {
	turnLoop     *adk.TurnLoop[string, *schema.Message] // Eino 框架提供的事件驱动对话循环
	cancel       context.CancelFunc                      // 取消 Session 的后台 context
	lastActive   time.Time                               // 最近活跃时间，用于超时清理
	sessionID    string                                  // 会话唯一标识

	// SSE 写入器（每请求注册一次，请求结束后清除）
	writerMu     sync.Mutex
	activeWriter SSEWriter   // 当前活跃 HTTP 请求的 SSE 流写入器
	activeDoneCh chan error  // 信号通道：SSE 输出完成后通知 HTTP handler 返回
}

// ============================================
// createSessionLoop — 创建并启动 TurnLoop
// ============================================

// createSessionLoop 为指定会话创建 SessionLoop 并启动 TurnLoop 后台 goroutine
// 三个回调通过闭包捕获 sessionID，实现对 SessionStore 和 Phase Agent 的访问
// Store + CheckpointID 后续版本启用（用于审批中断/恢复场景）
func (m *LoopManager) createSessionLoop(sessionID string) *SessionLoop {
	ctx, cancel := context.WithCancel(context.Background())

	sl := &SessionLoop{
		sessionID:  sessionID,
		cancel:     cancel,
		lastActive: time.Now(),
	}

	cfg := adk.TurnLoopConfig[string, *schema.Message]{
		GenInput:      m.makeGenInput(sessionID),
		PrepareAgent:  m.makePrepareAgent(sessionID),
		OnAgentEvents: m.makeOnAgentEvents(sessionID),
		// Store + CheckpointID 后续版本启用（用于审批中断/恢复场景）
	}

	sl.turnLoop = adk.NewTurnLoop(cfg)
	m.logger.Debug("TurnLoop创建成功，启动后台goroutine",
		zap.String("sessionID", sessionID),
		zap.Bool("启用Checkpoint", false)) // 后续版本启用
	sl.turnLoop.Run(ctx) // 非阻塞启动后台 goroutine，等待 Push 触发首轮对话

	return sl
}

// ============================================
// registerWriter — 注册当前 HTTP 请求的 SSE 写入器
// ============================================

// registerWriter 保存活跃请求的 SSE 写入器和完成信号通道
// 在 PushMessage 中调用，供 OnAgentEvents 回调使用
func (sl *SessionLoop) registerWriter(writer SSEWriter) chan error {
	sl.writerMu.Lock()
	defer sl.writerMu.Unlock()
	sl.activeWriter = writer
	sl.activeDoneCh = make(chan error, 1)
	return sl.activeDoneCh
}

// ============================================
// getActiveWriter — 获取当前活跃请求的 SSE 写入器
// ============================================

// getActiveWriter 读取活跃的 SSE 写入器（不加锁，仅在单 goroutine 内安全调用）
// 由 OnAgentEvents 回调调用，运行在 TurnLoop 的 goroutine 中
func (sl *SessionLoop) getActiveWriter() SSEWriter {
	sl.writerMu.Lock()
	defer sl.writerMu.Unlock()
	return sl.activeWriter
}

// ============================================
// clearActiveWriter — 清除活跃请求并通知 HTTP handler
// ============================================

// clearActiveWriter 关闭 doneCh 并清除活跃写入器引用
// 在 OnAgentEvents 完成后调用，通知 HTTP handler SSE 流已结束
func (sl *SessionLoop) clearActiveWriter(err error) {
	sl.writerMu.Lock()
	defer sl.writerMu.Unlock()
	if sl.activeDoneCh != nil {
		sl.activeDoneCh <- err
		close(sl.activeDoneCh)
		sl.activeDoneCh = nil
	}
	sl.activeWriter = nil
}

// ============================================
// makeGenInput — GenInput 回调（加载历史 + 状态 + 消息构建）
// ============================================

// makeGenInput 返回 GenInput 闭包，负责每次 Turn 开始前的上下文准备：
//  1. 从 SessionStore 加载对话历史
//  2. 加载 ReimbursementState 并注入 context（供工具通过 ProcessState 访问）
//  3. 将用户消息持久化到 SessionStore
//  4. 构建 AgentInput 消息列表（历史 + 本轮消息）
func (m *LoopManager) makeGenInput(sessionID string) func(
	ctx context.Context,
	loop *adk.TurnLoop[string, *schema.Message],
	items []string,
) (*adk.GenInputResult[string, *schema.Message], error) {

	return func(ctx context.Context, _ *adk.TurnLoop[string, *schema.Message],
		items []string) (*adk.GenInputResult[string, *schema.Message], error) {

		// ── 1. 加载对话历史 ──
		history, err := m.store.GetHistory(ctx, sessionID, m.config.MaxHistoryTurns*2)
		if err != nil {
			m.logger.Warn("加载对话历史失败",
				zap.String("sessionID", sessionID),
				zap.Error(err))
			history = nil
		}

		// ── 2. 加载业务状态（首次为空，不需要提前创建）──
		var rs ReimbursementState
		found, _ := m.store.GetState(ctx, sessionID, infra.StateKeyReimbursement, &rs)
		if !found {
			// 首次对话：从 SessionStore 加载用户身份，填充 EmployeeID
			var identity map[string]any
			if ok, _ := m.store.GetState(ctx, sessionID, infra.StateKeyUserIdentity, &identity); ok {
				if eid, ok := identity["employee_id"].(string); ok {
					rs.EmployeeID = eid
				}
			}
			m.logger.Debug("首次对话，已注入用户身份",
				zap.String("sessionID", sessionID),
				zap.String("employeeID", rs.EmployeeID))
		}
		if found || rs.EmployeeID != "" {
			ctx = context.WithValue(ctx, StateContextKey{}, &rs)
		}

		// ── 2.5. 注入 sessionID 到 context（供工具调用 store.SaveState 使用）──
		ctx = context.WithValue(ctx, SessionIDContextKey{}, sessionID)

		// ── 3. 保存用户消息 ──
		for _, item := range items {
			userMsg := schema.UserMessage(item)
			if err := m.store.SaveMessages(ctx, sessionID, []*schema.Message{userMsg}); err != nil {
				m.logger.Warn("保存用户消息失败", zap.Error(err))
			}
		}

		// ── 4. 构建消息列表 ──
		msgs := make([]*schema.Message, 0, len(history)+len(items))
		msgs = append(msgs, history...)
		for _, item := range items {
			msgs = append(msgs, schema.UserMessage(item))
		}

		m.logger.Debug("GenInput完成",
			zap.String("sessionID", sessionID),
			zap.Int("历史消息数", len(history)),
			zap.Int("本轮消息数", len(items)),
			zap.String("当前阶段", rs.CurrentPhase))

		return &adk.GenInputResult[string, *schema.Message]{
			RunCtx:   ctx, // 传递注入了 StateContextKey 和 SessionIDContextKey 的 context
			Input:    &adk.AgentInput{Messages: msgs, EnableStreaming: true},
			Consumed: items,
		}, nil
	}
}

// ============================================
// makePrepareAgent — PrepareAgent 回调（意图分类 + 阶段路由）
// ============================================

// makePrepareAgent 返回 PrepareAgent 闭包，负责选择当前 Turn 使用的 Agent
//  1. 意图分类（关键词匹配）
//  2. 简单意图（进度/预算/政策/修改/通用）→ 直接返回对应 Agent
//  3. 报销流程 → 根据 ReimbursementState 选择 Phase Agent（物理隔离工具集）
func (m *LoopManager) makePrepareAgent(sessionID string) func(
	ctx context.Context,
	loop *adk.TurnLoop[string, *schema.Message],
	consumed []string,
) (adk.Agent, error) {

	return func(ctx context.Context, _ *adk.TurnLoop[string, *schema.Message],
		consumed []string) (adk.Agent, error) {

		// ── 1. 意图分类 ──
		route := classifyByKeywords(consumed[0])
		m.logger.Debug("意图分类",
			zap.String("sessionID", sessionID),
			zap.String("意图", route),
			zap.String("用户消息", consumed[0]))

		// ── 2. 简单意图 → 直接返回对应 Agent ──
		switch route {
		case "query_progress":
			return m.progressAgent, nil
		case "query_budget":
			return m.budgetAgent, nil
		case "policy_question":
			return m.policyAgent, nil
		case "modify_reimbursement":
			return m.modifyAgent, nil
		case "general_chat":
			return m.chatAgent, nil
		// "new_reimbursement" → 继续，根据 ReimbursementState 选择 Phase Agent
		}

		// ── 3. 报销流程：根据 ReimbursementState 选择 Phase Agent ──
		var rs ReimbursementState
		_, _ = m.store.GetState(ctx, sessionID, infra.StateKeyReimbursement, &rs)

		agent := m.selectPhaseAgent(&rs)
		m.logger.Info("选择Phase Agent",
			zap.String("sessionID", sessionID),
			zap.String("当前阶段", rs.CurrentPhase),
			zap.String("Agent", agent.Name(ctx)))
		return agent, nil
	}
}

// ============================================
// selectPhaseAgent — 根据状态选择 Phase Agent
// ============================================

// selectPhaseAgent 根据 ReimbursementState 各字段决定返回哪个 Phase Agent
// 这是流控的核心：通过不同的 Agent 实例实现工具集的物理隔离
//
// 状态流转：
//
//	Phase1 (信息收集) → Phase2 (校验确认) → Phase3 (执行提交) → ChatAgent (已提交)
func (m *LoopManager) selectPhaseAgent(rs *ReimbursementState) adk.Agent {
	switch {
	case rs.ReimbursementNo != "":
		if m.logger != nil {
			m.logger.Debug("阶段路由：已提交报销单，使用通用对话Agent",
				zap.String("报销单号", rs.ReimbursementNo))
		}
		return m.chatAgent
	case rs.FinalConfirmed:
		if m.logger != nil {
			m.logger.Debug("阶段路由：用户已最终确认，进入Phase3执行提交",
				zap.Int64("总金额(分)", rs.TotalAmount),
				zap.Int("票据数", len(rs.Invoices)))
		}
		return m.phase3Agent
	case rs.UserConfirmed:
		if m.logger != nil {
			m.logger.Debug("阶段路由：用户已确认票据，进入Phase2校验阶段",
				zap.Int("票据数", len(rs.Invoices)))
		}
		return m.phase2Agent
	default:
		if m.logger != nil {
			m.logger.Debug("阶段路由：默认进入Phase1信息收集阶段")
		}
		return m.phase1Agent
	}
}

// ============================================
// makeOnAgentEvents — OnAgentEvents 回调（Agent 事件 → SSE 输出）
// ============================================

// makeOnAgentEvents 返回 OnAgentEvents 闭包，负责消费 Agent 事件流并推送 SSE
//
// 事件处理流程：
//  1. 获取活跃请求的 SSE 写入器
//  2. 发送 thinking 事件（前端显示加载状态）
//  3. 循环消费事件流：
//     a. 检测 Preempt/Stop 信号（优先检查，确保及时响应框架控制）
//     b. 读取事件
//     c. CancelError 静默返回（框架已自动处理）
//     d. Assistant 事件 → 流式 SSE 推送 content
//     e. Tool 事件 → 推送工具结果
//  4. 持久化 assistant 完整回复
//  5. 发送 done 事件
//  6. 通知 HTTP handler 完成
func (m *LoopManager) makeOnAgentEvents(sessionID string) func(
	ctx context.Context,
	tc *adk.TurnContext[string, *schema.Message],
	events *adk.AsyncIterator[*adk.AgentEvent],
) error {

	return func(ctx context.Context, tc *adk.TurnContext[string, *schema.Message],
		events *adk.AsyncIterator[*adk.AgentEvent]) error {

		// ── 1. 获取活跃 SSE 写入器 ──
		sl := m.getSessionLoop(sessionID)
		if sl == nil {
			m.logger.Warn("会话已不存在，跳过事件输出",
				zap.String("sessionID", sessionID))
			return nil
		}
		writer := sl.getActiveWriter()
		if writer == nil {
			m.logger.Warn("SSE写入器未就绪，跳过事件输出",
				zap.String("sessionID", sessionID))
			return nil
		}

		// ── 2. 发送 thinking 事件 ──
		_ = writer.WriteEvent(NewThinkingEvent("正在处理..."))
		_ = writer.Flush()

		var fullContent string

		// ── 3. 消费事件流 ──
		for {
			// FIRST: 检查 Preempt / Stop 信号（必须优先！）
			select {
			case <-tc.Preempted:
				m.logger.Debug("当前Turn被Preempt",
					zap.String("sessionID", sessionID))
				// 框架会将 CancelError 吸收并自动开始新 Turn，这里只需返回 nil
				sl.clearActiveWriter(nil)
				return nil
			case <-tc.Stopped:
				m.logger.Debug("TurnLoop被Stop",
					zap.String("sessionID", sessionID))
				sl.clearActiveWriter(nil)
				return nil
			default:
			}

			event, ok := events.Next()
			if !ok {
				break // 事件流结束
			}

			// ── 3c. 错误处理 ──
			if event.Err != nil {
				// 框架已自动捕获 CancelError（Preempt/Stop），回调不应传播
				if errors.As(event.Err, new(*adk.CancelError)) {
					// 静默返回，框架会处理后续流程（Preempt→新Turn，Stop→退出）
					sl.clearActiveWriter(nil)
					return nil
				}
				// 真实错误才传播并在 SSE 中展示
				_ = writer.WriteEvent(NewErrorEvent(event.Err.Error(), false, "agent_error"))
				_ = writer.Flush()
				sl.clearActiveWriter(event.Err)
				return event.Err
			}

			// ── 3d. 跳过空输出 ──
			if event.Output == nil || event.Output.MessageOutput == nil {
				continue
			}

			mv := event.Output.MessageOutput

			switch mv.Role {
			case schema.Assistant:
				// LLM 文本输出（流式 or 完整）
				if mv.IsStreaming {
					// 流式：逐 chunk 推送增量
					for {
						chunk, err := mv.MessageStream.Recv()
						if err != nil {
							break
						}
						if chunk.Content != "" {
							fullContent += chunk.Content
							_ = writer.WriteEvent(NewMessageEvent(chunk.Content, true))
							_ = writer.Flush()
						}
					}
				} else if mv.Message != nil {
					fullContent = mv.Message.Content
					_ = writer.WriteEvent(NewMessageEvent(mv.Message.Content, false))
					_ = writer.Flush()
				}

			case schema.Tool:
				// 工具调用结果
				_ = writer.WriteEvent(NewToolResultEvent(mv.ToolName, mv.Message.Content))
				_ = writer.Flush()
			}
		}

		// ── 4. 持久化 assistant 完整回复 ──
		if fullContent != "" {
			assistantMsg := schema.AssistantMessage(fullContent, nil)
			if err := m.store.SaveMessages(ctx, sessionID, []*schema.Message{assistantMsg}); err != nil {
				m.logger.Warn("保存assistant消息失败", zap.Error(err))
			}
		}

		// ── 5. 发送完成事件 ──
		_ = writer.WriteEvent(NewDoneEvent())
		_ = writer.Flush()

		// ── 6. 通知 HTTP handler 流已结束 ──
		sl.clearActiveWriter(nil)

		m.logger.Debug("Turn事件消费完成",
			zap.String("sessionID", sessionID),
			zap.Int("回复长度", len(fullContent)))
		return nil
	}
}

// ============================================
// getSessionLoop — 安全获取 SessionLoop
// ============================================

// getSessionLoop 从 LoopManager 的会话表中安全获取 SessionLoop 指针
// 用于 OnAgentEvents 回调中：回调运行在 TurnLoop goroutine 中，通过闭包捕获的
// sessionID 从 LoopManager 注册表中查找对应的 SessionLoop
func (m *LoopManager) getSessionLoop(sessionID string) *SessionLoop {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.loops[sessionID]
}

// ============================================
// classifyByKeywords — 关键词意图分类（降级方案）
// ============================================

// classifyByKeywords 基于关键词对用户消息进行意图分类
// 源自 graph/root.go 的降级分类逻辑，作为 LLM 分类不可用时的可靠后备
//
// 注意：检查顺序很重要——更具体的意图（进度/预算/政策/修改）必须先于通用"报销"，
// 否则"我的报销进度到哪了"会被错误分类为 new_reimbursement
func classifyByKeywords(content string) string {
	content = truncateStr(content, 100)
	if containsAnyStr(content, "进度", "到哪", "批了吗", "状态", "审批") {
		return "query_progress"
	}
	if containsAnyStr(content, "预算", "还剩", "余额", "够不够") {
		return "query_budget"
	}
	if containsAnyStr(content, "标准", "规定", "多少", "可以报吗", "政策") {
		return "policy_question"
	}
	if containsAnyStr(content, "改", "修改", "重新提交", "驳回", "被退") {
		return "modify_reimbursement"
	}
	if containsAnyStr(content, "报销", "提交", "发票", "申请报销") {
		return "new_reimbursement"
	}
	return "general_chat"
}

// containsAnyStr 判断字符串 s 中是否包含 keywords 中的任意一个关键词
func containsAnyStr(s string, keywords ...string) bool {
	for _, kw := range keywords {
		if len(s) >= len(kw) {
			for i := 0; i <= len(s)-len(kw); i++ {
				if s[i:i+len(kw)] == kw {
					return true
				}
			}
		}
	}
	return false
}

// truncateStr 截断字符串到指定长度，超过则追加 "..."
func truncateStr(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}

// ============================================
// 说明：此文件通过 Makefile/编辑器保存时自动格式化，手工对齐是可选的
// ============================================
