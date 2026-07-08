package common

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
)

// RequestMetadata 请求元数据，通过中间件注入到 gin.Context 中
type RequestMetadata struct {
	UserID     uint   `json:"user_id"`
	EmployeeID string `json:"employee_id"` // 工号
	Role       string `json:"role"`       // employee / approver / admin
	Request    *http.Request
	ClientIP   string
	UserAgent  string
	RequestID  string
}

type ctxKey struct{}

// GetRequestMetadata 从 gin.Context 中获取请求元数据
func GetRequestMetadata(c *gin.Context) *RequestMetadata {
	res, ok := c.Value("request_metadata").(*RequestMetadata)
	if !ok || res == nil {
		return &RequestMetadata{}
	}
	return res
}

// SetRequestMetadata 设置请求元数据到 gin.Context 中
func SetRequestMetadata(c *gin.Context, metadata *RequestMetadata) {
	c.Set("request_metadata", metadata)
}

// WithMeta 将请求元数据注入 context.Context（供 blades Resolver/InstructionProvider 读取角色）
func WithMeta(ctx context.Context, meta *RequestMetadata) context.Context {
	return context.WithValue(ctx, ctxKey{}, meta)
}

// Meta 从 context.Context 中读取请求元数据
func Meta(ctx context.Context) *RequestMetadata {
	v, _ := ctx.Value(ctxKey{}).(*RequestMetadata)
	return v
}
