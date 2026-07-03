package common

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// RequestMetadata 请求元数据，通过中间件注入到 gin.Context 中
type RequestMetadata struct {
	UserID    uint
	Request   *http.Request
	ClientIP  string
	UserAgent string
	RequestID string
}

// GetRequestMetadata 从 context 中获取请求元数据
func GetRequestMetadata(c *gin.Context) *RequestMetadata {
	res, ok := c.Value("request_metadata").(*RequestMetadata)
	if !ok || res == nil {
		return &RequestMetadata{}
	}
	return res
}

// SetRequestMetadata 设置请求元数据到 context 中
func SetRequestMetadata(c *gin.Context, metadata *RequestMetadata) {
	c.Set("request_metadata", metadata)
}
