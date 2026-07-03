package demo

import (
	"net/http"
	"strconv"

	"gin-template/log"
	"gin-template/model"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// DemoService HTTP 服务层 - 处理请求解析、参数校验、响应格式化
type DemoService struct {
	demoBiz *DemoBiz
	logger  *log.Logger
}

func NewDemoService(demoBiz *DemoBiz, logger *log.Logger) *DemoService {
	return &DemoService{
		demoBiz: demoBiz,
		logger:  logger,
	}
}

// Create 创建 Demo
// @Summary 创建 Demo
// @Tags demo
// @Accept json
// @Produce json
// @Param request body CreateDemoRequest true "创建请求"
// @Success 200 {object} DemoResponse
// @Router /api/demo [post]
func (s *DemoService) Create(c *gin.Context) {
	var req CreateDemoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		s.logger.Error("解析创建请求失败", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	demo, err := s.demoBiz.Create(req.Name, req.Description, 0)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建失败"})
		return
	}

	c.JSON(http.StatusOK, toResponse(demo))
}

// GetByID 获取 Demo 详情
// @Summary 获取 Demo
// @Tags demo
// @Produce json
// @Param id path int true "Demo ID"
// @Success 200 {object} DemoResponse
// @Router /api/demo/{id} [get]
func (s *DemoService) GetByID(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID 格式错误"})
		return
	}

	demo, err := s.demoBiz.GetByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "记录不存在"})
		return
	}

	c.JSON(http.StatusOK, toResponse(demo))
}

// List 获取 Demo 列表
// @Summary 获取 Demo 列表
// @Tags demo
// @Produce json
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(10)
// @Success 200 {object} ListDemoResponse
// @Router /api/demo [get]
func (s *DemoService) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))

	demos, total, err := s.demoBiz.List(page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败"})
		return
	}

	responses := make([]*DemoResponse, 0, len(demos))
	for _, d := range demos {
		responses = append(responses, toResponse(d))
	}

	c.JSON(http.StatusOK, ListDemoResponse{
		List:  responses,
		Total: total,
		Page:  page,
	})
}

// Update 更新 Demo
// @Summary 更新 Demo
// @Tags demo
// @Accept json
// @Produce json
// @Param id path int true "Demo ID"
// @Param request body UpdateDemoRequest true "更新请求"
// @Success 200 {object} DemoResponse
// @Router /api/demo/{id} [put]
func (s *DemoService) Update(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID 格式错误"})
		return
	}

	var req UpdateDemoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	demo, err := s.demoBiz.Update(uint(id), req.Name, req.Description)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新失败"})
		return
	}

	c.JSON(http.StatusOK, toResponse(demo))
}

// Delete 删除 Demo
// @Summary 删除 Demo
// @Tags demo
// @Produce json
// @Param id path int true "Demo ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/demo/{id} [delete]
func (s *DemoService) Delete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID 格式错误"})
		return
	}

	if err := s.demoBiz.Delete(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "删除成功"})
}

// toResponse 将 model 转换为响应 DTO
func toResponse(demo *model.Demo) *DemoResponse {
	return &DemoResponse{
		ID:          demo.ID,
		Name:        demo.Name,
		Description: demo.Description,
		Status:      demo.Status,
		CreatedBy:   demo.CreatedBy,
		CreatedAt:   demo.CreatedAt.Format("2006-01-02 15:04:05"),
		UpdatedAt:   demo.UpdatedAt.Format("2006-01-02 15:04:05"),
	}
}
