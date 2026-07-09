package model

import "gorm.io/gorm"

// Reimbursement 报销申请单
type Reimbursement struct {
	gorm.Model
	ReimbursementNo string         `gorm:"type:varchar(20);uniqueIndex;not null;comment:报销单号 REIMB-YYYY-NNNN" json:"reimbursement_no"`
	EmployeeID      string         `gorm:"type:varchar(20);index;not null;comment:申请人工号" json:"employee_id"`
	EmployeeName    string         `gorm:"type:varchar(50);not null;comment:申请人姓名" json:"employee_name"`
	DepartmentID         uint             `gorm:"index;not null;comment:所属部门ID" json:"department_id"`
	Department           *Department      `gorm:"foreignKey:DepartmentID" json:"department,omitempty"`
	TotalAmount     int64          `gorm:"not null;default:0;comment:报销总金额(分)" json:"total_amount"`
	Status          string         `gorm:"type:varchar(20);default:draft;index;comment:draft/pending/reviewing/approved/rejected" json:"status"`
	SubmitNote      string         `gorm:"type:text;comment:报销事由" json:"submit_note"`
	NeedSpecialApproval bool                `gorm:"default:false;comment:是否需要特殊审批" json:"need_special_approval"`
	Items               []ReimbursementItem `gorm:"foreignKey:ReimbursementID;constraint:OnDelete:CASCADE" json:"items,omitempty"`
	Approvals           []ApprovalRecord    `gorm:"foreignKey:ReimbursementID;constraint:OnDelete:CASCADE" json:"approvals,omitempty"`
}
