// Package testutil 提供测试基础设施，包括内存数据库初始化与清理
package testutil

import (
	"github.com/CycleZero/Reimbee/infra"
	"github.com/CycleZero/Reimbee/model"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// NewTestData 创建用于测试的内存 SQLite 数据库，自动迁移所有模型
func NewTestData() *infra.Data {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		panic("创建测试数据库失败: " + err.Error())
	}

	// 自动迁移所有业务模型（按依赖顺序）
	if err := db.AutoMigrate(
		&model.Department{},
		&model.Employee{},
		&model.DepartmentBudget{},
		&model.Reimbursement{},
		&model.InvoiceItem{},
		&model.ApprovalRecord{},
		&model.PolicyDocument{},
		&model.PolicyChunk{},
	); err != nil {
		panic("迁移测试数据库失败: " + err.Error())
	}

	return &infra.Data{DB: db}
}

// CleanDB 清空测试数据库的所有数据（保留表结构）
func CleanDB(data *infra.Data) {
	data.DB.Exec("DELETE FROM session_messages")
	data.DB.Exec("DELETE FROM policy_chunks")
	data.DB.Exec("DELETE FROM policy_documents")
	data.DB.Exec("DELETE FROM approval_records")
	data.DB.Exec("DELETE FROM invoice_items")
	data.DB.Exec("DELETE FROM reimbursements")
	data.DB.Exec("DELETE FROM department_budgets")
	data.DB.Exec("DELETE FROM employees")
	data.DB.Exec("DELETE FROM departments")
}

// SeedEmployee 创建一条测试用员工记录并返回
func SeedEmployee(data *infra.Data, employeeID, name string, deptID uint, isApprover bool) *model.Employee {
	emp := &model.Employee{
		EmployeeID:   employeeID,
		Name:         name,
		DepartmentID: deptID,
		Email:        name + "@test.com",
		Role:         "employee",
		IsApprover:   isApprover,
	}
	if isApprover {
		emp.Role = "approver"
	}
	data.DB.Create(emp)
	return emp
}

// SeedDepartment 创建一条测试用部门记录并返回
func SeedDepartment(data *infra.Data, name string) *model.Department {
	dept := &model.Department{Name: name}
	data.DB.Create(dept)
	return dept
}

// SeedBudget 创建一条测试用预算记录并返回
func SeedBudget(data *infra.Data, deptID uint, year int, budget int64) *model.DepartmentBudget {
	b := &model.DepartmentBudget{
		DepartmentID: deptID,
		FiscalYear:   year,
		AnnualBudget: budget,
	}
	data.DB.Create(b)
	return b
}

// SeedReimbursement 创建一条测试用报销单并返回
func SeedReimbursement(data *infra.Data, no, employeeID, employeeName string, deptID uint, status string, amount int64) *model.Reimbursement {
	rm := &model.Reimbursement{
		ReimbursementNo: no,
		EmployeeID:      employeeID,
		EmployeeName:    employeeName,
		DepartmentID:    deptID,
		TotalAmount:     amount,
		Status:          status,
	}
	data.DB.Create(rm)
	return rm
}

// SeedApprovalRecord 创建一条测试用审批记录并返回
func SeedApprovalRecord(data *infra.Data, reimbursementID uint, approverName string, action string) *model.ApprovalRecord {
	a := &model.ApprovalRecord{
		ReimbursementID: reimbursementID,
		ApproverName:    approverName,
		ApproverEmail:   approverName + "@test.com",
		Action:          action,
	}
	data.DB.Create(a)
	return a
}
