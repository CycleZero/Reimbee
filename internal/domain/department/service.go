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

// GetByID 根据 ID 获取部门详情
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
