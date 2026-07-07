package tools

import (
	"context"

	"github.com/CycleZero/Reimbee/internal/domain/agent/types"
)

// getSessionIDFromCtx 从 context 中提取 sessionID
// sessionID 由 GenInput 通过 SessionIDContextKey 注入
func getSessionIDFromCtx(ctx context.Context) string {
	if sid, ok := ctx.Value(types.SessionIDContextKey{}).(string); ok {
		return sid
	}
	return ""
}
