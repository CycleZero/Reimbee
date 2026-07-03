package model

import "gorm.io/gorm"

// Department 部门
type Department struct {
	gorm.Model
	Name          string            `gorm:"type:varchar(100);uniqueIndex;not null;comment:部门名称" json:"name"`
	ManagerID     *uint             `gorm:"comment:部门主管的员工ID" json:"manager_id"`
	Manager       *Employee         `gorm:"foreignKey:ManagerID" json:"manager,omitempty"`
	Budgets       []DepartmentBudget `gorm:"foreignKey:DepartmentID" json:"budgets,omitempty"`
	Employees     []Employee         `gorm:"foreignKey:DepartmentID" json:"employees,omitempty"`
}
