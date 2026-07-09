// Package tools RAG 政策检索工具
// 将合规知识库的向量检索能力封装为 Blades Tool，
// 供 ComplianceMiniAgent 在 ReAct 循环中调用。
// 工具只负责检索政策文档片段，不负责判定合规性。
package tools

import (
	"context"
	"fmt"

	"github.com/CycleZero/Reimbee/internal/domain/compliance"
	"github.com/CycleZero/Reimbee/log"
	"github.com/CycleZero/blades/tools"
	"go.uber.org/zap"
)

// SearchPolicyInput 政策检索工具的输入参数
type SearchPolicyInput struct {
	Query string `json:"query"` // 检索查询（自然语言，如"差旅住宿费报销标准"）
	Limit int    `json:"limit"` // 返回结果数上限，默认5条
}

// SearchPolicyOutput 政策检索工具的输出结果
type SearchPolicyOutput struct {
	Chunks []PolicyChunk `json:"chunks"` // 检索到的政策文档片段
}

// PolicyChunk 单个政策文档片段（工具层 DTO）
type PolicyChunk struct {
	RuleID  string  `json:"rule_id"` // 规则ID（如 RULE-TRAVEL-002）
	Title   string  `json:"title"`   // 政策标题
	Content string  `json:"content"` // 政策原文片段
	Score   float64 `json:"score"`   // 语义相似度分数
}

// SearchPolicyTool Wire 命名类型（Blades tools.Tool）
type SearchPolicyTool struct{ tools.Tool }

// NewSearchPolicyTool 创建政策检索工具
func NewSearchPolicyTool(kb *compliance.KnowledgeBase, logger *log.Logger) *SearchPolicyTool {
	t, err := tools.NewFunc[SearchPolicyInput, SearchPolicyOutput](
		ToolSearchPolicy,
		"检索企业报销政策RAG知识库。输入自然语言查询（如'差旅住宿标准'），返回最相关的政策文档片段（包括费用标准、审批流程、特殊规定等）。",
		func(ctx context.Context, input SearchPolicyInput) (SearchPolicyOutput, error) {
			if input.Limit <= 0 {
				input.Limit = 5
			}

			chunks, err := kb.Search(ctx, input.Query, input.Limit)
			if err != nil {
				return SearchPolicyOutput{}, fmt.Errorf("政策检索失败: %w", err)
			}

			result := make([]PolicyChunk, 0, len(chunks))
			for _, c := range chunks {
				result = append(result, PolicyChunk{
					RuleID:  fmt.Sprintf("DOC-%d-CHUNK-%d", c.DocumentID, c.ChunkIndex),
					Title:   fmt.Sprintf("政策文档#%d", c.DocumentID),
					Content: c.Content,
					Score:   0.0, // 关键词降级时无 Score
				})
			}

			logger.Debug("政策检索完成",
				zap.String("查询", input.Query),
				zap.Int("命中数", len(result)))

			return SearchPolicyOutput{Chunks: result}, nil
		},
	)
	if err != nil {
		panic("创建search_policy工具失败: " + err.Error())
	}
	logger.Info("政策检索工具初始化完成")
	return &SearchPolicyTool{t}
}
