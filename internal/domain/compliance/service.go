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

func (s *ComplianceService) GetStatus(c *gin.Context) {
	c.JSON(http.StatusOK, s.kb.Status())
}

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
