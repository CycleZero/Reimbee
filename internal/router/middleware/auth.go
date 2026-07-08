package middleware

import (
	"net/http"
	"slices"
	"strings"

	"github.com/CycleZero/Reimbee/internal/common"
	"github.com/CycleZero/Reimbee/log"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// ============================================
// JWT 认证中间件
// ============================================

// JwtAuthMiddleWire JWT 认证中间件工厂函数
// AuthMiddleWire 变量在 metadata.go 中声明，由 router/provider.go 通过闭包注入 jwtSecret
//
// 参数:
//   - jwtSecret: JWT 签名密钥，与签发端一致（auth service 的 Login 使用相同密钥）
//
// 返回:
//   - func(optional bool) gin.HandlerFunc: 接收 optional 参数再返回中间件
//     optional=true:  令牌缺失或无效时不拦截，通过 c.Next() 放行（用于可选认证的端点）
//     optional=false: 令牌缺失或无效时返回 401，拦截请求
//
// 执行流程:
//   1. 从 Authorization header 提取 Bearer Token
//   2. Token 为空 → optional 则放行，否则 401
//   3. 解析 JWT（HMAC 签名验证）
//   4. 解析失败 → optional 则放行（记录 Warn），否则 401
//   5. 提取 claims 中的 user_id（uint）、employee_id（string）、role（string）
//   6. 注入 gin.Context: c.Set("user_id", ...), c.Set("employee_id", ...), c.Set("role", ...)
//   7. c.Next() 放行到后续中间件和 handler
func JwtAuthMiddleWire(jwtSecret string) func(optional bool) gin.HandlerFunc {
	return func(optional bool) gin.HandlerFunc {
		return func(c *gin.Context) {
			// ── 步骤 1: 提取 Bearer Token ──
			tokenString := c.GetHeader("Authorization")
			tokenString = extractBearerToken(tokenString)

			log.SugaredLogger().Debugw("JWT认证中间件开始",
				"路径", c.Request.URL.Path,
				"方法", c.Request.Method,
				"可选模式", optional,
				"令牌长度", len(tokenString))

			if tokenString == "" {
				if optional {
					log.SugaredLogger().Debugw("可选认证模式：令牌为空，放行")
					c.Next()
					return
				}
				log.SugaredLogger().Warnw("认证失败：未提供JWT令牌",
					"路径", c.Request.URL.Path)
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"message": "未提供认证令牌"})
				return
			}

			// ── 步骤 2: 解析并验证 JWT ──
			// 使用 HMAC 签名算法验证 token 未被篡改
			token, err := jwt.Parse(tokenString, func(token *jwt.Token) (any, error) {
				if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
					log.SugaredLogger().Warnw("JWT签名算法不匹配",
						"期望", "HMAC",
						"实际", token.Method.Alg())
					return nil, jwt.ErrSignatureInvalid
				}
				return []byte(jwtSecret), nil
			})

			if err != nil || !token.Valid {
				if optional {
					log.SugaredLogger().Warnw("可选认证模式：JWT解析失败，放行",
						"错误", err,
						"路径", c.Request.URL.Path)
					c.Next()
					return
				}
				log.SugaredLogger().Warnw("认证失败：JWT无效",
					"错误", err,
					"路径", c.Request.URL.Path)
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"message": "无效的认证令牌"})
				return
			}

			// ── 步骤 3: 提取 claims 中的用户身份信息 ──
			// JWT claims 格式（由 auth service 的 Login 签发）:
			//   user_id:     float64（JWT 标准数字类型） → uint
			//   employee_id: string
			//   role:        string（"employee" / "approver" / "admin"）
			if claims, ok := token.Claims.(jwt.MapClaims); ok {
				var uid uint
				var eid, name, role string

				if userID, ok := claims["user_id"]; ok {
					if v, ok := userID.(float64); ok {
						uid = uint(v)
						c.Set("user_id", uid)
					}
				}
				if empID, ok := claims["employee_id"]; ok {
					if v, ok := empID.(string); ok {
						eid = v
						c.Set("employee_id", eid)
					}
				}
				if n, ok := claims["name"]; ok {
					if v, ok := n.(string); ok {
						name = v
					}
				}
				if r, ok := claims["role"]; ok {
					if v, ok := r.(string); ok {
						role = v
						c.Set("role", role)
					}
				}

				// 更新 AddMetaData 创建的 RequestMetadata
				if meta := common.GetRequestMetadata(c); meta != nil {
					meta.UserID = uid
					meta.EmployeeID = eid
					meta.EmployeeName = name
					meta.Role = role
				}

				log.SugaredLogger().Debugw("JWT认证成功，claims已更新到RequestMetadata",
					"路径", c.Request.URL.Path,
					"user_id", uid,
					"employee_id", eid,
					"role", role)
			} else {
				log.SugaredLogger().Warnw("JWT claims类型不匹配，无法提取用户信息",
					"路径", c.Request.URL.Path)
			}

			c.Next()
		}
	}
}

// extractBearerToken 从 Authorization header 中提取 Bearer Token
// 支持格式: "Bearer eyJhbGciOi..." → "eyJhbGciOi..."
// 无 "Bearer " 前缀时原样返回
func extractBearerToken(token string) string {
	if after, ok := strings.CutPrefix(token, "Bearer "); ok {
		return after
	}
	return token
}

// InArray 检查字符串是否在数组中（使用 Go 1.21+ 的 slices.Contains）
func InArray(str string, arr []string) bool {
	return slices.Contains(arr, str)
}
