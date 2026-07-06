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
// @Summary 获取员工列表
// @Description 分页查询所有员工，返回员工列表及总数
// @Tags 员工管理
// @Accept json
// @Produce json
// @Param page query int false "页码，默认1"
// @Param page_size query int false "每页数量，默认10"
// @Success 200 {object} ListEmployeeResponse "员工列表"
// @Failure 500 {object} map[string]interface{} "服务器内部错误"
// @Router /api/employees [get]
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

// GetByID 根据ID获取员工详情
// @Summary 获取员工详情
// @Description 根据员工ID获取单个员工的详细信息
// @Tags 员工管理
// @Accept json
// @Produce json
// @Param id path int true "员工ID"
// @Success 200 {object} EmployeeResponse "员工详情"
// @Failure 400 {object} map[string]interface{} "员工ID格式错误"
// @Failure 404 {object} map[string]interface{} "员工不存在"
// @Router /api/employees/{id} [get]
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
// @Summary 获取审批人列表
// @Description 获取所有具有审批权限的员工列表
// @Tags 员工管理
// @Accept json
// @Produce json
// @Success 200 {array} EmployeeResponse "审批人列表"
// @Failure 500 {object} map[string]interface{} "服务器内部错误"
// @Router /api/employees/approvers [get]
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
// @Summary 创建新员工
// @Description 管理员创建新员工，工号必须唯一
// @Tags 员工管理
// @Accept json
// @Produce json
// @Param request body CreateEmployeeRequest true "创建员工请求"
// @Success 201 {object} EmployeeResponse "员工创建成功"
// @Failure 400 {object} map[string]interface{} "请求参数错误"
// @Failure 409 {object} map[string]interface{} "工号或邮箱已存在"
// @Router /api/employees [post]
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
// @Summary 更新员工信息
// @Description 管理员更新指定员工的姓名、邮箱、部门或角色
// @Tags 员工管理
// @Accept json
// @Produce json
// @Param id path int true "员工ID"
// @Param request body UpdateEmployeeRequest true "更新员工请求"
// @Success 200 {object} EmployeeResponse "员工更新成功"
// @Failure 400 {object} map[string]interface{} "请求参数错误"
// @Failure 409 {object} map[string]interface{} "工号或邮箱冲突"
// @Router /api/employees/{id} [put]
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
// @Summary 删除员工
// @Description 管理员删除指定员工（如有未完结的报销单则不可删除）
// @Tags 员工管理
// @Accept json
// @Produce json
// @Param id path int true "员工ID"
// @Success 200 {object} map[string]interface{} "员工删除成功"
// @Failure 400 {object} map[string]interface{} "员工ID格式错误"
// @Failure 409 {object} map[string]interface{} "员工存在关联数据，无法删除"
// @Router /api/employees/{id} [delete]
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
