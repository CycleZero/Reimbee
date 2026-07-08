// Package agent 可观测性中间件
package agent

import (
	"context"
	"fmt"

	"github.com/CycleZero/blades"
	"go.uber.org/zap"
)

// ============================================
// LoggingModelProvider —— 截获 OpenAI 原始请求
// ============================================

// LoggingModelProvider 包装 blades.ModelProvider，记录所有发往 LLM 的请求和响应
type LoggingModelProvider struct {
	inner  blades.ModelProvider
	logger *zap.Logger
}

// NewLoggingModelProvider 创建带日志的模型提供者
func NewLoggingModelProvider(inner blades.ModelProvider, logger *zap.Logger) *LoggingModelProvider {
	return &LoggingModelProvider{inner: inner, logger: logger}
}

func (p *LoggingModelProvider) Name() string { return p.inner.Name() }

func (p *LoggingModelProvider) Generate(ctx context.Context, req *blades.ModelRequest) (*blades.ModelResponse, error) {
	p.logRequest("Generate", req)

	resp, err := p.inner.Generate(ctx, req)
	if err != nil {
		p.logger.Error("LLM调用失败", zap.Error(err))
		return nil, err
	}

	p.logResponse(resp)
	return resp, nil
}

func (p *LoggingModelProvider) NewStreaming(ctx context.Context, req *blades.ModelRequest) blades.Generator[*blades.ModelResponse, error] {
	p.logRequest("Streaming", req)

	inner := p.inner.NewStreaming(ctx, req)
	return func(yield func(*blades.ModelResponse, error) bool) {
		var final *blades.ModelResponse
		for resp, err := range inner {
			if err != nil {
				p.logger.Error("流式LLM调用失败", zap.Error(err))
				yield(nil, err)
				return
			}
			final = resp
			if !yield(resp, nil) {
				return
			}
		}
		p.logResponse(final)
	}
}

func (p *LoggingModelProvider) logRequest(method string, req *blades.ModelRequest) {
	fields := []zap.Field{
		zap.String("方法", method),
		zap.Int("消息数", len(req.Messages)),
		zap.Int("工具数", len(req.Tools)),
	}

	// 记录系统指令
	if req.Instruction != nil {
		fields = append(fields, zap.String("系统指令", req.Instruction.Text()))
	}

	// 记录每条消息
	for i, msg := range req.Messages {
		role := string(msg.Role)
		text := msg.Text()
		if len(text) > 200 {
			text = text[:200] + "..."
		}

		// 工具调用消息
		toolCalls := []string{}
		for _, part := range msg.Parts {
			if tp, ok := any(part).(blades.ToolPart); ok {
				toolCalls = append(toolCalls, tp.Name+"("+tp.Request+")")
			}
		}

		if len(toolCalls) > 0 {
			fields = append(fields,
				zap.Int("msg_seq", i),
				zap.String("msg_role", role),
				zap.String("msg_text", text),
				zap.Strings("msg_tool_calls", toolCalls),
			)
		} else {
			fields = append(fields,
				zap.Int("msg_seq", i),
				zap.String("msg_role", role),
				zap.String("msg_text", text),
			)
		}
	}

	// 记录工具定义
	toolNames := make([]string, 0, len(req.Tools))
	for _, t := range req.Tools {
		toolNames = append(toolNames, t.Name())
	}
	fields = append(fields, zap.Strings("工具清单", toolNames))

	p.logger.Info("LLM请求", fields...)
}

func (p *LoggingModelProvider) logResponse(resp *blades.ModelResponse) {
	if resp == nil || resp.Message == nil {
		return
	}

	msg := resp.Message
	fields := []zap.Field{
		zap.String("role", string(msg.Role)),
		zap.Int("parts数量", len(msg.Parts)),
	}

	// 记录 text 内容
	if text := msg.Text(); text != "" {
		if len(text) > 200 {
			text = text[:200] + "..."
		}
		fields = append(fields, zap.String("text", text))
	}

	// 记录 tool calls
	toolCalls := []string{}
	for _, part := range msg.Parts {
		if tp, ok := any(part).(blades.ToolPart); ok {
			toolCalls = append(toolCalls, tp.Name+"("+tp.Request+")")
		}
	}
	if len(toolCalls) > 0 {
		fields = append(fields, zap.Strings("tool_calls", toolCalls))
	}

	// token 用量
	if msg.TokenUsage.TotalTokens > 0 {
		fields = append(fields,
			zap.Int64("input_tokens", msg.TokenUsage.InputTokens),
			zap.Int64("output_tokens", msg.TokenUsage.OutputTokens),
			zap.Int64("total_tokens", msg.TokenUsage.TotalTokens),
		)
	}

	// finish reason
	if msg.FinishReason != "" {
		fields = append(fields, zap.String("finish_reason", msg.FinishReason))
	}

	p.logger.Info("LLM响应", fields...)
}

// ============================================
// MessageLoggingMiddleware — 记录消息流
// ============================================

// MessageLoggingMiddleware 记录 Agent 产出的每条消息
func MessageLoggingMiddleware(logger *zap.Logger) blades.Middleware {
	return func(next blades.Handler) blades.Handler {
		return blades.HandleFunc(func(ctx context.Context, invocation *blades.Invocation) blades.Generator[*blades.Message, error] {
			return func(yield func(*blades.Message, error) bool) {
				logger.Debug("开始Agent调用",
					zap.String("invocationID", invocation.ID),
					zap.String("用户消息", invocation.Message.Text()))

				msgCount := 0
				for msg, err := range next.Handle(ctx, invocation) {
					if err != nil {
						logger.Error("Agent调用错误", zap.Error(err))
						yield(nil, err)
						return
					}
					msgCount++
					logMessage(msgCount, msg, logger)
					if !yield(msg, nil) {
						break
					}
				}

				logger.Debug("Agent调用完成", zap.Int("产出消息数", msgCount))
			}
		})
	}
}

func logMessage(seq int, msg *blades.Message, logger *zap.Logger) {
	fields := []zap.Field{
		zap.Int("seq", seq),
		zap.String("role", string(msg.Role)),
		zap.String("status", string(msg.Status)),
	}

	// 推理内容
	if r := msg.Reasoning(); r != "" {
		if len(r) > 200 {
			r = r[:200] + "..."
		}
		fields = append(fields, zap.String("reasoning", r))
	}

	// 正文
	if text := msg.Text(); text != "" {
		if len(text) > 200 {
			text = text[:200] + "..."
		}
		fields = append(fields, zap.String("text", text))
	}

	// 工具调用
	for _, part := range msg.Parts {
		if tp, ok := any(part).(blades.ToolPart); ok {
			req := tp.Request
			if len(req) > 200 {
				req = req[:200] + "..."
			}
			resp := tp.Response
			if len(resp) > 200 {
				resp = resp[:200] + "..."
			}
			fields = append(fields,
				zap.Bool("tool_completed", tp.Completed),
				zap.String("tool_name", tp.Name),
				zap.String("tool_request", req),
				zap.String("tool_response", resp),
			)
		}
	}

	// token 用量
	if msg.TokenUsage.TotalTokens > 0 {
		fields = append(fields, zap.Int64("tokens", msg.TokenUsage.TotalTokens))
	}

	// actions
	if len(msg.Actions) > 0 {
		actionKVs := []string{}
		for k, v := range msg.Actions {
			actionKVs = append(actionKVs, k+"="+fmt.Sprint(v))
		}
		fields = append(fields, zap.Strings("actions", actionKVs))
	}

	logger.Debug("Agent消息", fields...)
	rmb, err := msg.MarshalJSON()
	rm := string(rmb)
	if err != nil {
		logger.Debug("原始msg", zap.Error(err))
	} else {
		logger.Debug("原始msg", zap.String("原始消息json", rm))
	}

}

var _ blades.ModelProvider = (*LoggingModelProvider)(nil)
