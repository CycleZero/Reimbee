package agent

import (
	"context"
	"fmt"
	"io"

	"github.com/CycleZero/Reimbee/infra"
	"github.com/CycleZero/Reimbee/log"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"go.uber.org/zap"
)

// ============================================
// AgentRunner — Graph 执行引擎 + 会话管理
// ============================================

// AgentRunner 智能体运行引擎，负责：
// 1. 管理编译后的 Root Graph 实例
// 2. 协调 Session 持久化（对话历史）+ Checkpoint 持久化（Graph 状态）
// 3. 执行 StreamChat 流程：加载历史 → 执行 Graph → SSE 流式输出 → 持久化
type AgentRunner struct {
	rootGraph    compose.Runnable[AgentInput, *schema.Message] // 编译后的顶层 Graph
	sessionStore infra.SessionStore                            // 对话历史持久化（MySQL + Redis 缓存）
	checkpoint   CheckpointStore                               // Graph 状态持久化（MySQL）
	config       *AgentConfig                                  // 运行时配置
	logger       *log.Logger
}

// NewAgentRunner 创建 AgentRunner 实例
func NewAgentRunner(
	rootGraph compose.Runnable[AgentInput, *schema.Message],
	sessionStore infra.SessionStore,
	checkpoint CheckpointStore,
	config *AgentConfig,
	logger *log.Logger,
) *AgentRunner {
	logger.Debug("初始化智能体运行引擎")
	return &AgentRunner{
		rootGraph:    rootGraph,
		sessionStore: sessionStore,
		checkpoint:   checkpoint,
		config:       config,
		logger:       logger,
	}
}

// ============================================
// StreamChat 核心方法
// ============================================

// StreamChat 执行一次对话交互，通过 SSE Writer 流式返回结果
// 流程：加载历史 → 构建 Input → 执行 Graph → 流式 SSE → 持久化
func (r *AgentRunner) StreamChat(ctx context.Context, input AgentInput, sseWriter SSEWriter) error {
	r.logger.Debug("开始执行对话",
		zap.String("sessionID", input.SessionID),
		zap.String("用户消息", truncateStr(input.Message, 50)),
	)

	// 1. 发送 thinking 事件
	_ = sseWriter.WriteEvent(NewThinkingEvent("正在理解您的需求..."))
	_ = sseWriter.Flush()

	// 2. 获取历史消息并注入到 Graph Input
	history, err := r.sessionStore.GetHistory(ctx, input.SessionID, r.config.MaxHistoryTurns*2)
	if err != nil {
		r.logger.Warn("获取对话历史失败，使用空历史", zap.Error(err))
		history = nil
	}

	// 3. 持久化用户消息
	userMsg := schema.UserMessage(input.Message)
	if err := r.sessionStore.SaveMessages(ctx, input.SessionID, []*schema.Message{userMsg}); err != nil {
		r.logger.Warn("保存用户消息失败", zap.Error(err))
	}

	r.logger.Debug("对话上下文准备完成",
		zap.Int("历史消息数", len(history)),
		zap.String("sessionID", input.SessionID),
	)

	// 4. 尝试流式执行 Graph
	stream, err := r.rootGraph.Stream(ctx, input)
	if err != nil {
		r.logger.Warn("Graph流式执行失败，降级为Invoke", zap.Error(err))
		return r.invokeFallback(ctx, input, sseWriter)
	}

	// 5. 流式读取 + SSE token-by-token 推送
	var fullContent string
	for {
		chunk, recvErr := stream.Recv()
		if recvErr != nil {
			if recvErr == io.EOF {
				break
			}
			r.logger.Error("流式读取失败", zap.Error(recvErr))
			_ = sseWriter.WriteEvent(NewErrorEvent("流式读取中断: "+recvErr.Error(), true, "stream_error"))
			_ = sseWriter.Flush()
			return fmt.Errorf("流式读取失败: %w", recvErr)
		}

		if chunk != nil && chunk.Content != "" {
			fullContent += chunk.Content
			_ = sseWriter.WriteEvent(NewMessageEvent(chunk.Content, true))
			_ = sseWriter.Flush()
		}
	}

	// 6. 持久化 assistant 完整回复
	if fullContent != "" {
		assistantPersist := schema.AssistantMessage(fullContent, nil)
		if err := r.sessionStore.SaveMessages(ctx, input.SessionID, []*schema.Message{assistantPersist}); err != nil {
			r.logger.Warn("保存assistant消息失败", zap.Error(err))
		}
	}

	// 7. 发送完成事件
	_ = sseWriter.WriteEvent(NewDoneEvent())
	_ = sseWriter.Flush()

	r.logger.Debug("流式对话执行完成",
		zap.String("sessionID", input.SessionID),
		zap.Int("回复长度", len(fullContent)))
	return nil
}

// invokeFallback 降级为 Invoke（非流式），当 Stream 不可用时使用
func (r *AgentRunner) invokeFallback(ctx context.Context, input AgentInput, sseWriter SSEWriter) error {
	assistantMsg, err := r.rootGraph.Invoke(ctx, input)
	if err != nil {
		r.logger.Error("Graph执行失败", zap.Error(err))
		_ = sseWriter.WriteEvent(NewErrorEvent(fmt.Sprintf("处理请求时出错: %v", err), true, "graph_error"))
		_ = sseWriter.Flush()
		return fmt.Errorf("Graph执行失败: %w", err)
	}

	if assistantMsg != nil && assistantMsg.Content != "" {
		_ = sseWriter.WriteEvent(NewMessageEvent(assistantMsg.Content, false))
		_ = sseWriter.Flush()

		assistantPersist := schema.AssistantMessage(assistantMsg.Content, nil)
		if err := r.sessionStore.SaveMessages(ctx, input.SessionID, []*schema.Message{assistantPersist}); err != nil {
			r.logger.Warn("保存assistant消息失败", zap.Error(err))
		}
	}

	_ = sseWriter.WriteEvent(NewDoneEvent())
	_ = sseWriter.Flush()
	return nil
}

// ============================================
// 辅助方法
// ============================================

// BuildAgentInput 从请求上下文构建 AgentInput
// employeeID 和 role 从 JWT claims 中提取，sessionID 由前端生成
func BuildAgentInput(sessionID, message, employeeID string, userID uint, role string) AgentInput {
	return AgentInput{
		SessionID:  sessionID,
		UserID:     userID,
		EmployeeID: employeeID,
		Role:       role,
		Message:    message,
	}
}

// truncateStr 截断字符串到指定长度，超过则追加 "..."
func truncateStr(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}
