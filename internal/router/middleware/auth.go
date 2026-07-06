package middleware

import (
	"net/http"
	"slices"
	"strings"

	"github.com/CycleZero/Reimbee/log"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// JwtAuthMiddleWire JWT 认证中间件工厂函数
// optional: true 表示可选认证（token 无效不拦截），false 表示强制认证
func JwtAuthMiddleWire(jwtSecret string) func(optional bool) gin.HandlerFunc {
	return func(optional bool) gin.HandlerFunc {
		return func(c *gin.Context) {
			tokenString := c.GetHeader("Authorization")
			tokenString = extractBearerToken(tokenString)

			if tokenString == "" {
				if optional {
					c.Next()
					return
				}
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"message": "未提供认证令牌"})
				return
			}

			// 解析 JWT
			token, err := jwt.Parse(tokenString, func(token *jwt.Token) (any, error) {
				if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, jwt.ErrSignatureInvalid
				}
				return []byte(jwtSecret), nil
			})

			if err != nil || !token.Valid {
				if optional {
					log.SugaredLogger().Warnf("JWT 解析失败: %v", err)
					c.Next()
					return
				}
				log.SugaredLogger().Warnf("认证失败: %v", err)
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"message": "无效的认证令牌"})
				return
			}

		// 提取 claims 中的 user_id, employee_id, role
		if claims, ok := token.Claims.(jwt.MapClaims); ok {
			if userID, ok := claims["user_id"]; ok {
				if uid, ok := userID.(float64); ok {
					c.Set("user_id", uint(uid))
				}
			}
			if empID, ok := claims["employee_id"]; ok {
				if eid, ok := empID.(string); ok {
					c.Set("employee_id", eid)
				}
			}
			if role, ok := claims["role"]; ok {
				if r, ok := role.(string); ok {
					c.Set("role", r)
				}
			}
		}

			c.Next()
		}
	}
}

func extractBearerToken(token string) string {
	if after, ok := strings.CutPrefix(token, "Bearer "); ok {
		return after
	}
	return token
}

// InArray 检查字符串是否在数组中
func InArray(str string, arr []string) bool {
	return slices.Contains(arr, str)
}
