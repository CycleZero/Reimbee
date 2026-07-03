package middleware

import (
	"crypto/rand"
	"math/big"
	"strconv"
	"time"

	"gin-template/internal/common"

	"github.com/gin-gonic/gin"
)

var (
	IsMiddleWireRegisterFinished = false
	AuthMiddleWire               func(optional bool) gin.HandlerFunc
)

// AddMetaData 为每个请求添加元数据（RequestID、ClientIP 等）
func AddMetaData() gin.HandlerFunc {
	return func(c *gin.Context) {
		meta := &common.RequestMetadata{
			UserID:    0,
			Request:   c.Request,
			ClientIP:  c.ClientIP(),
			UserAgent: c.Request.UserAgent(),
			RequestID: generateRequestID(),
		}
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
