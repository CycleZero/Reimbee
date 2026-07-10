package compliance

import (
	"net/http"
	"strconv"

	"github.com/CycleZero/Reimbee/log"
	"github.com/CycleZero/Reimbee/model"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type ComplianceService struct {
	biz        *ComplianceBiz
	kb         *KnowledgeBase
	policyRepo *PolicyRepo
	logger     *log.Logger
}

func NewComplianceService(biz *ComplianceBiz, kb *KnowledgeBase, policyRepo *PolicyRepo, logger *log.Logger) *ComplianceService {
	return &ComplianceService{biz: biz, kb: kb, policyRepo: policyRepo, logger: logger}
}

// IndexDocument 索引政策文档
// @Summary 索引政策文档（上传并向量化）
// @Description 将政策/合规文档上传到知识库，自动分块并建立向量索引。
// @Description 文档内容支持 Markdown 格式，按段落自动分块，每块 500 字节，相邻块重叠 50 字节。
// @Description 向量化依赖 embedding 配置（Qwen DashScope / Ollama），不可用时降级为关键词匹配。
// @Tags 知识库管理
// @Accept json
// @Produce json
// @Param request body object{title=string,content=string,version=string,effective_date=string} true "文档信息：title=标题 content=Markdown正文 version=版本号(默认v1) effective_date=生效日期"
// @Param Authorization header string true "Bearer JWT Token（需要管理员权限）"
// @Success 201 {object} map[string]interface{} "索引成功，返回文档ID"
// @Failure 400 {object} map[string]interface{} "请求参数错误（缺少 title 或 content）"
// @Failure 500 {object} map[string]interface{} "索引失败（Embedding API 不可用且分块写入异常）"
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}
	if req.Version == "" {
		req.Version = "v1"
	}
	doc := &model.PolicyDocument{
		Title: req.Title, Content: req.Content,
		Version: req.Version, EffectiveDate: req.EffectiveDate, Status: "active",
	}
	if err := s.kb.IndexDocument(c.Request.Context(), doc); err != nil {
		s.logger.Error("索引政策文档失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "索引文档失败"})
		return
	}
	s.logger.Info("政策文档索引成功", zap.Uint("文档ID", doc.ID), zap.String("标题", doc.Title))
	c.JSON(http.StatusCreated, gin.H{"id": strconv.FormatUint(uint64(doc.ID), 10), "message": "索引成功"})
}

// CheckCompliance 合规检查
// @Summary 执行合规检查
// @Description 对报销票据进行合规审核，支持单张票据和多明细两种模式。
// @Description 检查内容包括：费用类别匹配、金额限额校验、发票有效期检查。
// @Description 检查结果分为 pass(通过)、warning(警告)、error(违规) 三级。
// @Tags 合规审核
// @Accept json
// @Produce json
// @Param request body ComplianceInput true "合规检查输入（单张模式传 amount/category/invoice_date，多明细模式传 items[]）"
// @Param Authorization header string true "Bearer JWT Token"
// @Success 200 {object} ComplianceOutput "检查通过或有警告/违规"
// @Failure 400 {object} map[string]interface{} "请求参数错误"
// @Failure 500 {object} map[string]interface{} "合规检查执行失败"
// @Router /api/policies/check [post]
func (s *ComplianceService) CheckCompliance(c *gin.Context) {
	var input ComplianceInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}
	output, err := s.biz.CheckCompliance(c.Request.Context(), &input)
	if err != nil {
		s.logger.Error("合规检查失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "合规检查失败"})
		return
	}
	c.JSON(http.StatusOK, output)
}

// ListDocuments 获取文档列表
// @Summary 分页查询知识库文档列表
// @Description 按分页查询所有已索引的政策文档，返回文档基本信息（不含正文和分块）。
// @Tags 知识库管理
// @Accept json
// @Produce json
// @Param page query int false "页码（默认1）"
// @Param page_size query int false "每页数量（默认10）"
// @Param Authorization header string true "Bearer JWT Token（需要管理员权限）"
// @Success 200 {object} map[string]interface{} "文档列表（list/total/page）"
// @Failure 500 {object} map[string]interface{} "查询失败"
// @Router /api/admin/policies [get]
func (s *ComplianceService) ListDocuments(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	docs, total, err := s.policyRepo.List(page, pageSize)
	if err != nil {
		s.logger.Error("查询文档列表失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败"})
		return
	}
	resp := make([]PolicyDocumentResponse, 0, len(docs))
	for _, doc := range docs {
		resp = append(resp, PolicyDocumentResponse{
			ID: doc.ID, Title: doc.Title, Version: doc.Version,
			EffectiveDate: doc.EffectiveDate, Status: doc.Status,
			ChunkCount: len(doc.Chunks),
			CreatedAt: doc.CreatedAt.Format("2006-01-02 15:04:05"),
			UpdatedAt: doc.UpdatedAt.Format("2006-01-02 15:04:05"),
		})
	}
	c.JSON(http.StatusOK, gin.H{"list": resp, "total": total, "page": page})
}

// GetDocument 获取文档详情
// @Summary 获取单个文档详情（含分块）
// @Description 根据文档 ID 获取完整内容、分块列表和元数据。
// @Tags 知识库管理
// @Accept json
// @Produce json
// @Param id path int true "文档ID"
// @Param Authorization header string true "Bearer JWT Token（需要管理员权限）"
// @Success 200 {object} PolicyDocumentDetailResponse "文档详情"
// @Failure 400 {object} map[string]interface{} "ID格式错误"
// @Failure 404 {object} map[string]interface{} "文档不存在"
// @Router /api/admin/policies/{id} [get]
func (s *ComplianceService) GetDocument(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID格式错误"})
		return
	}
	doc, err := s.policyRepo.GetByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "文档不存在"})
		return
	}
	chunks := make([]ChunkResponse, 0, len(doc.Chunks))
	for _, c := range doc.Chunks {
		chunks = append(chunks, ChunkResponse{ID: c.ID, ChunkIndex: c.ChunkIndex, Content: c.Content})
	}
	c.JSON(http.StatusOK, PolicyDocumentDetailResponse{
		ID: doc.ID, Title: doc.Title, Content: doc.Content,
		Version: doc.Version, EffectiveDate: doc.EffectiveDate, Status: doc.Status,
		ChunkCount: len(doc.Chunks), Chunks: chunks,
		CreatedAt: doc.CreatedAt.Format("2006-01-02 15:04:05"),
		UpdatedAt: doc.UpdatedAt.Format("2006-01-02 15:04:05"),
	})
}

// UpdateDocument 更新文档
// @Summary 更新知识库文档
// @Description 修改文档标题、内容、版本号等元数据，内容变更时自动重建向量索引。
// @Tags 知识库管理
// @Accept json
// @Produce json
// @Param id path int true "文档ID"
// @Param request body UpdatePolicyRequest true "更新请求体：title/content/version/effective_date/status"
// @Param Authorization header string true "Bearer JWT Token（需要管理员权限）"
// @Success 200 {object} map[string]interface{} "更新成功"
// @Failure 400 {object} map[string]interface{} "ID格式错误或请求参数错误"
// @Failure 404 {object} map[string]interface{} "文档不存在"
// @Failure 500 {object} map[string]interface{} "更新失败或重建索引失败"
// @Router /api/admin/policies/{id} [put]
func (s *ComplianceService) UpdateDocument(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID格式错误"})
		return
	}
	var req UpdatePolicyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}
	doc, err := s.policyRepo.GetByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "文档不存在"})
		return
	}

	contentChanged := doc.Content != req.Content
	doc.Title = req.Title
	doc.Content = req.Content
	doc.Version = req.Version
	doc.EffectiveDate = req.EffectiveDate
	if req.Status != "" {
		doc.Status = req.Status
	}

	if err := s.policyRepo.Update(doc); err != nil {
		s.logger.Error("更新文档失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新失败"})
		return
	}

	if contentChanged {
		if err := s.kb.ReIndex(c.Request.Context(), doc); err != nil {
			s.logger.Error("重建索引失败", zap.Error(err))
		}
	}

	s.logger.Info("文档更新成功", zap.Uint("ID", doc.ID))
	c.JSON(http.StatusOK, gin.H{"message": "更新成功"})
}

// DeleteDocument 删除文档
// @Summary 删除知识库文档
// @Description 删除指定文档及其所有分块和向量索引。
// @Tags 知识库管理
// @Accept json
// @Produce json
// @Param id path int true "文档ID"
// @Param Authorization header string true "Bearer JWT Token（需要管理员权限）"
// @Success 200 {object} map[string]interface{} "删除成功"
// @Failure 400 {object} map[string]interface{} "ID格式错误"
// @Failure 500 {object} map[string]interface{} "删除失败"
// @Router /api/admin/policies/{id} [delete]
func (s *ComplianceService) DeleteDocument(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID格式错误"})
		return
	}
	if err := s.kb.DeleteDocument(c.Request.Context(), uint(id)); err != nil {
		s.logger.Error("删除文档失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "删除成功"})
}

// GetStatus 获取知识库运行状态
// @Summary 查看知识库运行状态
// @Description 返回文档总数、分块总数、当前检索模式（向量/关键词）、嵌入模型和向量库信息。
// @Tags 知识库管理
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer JWT Token（需要管理员权限）"
// @Success 200 {object} KnowledgeBaseStatus "知识库状态"
// @Router /api/admin/policies/status [get]
func (s *ComplianceService) GetStatus(c *gin.Context) {
	c.JSON(http.StatusOK, s.kb.Status())
}

// SearchTest 搜索测试
// @Summary 知识库搜索测试
// @Description 对已索引的政策文档执行搜索，返回匹配的分块及来源文档信息。用于验证知识库检索效果。
// @Tags 知识库管理
// @Accept json
// @Produce json
// @Param query query string true "搜索关键词或自然语言问题"
// @Param limit query int false "返回结果数（默认5）"
// @Param Authorization header string true "Bearer JWT Token（需要管理员权限）"
// @Success 200 {object} SearchTestResponse "搜索结果"
// @Failure 400 {object} map[string]interface{} "缺少 query 参数"
// @Failure 500 {object} map[string]interface{} "搜索执行失败"
// @Router /api/admin/policies/search [get]
func (s *ComplianceService) SearchTest(c *gin.Context) {
	query := c.Query("query")
	if query == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少 query 参数"})
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "5"))

	chunks, err := s.kb.Search(c.Request.Context(), query, limit)
	if err != nil {
		s.logger.Error("搜索测试失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "搜索失败"})
		return
	}

	mode := s.kb.Status().SearchMode
	results := make([]SearchTestChunk, 0, len(chunks))
	for _, c := range chunks {
		results = append(results, SearchTestChunk{
			DocumentID:    c.DocumentID,
			DocumentTitle: s.kb.GetDocumentTitle(c.DocumentID),
			ChunkIndex:    c.ChunkIndex,
			Content:       c.Content,
		})
	}

	c.JSON(http.StatusOK, SearchTestResponse{Query: query, Mode: mode, Chunks: results})
}
