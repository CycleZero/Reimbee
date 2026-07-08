package agent

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

type SSEEvent struct {
	Type string
	Data interface{}
}

const (
	EventReasoning   = "reasoning"
	EventThinking    = "thinking"
	EventMessage     = "message"
	EventToolCall    = "tool_call"
	EventToolResult  = "tool_result"
	EventDone        = "done"
	EventError       = "error"
	EventInterrupted = "interrupted"
)

type thinkingData struct {
	Text string `json:"text"`
}

type reasoningData struct {
	Text  string `json:"text"`
	Delta bool   `json:"delta"`
}

type messageData struct {
	Text  string `json:"text"`
	Delta bool   `json:"delta"`
}

type toolCallData struct {
	Name  string `json:"name"`
	Input string `json:"input"`
}

type toolResultData struct {
	Name   string `json:"name"`
	Output string `json:"output"`
}

type errorData struct {
	Error string `json:"error"`
}

type interruptedData struct {
	Reason string `json:"reason"`
}

func NewThinkingEvent(text string) SSEEvent {
	return SSEEvent{Type: EventThinking, Data: thinkingData{Text: text}}
}

func NewReasoningEvent(text string, delta bool) SSEEvent {
	return SSEEvent{Type: EventReasoning, Data: reasoningData{Text: text, Delta: delta}}
}

func NewMessageEvent(text string, delta bool) SSEEvent {
	return SSEEvent{Type: EventMessage, Data: messageData{Text: text, Delta: delta}}
}

func NewToolCallEvent(name, input string) SSEEvent {
	return SSEEvent{Type: EventToolCall, Data: toolCallData{Name: name, Input: input}}
}

func NewToolResultEvent(name, output string) SSEEvent {
	return SSEEvent{Type: EventToolResult, Data: toolResultData{Name: name, Output: output}}
}

func NewDoneEvent() SSEEvent {
	return SSEEvent{Type: EventDone, Data: map[string]string{}}
}

func NewErrorEvent(err string) SSEEvent {
	return SSEEvent{Type: EventError, Data: errorData{Error: err}}
}

func NewInterruptedEvent(reason string) SSEEvent {
	return SSEEvent{Type: EventInterrupted, Data: interruptedData{Reason: reason}}
}

type GinSSEWriter struct {
	w gin.ResponseWriter
}

func NewGinSSEWriter(c *gin.Context) (*GinSSEWriter, error) {
	if _, ok := c.Writer.(http.Flusher); !ok {
		return nil, fmt.Errorf("服务器不支持流式响应")
	}
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	return &GinSSEWriter{w: c.Writer}, nil
}

func (w *GinSSEWriter) WriteEvent(event SSEEvent) error {
	payload, err := json.Marshal(event.Data)
	if err != nil {
		return fmt.Errorf("SSE数据序列化失败: %w", err)
	}
	_, err = fmt.Fprintf(w.w, "event: %s\ndata: %s\n\n", event.Type, string(payload))
	return err
}

func (w *GinSSEWriter) Flush() {
	if f, ok := w.w.(http.Flusher); ok {
		f.Flush()
	}
}
