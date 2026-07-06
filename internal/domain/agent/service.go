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
	runner *AgentRunner
	logger *log.Logger
}

// NewAgentService 创建 Agent HTTP 服务实例
func NewAgentService(runner *AgentRunner, logger *log.Logger) *AgentService {
	logger.Debug("初始化Agent HTTP服务")
	return &AgentService{runner: runner, logger: logger}
}

// ============================================
// SSE 流式对话端点
// ============================================

// HandleChat 处理 SSE 流式对话请求
// @Summary Agent 流式对话
// @Description 通过 SSE（Server-Sent Events）与报销智能助手进行实时对话交互
// @Tags Agent对话
// @Accept json
// @Produce text/event-stream
// @Param session_id query string true "会话ID（UUID v7）"
// @Param message query string true "用户消息内容"
// @Success 200 {string} string "SSE事件流"
// @Failure 400 {object} map[string]interface{} "缺少必填参数"
// @Failure 500 {object} map[string]interface{} "服务器不支持流式响应"
// @Router /api/chat/stream [get]
func (s *AgentService) HandleChat(c *gin.Context) {
	// 1. 解析请求参数
	sessionID := c.Query("session_id")
	message := c.Query("message")

	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少 session_id 参数"})
		return
	}
	if message == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少 message 参数"})
		return
	}

	// 2. 从 JWT 中间件注入的 context 中提取用户信息
	userID := getUintFromContext(c, "user_id")
	employeeID := getStringFromContext(c, "employee_id")
	role := getStringFromContext(c, "role")

	// 3. 创建 SSE 写入器（自动设置响应头）
	sseWriter, err := NewGinSSEWriter(c)
	if err != nil {
		s.logger.Error("创建SSE写入器失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "服务器不支持流式响应"})
		return
	}

	// 4. 构建 AgentInput
	input := BuildAgentInput(sessionID, message, employeeID, userID, role)

	s.logger.Info("SSE对话请求",
		zap.String("sessionID", sessionID),
		zap.String("用户", employeeID),
		zap.String("消息", message[:min(len(message), 50)]),
	)

	// 5. 执行对话（流式返回 SSE 事件）
	if err := s.runner.StreamChat(c.Request.Context(), input, sseWriter); err != nil {
		s.logger.Error("对话执行失败", zap.Error(err))
		// 错误已通过 SSE error 事件发送，此处无需再次响应
		return
	}

	s.logger.Debug("SSE对话流结束", zap.String("sessionID", sessionID))
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
