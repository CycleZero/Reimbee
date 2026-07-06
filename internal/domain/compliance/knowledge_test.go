package compliance

import (
	"context"
	"testing"

	zaplog "github.com/CycleZero/Reimbee/log"
	"go.uber.org/zap"

	"github.com/CycleZero/Reimbee/infra"
	"github.com/CycleZero/Reimbee/infra/embedding"
	"github.com/CycleZero/Reimbee/infra/vectorstore"
	"github.com/CycleZero/Reimbee/internal/testutil"
	"github.com/CycleZero/Reimbee/model"
)

// ============================================================
// 全局初始化：确保 vectorstore 内部调用 log.GetLogger() 不 panic
// ============================================================

func init() {
	// 设置全局 logger 为 nop，避免 InMemoryStore 等方法中调用 log.GetLogger() 时 panic
	zaplog.SetGlobalLogger(&zaplog.Logger{Logger: zap.NewNop()})
}

// ============================================================
// mockEmbedder：用于测试的固定向量嵌入器
// ============================================================

type mockEmbedder struct {
	dim int
}

func (m *mockEmbedder) Embed(_ context.Context, texts []string) ([][]float64, error) {
	results := make([][]float64, len(texts))
	for i, t := range texts {
		v := make([]float64, m.dim)
		for j := 0; j < m.dim && j < len(t); j++ {
			v[j] = float64(t[j]) / 255.0
		}
		results[i] = v
	}
	return results, nil
}

func (m *mockEmbedder) Dimensions() int                     { return m.dim }
func (m *mockEmbedder) ModelName() string                   { return "mock" }
func (m *mockEmbedder) HealthCheck(_ context.Context) error { return nil }

// 编译期接口检查
var _ embedding.Embedder = (*mockEmbedder)(nil)

// ============================================================
// 测试辅助
// ============================================================

// newTestLogger 创建适用于测试的 Logger 实例
func newTestLogger() *zaplog.Logger {
	return &zaplog.Logger{Logger: zap.NewNop()}
}

// newTestKnowledgeBase 创建带测试 DB 的 KnowledgeBase 实例
// embedder 和 vectorStore 传 nil 时仅使用关键词匹配
func newTestKnowledgeBase(t *testing.T, embedder embedding.Embedder, vectorStore vectorstore.VectorStore) (*KnowledgeBase, *infra.Data) {
	t.Helper()
	data := testutil.NewTestData()
	logger := newTestLogger()
	kb := NewKnowledgeBase(data, embedder, vectorStore, logger)
	return kb, data
}

// samplePolicyContent 返回一段模拟的差旅报销政策文本
var samplePolicyContent = `差旅费报销管理办法

一、住宿标准
员工出差住宿标准不超过 300元/晚，一线城市不超过 500元/晚。
超出部分由个人承担，低于标准按实报销。

二、交通标准
市内交通费不超过 80元/天，出差目的地城市间交通费按实际发生报销。
高铁二等座、飞机经济舱为报销上限。

三、补助标准
出差餐补不超过 100元/天，半天按 50元 计算。
出差期间已由接待单位安排用餐的不再发放餐补。`

// samplePolicyTitle 测试用政策文档标题
const samplePolicyTitle = "差旅费报销管理办法"

// ============================================================
// 测试用例
// ============================================================

// TestKnowledgeBase_IndexAndSearch 端到端测试：索引文档后通过向量语义搜索检索
func TestKnowledgeBase_IndexAndSearch(t *testing.T) {
	t.Run("索引文档后通过向量搜索检索相关内容", func(t *testing.T) {
		embedder := &mockEmbedder{dim: 3}
		store := vectorstore.NewInMemoryStore(3)
		kb, data := newTestKnowledgeBase(t, embedder, store)
		defer testutil.CleanDB(data)

		doc := &model.PolicyDocument{
			Title:         samplePolicyTitle,
			Content:       samplePolicyContent,
			Version:       "1.0",
			EffectiveDate: "2026-01-01",
		}

		ctx := context.Background()
		if err := kb.IndexDocument(ctx, doc); err != nil {
			t.Fatalf("索引文档失败: %v", err)
		}

		// 验证文档已保存到数据库
		if doc.ID == 0 {
			t.Fatal("期望文档 ID 被填充，但为 0")
		}

		// 验证分块已创建
		if len(kb.chunks) == 0 {
			t.Fatal("期望文档分块被创建，但分块列表为空")
		}

		// 向量语义搜索：查询与索引内容语义相关
		results, err := kb.Search(ctx, "住宿报销标准", 3)
		if err != nil {
			t.Fatalf("搜索失败: %v", err)
		}
		if len(results) == 0 {
			t.Fatal("期望搜索结果非空，但返回了空列表")
		}

		t.Logf("向量搜索返回 %d 条结果", len(results))
		for i, r := range results {
			t.Logf("  结果[%d]: %s", i, r.Content[:min(60, len(r.Content))])
		}
	})

	t.Run("搜索不相关查询仍有结果（单文档场景）", func(t *testing.T) {
		embedder := &mockEmbedder{dim: 3}
		store := vectorstore.NewInMemoryStore(3)
		kb, data := newTestKnowledgeBase(t, embedder, store)
		defer testutil.CleanDB(data)

		doc := &model.PolicyDocument{
			Title:         samplePolicyTitle,
			Content:       samplePolicyContent,
			Version:       "1.0",
			EffectiveDate: "2026-01-01",
		}

		ctx := context.Background()
		if err := kb.IndexDocument(ctx, doc); err != nil {
			t.Fatalf("索引文档失败: %v", err)
		}

		// 搜索不相关内容，向量搜索仍应返回分块（按相似度排序）
		results, err := kb.Search(ctx, "办公用品采购", 1)
		if err != nil {
			t.Fatalf("搜索失败: %v", err)
		}
		if len(results) == 0 {
			t.Fatal("期望至少有 1 条结果（单文档的最近邻始终存在），但为空")
		}
	})
}

// TestKnowledgeBase_Search_Fallback 测试无向量存储时的关键词降级搜索
func TestKnowledgeBase_Search_Fallback(t *testing.T) {
	t.Run("关键词匹配成功-返回相关分块", func(t *testing.T) {
		kb, data := newTestKnowledgeBase(t, nil, nil) // 无 embedder 和 vectorStore
		defer testutil.CleanDB(data)

		doc := &model.PolicyDocument{
			Title:         samplePolicyTitle,
			Content:       samplePolicyContent,
			Version:       "1.0",
			EffectiveDate: "2026-01-01",
		}

		ctx := context.Background()
		if err := kb.IndexDocument(ctx, doc); err != nil {
			t.Fatalf("索引文档失败: %v", err)
		}

		// 用文档中明确存在的关键词搜索
		results, err := kb.Search(ctx, "住宿 标准 300元", 3)
		if err != nil {
			t.Fatalf("关键词搜索失败: %v", err)
		}
		if len(results) == 0 {
			t.Fatal("期望关键词匹配返回结果，但返回空列表")
		}

		t.Logf("关键词搜索返回 %d 条结果", len(results))
	})
}

// TestKnowledgeBase_Search_Fallback 测试关键词降级搜索的无关查询
func TestKnowledgeBase_Search_NoMatch(t *testing.T) {
	t.Run("无关关键词返回空结果", func(t *testing.T) {
		kb, data := newTestKnowledgeBase(t, nil, nil)
		defer testutil.CleanDB(data)

		doc := &model.PolicyDocument{
			Title:         samplePolicyTitle,
			Content:       samplePolicyContent,
			Version:       "1.0",
			EffectiveDate: "2026-01-01",
		}

		ctx := context.Background()
		if err := kb.IndexDocument(ctx, doc); err != nil {
			t.Fatalf("索引文档失败: %v", err)
		}

		// 搜索完全不相关的关键词
		results, err := kb.Search(ctx, "火星探险", 3)
		if err != nil {
			t.Fatalf("搜索失败: %v", err)
		}
		if len(results) != 0 {
			t.Errorf("期望无关查询返回空结果，但实际返回 %d 条", len(results))
		}
	})
}

// TestKnowledgeBase_Search_Empty 测试空知识库搜索
func TestKnowledgeBase_Search_Empty(t *testing.T) {
	t.Run("空知识库搜索返回 nil", func(t *testing.T) {
		kb, data := newTestKnowledgeBase(t, nil, nil)
		defer testutil.CleanDB(data)

		ctx := context.Background()

		// 未索引任何文档，直接搜索
		results, err := kb.Search(ctx, "住宿标准", 5)
		if err != nil {
			t.Fatalf("空知识库搜索失败: %v", err)
		}
		if results != nil {
			t.Errorf("期望空知识库搜索返回 nil，但实际返回 %v", results)
		}
	})
}
