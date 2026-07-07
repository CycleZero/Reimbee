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

	// 认证路由（无需 JWT）
	authGroup := root.Group("/api/auth")
	{
		authGroup.POST("/login", hub.AuthService.Login)
		authGroup.POST("/register", hub.AuthService.Register)
	}

	api := root.Group("/api", middleware.AuthMiddleWire(false))

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

		// 报销单级操作（强制通过/驳回）
		approver.POST("/reimbursements/:id/approve", hub.ReimbursementService.Approve)
		approver.POST("/reimbursements/:id/reject", hub.ReimbursementService.Reject)

		// 待审批列表
		approver.GET("/reimbursements/pending", hub.ReimbursementService.ListPending)

		// 员工列表（审批人需要知道有哪些员工）
		approver.GET("/employees", hub.EmployeeService.List)
		approver.GET("/employees/:id", hub.EmployeeService.GetByID)
	}

	// ==========================================
	// 通用路由（所有已认证用户可访问）
	// ==========================================
	{
		// 部门查询
		api.GET("/departments", hub.DepartmentService.List)
		api.GET("/departments/:id", hub.DepartmentService.GetByID)

		// 审批人列表
		api.GET("/employees/approvers", hub.EmployeeService.ListApprovers)

		// 预算看板
		api.GET("/budgets/dashboard", hub.BudgetService.Dashboard)
		api.GET("/budgets/:id", hub.BudgetService.GetByID)

		// 报销单——查询
		api.GET("/reimbursements", hub.ReimbursementService.List)
		api.GET("/reimbursements/no/:no", hub.ReimbursementService.GetByNo)
		api.GET("/reimbursements/:id", hub.ReimbursementService.GetByID)

		// 审批进度查询
		api.GET("/reimbursements/:id/approvals", hub.ApprovalService.GetProgress)

		// 报销单——创建与提交
		api.POST("/reimbursements", hub.ReimbursementService.Create)
		api.POST("/reimbursements/:id/submit", hub.ReimbursementService.Submit)

		// 票据上传（Agent Phase 1 信息收集的前置步骤）
		api.POST("/reimbursements/upload", hub.ReimbursementService.UploadInvoice)
	}

	// SSE 对话接口（Agent 流式响应）
	api.GET("/chat/stream", hub.AgentService.HandleChat)
}
