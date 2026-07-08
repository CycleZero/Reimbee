package tools_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/CycleZero/Reimbee/infra"
	"github.com/CycleZero/Reimbee/internal/domain/agent/tools"
	"github.com/CycleZero/Reimbee/internal/domain/agent/types"
	"github.com/CycleZero/Reimbee/log"
	"go.uber.org/zap"
)

// fixedClock 返回固定的当前时间 2026-07-08
func fixedClock() time.Time {
	return time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC)
}

// setupCheckDeadlineTool 创建带假存储和固定时钟的工具实例
func setupCheckDeadlineTool(store *fakeStateStore) *tools.CheckDeadlineTool {
	logger := &log.Logger{Logger: zap.NewNop()}
	tool := tools.NewCheckDeadlineTool(store, logger)
	tool.Now = fixedClock // 替换为固定时钟
	return tool
}

// assertDeadlineResult 校验单条票据结果
func assertDeadlineResult(t *testing.T, result tools.DeadlineResult, wantIndex int, wantStatus string, wantDays int) {
	t.Helper()
	if result.Index != wantIndex {
		t.Errorf("Index: 期望 %d, 实际 %d", wantIndex, result.Index)
	}
	if result.Status != wantStatus {
		t.Errorf("Status: 期望 %q, 实际 %q", wantStatus, result.Status)
	}
	if result.DaysRemaining != wantDays {
		t.Errorf("DaysRemaining: 期望 %d, 实际 %d", wantDays, result.DaysRemaining)
	}
}

// seedCheckDeadlineState 将测试票据写入假存储
func seedCheckDeadlineState(t *testing.T, store *fakeStateStore, sessionID string, invoices []types.InvoiceState) {
	t.Helper()
	var total int64
	for _, inv := range invoices {
		total += inv.Amount
	}
	state := &types.ReimbursementState{
		Invoices:     invoices,
		TotalAmount:  total,
		CurrentPhase: "phase1_collect",
	}
	if err := store.SaveState(context.Background(), sessionID, infra.StateKeyReimbursement, state); err != nil {
		t.Fatalf("种子数据写入失败: %v", err)
	}
}

// TestCheckDeadline_Valid 正常票据：开票日期 2026-06-01，距今37天，剩余53天 → valid
func TestCheckDeadline_Valid(t *testing.T) {
	store := newFakeStateStore()
	tool := setupCheckDeadlineTool(store)

	seedCheckDeadlineState(t, store, "test-session", []types.InvoiceState{
		{Amount: 10000, Category: "办公用品", Date: "2026-06-01"},
	})

	ctx := tools.WithSessionID(context.Background(), "test-session")
	output, err := tool.Handle(ctx, `{"validity_days":90}`)
	if err != nil {
		t.Fatalf("Handle 失败: %v", err)
	}

	var result tools.CheckDeadlineOutput
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("解析输出失败: %v", err)
	}

	if len(result.Results) != 1 {
		t.Fatalf("期望 1 条结果, 实际 %d 条", len(result.Results))
	}

	assertDeadlineResult(t, result.Results[0], 0, "valid", 53)
	if result.Results[0].Amount != 100.0 {
		t.Errorf("Amount: 期望 100.0, 实际 %f", result.Results[0].Amount)
	}
	if result.Summary.TotalCount != 1 {
		t.Errorf("TotalCount: 期望 1, 实际 %d", result.Summary.TotalCount)
	}
	if result.Summary.HasExpired || result.Summary.HasApproaching || result.Summary.HasUnknown {
		t.Error("不应该有任何警告标志")
	}
}

// TestCheckDeadline_Approaching 即将过期：开票日期 2026-04-13，距今86天，剩余4天 → approaching
func TestCheckDeadline_Approaching(t *testing.T) {
	store := newFakeStateStore()
	tool := setupCheckDeadlineTool(store)

	seedCheckDeadlineState(t, store, "test-session", []types.InvoiceState{
		{Amount: 5000, Category: "交通费", Date: "2026-04-13"},
	})

	ctx := tools.WithSessionID(context.Background(), "test-session")
	output, err := tool.Handle(ctx, `{"validity_days":90}`)
	if err != nil {
		t.Fatalf("Handle 失败: %v", err)
	}

	var result tools.CheckDeadlineOutput
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("解析输出失败: %v", err)
	}

	assertDeadlineResult(t, result.Results[0], 0, "approaching", 4)
	if !result.Summary.HasApproaching {
		t.Error("HasApproaching 应为 true")
	}
}

// TestCheckDeadline_Expired 已过期：开票日期 2026-04-01，距今98天，剩余-8天 → expired
func TestCheckDeadline_Expired(t *testing.T) {
	store := newFakeStateStore()
	tool := setupCheckDeadlineTool(store)

	seedCheckDeadlineState(t, store, "test-session", []types.InvoiceState{
		{Amount: 3000, Category: "差旅费", Date: "2026-04-01"},
	})

	ctx := tools.WithSessionID(context.Background(), "test-session")
	output, err := tool.Handle(ctx, `{"validity_days":90}`)
	if err != nil {
		t.Fatalf("Handle 失败: %v", err)
	}

	var result tools.CheckDeadlineOutput
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("解析输出失败: %v", err)
	}

	assertDeadlineResult(t, result.Results[0], 0, "expired", -8)
	if !result.Summary.HasExpired {
		t.Error("HasExpired 应为 true")
	}
}

// TestCheckDeadline_UnknownEmpty 日期为空 → unknown
func TestCheckDeadline_UnknownEmpty(t *testing.T) {
	store := newFakeStateStore()
	tool := setupCheckDeadlineTool(store)

	seedCheckDeadlineState(t, store, "test-session", []types.InvoiceState{
		{Amount: 2000, Category: "餐饮费", Date: ""},
	})

	ctx := tools.WithSessionID(context.Background(), "test-session")
	output, err := tool.Handle(ctx, `{"validity_days":90}`)
	if err != nil {
		t.Fatalf("Handle 失败: %v", err)
	}

	var result tools.CheckDeadlineOutput
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("解析输出失败: %v", err)
	}

	assertDeadlineResult(t, result.Results[0], 0, "unknown", 0)
	if !result.Summary.HasUnknown {
		t.Error("HasUnknown 应为 true")
	}
}

// TestCheckDeadline_UnknownMalformed 日期格式错误 → unknown
func TestCheckDeadline_UnknownMalformed(t *testing.T) {
	store := newFakeStateStore()
	tool := setupCheckDeadlineTool(store)

	seedCheckDeadlineState(t, store, "test-session", []types.InvoiceState{
		{Amount: 2000, Category: "其他", Date: "invalid-date"},
	})

	ctx := tools.WithSessionID(context.Background(), "test-session")
	output, err := tool.Handle(ctx, `{"validity_days":90}`)
	if err != nil {
		t.Fatalf("Handle 失败: %v", err)
	}

	var result tools.CheckDeadlineOutput
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("解析输出失败: %v", err)
	}

	assertDeadlineResult(t, result.Results[0], 0, "unknown", 0)
}

// TestCheckDeadline_UnknownFuture 未来日期 → unknown
func TestCheckDeadline_UnknownFuture(t *testing.T) {
	store := newFakeStateStore()
	tool := setupCheckDeadlineTool(store)

	seedCheckDeadlineState(t, store, "test-session", []types.InvoiceState{
		{Amount: 4000, Category: "设备采购", Date: "2026-08-01"},
	})

	ctx := tools.WithSessionID(context.Background(), "test-session")
	output, err := tool.Handle(ctx, `{"validity_days":90}`)
	if err != nil {
		t.Fatalf("Handle 失败: %v", err)
	}

	var result tools.CheckDeadlineOutput
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("解析输出失败: %v", err)
	}

	assertDeadlineResult(t, result.Results[0], 0, "unknown", 0)
}

// TestCheckDeadline_EmptyState 无票据 → TotalCount=0
func TestCheckDeadline_EmptyState(t *testing.T) {
	store := newFakeStateStore()
	tool := setupCheckDeadlineTool(store)

	// 不填充任何票据数据

	ctx := tools.WithSessionID(context.Background(), "test-session")
	output, err := tool.Handle(ctx, `{"validity_days":90}`)
	if err != nil {
		t.Fatalf("Handle 失败: %v", err)
	}

	var result tools.CheckDeadlineOutput
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("解析输出失败: %v", err)
	}

	if result.Summary.TotalCount != 0 {
		t.Errorf("TotalCount: 期望 0, 实际 %d", result.Summary.TotalCount)
	}
	if len(result.Results) != 0 {
		t.Errorf("结果数: 期望 0, 实际 %d", len(result.Results))
	}
}

// TestCheckDeadline_CustomValidity 自定义有效期30天，Date=2026-06-01（37天前）→ expired
func TestCheckDeadline_CustomValidity(t *testing.T) {
	store := newFakeStateStore()
	tool := setupCheckDeadlineTool(store)

	seedCheckDeadlineState(t, store, "test-session", []types.InvoiceState{
		{Amount: 10000, Category: "办公用品", Date: "2026-06-01"},
	})

	ctx := tools.WithSessionID(context.Background(), "test-session")
	// 30天有效期，2026-06-01距今37天 → 30-37=-7 → expired
	output, err := tool.Handle(ctx, `{"validity_days":30}`)
	if err != nil {
		t.Fatalf("Handle 失败: %v", err)
	}

	var result tools.CheckDeadlineOutput
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("解析输出失败: %v", err)
	}

	assertDeadlineResult(t, result.Results[0], 0, "expired", -7)
}
