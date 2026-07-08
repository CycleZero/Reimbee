// Package agent HTTP 服务层——仅解析参数、调用 biz、返回响应
package agent

import (
	"net/http"
	"strconv"

	"github.com/CycleZero/Reimbee/internal/common"
	"github.com/CycleZero/Reimbee/log"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type AgentService struct {
	agent  *ReimburseAgent
	logger *log.Logger
}

func NewAgentService(agent *ReimburseAgent, logger *log.Logger) *AgentService {
	logger.Info("Agent HTTP服务初始化完成")
	return &AgentService{agent: agent, logger: logger}
}

func (s *AgentService) HandleChat(c *gin.Context) {
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

	meta := common.GetRequestMetadata(c)

	writer, err := NewGinSSEWriter(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "服务器不支持流式响应"})
		return
	}

	s.logger.Info("开始对话", zap.String("sessionID", sessionID),
		zap.String("employeeID", meta.EmployeeID))

	if err := s.agent.Run(c.Request.Context(), RunParams{
		SessionID:    sessionID,
		Message:      message,
		UserID:       meta.UserID,
		EmployeeID:   meta.EmployeeID,
		EmployeeName: meta.EmployeeID,
		Role:         meta.Role,
	}, writer); err != nil {
		s.logger.Error("对话失败", zap.Error(err))
	}
}

func (s *AgentService) ListSessions(c *gin.Context) {
	meta := common.GetRequestMetadata(c)
	cursor := c.Query("cursor")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	resp, err := s.agent.ListSessions(c.Request.Context(), meta.UserID, cursor, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败"})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (s *AgentService) GetHistory(c *gin.Context) {
	sessionID := c.Param("id")
	cursor, _ := strconv.Atoi(c.DefaultQuery("cursor", "0"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	resp, err := s.agent.GetHistory(c.Request.Context(), sessionID, uint(cursor), limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败"})
		return
	}
	c.JSON(http.StatusOK, resp)
}
