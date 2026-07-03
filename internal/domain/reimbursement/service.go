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

// GetByID 根据 ID 获取报销单详情
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
