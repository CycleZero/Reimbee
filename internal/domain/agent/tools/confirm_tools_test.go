// Package tools_test 工具层黑盒测试 — 确认工具 & SaveState 持久化模式验证
// 覆盖：confirm_invoice、confirm_submit、OCR SaveState、合规 SaveState、预算 SaveState、全流程状态流转
package tools_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"sync"
	"testing"

	"github.com/CycleZero/Reimbee/infra"
	"github.com/CycleZero/Reimbee/internal/domain/agent/tools"
	agenttypes "github.com/CycleZero/Reimbee/internal/domain/agent/types"
	"github.com/cloudwego/eino/schema"
)

// ============================================
// mockSessionStore — 完整内存 SessionStore（实现全部 6 个方法）
// ============================================

type mockSessionStore struct {
	mu       sync.Mutex
	messages map[string][]*schema.Message
	states   map[string]map[string][]byte
}

func newMockStore() *mockSessionStore {
	return &mockSessionStore{
		messages: make(map[string][]*schema.Message),
		states:   make(map[string]map[string][]byte),
	}
}

func (m *mockSessionStore) SaveMessages(_ context.Context, sessionID string, msgs []*schema.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, msg := range msgs {
		data, _ := json.Marshal(msg)
		var copyMsg schema.Message
		_ = json.Unmarshal(data, &copyMsg)
		m.messages[sessionID] = append(m.messages[sessionID], &copyMsg)
	}
	return nil
}

func (m *mockSessionStore) GetHistory(_ context.Context, sessionID string, limit int) ([]*schema.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	all := m.messages[sessionID]
	if len(all) == 0 {
		return nil, nil
	}
	if limit <= 0 || limit > len(all) {
		limit = len(all)
	}
	start := max(len(all)-limit, 0)
	result := make([]*schema.Message, limit)
	for i := 0; i < limit; i++ {
		data, _ := json.Marshal(all[start+i])
		var msg schema.Message
		_ = json.Unmarshal(data, &msg)
		result[i] = &msg
	}
	return result, nil
}

func (m *mockSessionStore) Clear(_ context.Context, sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.messages, sessionID)
	return nil
}

func (m *mockSessionStore) SaveState(_ context.Context, sessionID string, key string, state any) error {
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.states[sessionID] == nil {
		m.states[sessionID] = make(map[string][]byte)
	}
	m.states[sessionID][key] = data
	return nil
}

func (m *mockSessionStore) GetState(_ context.Context, sessionID string, key string, target any) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	entries := m.states[sessionID]
	if entries == nil {
		return false, nil
	}
	data, ok := entries[key]
	if !ok {
		return false, nil
	}
	return true, json.Unmarshal(data, target)
}

func (m *mockSessionStore) DeleteState(_ context.Context, sessionID string, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.states[sessionID] != nil {
		delete(m.states[sessionID], key)
	}
	return nil
}

// 编译期验证接口实现
var _ infra.SessionStore = (*mockSessionStore)(nil)

// sessionCtx 创建携带 sessionID 的 context（模拟 GenInput 注入）
func sessionCtx(sessionID string) context.Context {
	return context.WithValue(context.Background(), agenttypes.SessionIDContextKey{}, sessionID)
}

// ============================================
// ConfirmInvoice 工具测试
// ============================================

// TestConfirmInvoice_SetsUserConfirmed 验证票据确认后 UserConfirmed=true
func TestConfirmInvoice_SetsUserConfirmed(t *testing.T) {
	store := newMockStore()

	// 预填充 Phase 1 初始状态
	var preState agenttypes.ReimbursementState
	preState.Invoices = []agenttypes.InvoiceState{
		{Amount: 50000, Category: "差旅-交通"},
	}
	preState.CurrentPhase = "phase1_collect"
	_ = store.SaveState(context.Background(), "session-1", infra.StateKeyReimbursement, &preState)

	toolImpl := tools.NewConfirmInvoiceTool(store, testLogger(t))
	ctx := sessionCtx("session-1")

	output, err := toolImpl.InvokableRun(ctx, `{"confirmed": true}`)
	if err != nil {
		t.Fatalf("InvokableRun 意外报错: %v", err)
	}

	var out struct {
		Status string `json:"status"`
	}
	json.Unmarshal([]byte(output), &out)
	if out.Status != "confirmed" {
		t.Errorf("期望 status=confirmed, 实际=%q", out.Status)
	}

	// 验证状态已更新
	var state agenttypes.ReimbursementState
	found, _ := store.GetState(context.Background(), "session-1", infra.StateKeyReimbursement, &state)
	if !found {
		t.Fatal("确认后未找到 reimbursement 状态")
	}
	if !state.UserConfirmed {
		t.Error("Confirmed=true 后 UserConfirmed 应为 true")
	}
	if state.CurrentPhase != "phase2_validate" {
		t.Errorf("CurrentPhase 应为 phase2_validate，实际=%q", state.CurrentPhase)
	}
}

// TestConfirmInvoice_KeepsStateOnFalse 验证未确认时不修改状态
func TestConfirmInvoice_KeepsStateOnFalse(t *testing.T) {
	store := newMockStore()

	var preState agenttypes.ReimbursementState
	preState.Invoices = []agenttypes.InvoiceState{
		{Amount: 30000, Category: "办公用品"},
	}
	preState.CurrentPhase = "phase1_collect"
	preState.UserConfirmed = false
	_ = store.SaveState(context.Background(), "session-2", infra.StateKeyReimbursement, &preState)

	toolImpl := tools.NewConfirmInvoiceTool(store, testLogger(t))
	ctx := sessionCtx("session-2")

	output, _ := toolImpl.InvokableRun(ctx, `{"confirmed": false}`)
	var out struct {
		Status string `json:"status"`
	}
	json.Unmarshal([]byte(output), &out)
	if out.Status != "pending" {
		t.Errorf("期望 status=pending, 实际=%q", out.Status)
	}

	var state agenttypes.ReimbursementState
	store.GetState(context.Background(), "session-2", infra.StateKeyReimbursement, &state)
	if state.UserConfirmed {
		t.Error("Confirmed=false 不应设置 UserConfirmed=true")
	}
	if state.CurrentPhase != "phase1_collect" {
		t.Errorf("CurrentPhase 应保持 phase1_collect，实际=%q", state.CurrentPhase)
	}
}

// TestConfirmInvoice_MultipleCalls 验证多次确认幂等
func TestConfirmInvoice_MultipleCalls(t *testing.T) {
	store := newMockStore()

	var preState agenttypes.ReimbursementState
	preState.Invoices = []agenttypes.InvoiceState{
		{Amount: 50000, Category: "交通"},
	}
	preState.CurrentPhase = "phase1_collect"
	store.SaveState(context.Background(), "session-multi", infra.StateKeyReimbursement, &preState)

	toolImpl := tools.NewConfirmInvoiceTool(store, testLogger(t))
	ctx := sessionCtx("session-multi")

	toolImpl.InvokableRun(ctx, `{"confirmed": true}`)
	output, _ := toolImpl.InvokableRun(ctx, `{"confirmed": true}`)

	var out struct {
		Status string `json:"status"`
	}
	json.Unmarshal([]byte(output), &out)
	if out.Status != "confirmed" {
		t.Errorf("第二次确认期望 status=confirmed, 实际=%q", out.Status)
	}

	var state agenttypes.ReimbursementState
	store.GetState(context.Background(), "session-multi", infra.StateKeyReimbursement, &state)
	if !state.UserConfirmed {
		t.Error("多次确认后 UserConfirmed 应仍为 true")
	}
}

// TestConfirmInvoice_NoSessionID 验证无 sessionID 时不 panic
func TestConfirmInvoice_NoSessionID(t *testing.T) {
	store := newMockStore()
	toolImpl := tools.NewConfirmInvoiceTool(store, testLogger(t))
	ctx := context.Background()

	output, err := toolImpl.InvokableRun(ctx, `{"confirmed": true}`)
	if err != nil {
		t.Fatalf("InvokableRun 意外报错: %v", err)
	}
	if output == "" {
		t.Error("即使无 sessionID，工具也应返回输出")
	}
}

// TestConfirmInvoice_PreservesExistingInvoices 验证确认操作不丢失已有票据
func TestConfirmInvoice_PreservesExistingInvoices(t *testing.T) {
	store := newMockStore()

	var preState agenttypes.ReimbursementState
	preState.Invoices = []agenttypes.InvoiceState{
		{Index: 1, Amount: 50000, Category: "差旅-交通"},
		{Index: 2, Amount: 30000, Category: "办公用品"},
	}
	preState.TotalAmount = 80000
	preState.CurrentPhase = "phase1_collect"
	store.SaveState(context.Background(), "session-data", infra.StateKeyReimbursement, &preState)

	toolImpl := tools.NewConfirmInvoiceTool(store, testLogger(t))
	ctx := sessionCtx("session-data")

	toolImpl.InvokableRun(ctx, `{"confirmed": true}`)

	var state agenttypes.ReimbursementState
	store.GetState(context.Background(), "session-data", infra.StateKeyReimbursement, &state)

	if len(state.Invoices) != 2 {
		t.Errorf("确认后应保留 2 张票据，实际 %d 张", len(state.Invoices))
	}
	if state.TotalAmount != 80000 {
		t.Errorf("TotalAmount 应为 80000，实际=%d", state.TotalAmount)
	}
}

// ============================================
// ConfirmSubmit 工具测试
// ============================================

// TestConfirmSubmit_SetsFinalConfirmed 验证最终确认后 FinalConfirmed=true
func TestConfirmSubmit_SetsFinalConfirmed(t *testing.T) {
	store := newMockStore()

	// 预填充 Phase 2 完整状态
	var preState agenttypes.ReimbursementState
	preState.Invoices = []agenttypes.InvoiceState{
		{Amount: 80000, Category: "差旅-住宿"},
	}
	preState.TotalAmount = 80000
	preState.CurrentPhase = "phase2_validate"
	preState.UserConfirmed = true
	preState.ComplianceResult = &agenttypes.ComplianceCheckResult{
		Result:  "pass",
		Message: "合规检查通过",
	}
	preState.BudgetResult = &agenttypes.BudgetCheckResult{
		Remaining:           500000,
		NeedSpecialApproval: false,
		UsageRate:           0.3,
	}
	store.SaveState(context.Background(), "session-3", infra.StateKeyReimbursement, &preState)

	toolImpl := tools.NewConfirmSubmitTool(store, testLogger(t))
	ctx := sessionCtx("session-3")

	output, _ := toolImpl.InvokableRun(ctx, `{"confirmed": true}`)
	var out struct {
		Status string `json:"status"`
	}
	json.Unmarshal([]byte(output), &out)
	if out.Status != "confirmed" {
		t.Errorf("期望 status=confirmed, 实际=%q", out.Status)
	}

	var state agenttypes.ReimbursementState
	found, _ := store.GetState(context.Background(), "session-3", infra.StateKeyReimbursement, &state)
	if !found {
		t.Fatal("确认提交后未找到 reimbursement 状态")
	}
	if !state.FinalConfirmed {
		t.Error("Confirmed=true 后 FinalConfirmed 应为 true")
	}
	if state.CurrentPhase != "phase3_execute" {
		t.Errorf("CurrentPhase 应为 phase3_execute，实际=%q", state.CurrentPhase)
	}
	// 验证原有数据保留
	if len(state.Invoices) != 1 || state.ComplianceResult == nil || state.BudgetResult == nil {
		t.Error("确认提交后应保留票据、合规、预算数据")
	}
}

// TestConfirmSubmit_KeepsStateOnFalse 验证未最终确认时状态不变
func TestConfirmSubmit_KeepsStateOnFalse(t *testing.T) {
	store := newMockStore()

	var preState agenttypes.ReimbursementState
	preState.Invoices = []agenttypes.InvoiceState{
		{Amount: 60000, Category: "招待费"},
	}
	preState.CurrentPhase = "phase2_validate"
	preState.UserConfirmed = true
	preState.FinalConfirmed = false
	store.SaveState(context.Background(), "session-4", infra.StateKeyReimbursement, &preState)

	toolImpl := tools.NewConfirmSubmitTool(store, testLogger(t))
	ctx := sessionCtx("session-4")

	output, _ := toolImpl.InvokableRun(ctx, `{"confirmed": false}`)
	var out struct {
		Status string `json:"status"`
	}
	json.Unmarshal([]byte(output), &out)
	if out.Status != "pending" {
		t.Errorf("期望 status=pending, 实际=%q", out.Status)
	}

	var state agenttypes.ReimbursementState
	store.GetState(context.Background(), "session-4", infra.StateKeyReimbursement, &state)
	if state.FinalConfirmed {
		t.Error("Confirmed=false 不应设置 FinalConfirmed=true")
	}
}

// TestConfirmSubmit_NoSessionID 验证无 sessionID 时不 panic
func TestConfirmSubmit_NoSessionID(t *testing.T) {
	store := newMockStore()
	toolImpl := tools.NewConfirmSubmitTool(store, testLogger(t))
	ctx := context.Background()

	output, err := toolImpl.InvokableRun(ctx, `{"confirmed": true}`)
	if err != nil {
		t.Fatalf("InvokableRun 意外报错: %v", err)
	}
	if output == "" {
		t.Error("即使无 sessionID，工具也应返回输出")
	}
}

// ============================================
// mockOCRRecognizer — 实现 infra.OCRRecognizer
// ============================================

type mockOCRRecognizer struct {
	result *infra.InvoiceResult
	err    error
}

func (m *mockOCRRecognizer) Recognize(_ context.Context, _ []byte, _ string) (*infra.InvoiceResult, error) {
	return m.result, m.err
}

func (m *mockOCRRecognizer) Name() string             { return "mock-ocr" }
func (m *mockOCRRecognizer) HealthCheck(_ context.Context) error { return nil }

var _ infra.OCRRecognizer = (*mockOCRRecognizer)(nil)

// ============================================
// mockFileStorage — 实现 infra.FileStorage
// ============================================

type mockFileStorage struct {
	data []byte
	err  error
}

func (m *mockFileStorage) Save(_ context.Context, _, _ string, _ io.Reader) (*infra.UploadedFile, error) {
	return nil, nil
}
func (m *mockFileStorage) Get(_ context.Context, _ string) (io.ReadCloser, error) {
	if m.err != nil {
		return nil, m.err
	}
	return io.NopCloser(bytes.NewReader(m.data)), nil
}
func (m *mockFileStorage) Delete(_ context.Context, _ string) error { return nil }
func (m *mockFileStorage) URL(_ context.Context, _ string) string   { return "" }

var _ infra.FileStorage = (*mockFileStorage)(nil)

// ============================================
// OCR 工具 — SaveState 持久化测试
// ============================================

// TestOCRTool_SaveState_InvoicesPopulated 验证 OCR 识别后 Invoices 和 TotalAmount 被持久化
func TestOCRTool_SaveState_InvoicesPopulated(t *testing.T) {
	store := newMockStore()
	logger := testLogger(t)

	recognizer := &mockOCRRecognizer{
		result: &infra.InvoiceResult{
			InvoiceCode:   "1234567890",
			InvoiceNumber: "09876543",
			Amount:        150.50, // 元
			Date:          "2026-07-01",
			SellerName:    "测试公司",
			Category:      "办公用品",
			Confidence:    0.95,
		},
	}
	storage := &mockFileStorage{data: []byte("fake-image-data")}

	tool := tools.NewOCRTool(recognizer, storage, store, logger)
	ctx := sessionCtx("session-ocr-save")

	resultJSON, err := tool.InvokableRun(ctx, `{"image_path":"/uploads/test-invoice.jpg"}`)
	if err != nil {
		t.Fatalf("OCR InvokableRun 失败: %v", err)
	}

	var output struct {
		Amount   int64   `json:"amount"`
		Category string  `json:"category"`
	}
	json.Unmarshal([]byte(resultJSON), &output)
	if output.Amount != 15050 {
		t.Errorf("期望金额(分)=15050, 实际=%d", output.Amount)
	}
	if output.Category != "办公用品" {
		t.Errorf("期望类别=办公用品, 实际=%q", output.Category)
	}

	// 验证 ReimbursementState 持久化
	var state agenttypes.ReimbursementState
	found, _ := store.GetState(context.Background(), "session-ocr-save", infra.StateKeyReimbursement, &state)
	if !found {
		t.Fatal("OCR 后期望状态已持久化")
	}
	if len(state.Invoices) != 1 {
		t.Fatalf("期望 1 张票据，实际 %d 张", len(state.Invoices))
	}
	if state.Invoices[0].Amount != 15050 {
		t.Errorf("Invoices[0].Amount = %d, want 15050", state.Invoices[0].Amount)
	}
	if state.TotalAmount != 15050 {
		t.Errorf("TotalAmount = %d, want 15050", state.TotalAmount)
	}
	if state.CurrentPhase != "phase1_collect" {
		t.Errorf("CurrentPhase = %q, want phase1_collect", state.CurrentPhase)
	}
}

// TestOCRTool_SaveState_Accumulate 验证多次 OCR 累积 TotalAmount
func TestOCRTool_SaveState_Accumulate(t *testing.T) {
	store := newMockStore()
	logger := testLogger(t)

	recognizer := &mockOCRRecognizer{
		result: &infra.InvoiceResult{Amount: 100.00, Category: "交通", Date: "2026-07-01"},
	}
	storage := &mockFileStorage{data: []byte("img1")}
	tool := tools.NewOCRTool(recognizer, storage, store, logger)
	ctx := sessionCtx("session-ocr-accum")

	// 第一次识别
	tool.InvokableRun(ctx, `{"image_path":"/uploads/1.jpg"}`)

	// 修改 mock 返回值
	recognizer.result = &infra.InvoiceResult{Amount: 200.00, Category: "住宿", Date: "2026-07-02"}

	// 第二次识别
	tool.InvokableRun(ctx, `{"image_path":"/uploads/2.jpg"}`)

	var state agenttypes.ReimbursementState
	store.GetState(context.Background(), "session-ocr-accum", infra.StateKeyReimbursement, &state)
	if len(state.Invoices) != 2 {
		t.Fatalf("期望 2 张票据，实际 %d 张", len(state.Invoices))
	}
	if state.TotalAmount != 30000 {
		t.Errorf("TotalAmount = %d, want 30000（100+200 元）", state.TotalAmount)
	}
}

// ============================================
// Compliance SaveState 持久化模式测试
// 直接通过 mock SessionStore 模拟 compliance_tool 的 GetState→修改→SaveState 逻辑
// ============================================

// TestComplianceSaveState_Pass 验证合规检查通过的 SaveState 模式
func TestComplianceSaveState_Pass(t *testing.T) {
	store := newMockStore()
	ctx := context.Background()
	sid := "session-compliance-pass"

	// 初始状态（模拟合规工具调用前）
	initial := agenttypes.ReimbursementState{
		CurrentPhase: "phase1_collect",
		Invoices:     []agenttypes.InvoiceState{{Amount: 10000, Category: "交通"}},
	}
	store.SaveState(ctx, sid, infra.StateKeyReimbursement, &initial)

	// 模拟合规工具的 SaveState 逻辑
	var state agenttypes.ReimbursementState
	store.GetState(ctx, sid, infra.StateKeyReimbursement, &state)
	state.ComplianceResult = &agenttypes.ComplianceCheckResult{
		Result:  "pass",
		Level:   "pass",
		Message: "交通费在标准范围内，通过合规检查",
		RuleID:  "TRAVEL-001",
	}
	store.SaveState(ctx, sid, infra.StateKeyReimbursement, &state)

	// 验证
	var restored agenttypes.ReimbursementState
	store.GetState(ctx, sid, infra.StateKeyReimbursement, &restored)
	if restored.ComplianceResult == nil {
		t.Fatal("ComplianceResult 不应为 nil")
	}
	if restored.ComplianceResult.Result != "pass" {
		t.Errorf("Result = %q, want pass", restored.ComplianceResult.Result)
	}
	if restored.ComplianceResult.RuleID != "TRAVEL-001" {
		t.Errorf("RuleID = %q, want TRAVEL-001", restored.ComplianceResult.RuleID)
	}
	if len(restored.Invoices) != 1 {
		t.Errorf("Invoices 不应丢失，期望 1 张，实际 %d 张", len(restored.Invoices))
	}
}

// TestComplianceSaveState_Warning 验证合规警告场景
func TestComplianceSaveState_Warning(t *testing.T) {
	store := newMockStore()
	ctx := context.Background()
	sid := "session-compliance-warn"

	var state agenttypes.ReimbursementState
	state.ComplianceResult = &agenttypes.ComplianceCheckResult{
		Result:  "warning",
		Level:   "warning",
		Message: "差旅住宿接近上限，需额外审批",
		RuleID:  "HOTEL-002",
	}
	store.SaveState(ctx, sid, infra.StateKeyReimbursement, &state)

	var restored agenttypes.ReimbursementState
	store.GetState(ctx, sid, infra.StateKeyReimbursement, &restored)
	if restored.ComplianceResult.Result != "warning" {
		t.Errorf("期望 warning, 实际=%q", restored.ComplianceResult.Result)
	}
}

// TestComplianceSaveState_Error 验证合规错误场景
func TestComplianceSaveState_Error(t *testing.T) {
	store := newMockStore()
	ctx := context.Background()
	sid := "session-compliance-err"

	var state agenttypes.ReimbursementState
	state.ComplianceResult = &agenttypes.ComplianceCheckResult{
		Result:  "error",
		Level:   "error",
		Message: "招待费超出单次限额500元",
		RuleID:  "ENTERTAIN-003",
	}
	store.SaveState(ctx, sid, infra.StateKeyReimbursement, &state)

	var restored agenttypes.ReimbursementState
	store.GetState(ctx, sid, infra.StateKeyReimbursement, &restored)
	if restored.ComplianceResult.Result != "error" {
		t.Errorf("期望 error, 实际=%q", restored.ComplianceResult.Result)
	}
}

// ============================================
// Budget SaveState 持久化模式测试
// ============================================

// TestBudgetSaveState_Normal 验证预算充足的 SaveState 模式
func TestBudgetSaveState_Normal(t *testing.T) {
	store := newMockStore()
	ctx := context.Background()
	sid := "session-budget-normal"

	// 初始状态
	initial := agenttypes.ReimbursementState{
		CurrentPhase: "phase2_validate",
		DepartmentID: 101,
		Invoices:     []agenttypes.InvoiceState{{Amount: 50000, Category: "办公用品"}},
	}
	store.SaveState(ctx, sid, infra.StateKeyReimbursement, &initial)

	// 模拟预算工具的 SaveState 逻辑
	var state agenttypes.ReimbursementState
	store.GetState(ctx, sid, infra.StateKeyReimbursement, &state)
	state.BudgetResult = &agenttypes.BudgetCheckResult{
		Remaining:           2000000, // 20,000 元
		NeedSpecialApproval: false,
		UsageRate:           0.35,
	}
	store.SaveState(ctx, sid, infra.StateKeyReimbursement, &state)

	// 验证
	var restored agenttypes.ReimbursementState
	store.GetState(ctx, sid, infra.StateKeyReimbursement, &restored)
	if restored.BudgetResult == nil {
		t.Fatal("BudgetResult 不应为 nil")
	}
	if restored.BudgetResult.Remaining != 2000000 {
		t.Errorf("Remaining = %d, want 2000000", restored.BudgetResult.Remaining)
	}
	if restored.BudgetResult.NeedSpecialApproval {
		t.Error("预算充足时 NeedSpecialApproval 应为 false")
	}
	if len(restored.Invoices) != 1 {
		t.Error("原有票据数据不应丢失")
	}
}

// TestBudgetSaveState_NeedApproval 验证预算不足触发审批
func TestBudgetSaveState_NeedApproval(t *testing.T) {
	store := newMockStore()
	ctx := context.Background()
	sid := "session-budget-approval"

	var state agenttypes.ReimbursementState
	state.DepartmentID = 202
	state.BudgetResult = &agenttypes.BudgetCheckResult{
		Remaining:           500000,
		NeedSpecialApproval: true,
		UsageRate:           0.92,
	}
	store.SaveState(ctx, sid, infra.StateKeyReimbursement, &state)

	var restored agenttypes.ReimbursementState
	store.GetState(ctx, sid, infra.StateKeyReimbursement, &restored)
	if !restored.BudgetResult.NeedSpecialApproval {
		t.Error("预算紧张时 NeedSpecialApproval 应为 true")
	}
	if restored.BudgetResult.UsageRate != 0.92 {
		t.Errorf("UsageRate = %f, want 0.92", restored.BudgetResult.UsageRate)
	}
}

// ============================================
// 三阶段完整状态流转测试
// ============================================

// TestFullStateTransitionFlow 验证 Phase 1→2→3 完整状态流转
// 通过 mock SessionStore 直接模拟各工具的状态变更
func TestFullStateTransitionFlow(t *testing.T) {
	store := newMockStore()
	ctx := context.Background()
	sid := "session-full-flow"

	// ── Phase 1：OCR 识别票据 ──
	var state agenttypes.ReimbursementState
	state.CurrentPhase = "phase1_collect"
	state.Invoices = append(state.Invoices, agenttypes.InvoiceState{Amount: 10000, Category: "交通"})
	state.TotalAmount = 10000
	store.SaveState(ctx, sid, infra.StateKeyReimbursement, &state)

	// ── Phase 1→2：确认票据 ──
	var state2 agenttypes.ReimbursementState
	store.GetState(ctx, sid, infra.StateKeyReimbursement, &state2)
	state2.UserConfirmed = true
	state2.CurrentPhase = "phase2_validate"
	store.SaveState(ctx, sid, infra.StateKeyReimbursement, &state2)

	var afterP1 agenttypes.ReimbursementState
	store.GetState(ctx, sid, infra.StateKeyReimbursement, &afterP1)
	if !afterP1.UserConfirmed || afterP1.CurrentPhase != "phase2_validate" {
		t.Fatal("Phase 1→2 过渡失败")
	}

	// ── Phase 2：合规 + 预算检查 ──
	var state3 agenttypes.ReimbursementState
	store.GetState(ctx, sid, infra.StateKeyReimbursement, &state3)
	state3.ComplianceResult = &agenttypes.ComplianceCheckResult{
		Result: "pass", Level: "pass", Message: "通过", RuleID: "TRAVEL-001",
	}
	state3.BudgetResult = &agenttypes.BudgetCheckResult{
		Remaining: 1000000, NeedSpecialApproval: false, UsageRate: 0.25,
	}
	store.SaveState(ctx, sid, infra.StateKeyReimbursement, &state3)

	var afterP2 agenttypes.ReimbursementState
	store.GetState(ctx, sid, infra.StateKeyReimbursement, &afterP2)
	if afterP2.ComplianceResult == nil || afterP2.BudgetResult == nil {
		t.Fatal("Phase 2 后合规和预算结果不应为 nil")
	}

	// ── Phase 2→3：最终确认 ──
	var state4 agenttypes.ReimbursementState
	store.GetState(ctx, sid, infra.StateKeyReimbursement, &state4)
	state4.FinalConfirmed = true
	state4.CurrentPhase = "phase3_execute"
	store.SaveState(ctx, sid, infra.StateKeyReimbursement, &state4)

	var afterP3 agenttypes.ReimbursementState
	store.GetState(ctx, sid, infra.StateKeyReimbursement, &afterP3)
	if !afterP3.FinalConfirmed || afterP3.CurrentPhase != "phase3_execute" {
		t.Fatal("Phase 2→3 过渡失败")
	}
	if len(afterP3.Invoices) != 1 || afterP3.TotalAmount != 10000 {
		t.Error("Phase 3 时初始票据数据应保持完整")
	}
	if afterP3.ComplianceResult == nil || afterP3.BudgetResult == nil {
		t.Error("Phase 3 时合规和预算结果应保留")
	}
}

// ============================================
// mockSessionStore 接口契约验证
// ============================================

// compileTimeCheck 确保 mockSessionStore 实现与 infra.SessionStore 保持同步
// 若接口新增方法而 mock 未同步更新，此行将编译报错
var _ interface {
	SaveMessages(context.Context, string, []*schema.Message) error
	GetHistory(context.Context, string, int) ([]*schema.Message, error)
	Clear(context.Context, string) error
	SaveState(context.Context, string, string, any) error
	GetState(context.Context, string, string, any) (bool, error)
	DeleteState(context.Context, string, string) error
} = (*mockSessionStore)(nil)
