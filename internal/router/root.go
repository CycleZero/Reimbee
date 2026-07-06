package router

import (
	"github.com/CycleZero/Reimbee/internal/domain"
	"github.com/CycleZero/Reimbee/internal/router/middleware"

	"github.com/gin-gonic/gin"
)

// RegisterFunc 路由注册函数类型
type RegisterFunc func(root gin.IRouter, serviceHub *domain.ServiceHub)

// NewRegisterFunc 创建路由注册函数
func NewRegisterFunc() RegisterFunc {
	return RegisterRouter
}

// RegisterRouter 注册所有路由（含 RBAC 权限控制）
func RegisterRouter(root gin.IRouter, hub *domain.ServiceHub) {
	if !middleware.IsMiddleWireRegisterFinished {
		panic("中间件注册未完成")
	}

	// 全局中间件
	root.Use(middleware.CORS())
	root.Use(middleware.AddMetaData())

	// 健康检查（无需认证）
	root.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	api := root.Group("/api")

	// ==========================================
	// 管理员专属路由
	// ==========================================
	admin := api.Group("", middleware.RequireAdmin())
	{
		// 部门管理（CUD）
		dept := admin.Group("/departments")
		{
			dept.POST("", hub.DepartmentService.Create)
			dept.PUT("/:id", hub.DepartmentService.Update)
			dept.DELETE("/:id", hub.DepartmentService.Delete)
		}

		// 员工管理（CUD）
		emp := admin.Group("/employees")
		{
			emp.POST("", hub.EmployeeService.Create)
			emp.PUT("/:id", hub.EmployeeService.Update)
			emp.DELETE("/:id", hub.EmployeeService.Delete)
		}

		// 预算管理（CUD）
		budget := admin.Group("/budgets")
		{
			budget.POST("", hub.BudgetService.Create)
			budget.PUT("/:id", hub.BudgetService.Update)
		}
	}

	// ==========================================
	// 审批人 + 管理员路由
	// ==========================================
	approver := api.Group("", middleware.RequireApprover())
	{
		// 审批操作
		approver.POST("/approvals/:id/approve", hub.ApprovalService.Approve)
		approver.POST("/approvals/:id/reject", hub.ApprovalService.Reject)

		// 待审批列表
		approver.GET("/reimbursements/pending", hub.ReimbursementService.ListPending)

		// 员工列表（审批人需要知道有哪些员工）
		approver.GET("/employees", hub.EmployeeService.List)
		approver.GET("/employees/:id", hub.EmployeeService.GetByID)
	}

	// ==========================================
	// 所有已认证用户路由
	// ==========================================
	auth := api.Group("", middleware.AuthMiddleWire(false))
	{
		// 部门查询（任何人可查看）
		dept := auth.Group("/departments")
		{
			dept.GET("", hub.DepartmentService.List)
			dept.GET("/:id", hub.DepartmentService.GetByID)
		}

		// 审批人列表（任何人可查看）
		auth.GET("/employees/approvers", hub.EmployeeService.ListApprovers)

		// 预算看板（任何人可查看）
		auth.GET("/budgets/dashboard", hub.BudgetService.Dashboard)
		auth.GET("/budgets/:id", hub.BudgetService.GetByID)

		// 报销单——查询（任何人可查自己的）
		reimb := auth.Group("/reimbursements")
		{
			reimb.GET("", hub.ReimbursementService.List)
			reimb.GET("/no/:no", hub.ReimbursementService.GetByNo)
			reimb.GET("/:id", hub.ReimbursementService.GetByID)

			// 审批进度查询
			reimb.GET("/:id/approvals", hub.ApprovalService.GetProgress)
		}

		// 报销单——创建与提交（员工操作）
		reimb.POST("", hub.ReimbursementService.Create)
		reimb.POST("/:id/submit", hub.ReimbursementService.Submit)

		// 报销单——审批（审批人操作，权限由 RequireApprover 保证）
		// approve/reject 已在上方 approver 组中定义
	}

	// SSE 对话接口（WebSocket-like，使用 Query 参数传递 token）
	api.GET("/chat/stream", func(c *gin.Context) {
		c.JSON(501, gin.H{"message": "Agent 对话接口待实现"})
	})
}
