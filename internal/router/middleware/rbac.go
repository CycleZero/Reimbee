package middleware

import (
	"fmt"
	"net/http"

	"github.com/CycleZero/Reimbee/log"
	"github.com/CycleZero/Reimbee/model"
	"github.com/gin-gonic/gin"
)

// ============================================
// RBAC 角色权限控制中间件
// ============================================
// 依赖 JWT 认证中间件先执行，将 role 注入 gin.Context
// 执行顺序: JWT 认证 → RBAC 鉴权 → Handler

// RequireRole 角色权限控制中间件工厂
//
// 参数:
//   - allowedRoles: 允许访问的角色列表，如 ["admin"] 或 ["approver", "admin"]
//
// 执行流程:
//   1. 从 gin.Context 读取 JWT 中间件注入的 "role" 值
//   2. role 不存在 → 403（缺少角色信息，通常表示 JWT 中间件未执行或 token 无 role claim）
//   3. role 类型错误 → 403（角色信息格式错误）
//   4. 遍历 allowedRoles，匹配成功 → c.Next() 放行
//   5. 全部不匹配 → 403（当前角色无此操作权限）
//
// 使用示例:
//
//	admin := root.Group("/api", middleware.RequireRole("admin"))
//	approverOrAdmin := root.Group("/api", middleware.RequireRole("approver", "admin"))
func RequireRole(allowedRoles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// ── 步骤 1: 读取角色信息 ──
		role, exists := c.Get("role")
		if !exists {
			log.SugaredLogger().Warnw("RBAC拒绝：缺少角色信息",
				"路径", c.Request.URL.Path,
				"方法", c.Request.Method)
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "无权访问：缺少角色信息"})
			return
		}

		userRole, ok := role.(string)
		if !ok {
			log.SugaredLogger().Warnw("RBAC拒绝：角色信息类型错误",
				"路径", c.Request.URL.Path,
				"实际类型", fmt.Sprintf("%T", role))
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "无权访问：角色信息格式错误"})
			return
		}

		// ── 步骤 2: 检查角色是否在允许列表中 ──
		for _, allowed := range allowedRoles {
			if userRole == allowed {
				log.SugaredLogger().Debugw("RBAC放行",
					"路径", c.Request.URL.Path,
					"角色", userRole,
					"允许角色", allowedRoles)
				c.Next()
				return
			}
		}

		// ── 步骤 3: 拒绝 ──
		log.SugaredLogger().Warnw("RBAC拒绝：角色无权限",
			"路径", c.Request.URL.Path,
			"当前角色", userRole,
			"允许角色", allowedRoles)
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "无权访问：当前角色无此操作权限"})
	}
}

// RequireApprover 要求审批人或管理员角色
// 用于审批操作（审批报销单、查看待审批列表）
func RequireApprover() gin.HandlerFunc {
	return RequireRole(model.RoleApprover, model.RoleAdmin)
}

// RequireAdmin 要求管理员角色
// 用于管理操作（创建/修改/删除部门、员工、预算）
func RequireAdmin() gin.HandlerFunc {
	return RequireRole(model.RoleAdmin)
}

// GetCurrentRole 从 gin.Context 安全获取当前用户角色
// 返回值: 角色字符串（"employee"/"approver"/"admin"），失败返回 ""
func GetCurrentRole(c *gin.Context) string {
	role, _ := c.Get("role")
	if r, ok := role.(string); ok {
		return r
	}
	return ""
}

// GetCurrentEmployeeID 从 gin.Context 安全获取当前用户工号
// 返回值: 工号字符串，失败返回 ""
func GetCurrentEmployeeID(c *gin.Context) string {
	eid, _ := c.Get("employee_id")
	if s, ok := eid.(string); ok {
		return s
	}
	return ""
}

// GetCurrentEmployeeName 从 gin.Context 安全获取当前用户姓名
func GetCurrentEmployeeName(c *gin.Context) string {
	name, _ := c.Get("employee_name")
	if s, ok := name.(string); ok {
		return s
	}
	return ""
}
