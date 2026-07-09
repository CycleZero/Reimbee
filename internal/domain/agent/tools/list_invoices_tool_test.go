package tools_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/CycleZero/Reimbee/infra"
	"github.com/CycleZero/Reimbee/internal/domain/agent/tools"
	"github.com/CycleZero/Reimbee/internal/domain/agent/types"
	"github.com/CycleZero/Reimbee/log"
	"go.uber.org/zap"
)

// fakeStateStore 内存实现的 StateStore，用于测试
type fakeStateStore struct {
	mu   sync.RWMutex
	data map[string]map[string][]byte // sessionID -> key -> json bytes
}

func newFakeStateStore() *fakeStateStore {
	return &fakeStateStore{
		data: make(map[string]map[string][]byte),
	}
}

func (f *fakeStateStore) SaveState(_ context.Context, sessionID string, key string, state any) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	b, err := json.Marshal(state)
	if err != nil {
		return err
	}

	if f.data[sessionID] == nil {
		f.data[sessionID] = make(map[string][]byte)
	}
	f.data[sessionID][key] = b
	return nil
}

func (f *fakeStateStore) GetState(_ context.Context, sessionID string, key string, target any) (bool, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	b, ok := f.data[sessionID][key]
	if !ok {
		return false, nil
	}

	// 通过 JSON 往返确保与真实实现行为一致
	tmp, err := cloneJSONBytes(b)
	if err != nil {
		return false, err
	}
	if err := json.Unmarshal(tmp, target); err != nil {
		return false, err
	}
	return true, nil
}

func (f *fakeStateStore) DeleteState(_ context.Context, sessionID string, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.data[sessionID] != nil {
		delete(f.data[sessionID], key)
	}
	return nil
}

// cloneJSONBytes 通过 JSON 序列化/反序列化复制字节切片
func cloneJSONBytes(b []byte) ([]byte, error) {
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return nil, err
	}
	return json.Marshal(v)
}

func TestListInvoices_Empty(t *testing.T) {
	store := newFakeStateStore()
	logger := &log.Logger{Logger: zap.NewNop()}

	tool := tools.NewListInvoicesTool(store, logger)

	// 注入空状态
	_ = store.SaveState(context.Background(), "test-session", infra.StateKeyReimbursement, &types.ReimbursementState{
		Items:           []types.ItemState{},
		PendingReceipts: []types.ReceiptState{},
		TotalAmount:     0,
	})

	ctx := tools.WithSessionID(context.Background(), "test-session")
	output, err := tool.Handle(ctx, `{}`)
	if err != nil {
		t.Fatalf("调用失败: %v", err)
	}

	var result tools.ListInvoicesOutput
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("解析输出失败: %v", err)
	}

	if result.TotalCount != 0 {
		t.Errorf("期望 TotalCount=0, 实际=%d", result.TotalCount)
	}
	if len(result.Items) != 0 {
		t.Errorf("期望 Items 为空, 实际长度=%d", len(result.Items))
	}
}

func TestListInvoices_NoSession(t *testing.T) {
	store := newFakeStateStore()
	logger := &log.Logger{Logger: zap.NewNop()}

	tool := tools.NewListInvoicesTool(store, logger)

	// 不注入 sessionID，context 中无会话信息
	ctx := context.Background()
	output, err := tool.Handle(ctx, `{}`)
	if err != nil {
		t.Fatalf("调用失败: %v", err)
	}

	var result tools.ListInvoicesOutput
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("解析输出失败: %v", err)
	}

	if result.TotalCount != 0 {
		t.Errorf("期望 TotalCount=0, 实际=%d", result.TotalCount)
	}
	if len(result.Items) != 0 {
		t.Errorf("期望 Items 为空, 实际长度=%d", len(result.Items))
	}
}

func TestListInvoices_WithInvoices(t *testing.T) {
	store := newFakeStateStore()
	logger := &log.Logger{Logger: zap.NewNop()}

	// 预置两条票据（分属两个明细），金额以分为单位
	state := &types.ReimbursementState{
		Items: []types.ItemState{
			{
				Category: "差旅费",
				Amount:   15000,
				Receipts: []types.ReceiptState{{
					ImagePath: "/images/invoice1.jpg",
					Amount:    15000, // 150.00 元
					Category:  "差旅费",
					Date:      "2026-07-01",
				}},
			},
			{
				Category: "办公用品",
				Amount:   8500,
				Receipts: []types.ReceiptState{{
					ImagePath: "/images/invoice2.jpg",
					Amount:    8500, // 85.00 元
					Category:  "办公用品",
					Date:      "2026-07-03",
				}},
			},
		},
		TotalAmount: 23500, // 235.00 元
	}

	if err := store.SaveState(context.Background(), "test-session", infra.StateKeyReimbursement, state); err != nil {
		t.Fatalf("预置状态失败: %v", err)
	}

	// 验证 GetState 能正确读出（JSON 往返一致性）
	var restored types.ReimbursementState
	found, err := store.GetState(context.Background(), "test-session", infra.StateKeyReimbursement, &restored)
	if err != nil {
		t.Fatalf("GetState 失败: %v", err)
	}
	if !found {
		t.Fatal("GetState 未找到预置的状态")
	}

	tool := tools.NewListInvoicesTool(store, logger)

	ctx := tools.WithSessionID(context.Background(), "test-session")
	output, err := tool.Handle(ctx, `{}`)
	if err != nil {
		t.Fatalf("调用失败: %v", err)
	}

	var result tools.ListInvoicesOutput
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("解析输出失败: %v", err)
	}

	// 验证 TotalCount
	if result.TotalCount != 2 {
		t.Fatalf("期望 TotalCount=2, 实际=%d", result.TotalCount)
	}

	// 验证 TotalAmount（分转元）
	expectedTotal := 235.0
	if result.TotalAmount != expectedTotal {
		t.Errorf("期望 TotalAmount=%.2f, 实际=%.2f", expectedTotal, result.TotalAmount)
	}

	// 验证第一张票据
	if len(result.Items) < 2 {
		t.Fatalf("期望 2 条票据, 实际=%d", len(result.Items))
	}

	inv1 := result.Items[0]
	if inv1.Index != 1 {
		t.Errorf("票据1 期望 Index=1, 实际=%d", inv1.Index)
	}
	if inv1.Category != "差旅费" {
		t.Errorf("票据1 期望 Category='差旅费', 实际='%s'", inv1.Category)
	}
	if inv1.Amount != 150.0 {
		t.Errorf("票据1 期望 Amount=150.00, 实际=%.2f", inv1.Amount)
	}
	if inv1.Receipts[0].Date != "2026-07-01" {
		t.Errorf("票据1 期望 Date='2026-07-01', 实际='%s'", inv1.Receipts[0].Date)
	}
	if inv1.Receipts[0].ImagePath != "/images/invoice1.jpg" {
		t.Errorf("票据1 期望 ImagePath='/images/invoice1.jpg', 实际='%s'", inv1.Receipts[0].ImagePath)
	}

	// 验证第二张票据
	inv2 := result.Items[1]
	if inv2.Index != 2 {
		t.Errorf("票据2 期望 Index=2, 实际=%d", inv2.Index)
	}
	if inv2.Category != "办公用品" {
		t.Errorf("票据2 期望 Category='办公用品', 实际='%s'", inv2.Category)
	}
	if inv2.Amount != 85.0 {
		t.Errorf("票据2 期望 Amount=85.00, 实际=%.2f", inv2.Amount)
	}
	if inv2.Receipts[0].Date != "2026-07-03" {
		t.Errorf("票据2 期望 Date='2026-07-03', 实际='%s'", inv2.Receipts[0].Date)
	}
	if inv2.Receipts[0].ImagePath != "/images/invoice2.jpg" {
		t.Errorf("票据2 期望 ImagePath='/images/invoice2.jpg', 实际='%s'", inv2.Receipts[0].ImagePath)
	}
}

func TestFakeStateStore_DeleteState(t *testing.T) {
	store := newFakeStateStore()

	ctx := context.Background()
	_ = store.SaveState(ctx, "sid", "key1", "value1")

	found, _ := store.GetState(ctx, "sid", "key1", new(string))
	if !found {
		t.Fatal("SaveState 后 GetState 应能找到")
	}

	if err := store.DeleteState(ctx, "sid", "key1"); err != nil {
		t.Fatalf("DeleteState 失败: %v", err)
	}

	var v string
	found, err := store.GetState(ctx, "sid", "key1", &v)
	if err != nil {
		t.Fatalf("删除后 GetState 出错: %v", err)
	}
	if found {
		t.Error("DeleteState 后 GetState 应返回 false")
	}
}

// 确保 fakeStateStore 实现 infra.StateStore 接口
var _ infra.StateStore = (*fakeStateStore)(nil)

// 消除 encoding/json import 的编译警告（用于 runtime 错误构造，但在此文件未直接使用 strings 包）
var _ = errors.New
