package department

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/CycleZero/Reimbee/log"
	"github.com/CycleZero/Reimbee/model"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// DepartmentService 部门 HTTP 服务层——处理请求解析、参数校验、响应格式化
type DepartmentService struct {
	biz    *DepartmentBiz
	logger *log.Logger
}

// NewDepartmentService 创建部门 HTTP 服务层实例
func NewDepartmentService(biz *DepartmentBiz, logger *log.Logger) *DepartmentService {
	return &DepartmentService{biz: biz, logger: logger}
}

// List 获取部门列表
// @Summary 获取部门列表
// @Description 分页查询所有部门，返回部门列表及总数
// @Tags 部门管理
// @Accept json
// @Produce json
// @Param page query int false "页码，默认1"
// @Param page_size query int false "每页数量，默认10"
// @Success 200 {object} ListDepartmentResponse "部门列表"
// @Failure 500 {object} map[string]interface{} "服务器内部错误"
// @Router /api/departments [get]
func (s *DepartmentService) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))

	depts, total, err := s.biz.List(page, pageSize)
	if err != nil {
		s.logger.Error("获取部门列表失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取部门列表失败"})
		return
	}

	resp := make([]*DepartmentResponse, 0, len(depts))
	for _, d := range depts {
		resp = append(resp, toDepartmentResponse(d))
	}
	c.JSON(http.StatusOK, ListDepartmentResponse{List: resp, Total: total, Page: page})
}

// GetByID 根据ID获取部门详情
// @Summary 获取部门详情
// @Description 根据部门ID获取单个部门的详细信息
// @Tags 部门管理
// @Accept json
// @Produce json
// @Param id path int true "部门ID"
// @Success 200 {object} DepartmentResponse "部门详情"
// @Failure 400 {object} map[string]interface{} "部门ID格式错误"
// @Failure 404 {object} map[string]interface{} "部门不存在"
// @Router /api/departments/{id} [get]
func (s *DepartmentService) GetByID(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "部门ID格式错误"})
		return
	}

	dept, err := s.biz.GetByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "部门不存在"})
		return
	}
	c.JSON(http.StatusOK, toDepartmentResponse(dept))
}

// Create 创建部门
// @Summary 创建新部门
// @Description 管理员创建新部门，部门名称必须唯一
// @Tags 部门管理
// @Accept json
// @Produce json
// @Param request body CreateDepartmentRequest true "创建部门请求"
// @Success 201 {object} DepartmentResponse "部门创建成功"
// @Failure 400 {object} map[string]interface{} "请求参数错误"
// @Failure 409 {object} map[string]interface{} "部门名称已存在"
// @Router /api/departments [post]
func (s *DepartmentService) Create(c *gin.Context) {
	var req CreateDepartmentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	dept, err := s.biz.Create(req.Name, req.ManagerID)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, toDepartmentResponse(dept))
}

// Update 更新部门
// @Summary 更新部门信息
// @Description 管理员更新指定部门的名称或主管
// @Tags 部门管理
// @Accept json
// @Produce json
// @Param id path int true "部门ID"
// @Param request body UpdateDepartmentRequest true "更新部门请求"
// @Success 200 {object} DepartmentResponse "部门更新成功"
// @Failure 400 {object} map[string]interface{} "请求参数错误"
// @Failure 404 {object} map[string]interface{} "部门不存在"
// @Failure 409 {object} map[string]interface{} "部门名称冲突"
// @Router /api/departments/{id} [put]
func (s *DepartmentService) Update(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "部门ID格式错误"})
		return
	}

	var req UpdateDepartmentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	dept, err := s.biz.Update(uint(id), req.Name, req.ManagerID)
	if err != nil {
		if strings.Contains(err.Error(), "不存在") {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		} else {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		}
		return
	}
	c.JSON(http.StatusOK, toDepartmentResponse(dept))
}

// Delete 删除部门
// @Summary 删除部门
// @Description 管理员删除指定部门（如有员工归属则不可删除）
// @Tags 部门管理
// @Accept json
// @Produce json
// @Param id path int true "部门ID"
// @Success 200 {object} map[string]interface{} "部门删除成功"
// @Failure 400 {object} map[string]interface{} "部门ID格式错误"
// @Failure 404 {object} map[string]interface{} "部门不存在"
// @Failure 409 {object} map[string]interface{} "部门下存在关联数据，无法删除"
// @Router /api/departments/{id} [delete]
func (s *DepartmentService) Delete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "部门ID格式错误"})
		return
	}

	if err := s.biz.Delete(uint(id)); err != nil {
		if strings.Contains(err.Error(), "不存在") {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		} else {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "部门删除成功"})
}

// toDepartmentResponse 将模型转换为响应 DTO
func toDepartmentResponse(d *model.Department) *DepartmentResponse {
	return &DepartmentResponse{
		ID:        d.ID,
		Name:      d.Name,
		ManagerID: d.ManagerID,
		CreatedAt: d.CreatedAt.Format("2006-01-02 15:04:05"),
		UpdatedAt: d.UpdatedAt.Format("2006-01-02 15:04:05"),
	}
}
