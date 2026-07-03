package model

import "gorm.io/gorm"

// Employee 员工信息
type Employee struct {
	gorm.Model
	EmployeeID   string      `gorm:"type:varchar(20);uniqueIndex;not null;comment:工号" json:"employee_id"`
	Name         string      `gorm:"type:varchar(50);not null;comment:姓名" json:"name"`
	DepartmentID uint        `gorm:"index;comment:所属部门ID" json:"department_id"`
	Department   *Department `gorm:"foreignKey:DepartmentID" json:"department,omitempty"`
	Email        string      `gorm:"type:varchar(100);comment:工作邮箱" json:"email"`
	Role         string      `gorm:"type:varchar(20);default:employee;comment:employee/approver/admin" json:"role"`
	IsApprover   bool        `gorm:"default:false;comment:是否为审批人" json:"is_approver"`
}
