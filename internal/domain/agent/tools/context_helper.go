// Package tools context 辅助函数
package tools

import "context"

type sessionIDKey struct{}

// WithSessionID 将 sessionID 注入 context，供工具通过 getSessionID 读取
func WithSessionID(ctx context.Context, sessionID string) context.Context {
	return context.WithValue(ctx, sessionIDKey{}, sessionID)
}

// getSessionID 从 context 中提取 sessionID（由 biz.Run 注入）
func getSessionID(ctx context.Context) string {
	if v, ok := ctx.Value(sessionIDKey{}).(string); ok {
		return v
	}
	return ""
}
