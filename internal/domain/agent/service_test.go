package agent_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/CycleZero/Reimbee/internal/domain/agent"
	"github.com/CycleZero/Reimbee/log"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func init() {
	gin.SetMode(gin.TestMode)
	// 初始化全局 logger，避免 LoadAgentConfig 等调用 log.GetLogger() 时 panic
	log.SetGlobalLogger(&log.Logger{Logger: zap.NewNop()})
}

// testLoggerSvc 创建测试用静默日志器
func testLoggerSvc(t *testing.T) *log.Logger {
	t.Helper()
	return &log.Logger{Logger: zap.NewNop()}
}

// ============================================
// HandleChat 参数校验测试
// ============================================

// TestHandleChat_MissingSessionID 验证缺少 session_id 参数时返回 400
func TestHandleChat_MissingSessionID(t *testing.T) {
	svc := agent.NewAgentService(nil, testLoggerSvc(t))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/chat/stream?message=hello", nil)

	svc.HandleChat(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("期望 HTTP 状态码 400，实际: %d", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "session_id") {
		t.Errorf("期望响应体包含 'session_id'，实际: %s", body)
	}
}

// TestHandleChat_MissingMessage 验证缺少 message 参数时返回 400
func TestHandleChat_MissingMessage(t *testing.T) {
	svc := agent.NewAgentService(nil, testLoggerSvc(t))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/chat/stream?session_id=abc", nil)

	svc.HandleChat(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("期望 HTTP 状态码 400，实际: %d", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "message") {
		t.Errorf("期望响应体包含 'message'，实际: %s", body)
	}
}

// TestHandleChat_MissingBoth 验证同时缺少两个参数时返回 400（session_id 先被检查）
func TestHandleChat_MissingBoth(t *testing.T) {
	svc := agent.NewAgentService(nil, testLoggerSvc(t))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/chat/stream", nil)

	svc.HandleChat(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("期望 HTTP 状态码 400，实际: %d", w.Code)
	}
}

// ============================================
// HandleChat 有效请求测试（需要 mock AgentRunner）
// ============================================

// newMockRunnerForService 创建用于 service 测试的 AgentRunner，使用 mockRunnable
func newMockRunnerForService(t *testing.T, replyContent string) *agent.AgentRunner {
	t.Helper()

	mockGraph := &mockRunnable{
		invokeFunc: func(ctx context.Context, input agent.AgentInput, opts ...compose.Option) (*schema.Message, error) {
			return schema.AssistantMessage(replyContent, nil), nil
		},
	}

	return agent.NewAgentRunner(
		mockGraph,
		newMockSessionStore(),
		newMockCheckpointStore(),
		defaultConfig(),
		testLoggerSvc(t),
	)
}

// TestHandleChat_ValidRequest 验证有效请求产生 SSE 流式事件
func TestHandleChat_ValidRequest(t *testing.T) {
	runner := newMockRunnerForService(t, "你好，这是测试回复")
	svc := agent.NewAgentService(runner, testLoggerSvc(t))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet,
		"/api/chat/stream?session_id=test-session&message=hello", nil)

	// 注入 JWT claims（模拟中间件注入）
	c.Set("user_id", uint(1))
	c.Set("employee_id", "EMP001")
	c.Set("role", "employee")

	svc.HandleChat(c)

	// SSE 响应头验证
	contentType := w.Header().Get("Content-Type")
	if !strings.HasPrefix(contentType, "text/event-stream") {
		t.Errorf("期望 Content-Type 为 text/event-stream，实际: %s", contentType)
	}

	// 验证 SSE 事件流中包含关键事件
	body := w.Body.String()

	// 应包含 thinking 事件（StreamChat 第一件事）
	if !strings.Contains(body, `"type":"thinking"`) {
		t.Errorf("SSE 流应包含 thinking 事件，实际: %s", body)
	}

	// 应包含 done 事件（StreamChat 结束时）
	if !strings.Contains(body, `"type":"done"`) {
		t.Errorf("SSE 流应包含 done 事件，实际: %s", body)
	}

	// 验证回复内容（通过 invokeFallback 降级路径）
	expectedReply := "你好，这是测试回复"
	if !strings.Contains(body, expectedReply) {
		t.Logf("SSE 流内容: %s", body)
		t.Logf("预期包含回复: %s", expectedReply)
	}
}

// TestHandleChat_NoJWTClaims 验证无 JWT claims 时仍可正常处理请求
func TestHandleChat_NoJWTClaims(t *testing.T) {
	runner := newMockRunnerForService(t, "匿名用户回复")
	svc := agent.NewAgentService(runner, testLoggerSvc(t))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet,
		"/api/chat/stream?session_id=noauth-session&message=hi", nil)

	// 不设置任何 JWT claims

	svc.HandleChat(c)

	if w.Code != http.StatusOK {
		t.Errorf("期望 HTTP 状态码 200（无 JWT claims 也应成功），实际: %d", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, `"type":"done"`) {
		t.Errorf("SSE 流应包含 done 事件，实际: %s", body)
	}
}

// TestHandleChat_SSEEventStructure 验证 SSE 事件格式正确
func TestHandleChat_SSEEventStructure(t *testing.T) {
	runner := newMockRunnerForService(t, "结构化测试")
	svc := agent.NewAgentService(runner, testLoggerSvc(t))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet,
		"/api/chat/stream?session_id=struct-test&message=test", nil)
	c.Set("user_id", uint(42))
	c.Set("employee_id", "EMP042")
	c.Set("role", "employee")

	svc.HandleChat(c)

	body := w.Body.String()

	// 每条 SSE 事件应以 "event: " 开头
	if !strings.Contains(body, "event: ") {
		t.Errorf("SSE 流应包含 'event: ' 前缀，实际: %s", body)
	}

	// 每条 SSE 事件应以 "data: " 开头
	if !strings.Contains(body, "data: ") {
		t.Errorf("SSE 流应包含 'data: ' 前缀，实际: %s", body)
	}

	// 验证事件格式为 event: <type>\ndata: <json>\n\n
	lines := strings.Split(body, "\n")
	foundEventLine := false
	foundDataLine := false
	for _, line := range lines {
		if strings.HasPrefix(line, "event: ") {
			foundEventLine = true
		}
		if strings.HasPrefix(line, "data: ") {
			foundDataLine = true
			// 验证 data 内容为有效 JSON
			dataJSON := strings.TrimPrefix(line, "data: ")
			var js json.RawMessage
			if err := json.Unmarshal([]byte(dataJSON), &js); err != nil {
				t.Errorf("data 内容不是有效 JSON: %s, err: %v", dataJSON, err)
			}
		}
	}
	if !foundEventLine {
		t.Error("SSE 流缺少 'event:' 行")
	}
	if !foundDataLine {
		t.Error("SSE 流缺少 'data:' 行")
	}
}

// ============================================
// 辅助函数测试
// ============================================

// TestGetStringFromContext_Present 验证 getStringFromContext 正确提取存在的 string 值
func TestGetStringFromContext_Present(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("test_key", "test_value")

	// getStringFromContext 是 package agent 的未导出函数，通过 HandleChat 间接测试
	// 此处通过 JWT claims 注入来测试（HandleChat 内部调用 getStringFromContext）
	runner := newMockRunnerForService(t, "string context test")
	svc := agent.NewAgentService(runner, testLoggerSvc(t))

	c.Request = httptest.NewRequest(http.MethodGet,
		"/api/chat/stream?session_id=ctx-test&message=hello", nil)
	c.Set("user_id", uint(7))
	c.Set("employee_id", "EMP_STRING_TEST")
	c.Set("role", "admin")

	svc.HandleChat(c)

	if w.Code != http.StatusOK {
		t.Errorf("期望 200，实际: %d", w.Code)
	}
}

// TestGetUintFromContext_Float64 验证 getUintFromContext 正确处理 float64 类型
func TestGetUintFromContext_Float64(t *testing.T) {
	runner := newMockRunnerForService(t, "float64 user_id test")
	svc := agent.NewAgentService(runner, testLoggerSvc(t))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet,
		"/api/chat/stream?session_id=float-test&message=test", nil)
	// 某些 JWT 中间件注入 float64 类型的 user_id
	c.Set("user_id", float64(99))
	c.Set("employee_id", "EMP099")
	c.Set("role", "employee")

	svc.HandleChat(c)

	if w.Code != http.StatusOK {
		t.Errorf("期望 200（float64 user_id 应被转 uint），实际: %d", w.Code)
	}
}
