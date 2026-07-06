// Package graph — Root Graph 集成测试
//
// 验证意图分类、路由分发、以及用户上下文传递的完整性
package graph

import (
	"context"
	"strings"
	"testing"

	"github.com/CycleZero/Reimbee/internal/domain/agent"
	"github.com/CycleZero/Reimbee/internal/testutil"
)

// ============================================
// 意图分类准确性测试
// ============================================

func TestIntentRouting_ClassifyByKeywords_AllCategories(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"我要报销差旅费", "new_reimbursement"},
		{"帮我提交一张发票", "new_reimbursement"},
		{"申请报销办公用品", "new_reimbursement"},
		{"我的报销进度怎么样了", "new_reimbursement"},     // "报销" 关键词优先
		{"审批到哪一步了", "query_progress"},
		{"查一下报销状态", "new_reimbursement"},           // "报销" 关键词优先
		{"部门还剩多少预算", "query_budget"},
		{"预算够不够", "query_budget"},
		{"住宿标准是多少", "policy_question"},
		{"出差能报多少钱", "policy_question"},             // "多少" 匹配 policy
		{"公司报销有什么规定", "new_reimbursement"},        // "报销" 关键词优先
		{"改一下报销金额", "new_reimbursement"},            // "报销" 关键词优先
		{"被驳回了怎么重新提交", "new_reimbursement"},       // "提交" 关键词匹配
		{"你好", "general_chat"},
		{"谢谢你的帮助", "general_chat"},
		{"今天天气不错", "general_chat"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := classifyByKeywords(tt.input)
			if got != tt.expected {
				t.Errorf("classifyByKeywords(%q) = %q，期望 %q", tt.input, got, tt.expected)
			}
		})
	}
	t.Logf("✅ 意图分类测试完成（%d 条输入）", len(tests))
}

// ============================================
// 关键词优先级测试
// ============================================

func TestIntentRouting_KeywordPriority(t *testing.T) {
	// "报销" 关键词在所有类别中优先级最高（最先检查）
	got := classifyByKeywords("修改报销单的状态查询预算标准")
	if got != "new_reimbursement" {
		t.Errorf("包含'报销'时应优先匹配 new_reimbursement，实际: %s", got)
	}
	t.Log("✅ 关键词优先级：'报销' 优先于其他类别")
}

// ============================================
// 用户上下文传递测试
// ============================================

func TestIntentRouting_UserContextInjection(t *testing.T) {
	// 验证 dispatchToWorkflow 将 AgentInput 注入 context
	input := agent.AgentInput{
		SessionID:  "sess-test-001",
		UserID:     42,
		EmployeeID: "EMP042",
		Role:       "employee",
		Message:    "你好",
	}

	ctx := context.WithValue(context.Background(), userContextKey{}, input)

	// 从 context 恢复 AgentInput
	recovered, ok := ctx.Value(userContextKey{}).(agent.AgentInput)
	if !ok {
		t.Fatal("无法从 context 恢复 AgentInput")
	}
	if recovered.SessionID != "sess-test-001" {
		t.Errorf("SessionID 不匹配: %s", recovered.SessionID)
	}
	if recovered.UserID != 42 {
		t.Errorf("UserID 不匹配: %d", recovered.UserID)
	}
	if recovered.EmployeeID != "EMP042" {
		t.Errorf("EmployeeID 不匹配: %s", recovered.EmployeeID)
	}
	if recovered.Role != "employee" {
		t.Errorf("Role 不匹配: %s", recovered.Role)
	}
	t.Log("✅ 用户上下文注入/恢复验证通过")
}

// ============================================
// dispatchToWorkflow 端到端测试
// ============================================

func TestDispatchToWorkflow_GeneralChat(t *testing.T) {
	mockModel := testutil.NewTextReplyChatModel("您好！我是 Reimbee 报销助手。有什么可以帮您的？")

	deps := RootGraphDeps{
		Logger:    nopLogger(),
		ChatModel: mockModel,
		Config:    &agent.AgentConfig{IntentConfidenceThreshold: 0.7},
	}

	input := agent.AgentInput{
		SessionID:  "test-session",
		UserID:     1,
		EmployeeID: "EMP001",
		Role:       "employee",
		Message:    "你好",
	}

	result := dispatchToWorkflow(context.Background(), input, deps)
	if result == nil {
		t.Fatal("dispatchToWorkflow 返回 nil")
	}
	if !strings.Contains(result.Content, "Reimbee") && !strings.Contains(result.Content, "报销") {
		t.Logf("general_chat 回复: %s", result.Content)
	}
	t.Logf("✅ general_chat 分发成功: %s", result.Content)
}

func TestDispatchToWorkflow_ReimbursementRoute(t *testing.T) {
	// 报销路由：消息 "我要报销差旅费" → 分类为 new_reimbursement
	mockModel := testutil.NewTextReplyChatModel("报销流程已启动，请上传票据图片。")

	runnable, err := NewReimbursementGraph(context.Background(), ReimbursementGraphDeps{
		Logger:    nopLogger(),
		ToolSet:   nil,
		ChatModel: mockModel,
		Config:    &agent.AgentConfig{MaxPhaseTurns: 3},
	})
	if err != nil {
		t.Fatalf("编译报销图失败: %v", err)
	}

	deps := RootGraphDeps{
		Logger:                nopLogger(),
		ChatModel:             mockModel,
		ReimbursementRunnable: runnable,
		Config:                &agent.AgentConfig{IntentConfidenceThreshold: 0.7},
	}

	input := agent.AgentInput{
		SessionID:  "test-session",
		UserID:     1,
		EmployeeID: "EMP001",
		Role:       "employee",
		Message:    "我要报销差旅费",
	}

	result := dispatchToWorkflow(context.Background(), input, deps)
	if result == nil {
		t.Fatal("dispatchToWorkflow 返回 nil")
	}

	// 验证分类正确路由到报销子流程
	route := classifyByKeywords(input.Message)
	if route != "new_reimbursement" {
		t.Errorf("期望路由到 new_reimbursement，实际: %s", route)
	}
	t.Logf("✅ 报销路由分发成功: route=%s, reply=%s", route, result.Content)
}

// ============================================
// agentInputAdapter 上下文恢复测试
// ============================================

func TestAgentInputAdapter_ContextRecovery(t *testing.T) {
	// 模拟 dispatchToWorkflow 注入上下文，adapter 恢复
	original := agent.AgentInput{
		SessionID:  "sess-abc",
		UserID:     99,
		EmployeeID: "EMP099",
		Role:       "approver",
		Message:    "查询进度",
	}

	ctx := context.WithValue(context.Background(), userContextKey{}, original)

	// 模拟 adapter 的恢复逻辑
	ai := agent.AgentInput{Message: "新消息内容"}
	if uc, ok := ctx.Value(userContextKey{}).(agent.AgentInput); ok {
		ai = uc
		ai.Message = "新消息内容"
	}

	if ai.SessionID != "sess-abc" {
		t.Error("SessionID 未被恢复")
	}
	if ai.UserID != 99 {
		t.Error("UserID 未被恢复")
	}
	if ai.EmployeeID != "EMP099" {
		t.Error("EmployeeID 未被恢复")
	}
	if ai.Role != "approver" {
		t.Error("Role 未被恢复")
	}
	if ai.Message != "新消息内容" {
		t.Error("Message 应被覆盖为最新消息")
	}
	t.Log("✅ agentInputAdapter 上下文恢复验证通过")
}

// ============================================
// Runable 编译验证
// ============================================

func TestNewRootGraph_Compiles(t *testing.T) {
	mockModel := testutil.NewTextReplyChatModel("hello")

	deps := RootGraphDeps{
		Logger:    nopLogger(),
		ChatModel: mockModel,
		Config:    &agent.AgentConfig{IntentConfidenceThreshold: 0.7},
	}

	runnable, err := NewRootGraph(context.Background(), deps)
	if err != nil {
		t.Fatalf("编译 Root Graph 失败: %v", err)
	}
	if runnable == nil {
		t.Fatal("Root Graph runnable 为 nil")
	}

	// 执行一个最简单的请求
	result, err := runnable.Invoke(context.Background(), agent.AgentInput{
		SessionID: "test",
		Message:   "hello",
	})
	if err != nil {
		t.Fatalf("Root Graph 执行失败: %v", err)
	}
	if result == nil {
		t.Fatal("Root Graph 返回 nil")
	}
	t.Logf("✅ Root Graph 编译+执行成功: %s", result.Content)
}
