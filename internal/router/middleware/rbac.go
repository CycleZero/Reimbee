package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// RequireRole 角色权限控制中间件工厂
// allowedRoles: 允许访问的角色列表，如 ["admin"] 或 ["approver", "admin"]
func RequireRole(allowedRoles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		role, exists := c.Get("role")
		if !exists {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "无权访问：缺少角色信息"})
			return
		}

		userRole, ok := role.(string)
		if !ok {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "无权访问：角色信息格式错误"})
			return
		}

		for _, allowed := range allowedRoles {
			if userRole == allowed {
				c.Next()
				return
			}
		}

		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "无权访问：当前角色无此操作权限"})
	}
}

// RequireApprover 要求审批人或管理员角色
func RequireApprover() gin.HandlerFunc {
	return RequireRole("approver", "admin")
}

// RequireAdmin 要求管理员角色
func RequireAdmin() gin.HandlerFunc {
	return RequireRole("admin")
}

// GetCurrentRole 从 Context 获取当前用户角色
func GetCurrentRole(c *gin.Context) string {
	role, _ := c.Get("role")
	if r, ok := role.(string); ok {
		return r
	}
	return ""
}

// GetCurrentEmployeeID 从 Context 获取当前用户工号
func GetCurrentEmployeeID(c *gin.Context) string {
	eid, _ := c.Get("employee_id")
	if s, ok := eid.(string); ok {
		return s
	}
	return ""
}
