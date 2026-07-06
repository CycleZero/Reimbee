package vectorstore

import (
	"context"
	"fmt"
	"math"
	"os"
	"sync"
	"testing"

	"github.com/CycleZero/Reimbee/log"
	"go.uber.org/zap"
)

// TestMain 初始化测试环境
func TestMain(m *testing.M) {
	// 使用 zap.NewNop() 创建无操作日志器，避免测试中调用 GetLogger 时 panic
	log.SetGlobalLogger(&log.Logger{Logger: zap.NewNop()})
	os.Exit(m.Run())
}

// =============================================================================
// TestInMemoryStore_Store — 测试向量存储功能
// =============================================================================

func TestInMemoryStore_Store(t *testing.T) {
	store := NewInMemoryStore(3)
	ctx := context.Background()

	// 子测试1：存储2个正确维度的向量 → 无错误
	t.Run("存储正确维度的向量", func(t *testing.T) {
		vectors := []Vector{
			{ID: "v1", Content: "第一条", Embedding: []float64{1.0, 0.0, 0.0}},
			{ID: "v2", Content: "第二条", Embedding: []float64{0.0, 1.0, 0.0}},
		}
		err := store.Store(ctx, vectors)
		if err != nil {
			t.Fatalf("存储正确维度向量失败: %v", err)
		}

		count, err := store.Count(ctx)
		if err != nil {
			t.Fatalf("Count 调用失败: %v", err)
		}
		if count != 2 {
			t.Errorf("期望 Count() 返回 2，实际 %d", count)
		}
	})

	// 子测试2：存储错误维度的向量 → 返回错误
	t.Run("存储错误维度的向量", func(t *testing.T) {
		store2 := NewInMemoryStore(3)
		vectors := []Vector{
			{ID: "bad", Content: "维度错误", Embedding: []float64{1.0, 2.0}}, // 只有2维
		}
		err := store2.Store(ctx, vectors)
		if err == nil {
			t.Fatal("期望维度不匹配返回错误，但返回 nil")
		}

		// 验证错误后计数为0（未部分提交）
		count, _ := store2.Count(ctx)
		if count != 0 {
			t.Errorf("维度错误后期望 Count()=0，实际 %d", count)
		}
	})

	// 子测试3：再次确认总计数
	t.Run("验证总数", func(t *testing.T) {
		count, err := store.Count(ctx)
		if err != nil {
			t.Fatalf("Count 调用失败: %v", err)
		}
		if count != 2 {
			t.Errorf("期望 Count() 返回 2，实际 %d", count)
		}
	})
}

// =============================================================================
// TestInMemoryStore_Search — 测试余弦相似度搜索
// =============================================================================

func TestInMemoryStore_Search(t *testing.T) {
	store := NewInMemoryStore(3)
	ctx := context.Background()

	// 插入3个互相正交的向量
	vectors := []Vector{
		{ID: "x", Content: "X轴方向", Embedding: []float64{1, 0, 0}},
		{ID: "y", Content: "Y轴方向", Embedding: []float64{0, 1, 0}},
		{ID: "z", Content: "Z轴方向", Embedding: []float64{0, 0, 1}},
	}
	if err := store.Store(ctx, vectors); err != nil {
		t.Fatalf("存储向量失败: %v", err)
	}

	// 子测试1：搜索 [1,0,0]，第一个结果应该是 "x"
	t.Run("精确匹配搜索", func(t *testing.T) {
		results, err := store.Search(ctx, []float64{1, 0, 0}, 3, nil)
		if err != nil {
			t.Fatalf("搜索失败: %v", err)
		}
		if len(results) != 3 {
			t.Fatalf("期望返回3个结果，实际 %d", len(results))
		}
		if results[0].ID != "x" {
			t.Errorf("第一个结果期望 ID='x'，实际 '%s'", results[0].ID)
		}
		// 余弦相似度 1.0（完全相同）
		if math.Abs(results[0].Score-1.0) > 1e-9 {
			t.Errorf("期望相似度 1.0，实际 %.10f", results[0].Score)
		}
	})

	// 子测试2：验证分数降序排列
	t.Run("分数降序排列", func(t *testing.T) {
		results, err := store.Search(ctx, []float64{1, 0, 0}, 3, nil)
		if err != nil {
			t.Fatalf("搜索失败: %v", err)
		}
		for i := 1; i < len(results); i++ {
			if results[i-1].Score < results[i].Score {
				t.Errorf("结果未按降序排列：results[%d].Score=%.4f < results[%d].Score=%.4f",
					i-1, results[i-1].Score, i, results[i].Score)
			}
		}
	})

	// 子测试3：topK=2 只返回2个
	t.Run("topK=2", func(t *testing.T) {
		results, err := store.Search(ctx, []float64{1, 0, 0}, 2, nil)
		if err != nil {
			t.Fatalf("搜索失败: %v", err)
		}
		if len(results) != 2 {
			t.Errorf("topK=2 期望返回2个结果，实际 %d", len(results))
		}
		if results[0].ID != "x" {
			t.Errorf("第一个结果期望 ID='x'，实际 '%s'", results[0].ID)
		}
	})

	// 子测试4：空库搜索返回空结果
	t.Run("空库搜索", func(t *testing.T) {
		emptyStore := NewInMemoryStore(3)
		results, err := emptyStore.Search(ctx, []float64{1, 0, 0}, 5, nil)
		if err != nil {
			t.Fatalf("空库搜索失败: %v", err)
		}
		if len(results) != 0 {
			t.Errorf("空库搜索期望返回0个结果，实际 %d", len(results))
		}
	})

	// 子测试5：查询向量维度错误
	t.Run("查询向量维度错误", func(t *testing.T) {
		_, err := store.Search(ctx, []float64{1, 0}, 3, nil)
		if err == nil {
			t.Fatal("期望维度不匹配返回错误，但返回 nil")
		}
	})
}

// =============================================================================
// TestInMemoryStore_Search_Filters — 测试元数据过滤
// =============================================================================

func TestInMemoryStore_Search_Filters(t *testing.T) {
	store := NewInMemoryStore(2)
	ctx := context.Background()

	vectors := []Vector{
		{ID: "doc1", Content: "科技文档", Embedding: []float64{1, 0}, Metadata: map[string]string{"category": "tech", "lang": "zh"}},
		{ID: "doc2", Content: "科技英文", Embedding: []float64{0.9, 0.1}, Metadata: map[string]string{"category": "tech", "lang": "en"}},
		{ID: "doc3", Content: "金融文档", Embedding: []float64{0, 1}, Metadata: map[string]string{"category": "finance", "lang": "zh"}},
	}
	if err := store.Store(ctx, vectors); err != nil {
		t.Fatalf("存储向量失败: %v", err)
	}

	// 子测试1：按 category=tech 过滤
	t.Run("按单个字段过滤", func(t *testing.T) {
		results, err := store.Search(ctx, []float64{1, 0}, 10, map[string]string{"category": "tech"})
		if err != nil {
			t.Fatalf("过滤搜索失败: %v", err)
		}
		if len(results) != 2 {
			t.Fatalf("期望2个 tech 结果，实际 %d", len(results))
		}
		for _, r := range results {
			if r.Metadata["category"] != "tech" {
				t.Errorf("过滤结果中包含非 tech 类别: %s (category=%s)", r.ID, r.Metadata["category"])
			}
		}
	})

	// 子测试2：按多个字段同时过滤
	t.Run("按多个字段过滤", func(t *testing.T) {
		results, err := store.Search(ctx, []float64{1, 0}, 10, map[string]string{
			"category": "tech",
			"lang":     "zh",
		})
		if err != nil {
			t.Fatalf("多字段过滤搜索失败: %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("期望1个结果，实际 %d", len(results))
		}
		if results[0].ID != "doc1" {
			t.Errorf("期望 ID='doc1'，实际 '%s'", results[0].ID)
		}
	})

	// 子测试3：无匹配的过滤
	t.Run("无匹配过滤条件", func(t *testing.T) {
		results, err := store.Search(ctx, []float64{1, 0}, 10, map[string]string{"category": "nonexistent"})
		if err != nil {
			t.Fatalf("无匹配过滤搜索失败: %v", err)
		}
		if len(results) != 0 {
			t.Errorf("期望0个结果，实际 %d", len(results))
		}
	})

	// 子测试4：nil filters 不过滤
	t.Run("nil过滤返回全部", func(t *testing.T) {
		results, err := store.Search(ctx, []float64{1, 0}, 10, nil)
		if err != nil {
			t.Fatalf("nil过滤搜索失败: %v", err)
		}
		if len(results) != 3 {
			t.Errorf("nil过滤期望返回全部3个结果，实际 %d", len(results))
		}
	})
}

// =============================================================================
// TestInMemoryStore_Delete — 测试向量删除
// =============================================================================

func TestInMemoryStore_Delete(t *testing.T) {
	store := NewInMemoryStore(2)
	ctx := context.Background()

	vectors := []Vector{
		{ID: "a", Content: "A", Embedding: []float64{1, 0}},
		{ID: "b", Content: "B", Embedding: []float64{0, 1}},
		{ID: "c", Content: "C", Embedding: []float64{1, 1}},
	}
	if err := store.Store(ctx, vectors); err != nil {
		t.Fatalf("存储向量失败: %v", err)
	}

	// 子测试1：删除1个，Count()=2
	t.Run("删除存在的ID", func(t *testing.T) {
		err := store.Delete(ctx, []string{"a"})
		if err != nil {
			t.Fatalf("删除失败: %v", err)
		}
		count, _ := store.Count(ctx)
		if count != 2 {
			t.Errorf("期望 Count()=2，实际 %d", count)
		}

		// 验证被删除的ID搜索不到
		results, _ := store.Search(ctx, []float64{1, 0}, 10, nil)
		for _, r := range results {
			if r.ID == "a" {
				t.Errorf("已删除的ID 'a' 仍出现在搜索结果中")
			}
		}
	})

	// 子测试2：删除不存在的ID → 无错误
	t.Run("删除不存在的ID静默忽略", func(t *testing.T) {
		err := store.Delete(ctx, []string{"nonexistent", "also-fake"})
		if err != nil {
			t.Fatalf("删除不存在ID应静默忽略，但返回错误: %v", err)
		}
		count, _ := store.Count(ctx)
		if count != 2 {
			t.Errorf("删除不存在ID后 Count() 应保持2，实际 %d", count)
		}
	})

	// 子测试3：删除空列表
	t.Run("删除空列表", func(t *testing.T) {
		err := store.Delete(ctx, []string{})
		if err != nil {
			t.Fatalf("删除空列表失败: %v", err)
		}
	})

	// 子测试4：混合删除（存在的 + 不存在的）
	t.Run("混合删除", func(t *testing.T) {
		err := store.Delete(ctx, []string{"b", "nonexistent"})
		if err != nil {
			t.Fatalf("混合删除失败: %v", err)
		}
		count, _ := store.Count(ctx)
		if count != 1 {
			t.Errorf("期望 Count()=1，实际 %d", count)
		}
	})
}

// =============================================================================
// TestInMemoryStore_Clear — 测试清空
// =============================================================================

func TestInMemoryStore_Clear(t *testing.T) {
	store := NewInMemoryStore(2)
	ctx := context.Background()

	// 子测试1：存储后清空，Count()=0
	t.Run("清空非空库", func(t *testing.T) {
		vectors := []Vector{
			{ID: "v1", Content: "测试1", Embedding: []float64{1, 0}},
			{ID: "v2", Content: "测试2", Embedding: []float64{0, 1}},
		}
		if err := store.Store(ctx, vectors); err != nil {
			t.Fatalf("存储失败: %v", err)
		}

		if err := store.Clear(ctx); err != nil {
			t.Fatalf("清空失败: %v", err)
		}

		count, err := store.Count(ctx)
		if err != nil {
			t.Fatalf("Count 调用失败: %v", err)
		}
		if count != 0 {
			t.Errorf("清空后期望 Count()=0，实际 %d", count)
		}
	})

	// 子测试2：清空已空的库
	t.Run("清空空库", func(t *testing.T) {
		emptyStore := NewInMemoryStore(2)
		if err := emptyStore.Clear(ctx); err != nil {
			t.Fatalf("清空空库失败: %v", err)
		}
		count, _ := emptyStore.Count(ctx)
		if count != 0 {
			t.Errorf("空库清空后 Count() 应为0，实际 %d", count)
		}
	})

	// 子测试3：清空后可重新存储
	t.Run("清空后重新存储", func(t *testing.T) {
		if err := store.Store(ctx, []Vector{
			{ID: "new", Content: "新数据", Embedding: []float64{0.5, 0.5}},
		}); err != nil {
			t.Fatalf("清空后重新存储失败: %v", err)
		}
		count, _ := store.Count(ctx)
		if count != 1 {
			t.Errorf("重新存储后期望 Count()=1，实际 %d", count)
		}
	})
}

// =============================================================================
// TestInMemoryStore_Concurrency — 测试并发安全性
// =============================================================================

func TestInMemoryStore_Concurrency(t *testing.T) {
	store := NewInMemoryStore(3)
	ctx := context.Background()

	const numGoroutines = 10
	const vectorsPerGoroutine = 100
	const totalVectors = numGoroutines * vectorsPerGoroutine

	var wg sync.WaitGroup
	errCh := make(chan error, numGoroutines)

	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func(gid int) {
			defer wg.Done()
			for i := 0; i < vectorsPerGoroutine; i++ {
				id := fmt.Sprintf("g%d-v%d", gid, i)
				vec := Vector{
					ID:        id,
					Content:   fmt.Sprintf("协程%d的第%d个向量", gid, i),
					Embedding: []float64{float64(gid), float64(i), 0},
				}
				if err := store.Store(ctx, []Vector{vec}); err != nil {
					errCh <- fmt.Errorf("协程%d Store(%s) 失败: %v", gid, id, err)
					return
				}
			}
		}(g)
	}

	wg.Wait()
	close(errCh)

	// 检查是否有并发写入错误
	for err := range errCh {
		t.Errorf("并发写入错误: %v", err)
	}

	// 验证总数
	count, err := store.Count(ctx)
	if err != nil {
		t.Fatalf("并发测试后 Count 失败: %v", err)
	}
	if count != totalVectors {
		t.Errorf("并发存储后期望 Count()=%d，实际 %d", count, totalVectors)
	}

	// 并发读取测试
	t.Run("并发读取", func(t *testing.T) {
		var readWg sync.WaitGroup
		for g := 0; g < 5; g++ {
			readWg.Add(1)
			go func() {
				defer readWg.Done()
				results, err := store.Search(ctx, []float64{1, 0, 0}, 10, nil)
				if err != nil {
					t.Errorf("并发搜索失败: %v", err)
					return
				}
				_ = results
			}()
		}
		readWg.Wait()
	})

	// 并发读写混合测试
	t.Run("并发读写混合", func(t *testing.T) {
		var mixWg sync.WaitGroup

		// 读取协程
		for g := 0; g < 3; g++ {
			mixWg.Add(1)
			go func() {
				defer mixWg.Done()
				for i := 0; i < 50; i++ {
					_, _ = store.Search(ctx, []float64{0, 0, 1}, 5, nil)
					_, _ = store.Count(ctx)
				}
			}()
		}

		// 写入协程
		for g := 0; g < 2; g++ {
			mixWg.Add(1)
			go func(gid int) {
				defer mixWg.Done()
				for i := 0; i < 20; i++ {
					id := fmt.Sprintf("mix-g%d-i%d", gid, i)
					_ = store.Store(ctx, []Vector{
						{ID: id, Content: "混合测试", Embedding: []float64{float64(gid), float64(i), 0}},
					})
				}
			}(g)
		}

		mixWg.Wait()

		// 验证数据一致性：Count ≥ 初始1000
		count, _ := store.Count(ctx)
		if count < totalVectors {
			t.Errorf("并发混合测试后 Count()=%d 不应小于 %d", count, totalVectors)
		}
	})
}

// =============================================================================
// TestCosineSimilarity — 测试余弦相似度计算
// =============================================================================

func TestCosineSimilarity(t *testing.T) {
	// 相同向量 → 相似度 1.0
	t.Run("相同向量", func(t *testing.T) {
		a := []float64{1, 2, 3}
		b := []float64{1, 2, 3}
		sim := cosineSimilarity(a, b)
		if math.Abs(sim-1.0) > 1e-9 {
			t.Errorf("相同向量期望相似度 1.0，实际 %.10f", sim)
		}
	})

	// 正交向量 → 相似度 0.0
	t.Run("正交向量", func(t *testing.T) {
		a := []float64{1, 0, 0}
		b := []float64{0, 1, 0}
		sim := cosineSimilarity(a, b)
		if math.Abs(sim-0.0) > 1e-9 {
			t.Errorf("正交向量期望相似度 0.0，实际 %.10f", sim)
		}
	})

	// 相反向量 → 相似度 -1.0
	t.Run("相反向量", func(t *testing.T) {
		a := []float64{1, 0}
		b := []float64{-1, 0}
		sim := cosineSimilarity(a, b)
		if math.Abs(sim-(-1.0)) > 1e-9 {
			t.Errorf("相反向量期望相似度 -1.0，实际 %.10f", sim)
		}
	})

	// 不同长度向量 → 相似度 0.0
	t.Run("不同长度向量", func(t *testing.T) {
		a := []float64{1, 2, 3}
		b := []float64{1, 2}
		sim := cosineSimilarity(a, b)
		if sim != 0 {
			t.Errorf("不同长度向量期望相似度 0，实际 %.10f", sim)
		}
	})

	// 零向量 → 相似度 0.0（防止除零）
	t.Run("零向量", func(t *testing.T) {
		a := []float64{0, 0, 0}
		b := []float64{1, 2, 3}
		sim := cosineSimilarity(a, b)
		if sim != 0 {
			t.Errorf("零向量期望相似度 0，实际 %.10f", sim)
		}
	})

	// 两个都是零向量
	t.Run("两个零向量", func(t *testing.T) {
		a := []float64{0, 0}
		b := []float64{0, 0}
		sim := cosineSimilarity(a, b)
		if sim != 0 {
			t.Errorf("两个零向量期望相似度 0，实际 %.10f", sim)
		}
	})

	// 部分相似的向量
	t.Run("部分相似向量", func(t *testing.T) {
		a := []float64{0.6, 0.8}
		b := []float64{0.8, 0.6}
		// dot = 0.6*0.8 + 0.8*0.6 = 0.48 + 0.48 = 0.96
		// normA = sqrt(0.36+0.64) = 1, normB = sqrt(0.64+0.36) = 1
		// expected = 0.96
		expected := 0.96
		sim := cosineSimilarity(a, b)
		if math.Abs(sim-expected) > 1e-9 {
			t.Errorf("期望相似度 %f，实际 %.10f", expected, sim)
		}
	})
}
