package reimbursement

import (
	"net/http"
	"path/filepath"
	"strconv"
	"time"

	"github.com/CycleZero/Reimbee/infra"
	"github.com/CycleZero/Reimbee/internal/common"
	"github.com/CycleZero/Reimbee/internal/domain/approval"
	"github.com/CycleZero/Reimbee/log"
	"github.com/CycleZero/Reimbee/model"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// ReimbursementService 报销单 HTTP 服务层
type ReimbursementService struct {
	biz          *ReimbursementBiz
	approvalBiz  *approval.ApprovalBiz
	storage      infra.FileStorage // v3.0: 文件上传存储
	logger       *log.Logger
}

// NewReimbursementService 创建报销单 HTTP 服务层实例
func NewReimbursementService(biz *ReimbursementBiz, approvalBiz *approval.ApprovalBiz, storage infra.FileStorage, logger *log.Logger) *ReimbursementService {
	return &ReimbursementService{biz: biz, approvalBiz: approvalBiz, storage: storage, logger: logger}
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
	// 数据隔离：默认按当前登录用户的工号过滤
	employeeID := c.Query("employee_id")
	if employeeID == "" {
		if meta := common.GetRequestMetadata(c); meta != nil {
			employeeID = meta.EmployeeID
		}
	}

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
	// 按当前审批人姓名过滤待审批报销单
	approverName := ""
	if meta := common.GetRequestMetadata(c); meta != nil {
		approverName = meta.EmployeeName
	}
	var rms []*model.Reimbursement
	var err error
	if approverName != "" {
		rms, err = s.biz.ListPendingByApprover(approverName)
	} else {
		rms, err = s.biz.ListPending()
	}
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

// UploadInvoice 上传票据图片
//
// @Summary 上传票据图片
// @Description 上传票据图片文件（支持 JPG/PNG/PDF），保存到文件存储（本地磁盘或 MinIO），返回文件路径供 Agent OCR 工具使用。
//
//	前端流程：用户选择图片 → 调用本接口上传 → 获得 file_path → 在对话中告知 Agent 路径
//	Agent 流程：LLM 调用 recognize_invoice 工具 → storage.Get(file_path) → OCR 识别 → 结果存入 ReimbursementState
//
// @Tags 报销管理
// @Accept multipart/form-data
// @Produce json
// @Param file formData file true "票据图片文件（JPG/PNG/PDF/BMP/TIFF，最大 10MB）"
// @Success 200 {object} UploadInvoiceResponse "上传成功，返回文件路径和访问 URL"
// @Failure 400 {object} map[string]interface{} "未选择文件或文件类型不支持"
// @Failure 413 {object} map[string]interface{} "文件过大"
// @Failure 500 {object} map[string]interface{} "文件存储失败"
// @Router /api/reimbursements/upload [post]
func (s *ReimbursementService) UploadInvoice(c *gin.Context) {
	s.logger.Debug("收到票据上传请求")

	// ── 步骤 0: 从 JWT 提取用户身份 ──
	employeeID, _ := c.Get("employee_id")
	empID, _ := employeeID.(string)
	if empID == "" {
		s.logger.Warn("无法获取上传用户身份，拒绝上传")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未认证，请先登录"})
		return
	}

	s.logger.Debug("上传用户身份已确认",
		zap.String("employeeID", empID))

	// ── 步骤 1: 获取上传文件 ──
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		s.logger.Warn("获取上传文件失败", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "请选择要上传的票据图片"})
		return
	}
	defer file.Close()

	// ── 步骤 2: 校验文件大小（限制 10MB）──
	const maxSize = 10 * 1024 * 1024
	if header.Size > maxSize {
		s.logger.Warn("上传文件过大",
			zap.Int64("文件大小(字节)", header.Size),
			zap.String("employeeID", empID))
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "文件大小超过 10MB 限制"})
		return
	}

	// ── 步骤 3: 校验 MIME 类型 ──
	mimeType := header.Header.Get("Content-Type")
	if !isSupportedImageType(mimeType) {
		s.logger.Warn("不支持的文件类型",
			zap.String("MIME类型", mimeType),
			zap.String("文件名", header.Filename))
		c.JSON(http.StatusBadRequest, gin.H{"error": "不支持的文件类型，仅支持 JPG/PNG/PDF/BMP/TIFF"})
		return
	}

	// ── 步骤 4: 生成用户隔离的存储路径 ──
	// 路径格式: {employeeID}/{日期}/{uuid}.{ext}
	// 例如: EMP001/2026/07/07/a1b2c3d4.jpg
	// 确保每个用户的文件存储在独立目录下，互不干扰
	fileID := uuid.New().String()
	ext := filepath.Ext(header.Filename)
	if ext == "" {
		ext = ".jpg"
	}
	savedName := fileID + ext
	now := time.Now()
	dateDir := now.Format("2006/01/02")
	relativePath := filepath.Join(empID, dateDir, savedName)

	s.logger.Debug("生成用户隔离存储路径",
		zap.String("employeeID", empID),
		zap.String("相对路径", relativePath))

	// ── 步骤 6: 写入文件存储 ──
	// 使用 FileStorage.Save 写入（本地磁盘或 MinIO）
	// pathPrefix 参数指示在基础路径下按用户分目录
	uploaded, err := s.storage.Save(c.Request.Context(), relativePath, mimeType, file)
	if err != nil {
		s.logger.Error("文件存储失败",
			zap.String("employeeID", empID),
			zap.String("文件名", header.Filename),
			zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "文件保存失败，请稍后重试"})
		return
	}

	// ── 步骤 7: 记录存储信息 ──
	s.logger.Info("票据上传成功",
		zap.String("employeeID", empID),
		zap.String("文件ID", uploaded.FileID),
		zap.String("存储路径", uploaded.Path),
		zap.Int64("文件大小(字节)", uploaded.Size))

	// ── 步骤 8: 返回文件路径（供 Agent OCR 工具使用）──
	c.JSON(http.StatusOK, UploadInvoiceResponse{
		FileID:   uploaded.FileID,
		FileName: header.Filename,
		FilePath: uploaded.Path,
		URL:      uploaded.URL,
		Size:     uploaded.Size,
	})
}

// isSupportedImageType 检查 MIME 类型是否在支持列表中
func isSupportedImageType(mimeType string) bool {
	switch mimeType {
	case "image/jpeg", "image/png", "image/bmp", "image/tiff",
		"application/pdf":
		return true
	default:
		return false
	}
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

	var req struct {
		Reason string `json:"reason"` // 驳回原因
	}
	_ = c.ShouldBindJSON(&req) // 驳回原因可选

	rm, err := s.biz.Reject(uint(id), req.Reason)
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
