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

// RegisterRouter 注册所有路由
func RegisterRouter(root gin.IRouter, hub *domain.ServiceHub) {
	if !middleware.IsMiddleWireRegisterFinished {
		panic("中间件注册未完成")
	}

	// 全局中间件
	root.Use(middleware.CORS())
	root.Use(middleware.AddMetaData())

	// 健康检查
	root.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	api := root.Group("/api")

	// 部门管理
	dept := api.Group("/departments")
	{
		dept.GET("", hub.DepartmentService.List)
		dept.GET("/:id", hub.DepartmentService.GetByID)
		dept.POST("", hub.DepartmentService.Create)
		dept.PUT("/:id", hub.DepartmentService.Update)
		dept.DELETE("/:id", hub.DepartmentService.Delete)
	}

	// 员工管理
	emp := api.Group("/employees")
	{
		emp.GET("", hub.EmployeeService.List)
		emp.GET("/approvers", hub.EmployeeService.ListApprovers)
		emp.GET("/:id", hub.EmployeeService.GetByID)
		emp.POST("", hub.EmployeeService.Create)
		emp.PUT("/:id", hub.EmployeeService.Update)
		emp.DELETE("/:id", hub.EmployeeService.Delete)
	}

	// 预算管理
	budget := api.Group("/budgets")
	{
		budget.GET("/dashboard", hub.BudgetService.Dashboard)
		budget.GET("/:id", hub.BudgetService.GetByID)
		budget.POST("", hub.BudgetService.Create)
		budget.PUT("/:id", hub.BudgetService.Update)
	}

	// 报销单管理
	reimb := api.Group("/reimbursements")
	{
		reimb.GET("", hub.ReimbursementService.List)
		reimb.GET("/pending", hub.ReimbursementService.ListPending)
		reimb.GET("/no/:no", hub.ReimbursementService.GetByNo)
		reimb.GET("/:id", hub.ReimbursementService.GetByID)
		reimb.POST("", hub.ReimbursementService.Create)
		reimb.POST("/:id/submit", hub.ReimbursementService.Submit)
		reimb.POST("/:id/approve", hub.ReimbursementService.Approve)
		reimb.POST("/:id/reject", hub.ReimbursementService.Reject)
	}

	// 审批管理
	approvals := api.Group("/reimbursements/:id/approvals")
	{
		approvals.GET("", hub.ApprovalService.GetProgress)
	}
	api.POST("/approvals/:id/approve", hub.ApprovalService.Approve)
	api.POST("/approvals/:id/reject", hub.ApprovalService.Reject)
}
