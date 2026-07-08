package approval

import (
	"github.com/CycleZero/Reimbee/model"
)

// ApprovalRecordResponse 审批记录响应
type ApprovalRecordResponse struct {
	ID              uint   `json:"id"`
	ReimbursementID uint   `json:"reimbursement_id"`
	ApproverName    string `json:"approver_name"`
	ApproverEmail   string `json:"approver_email"`
	Action          string `json:"action"`
	Comment         string `json:"comment"`
	ActionAt        string `json:"action_at,omitempty"`
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at"`
}

// toApprovalRecordResponse 将 model 层审批记录转为 HTTP 响应
func toApprovalRecordResponse(r *model.ApprovalRecord) *ApprovalRecordResponse {
	resp := &ApprovalRecordResponse{
		ID:              r.ID,
		ReimbursementID: r.ReimbursementID,
		ApproverName:    r.ApproverName,
		ApproverEmail:   r.ApproverEmail,
		Action:          r.Action,
		Comment:         r.Comment,
		CreatedAt:       r.CreatedAt.Format("2006-01-02 15:04:05"),
		UpdatedAt:       r.UpdatedAt.Format("2006-01-02 15:04:05"),
	}
	if r.ActionAt != nil {
		resp.ActionAt = r.ActionAt.Format("2006-01-02 15:04:05")
	}
	return resp
}
