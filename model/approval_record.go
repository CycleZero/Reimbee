package model

import (
	"time"

	"gorm.io/gorm"
)

// ApprovalRecord 审批记录
type ApprovalRecord struct {
	gorm.Model
	ReimbursementID uint       `gorm:"index;not null;comment:关联报销单ID" json:"reimbursement_id"`
	ApproverName    string     `gorm:"type:varchar(50);not null;comment:审批人姓名" json:"approver_name"`
	ApproverEmail   string     `gorm:"type:varchar(100);comment:审批人邮箱" json:"approver_email"`
	Action          string     `gorm:"type:varchar(20);default:pending;comment:pending/approved/rejected" json:"action"`
	Comment         string     `gorm:"type:text;comment:审批意见" json:"comment"`
	ActionAt        *time.Time `gorm:"comment:审批操作时间" json:"action_at"`
}
