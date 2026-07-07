// Package agent 智能体层，基于 Eino ADK TurnLoop + ChatModelAgent 的对话式报销流程编排
//
// 本文件定义 LoopManager —— 管理所有 Session 的 TurnLoop 生命周期，
// 负责创建/销毁 SessionLoop、超时清理、优雅关闭
package agent

import (
	"context"
	"sync"
	"time"

	"github.com/CycleZero/Reimbee/infra"
	"github.com/CycleZero/Reimbee/internal/domain/agent/tools"
	"github.com/CycleZero/Reimbee/log"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"go.uber.org/zap"
)

// ============================================
// LoopManager —— Session 生命周期管理
// ============================================

// LoopManager 管理所有 Session 的 TurnLoop 生命周期
type LoopManager struct {
	mu    sync.Mutex
	loops map[string]*SessionLoop
	store infra.SessionStore // 消息 + 业务状态持久化

	// 预创建的 8 个 Agent 实例（initAgents 填充）
	phase1Agent   *adk.ChatModelAgent
	phase2Agent   *adk.ChatModelAgent
	phase3Agent   *adk.ChatModelAgent
	chatAgent     *adk.ChatModelAgent
	progressAgent *adk.ChatModelAgent
	budgetAgent   *adk.ChatModelAgent
	policyAgent   *adk.ChatModelAgent
	modifyAgent   *adk.ChatModelAgent

	chatModel model.ToolCallingChatModel // LLM 实例（用于 PrepareAgent 意图分类）

	logger *log.Logger
	config *LoopConfig
}

// LoopManagerDeps Wire 依赖注入用
type LoopManagerDeps struct {
	Store     infra.SessionStore
	ChatModel model.ToolCallingChatModel
	ToolSet   *tools.ToolSet
	Logger    *log.Logger
	Config    *LoopConfig
}

// ============================================
// NewLoopManager 构造函数
// ============================================

// NewLoopManager 创建并启动 LoopManager（Wire 自动注入）
// 创建所有 Phase Agent 和子流程 Agent，启动后台清理 goroutine
func NewLoopManager(
	store infra.SessionStore,
	chatModel model.ToolCallingChatModel,
	toolSet *tools.ToolSet,
	logger *log.Logger,
	config *LoopConfig,
) *LoopManager {
	if config == nil {
		config = DefaultLoopConfig()
		logger.Warn("LoopConfig 未提供，使用默认配置",
			zap.Duration("会话TTL", config.SessionTTL),
			zap.Duration("清理间隔", config.CleanupInterval))
	}

	m := &LoopManager{
		loops:     make(map[string]*SessionLoop),
		store:     store,
		logger:    logger,
		config:    config,
		chatModel: chatModel,
	}

	// 构建依赖结构体，传递给 initAgents
	deps := LoopManagerDeps{
		Store:     store,
		ChatModel: chatModel,
		ToolSet:   toolSet,
		Logger:    logger,
		Config:    config,
	}

	// 初始化所有 Agent（定义于 phase_agents.go）
	m.initAgents(context.Background(), deps)

	// 启动后台清理协程
	go m.cleanupLoop()

	m.logger.Info("LoopManager初始化完成",
		zap.Int("Phase_Agent数", 3),
		zap.Int("子流程Agent数", 5),
		zap.Int("活跃会话数", 0))

	return m
}

// ============================================
// Session 管理
// ============================================

// GetOrCreate 获取或创建 SessionLoop
// 已存在则更新 lastActive 时间戳并返回，不存在则创建新的 TurnLoop
func (m *LoopManager) GetOrCreate(sessionID string) *SessionLoop {
	m.mu.Lock()
	defer m.mu.Unlock()

	if sl, ok := m.loops[sessionID]; ok {
		sl.lastActive = time.Now()
		return sl
	}

	// 创建新的 SessionLoop（定义于 session_loop.go）
	sl := m.createSessionLoop(sessionID)
	m.loops[sessionID] = sl
	m.logger.Info("创建新会话TurnLoop",
		zap.String("sessionID", sessionID),
		zap.Int("活跃会话数", len(m.loops)))
	return sl
}

// PushMessage 向指定会话推送用户消息
// 获取或创建 SessionLoop，注册当前请求的 SSEWriter 和完成通知通道，然后 Push 到 TurnLoop
func (m *LoopManager) PushMessage(sessionID string, message string, sseWriter SSEWriter, doneCh chan error) {
	sl := m.GetOrCreate(sessionID)

	// 注册当前请求的 SSE 写入器和完成通知通道
	sl.writerMu.Lock()
	sl.activeWriter = sseWriter
	sl.activeDoneCh = doneCh
	sl.writerMu.Unlock()

	// Push 消息到 TurnLoop（异步执行，结果通过 doneCh 返回给调用方）
	sl.turnLoop.Push(message)
}

// InjectUserIdentity 注入用户身份信息到会话
// 每次请求时将 JWT/元数据中的用户身份保存到 SessionStore，
// 供 GenInput 加载 ReimbursementState 时自动填充 EmployeeID 字段
func (m *LoopManager) InjectUserIdentity(sessionID string, userID uint, employeeID, role string) {
	_ = m.GetOrCreate(sessionID) // 确保 SessionLoop 已创建

	identity := map[string]any{
		"user_id":     userID,
		"employee_id": employeeID,
		"role":        role,
	}
	if err := m.store.SaveState(context.Background(), sessionID, infra.StateKeyUserIdentity, &identity); err != nil {
		m.logger.Warn("保存用户身份失败",
			zap.String("sessionID", sessionID),
			zap.Error(err))
		return
	}
	m.logger.Debug("用户身份已注入会话",
		zap.String("sessionID", sessionID),
		zap.String("employeeID", employeeID),
		zap.String("role", role))
}

// ============================================
// cleanupLoop — 后台超时清理
// ============================================

// cleanupLoop 后台 goroutine，定期检查并清理超时的空闲会话
// 按 config.CleanupInterval 间隔执行，对超过 SessionTTL 的会话执行 Stop + cancel + delete
func (m *LoopManager) cleanupLoop() {
	ticker := time.NewTicker(m.config.CleanupInterval)
	defer ticker.Stop()

	for range ticker.C {
		m.mu.Lock()
		for id, sl := range m.loops {
			if time.Since(sl.lastActive) > m.config.SessionTTL {
				m.logger.Info("清理超时会话",
					zap.String("sessionID", id),
					zap.Duration("空闲时长", time.Since(sl.lastActive)))

				// 优雅停止（等待当前执行完成）
				sl.turnLoop.Stop(adk.WithGracefulTimeout(5 * time.Second))
				sl.cancel()
				delete(m.loops, id)
			}
		}
		count := len(m.loops)
		m.mu.Unlock()

		if count > 0 {
			m.logger.Debug("后台清理检查完成", zap.Int("活跃会话数", count))
		}
	}
}

// ============================================
// Shutdown — 优雅关闭
// ============================================

// Shutdown 停止所有活跃会话，释放资源
// 应在程序退出前调用，确保所有 TurnLoop 正确停止
func (m *LoopManager) Shutdown() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.logger.Info("LoopManager开始优雅关闭", zap.Int("待关闭会话数", len(m.loops)))

	for id, sl := range m.loops {
		// 优雅停止：等待当前执行完成（最多 10s）
		sl.turnLoop.Stop(adk.WithGracefulTimeout(10 * time.Second))
		sl.cancel()
		m.logger.Info("关闭会话", zap.String("sessionID", id))
	}

	m.logger.Info("LoopManager已关闭，所有会话已停止")
}
