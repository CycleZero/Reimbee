package employee

import (
	"net/http"
	"strconv"

	"github.com/CycleZero/Reimbee/log"
	"github.com/CycleZero/Reimbee/model"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// EmployeeService 员工 HTTP 服务层
type EmployeeService struct {
	biz    *EmployeeBiz
	logger *log.Logger
}

// NewEmployeeService 创建员工 HTTP 服务层实例
func NewEmployeeService(biz *EmployeeBiz, logger *log.Logger) *EmployeeService {
	return &EmployeeService{biz: biz, logger: logger}
}

// List 获取员工列表
func (s *EmployeeService) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))

	emps, total, err := s.biz.List(page, pageSize)
	if err != nil {
		s.logger.Error("获取员工列表失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取员工列表失败"})
		return
	}

	resp := make([]*EmployeeResponse, 0, len(emps))
	for _, e := range emps {
		resp = append(resp, toEmployeeResponse(e))
	}
	c.JSON(http.StatusOK, ListEmployeeResponse{List: resp, Total: total, Page: page})
}

// GetByID 根据 ID 获取员工详情
func (s *EmployeeService) GetByID(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "员工ID格式错误"})
		return
	}

	emp, err := s.biz.GetByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "员工不存在"})
		return
	}
	c.JSON(http.StatusOK, toEmployeeResponse(emp))
}

// ListApprovers 获取审批人列表
func (s *EmployeeService) ListApprovers(c *gin.Context) {
	approvers, err := s.biz.ListApprovers()
	if err != nil {
		s.logger.Error("获取审批人列表失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取审批人列表失败"})
		return
	}

	resp := make([]*EmployeeResponse, 0, len(approvers))
	for _, e := range approvers {
		resp = append(resp, toEmployeeResponse(e))
	}
	c.JSON(http.StatusOK, resp)
}

// Create 创建员工
func (s *EmployeeService) Create(c *gin.Context) {
	var req CreateEmployeeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}
	if req.Role == "" {
		req.Role = "employee"
	}

	emp, err := s.biz.Create(req.EmployeeID, req.Name, req.Email, req.DepartmentID, req.Role)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, toEmployeeResponse(emp))
}

// Update 更新员工
func (s *EmployeeService) Update(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "员工ID格式错误"})
		return
	}

	var req UpdateEmployeeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}
	if req.Role == "" {
		req.Role = "employee"
	}

	emp, err := s.biz.Update(uint(id), req.Name, req.Email, req.DepartmentID, req.Role)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, toEmployeeResponse(emp))
}

// Delete 删除员工
func (s *EmployeeService) Delete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "员工ID格式错误"})
		return
	}

	if err := s.biz.Delete(uint(id)); err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "员工删除成功"})
}

// toEmployeeResponse 将模型转换为响应 DTO
func toEmployeeResponse(e *model.Employee) *EmployeeResponse {
	resp := &EmployeeResponse{
		ID:           e.ID,
		EmployeeID:   e.EmployeeID,
		Name:         e.Name,
		Email:        e.Email,
		DepartmentID: e.DepartmentID,
		Role:         e.Role,
		IsApprover:   e.IsApprover,
		CreatedAt:    e.CreatedAt.Format("2006-01-02 15:04:05"),
		UpdatedAt:    e.UpdatedAt.Format("2006-01-02 15:04:05"),
	}
	if e.Department != nil {
		resp.Department = e.Department.Name
	}
	return resp
}
