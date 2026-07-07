// Package agent 智能体层，基于 Eino ADK TurnLoop + ChatModelAgent 的对话式报销流程编排
package agent

import (
	"net/http"

	"github.com/CycleZero/Reimbee/log"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// ============================================
// AgentService — HTTP 服务层
// ============================================

// AgentService Agent 对话 HTTP 服务
// 提供 SSE 流式对话端点 /api/chat/stream
type AgentService struct {
	loopManager *LoopManager
	logger      *log.Logger
}

// NewAgentService 创建 Agent HTTP 服务实例
func NewAgentService(loopManager *LoopManager, logger *log.Logger) *AgentService {
	logger.Debug("初始化Agent HTTP服务")
	return &AgentService{loopManager: loopManager, logger: logger}
}

// ============================================
// SSE 流式对话端点
// ============================================

// HandleChat 处理 SSE 流式对话请求（v3.0 TurnLoop 模式）
//
// @Summary Agent 流式对话（SSE）
// @Description 通过 SSE（Server-Sent Events）与报销智能助手进行实时多轮对话交互。
//
//	支持完整的报销三阶段流程：
//	  Phase 1（信息收集）：上传票据 → OCR 识别 → 用户确认票据信息
//	  Phase 2（校验确认）：合规检查 → 预算检查 → 用户最终确认
//	  Phase 3（执行提交）：创建报销单 → 提交审批 → 生成 PDF → 发送邮件
//
//	每一轮对话返回以下 SSE 事件类型：
//	  - thinking:      LLM 思考中（含文字提示）
//	  - tool_call:     工具调用开始（工具名 + 输入参数）
//	  - tool_result:   工具调用结果（工具名 + 输出结果）
//	  - message:       LLM 文本输出（delta=true 为流式增量，delta=false 为完整消息）
//	  - phase_change:  报销阶段切换（from → to + 摘要）
//	  - confirm_required: 需要用户确认操作
//	  - error:         错误事件（含错误码、是否可重试）
//	  - done:          本轮对话完成
//
//	注意：本接口基于 TurnLoop 多轮运行时，
//	同一 sessionID 的多次请求共享对话上下文和报销流程状态，
//	支持 Preempt（抢占当前回答）和超时自动清理。
//
// @Tags Agent对话
// @Accept json
// @Produce text/event-stream
// @Param session_id query string true "会话ID（UUID v7），同一会话多次请求复用此 ID"
// @Param message query string true "用户消息内容，支持自然语言（中文）"
// @Param Authorization header string true "Bearer JWT Token（由 /api/auth/login 签发）"
// @Success 200 {string} string "SSE 事件流（text/event-stream）"
// @Failure 400 {object} map[string]interface{} "缺少必填参数（session_id 或 message）"
// @Failure 401 {object} map[string]interface{} "未认证（JWT 无效或过期）"
// @Failure 500 {object} map[string]interface{} "服务器不支持流式响应（http.Flusher 不可用）"
// @Failure 503 {object} map[string]interface{} "服务繁忙（TurnLoop 已停止，无法接收新消息）"
// @Router /api/chat/stream [get]
func (s *AgentService) HandleChat(c *gin.Context) {
	// ── 步骤 1: 解析查询参数 ──
	sessionID := c.Query("session_id")
	message := c.Query("message")

	s.logger.Debug("收到对话请求",
		zap.String("sessionID", sessionID),
		zap.String("消息(截断50字)", message[:min(len(message), 50)]))

	if sessionID == "" {
		s.logger.Warn("请求缺少session_id参数")
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少 session_id 参数"})
		return
	}
	if message == "" {
		s.logger.Warn("请求缺少message参数", zap.String("sessionID", sessionID))
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少 message 参数"})
		return
	}

	// ── 步骤 2: 提取 JWT claims 中的用户身份信息 ──
	userID := getUintFromContext(c, "user_id")
	employeeID := getStringFromContext(c, "employee_id")
	role := getStringFromContext(c, "role")

	s.logger.Debug("用户身份已提取",
		zap.String("sessionID", sessionID),
		zap.String("employeeID", employeeID),
		zap.String("role", role),
		zap.Uint("userID", userID))

	// ── 步骤 3: 创建 SSE 写入器（自动设置 Content-Type: text/event-stream 等响应头）──
	sseWriter, err := NewGinSSEWriter(c)
	if err != nil {
		s.logger.Error("创建SSE写入器失败",
			zap.String("sessionID", sessionID),
			zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "服务器不支持流式响应"})
		return
	}
	s.logger.Debug("SSE写入器已创建，响应头已发送",
		zap.String("sessionID", sessionID))

	// ── 步骤 4: 创建完成通知通道 ──
	// doneCh 由 OnAgentEvents 回调在流式输出完成后关闭，
	// HTTP handler 通过 <-doneCh 阻塞等待，实现请求-响应同步
	doneCh := make(chan error, 1)

	s.logger.Info("开始执行对话",
		zap.String("sessionID", sessionID),
		zap.String("employeeID", employeeID),
		zap.String("消息(截断50字)", message[:min(len(message), 50)]))

	// ── 步骤 5: 向 TurnLoop 推送消息（异步）──
	// PushMessage 内部流程：
	//   a. GetOrCreate(sessionID) → 获取或创建 SessionLoop + TurnLoop
	//   b. 注册当前请求的 SSE writer 和 doneCh 到 SessionLoop
	//   c. turnLoop.Push(message) → 触发 TurnLoop 内部状态机:
	//      GenInput → PrepareAgent → Agent.Run → OnAgentEvents(SSE输出) → 完成
	s.loopManager.PushMessage(sessionID, message, sseWriter, doneCh)
	s.logger.Debug("消息已推送到TurnLoop，等待执行完成",
		zap.String("sessionID", sessionID))

	// ── 步骤 6: 阻塞等待本轮 Turn 完成 ──
	// OnAgentEvents 执行完毕后通过 clearActiveWriter 向 doneCh 发送 nil，
	// HTTP handler 收到信号后返回，SSE 连接关闭
	if err := <-doneCh; err != nil {
		s.logger.Error("对话执行失败",
			zap.String("sessionID", sessionID),
			zap.Error(err))
	} else {
		s.logger.Debug("对话执行成功",
			zap.String("sessionID", sessionID))
	}

	_ = userID
	_ = role
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

// ============================================
// 辅助函数
// ============================================

// getStringFromContext 从 Gin Context 中安全提取字符串值
func getStringFromContext(c *gin.Context, key string) string {
	val, exists := c.Get(key)
	if !exists {
		return ""
	}
	s, ok := val.(string)
	if !ok {
		return ""
	}
	return s
}

// getUintFromContext 从 Gin Context 中安全提取 uint 值
func getUintFromContext(c *gin.Context, key string) uint {
	val, exists := c.Get(key)
	if !exists {
		return 0
	}
	switch v := val.(type) {
	case uint:
		return v
	case float64:
		return uint(v)
	default:
		return 0
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
