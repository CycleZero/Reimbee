package reimbursement

import (
	"net/http"
	"strconv"

	"github.com/CycleZero/Reimbee/internal/domain/approval"
	"github.com/CycleZero/Reimbee/log"
	"github.com/CycleZero/Reimbee/model"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// ReimbursementService 报销单 HTTP 服务层
type ReimbursementService struct {
	biz          *ReimbursementBiz
	approvalBiz  *approval.ApprovalBiz
	logger       *log.Logger
}

// NewReimbursementService 创建报销单 HTTP 服务层实例
func NewReimbursementService(biz *ReimbursementBiz, approvalBiz *approval.ApprovalBiz, logger *log.Logger) *ReimbursementService {
	return &ReimbursementService{biz: biz, approvalBiz: approvalBiz, logger: logger}
}

// List 获取报销单列表
// @Summary 获取报销单列表
// @Description 分页查询报销单，可按员工工号筛选
// @Tags 报销管理
// @Accept json
// @Produce json
// @Param page query int false "页码，默认1"
// @Param page_size query int false "每页数量，默认10"
// @Param employee_id query string false "员工工号（可选筛选）"
// @Success 200 {object} ListReimbursementResponse "报销单列表"
// @Failure 500 {object} map[string]interface{} "服务器内部错误"
// @Router /api/reimbursements [get]
func (s *ReimbursementService) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	employeeID := c.Query("employee_id")

	rms, total, err := s.biz.List(page, pageSize, employeeID)
	if err != nil {
		s.logger.Error("获取报销单列表失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取报销单列表失败"})
		return
	}

	resp := make([]*ReimbursementResponse, 0, len(rms))
	for _, rm := range rms {
		resp = append(resp, toReimbursementResponse(rm))
	}
	c.JSON(http.StatusOK, ListReimbursementResponse{List: resp, Total: total, Page: page})
}

// ListPending 获取待审批报销单
// @Summary 获取待审批报销单
// @Description 获取所有状态为待审批的报销单列表（审批人专用）
// @Tags 报销管理
// @Accept json
// @Produce json
// @Success 200 {array} ReimbursementResponse "待审批报销单列表"
// @Failure 500 {object} map[string]interface{} "服务器内部错误"
// @Router /api/reimbursements/pending [get]
func (s *ReimbursementService) ListPending(c *gin.Context) {
	rms, err := s.biz.ListPending()
	if err != nil {
		s.logger.Error("获取待审批报销单失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取待审批报销单失败"})
		return
	}

	resp := make([]*ReimbursementResponse, 0, len(rms))
	for _, rm := range rms {
		resp = append(resp, toReimbursementResponse(rm))
	}
	c.JSON(http.StatusOK, resp)
}

// GetByID 根据ID获取报销单详情
// @Summary 获取报销单详情（按ID）
// @Description 根据报销单ID获取报销单详细信息，包含票据和审批记录
// @Tags 报销管理
// @Accept json
// @Produce json
// @Param id path int true "报销单ID"
// @Success 200 {object} ReimbursementResponse "报销单详情"
// @Failure 400 {object} map[string]interface{} "报销单ID格式错误"
// @Failure 404 {object} map[string]interface{} "报销单不存在"
// @Router /api/reimbursements/{id} [get]
func (s *ReimbursementService) GetByID(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "报销单ID格式错误"})
		return
	}

	rm, err := s.biz.GetByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "报销单不存在"})
		return
	}
	c.JSON(http.StatusOK, toReimbursementResponse(rm))
}

// GetByNo 根据报销单号获取详情
// @Summary 获取报销单详情（按单号）
// @Description 根据报销单号获取报销单详细信息，包含票据和审批记录
// @Tags 报销管理
// @Accept json
// @Produce json
// @Param no path string true "报销单号（如 REIMB-2026-0001）"
// @Success 200 {object} ReimbursementResponse "报销单详情"
// @Failure 404 {object} map[string]interface{} "报销单不存在"
// @Router /api/reimbursements/no/{no} [get]
func (s *ReimbursementService) GetByNo(c *gin.Context) {
	no := c.Param("no")
	rm, err := s.biz.GetByNo(no)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "报销单不存在"})
		return
	}
	c.JSON(http.StatusOK, toReimbursementResponse(rm))
}

// Create 创建报销单（草稿）
// @Summary 创建报销单（草稿）
// @Description 员工创建报销单草稿，状态为 draft，需后续提交
// @Tags 报销管理
// @Accept json
// @Produce json
// @Param request body CreateReimbursementRequest true "创建报销单请求"
// @Success 201 {object} ReimbursementResponse "报销单创建成功"
// @Failure 400 {object} map[string]interface{} "请求参数错误"
// @Failure 500 {object} map[string]interface{} "服务器内部错误"
// @Router /api/reimbursements [post]
func (s *ReimbursementService) Create(c *gin.Context) {
	var req CreateReimbursementRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	rm, err := s.biz.Create(req.EmployeeID, req.EmployeeName, req.DepartmentID, req.SubmitNote)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, toReimbursementResponse(rm))
}

// Submit 提交报销单
// @Summary 提交报销单
// @Description 将草稿状态的报销单提交进入审批流程
// @Tags 报销管理
// @Accept json
// @Produce json
// @Param id path int true "报销单ID"
// @Param request body SubmitReimbursementRequest true "提交报销单请求"
// @Success 200 {object} ReimbursementResponse "报销单提交成功"
// @Failure 400 {object} map[string]interface{} "请求参数错误"
// @Failure 409 {object} map[string]interface{} "提交失败（状态不允许或预算不足）"
// @Router /api/reimbursements/{id}/submit [post]
func (s *ReimbursementService) Submit(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "报销单ID格式错误"})
		return
	}

	var req SubmitReimbursementRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	rm, err := s.biz.Submit(uint(id), req.TotalAmount)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, toReimbursementResponse(rm))
}

// Approve 审批通过报销单
// @Summary 审批通过报销单
// @Description 审批人通过报销单，更新报销单状态并记录审批操作
// @Tags 报销管理
// @Accept json
// @Produce json
// @Param id path int true "报销单ID"
// @Success 200 {object} ReimbursementResponse "审批通过成功"
// @Failure 400 {object} map[string]interface{} "报销单ID格式错误"
// @Failure 409 {object} map[string]interface{} "审批操作失败（状态不允许或无权限）"
// @Router /api/reimbursements/{id}/approve [post]
func (s *ReimbursementService) Approve(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "报销单ID格式错误"})
		return
	}

	rm, err := s.biz.Approve(uint(id))
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, toReimbursementResponse(rm))
}

// Reject 驳回报销单
// @Summary 驳回报销单
// @Description 审批人驳回报销单，报销单状态回退为 draft
// @Tags 报销管理
// @Accept json
// @Produce json
// @Param id path int true "报销单ID"
// @Success 200 {object} ReimbursementResponse "驳回成功"
// @Failure 400 {object} map[string]interface{} "报销单ID格式错误"
// @Failure 409 {object} map[string]interface{} "驳回操作失败（状态不允许或无权限）"
// @Router /api/reimbursements/{id}/reject [post]
func (s *ReimbursementService) Reject(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "报销单ID格式错误"})
		return
	}

	rm, err := s.biz.Reject(uint(id))
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, toReimbursementResponse(rm))
}

// toReimbursementResponse 将模型转换为响应 DTO
func toReimbursementResponse(rm *model.Reimbursement) *ReimbursementResponse {
	resp := &ReimbursementResponse{
		ID:                  rm.ID,
		ReimbursementNo:     rm.ReimbursementNo,
		EmployeeID:          rm.EmployeeID,
		EmployeeName:        rm.EmployeeName,
		DepartmentID:        rm.DepartmentID,
		TotalAmount:         rm.TotalAmount,
		Status:              rm.Status,
		SubmitNote:          rm.SubmitNote,
		NeedSpecialApproval: rm.NeedSpecialApproval,
		CreatedAt:           rm.CreatedAt.Format("2006-01-02 15:04:05"),
		UpdatedAt:           rm.UpdatedAt.Format("2006-01-02 15:04:05"),
	}
	if rm.Department != nil {
		resp.Department = rm.Department.Name
	}
	if rm.Invoices != nil {
		resp.Invoices = make([]*InvoiceItemResponse, 0, len(rm.Invoices))
		for _, inv := range rm.Invoices {
			resp.Invoices = append(resp.Invoices, &InvoiceItemResponse{
				ID:          inv.ID,
				Amount:      inv.Amount,
				InvoiceDate: inv.InvoiceDate,
				Category:    inv.Category,
				CheckResult: inv.CheckResult,
			})
		}
	}
	if rm.Approvals != nil {
		resp.Approvals = make([]*ApprovalInfoResponse, 0, len(rm.Approvals))
		for _, a := range rm.Approvals {
			info := &ApprovalInfoResponse{
				ID:           a.ID,
				ApproverName: a.ApproverName,
				Action:       a.Action,
				Comment:      a.Comment,
			}
			if a.ActionAt != nil {
				info.ActionAt = a.ActionAt.Format("2006-01-02 15:04:05")
			}
			resp.Approvals = append(resp.Approvals, info)
		}
	}
	return resp
}
