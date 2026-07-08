package middleware

import (
	"crypto/rand"
	"math/big"
	"strconv"
	"time"

	"github.com/CycleZero/Reimbee/internal/common"

	"github.com/gin-gonic/gin"
)

var (
	IsMiddleWireRegisterFinished = false
	AuthMiddleWire               func(optional bool) gin.HandlerFunc
)

// AddMetaData 为每个请求添加元数据（RequestID、ClientIP、用户身份等）
func AddMetaData() gin.HandlerFunc {
	return func(c *gin.Context) {
		meta := &common.RequestMetadata{
			UserID:     0,
			EmployeeID: GetCurrentEmployeeID(c),
			Role:       GetCurrentRole(c),
			Request:    c.Request,
			ClientIP:   c.ClientIP(),
			UserAgent:  c.Request.UserAgent(),
			RequestID:  generateRequestID(),
		}
		// 存入 request context（gin.Value 会 fallthrough 到 c.Request.Context）
		common.SetRequestMetadata(c, meta)
		c.Next()
	}
}

func generateRequestID() string {
	now := strconv.FormatInt(time.Now().UnixNano(), 10)
	randomSuffix, err := generateRandomString(8)
	if err != nil {
		return now
	}
	return now + randomSuffix
}

func generateRandomString(length int) (string, error) {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	result := make([]byte, length)
	for i := 0; i < length; i++ {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(letters))))
		if err != nil {
			return "", err
		}
		result[i] = letters[n.Int64()]
	}
	return string(result), nil
}
