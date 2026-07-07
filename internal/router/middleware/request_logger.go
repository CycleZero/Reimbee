// Package middleware 请求日志中间件
// 记录每个 HTTP 请求的详情：方法、路径、状态码、耗时、客户端 IP、请求 ID
// 在 CORS 和 AddMetaData 之后执行，确保能读取到 RequestID 和 ClientIP
package middleware

import (
	"time"

	"github.com/CycleZero/Reimbee/log"
	"github.com/gin-gonic/gin"
)

// RequestLogger 请求日志中间件
//
// 执行时机：CORS → AddMetaData → RequestLogger → 后续中间件/Handler
//
// 记录内容：
//   - 请求开始：方法 + 路径 + 客户端 IP + 请求 ID
//   - 请求结束：状态码 + 耗时 + 响应大小（bytes）
//   - 慢请求告警：超过 1s 的请求记录 Warn 级别
//
// 使用 Gin 的 c.Next() 模式：先记录开始时间 → 执行后续中间件和 Handler → 计算耗时并输出日志
func RequestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		// ── 请求开始 ──
		start := time.Now()
		path := c.Request.URL.Path
		rawQuery := c.Request.URL.RawQuery
		if rawQuery != "" {
			path = path + "?" + rawQuery
		}

		log.SugaredLogger().Infow("HTTP请求开始",
			"方法", c.Request.Method,
			"路径", path,
			"客户端IP", c.ClientIP(),
			"User-Agent", c.Request.UserAgent(),
		)

		// ── 执行后续中间件和 Handler ──
		c.Next()

		// ── 请求结束，记录耗时和状态 ──
		latency := time.Since(start)
		statusCode := c.Writer.Status()
		responseSize := c.Writer.Size()

		// 慢请求告警：超过 1 秒
		if latency > time.Second {
			log.SugaredLogger().Warnw("HTTP慢请求",
				"方法", c.Request.Method,
				"路径", path,
				"状态码", statusCode,
				"耗时", latency.String(),
				"响应大小(bytes)", responseSize,
				"客户端IP", c.ClientIP(),
			)
		} else {
			log.SugaredLogger().Infow("HTTP请求完成",
				"方法", c.Request.Method,
				"路径", path,
				"状态码", statusCode,
				"耗时", latency.String(),
				"响应大小(bytes)", responseSize,
			)
		}

		// 异常状态码告警
		if statusCode >= 500 {
			log.SugaredLogger().Errorw("HTTP服务器错误",
				"方法", c.Request.Method,
				"路径", path,
				"状态码", statusCode,
				"客户端IP", c.ClientIP(),
			)
		}
	}
}
