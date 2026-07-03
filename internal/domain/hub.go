package domain

import (
	"github.com/CycleZero/Reimbee/internal/domain/approval"
	"github.com/CycleZero/Reimbee/internal/domain/budget"
	"github.com/CycleZero/Reimbee/internal/domain/department"
	"github.com/CycleZero/Reimbee/internal/domain/employee"
	"github.com/CycleZero/Reimbee/internal/domain/reimbursement"
)

// ServiceHub 服务聚合中心，集中管理所有业务服务
type ServiceHub struct {
	DepartmentService    *department.DepartmentService
	EmployeeService      *employee.EmployeeService
	BudgetService        *budget.BudgetService
	ApprovalService      *approval.ApprovalService
	ReimbursementService *reimbursement.ReimbursementService
}

func NewServiceHub(
	dept *department.DepartmentService,
	emp *employee.EmployeeService,
	bgt *budget.BudgetService,
	app *approval.ApprovalService,
	reim *reimbursement.ReimbursementService,
) *ServiceHub {
	return &ServiceHub{
		DepartmentService:    dept,
		EmployeeService:      emp,
		BudgetService:        bgt,
		ApprovalService:      app,
		ReimbursementService: reim,
	}
}
