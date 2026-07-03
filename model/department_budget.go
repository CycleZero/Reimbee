package model

import "gorm.io/gorm"

// DepartmentBudget 部门预算
type DepartmentBudget struct {
	gorm.Model
	DepartmentID uint        `gorm:"uniqueIndex:idx_dept_year;not null;comment:部门ID" json:"department_id"`
	Department   *Department `gorm:"foreignKey:DepartmentID" json:"department,omitempty"`
	FiscalYear   int         `gorm:"uniqueIndex:idx_dept_year;not null;comment:财年" json:"fiscal_year"`
	AnnualBudget int64  `gorm:"not null;default:0;comment:年度预算(分)" json:"annual_budget"`
	SpentAmount  int64  `gorm:"not null;default:0;comment:已结算金额(分)" json:"spent_amount"`
	FrozenAmount int64  `gorm:"not null;default:0;comment:冻结金额(分)待审批" json:"frozen_amount"`
}
