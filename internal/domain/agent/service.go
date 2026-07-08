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

// HandleChat SSE 流式对话端点
// @Summary Agent 流式对话（SSE）
// @Description 通过 Server-Sent Events 与报销智能助手进行实时对话。
// @Description 支持多轮对话上下文，自动识别票据、合规审核、预算检查、提交报销。
// @Description
// @Description SSE 事件类型：
// @Description   - thinking:    LLM 思考中
// @Description   - reasoning:   LLM 推理过程（DeepSeek-R1 等模型）
// @Description   - message:     LLM 文本输出（delta=true 为流式增量，false 为完整消息）
// @Description   - tool_call:   工具调用开始（工具名 + 输入参数）
// @Description   - tool_result: 工具调用结果（工具名 + 输出）
// @Description   - interrupted: 等待审批确认
// @Description   - error:       错误
// @Description   - done:        本轮对话完成
// @Description
// @Description 同一 sessionID 的多次请求共享对话上下文和报销流程状态。
// @Tags Agent对话
// @Accept json
// @Produce text/event-stream
// @Param session_id query string true "会话ID（UUID），同一会话多次请求复用此ID"
// @Param message query string true "用户消息内容，支持自然语言（中文）"
// @Param Authorization header string true "Bearer JWT Token"
// @Success 200 {string} string "SSE 事件流（text/event-stream）"
// @Failure 400 {object} map[string]interface{} "缺少必填参数（session_id 或 message）"
// @Failure 401 {object} map[string]interface{} "未认证（JWT 无效或过期）"
// @Failure 500 {object} map[string]interface{} "服务器不支持流式响应"
// @Router /api/chat/stream [get]
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

// HandleApprove 审批中断恢复
// @Summary 审批中断恢复
// @Description 对中断等待确认的操作进行审批（确认/拒绝），审批后 Agent 继续执行。
// @Description 中断由工具在需要用户确认时触发（如提交报销单），前端弹出确认框。
// @Description 用户确认后调用此接口，Agent 从断点恢复继续执行。
// @Tags Agent对话
// @Accept json
// @Produce text/event-stream
// @Param request body object{approved=bool,reason=string,session_id=string} true "审批请求：approved=是否确认 reason=理由 session_id=会话ID"
// @Param Authorization header string true "Bearer JWT Token"
// @Success 200 {string} string "SSE 事件流（恢复执行结果）"
// @Failure 400 {object} map[string]interface{} "请求参数错误"
// @Failure 500 {object} map[string]interface{} "服务器不支持流式响应"
// @Router /api/chat/approve [post]
func (s *AgentService) HandleApprove(c *gin.Context) {
	var req struct {
		SessionID string `json:"session_id"`
		Approved  bool   `json:"approved"`
		Reason    string `json:"reason"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	writer, err := NewGinSSEWriter(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "服务器不支持流式响应"})
		return
	}

	s.logger.Info("审批恢复", zap.String("sessionID", req.SessionID),
		zap.Bool("approved", req.Approved))

	if err := s.agent.HandleApprove(c, req.SessionID, req.Approved, req.Reason, writer); err != nil {
		s.logger.Error("审批恢复失败", zap.Error(err))
	}
}

// ListSessions 游标分页查询当前用户的会话列表
// @Summary 获取会话列表
// @Description 游标分页查询当前用户的会话历史，按更新时间倒序
// @Tags Agent对话
// @Accept json
// @Produce json
// @Param cursor query string false "游标（上一页最后一条的 updated_at，首次传空）"
// @Param limit query int false "每页数量，默认20，最大50"
// @Param Authorization header string true "Bearer JWT Token"
// @Success 200 {object} ListSessionsResponse "会话列表"
// @Failure 500 {object} map[string]interface{} "查询失败"
// @Router /api/chat/sessions [get]
func (s *AgentService) ListSessions(c *gin.Context) {
	meta := common.GetRequestMetadata(c)
	cursor := c.Query("cursor")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	resp, err := s.agent.ListSessions(c, meta.UserID, cursor, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败"})
		return
	}
	c.JSON(http.StatusOK, resp)
}

// GetHistory 获取会话消息历史
// @Summary 获取会话消息历史
// @Description 获取指定会话的全部消息记录
// @Tags Agent对话
// @Accept json
// @Produce json
// @Param id path string true "会话ID"
// @Param Authorization header string true "Bearer JWT Token"
// @Success 200 {object} GetMessagesResponse "消息列表"
// @Failure 500 {object} map[string]interface{} "查询失败"
// @Router /api/chat/sessions/{id}/messages [get]
func (s *AgentService) GetHistory(c *gin.Context) {
	sessionID := c.Param("id")

	resp, err := s.agent.GetHistory(c.Request.Context(), sessionID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败"})
		return
	}
	c.JSON(http.StatusOK, resp)
}
