package approval

import (
	"net/http"
	"strconv"

	"github.com/CycleZero/Reimbee/log"
	"github.com/CycleZero/Reimbee/model"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// ApprovalService 审批 HTTP 服务层
type ApprovalService struct {
	biz    *ApprovalBiz
	logger *log.Logger
}

// NewApprovalService 创建审批 HTTP 服务层实例
func NewApprovalService(biz *ApprovalBiz, logger *log.Logger) *ApprovalService {
	return &ApprovalService{biz: biz, logger: logger}
}

// GetProgress 获取审批进度
// @Summary 获取审批进度
// @Description 根据报销单ID获取该报销单的审批流转记录
// @Tags 审批管理
// @Accept json
// @Produce json
// @Param id path int true "报销单ID"
// @Success 200 {array} ApprovalRecordResponse "审批记录列表"
// @Failure 400 {object} map[string]interface{} "报销单ID格式错误"
// @Failure 500 {object} map[string]interface{} "服务器内部错误"
// @Router /api/reimbursements/{id}/approvals [get]
func (s *ApprovalService) GetProgress(c *gin.Context) {
	reimbursementID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "报销单ID格式错误"})
		return
	}

	records, err := s.biz.GetProgress(uint(reimbursementID))
	if err != nil {
		s.logger.Error("获取审批进度失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取审批进度失败"})
		return
	}

	resp := make([]*ApprovalRecordResponse, 0, len(records))
	for _, r := range records {
		resp = append(resp, toApprovalRecordResponse(r))
	}
	c.JSON(http.StatusOK, resp)
}

// Approve 审批通过
// @Summary 审批通过
// @Description 审批人对指定审批记录进行通过操作，可附带审批意见
// @Tags 审批管理
// @Accept json
// @Produce json
// @Param id path int true "审批记录ID"
// @Param request body object false "审批请求体，comment字段为可选审批意见"
// @Success 200 {object} map[string]interface{} "审批已通过"
// @Failure 400 {object} map[string]interface{} "审批记录ID格式错误"
// @Failure 409 {object} map[string]interface{} "审批操作失败"
// @Router /api/approvals/{id}/approve [post]
func (s *ApprovalService) Approve(c *gin.Context) {
	recordID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "审批记录ID格式错误"})
		return
	}

	var req struct {
		Comment string `json:"comment"`
	}
	_ = c.ShouldBindJSON(&req) // 审批意见可选

	if err := s.biz.Approve(uint(recordID), req.Comment); err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "审批已通过"})
}

// Reject 驳回审批
// @Summary 驳回审批
// @Description 审批人对指定审批记录进行驳回操作，必须提供驳回原因
// @Tags 审批管理
// @Accept json
// @Produce json
// @Param id path int true "审批记录ID"
// @Param request body object true "驳回请求体，reason字段必填"
// @Success 200 {object} map[string]interface{} "审批已驳回"
// @Failure 400 {object} map[string]interface{} "请求参数错误或驳回原因不能为空"
// @Failure 409 {object} map[string]interface{} "驳回操作失败"
// @Router /api/approvals/{id}/reject [post]
func (s *ApprovalService) Reject(c *gin.Context) {
	recordID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "审批记录ID格式错误"})
		return
	}

	var req struct {
		Reason string `json:"reason" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "驳回原因不能为空"})
		return
	}

	if err := s.biz.Reject(uint(recordID), req.Reason); err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "审批已驳回"})
}

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
