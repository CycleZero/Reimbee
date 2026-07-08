package compliance

import (
	"net/http"
	"strconv"

	"github.com/CycleZero/Reimbee/log"
	"github.com/CycleZero/Reimbee/model"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// ComplianceService 合规管理 HTTP 服务层
type ComplianceService struct {
	biz    *ComplianceBiz
	kb     *KnowledgeBase
	logger *log.Logger
}

// NewComplianceService 创建合规管理 HTTP 服务层实例
func NewComplianceService(biz *ComplianceBiz, kb *KnowledgeBase, logger *log.Logger) *ComplianceService {
	return &ComplianceService{biz: biz, kb: kb, logger: logger}
}

// IndexDocument 索引政策文档
// @Summary 索引政策文档
// @Description 上传并索引一份合规政策文档，系统自动分块后存入知识库
// @Tags 合规管理
// @Accept json
// @Produce json
// @Param request body object true "政策文档请求体，需包含 title、content、version、effective_date 字段"
// @Success 201 {object} map[string]interface{} "文档索引成功，返回文档ID"
// @Failure 400 {object} map[string]interface{} "请求参数错误"
// @Failure 500 {object} map[string]interface{} "服务器内部错误"
// @Router /api/policies/ingest [post]
func (s *ComplianceService) IndexDocument(c *gin.Context) {
	var req struct {
		Title         string `json:"title" binding:"required"`
		Content       string `json:"content" binding:"required"`
		Version       string `json:"version"`
		EffectiveDate string `json:"effective_date"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		s.logger.Warn("索引政策文档请求参数错误", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误：缺少必填字段 title 或 content"})
		return
	}

	// 设置默认值
	if req.Version == "" {
		req.Version = "v1"
	}

	doc := &model.PolicyDocument{
		Title:         req.Title,
		Content:       req.Content,
		Version:       req.Version,
		EffectiveDate: req.EffectiveDate,
		Status:        "active",
	}

	ctx := c.Request.Context()
	if err := s.kb.IndexDocument(ctx, doc); err != nil {
		s.logger.Error("索引政策文档失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "索引政策文档失败"})
		return
	}

	s.logger.Info("政策文档索引成功",
		zap.Uint("文档ID", doc.ID),
		zap.String("标题", doc.Title))
	c.JSON(http.StatusCreated, gin.H{"id": strconv.FormatUint(uint64(doc.ID), 10), "message": "文档索引成功"})
}

// CheckCompliance 执行合规检查
// @Summary 执行合规检查
// @Description 根据输入的报销信息（类别、金额、日期），调用知识库检索 + 规则评估进行合规判定
// @Tags 合规管理
// @Accept json
// @Produce json
// @Param request body ComplianceInput true "合规检查输入参数"
// @Success 200 {object} ComplianceOutput "合规检查结果"
// @Failure 400 {object} map[string]interface{} "请求参数错误"
// @Failure 500 {object} map[string]interface{} "服务器内部错误"
// @Router /api/policies/check [post]
func (s *ComplianceService) CheckCompliance(c *gin.Context) {
	var input ComplianceInput
	if err := c.ShouldBindJSON(&input); err != nil {
		s.logger.Warn("合规检查请求参数错误", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误：缺少必填字段 category、amount 或 invoice_date"})
		return
	}

	ctx := c.Request.Context()
	output, err := s.biz.CheckCompliance(ctx, &input)
	if err != nil {
		s.logger.Error("合规检查执行失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "合规检查执行失败"})
		return
	}

	s.logger.Info("合规检查完成",
		zap.String("类别", input.Category),
		zap.Int64("金额(分)", input.Amount),
		zap.String("结果", output.Result))
	c.JSON(http.StatusOK, output)
}
