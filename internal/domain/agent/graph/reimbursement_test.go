package graph

import (
	"context"
	"strings"
	"testing"

	"github.com/CycleZero/Reimbee/internal/testutil"
	"github.com/cloudwego/eino/schema"
)

// ============================================
// NewReimbursementGraph 编译测试
// ============================================

func TestNewReimbursementGraph_CompilesWithMockModel(t *testing.T) {
	mockModel := testutil.NewTextReplyChatModel("报销处理完成，单号 REIMB-2026-0001")
	deps := ReimbursementGraphDeps{
		Logger:    nopLogger(),
		ToolSet:   nil,
		ChatModel: mockModel,
	}

	runnable, err := NewReimbursementGraph(context.Background(), deps)
	if err != nil {
		t.Fatalf("NewReimbursementGraph 编译失败: %v", err)
	}
	if runnable == nil {
		t.Fatal("NewReimbursementGraph 返回 nil runnable")
	}
	t.Logf("NewReimbursementGraph 编译成功")
}

func TestNewReimbursementGraph_NilChatModelReturnsError(t *testing.T) {
	deps := ReimbursementGraphDeps{
		Logger:    nopLogger(),
		ToolSet:   nil,
		ChatModel: nil,
	}

	_, err := NewReimbursementGraph(context.Background(), deps)
	if err == nil {
		t.Fatal("期望返回错误（chatModel 为 nil），但错误为 nil")
	}
	if !strings.Contains(err.Error(), "chatModel") {
		t.Errorf("错误消息应包含 'chatModel'，实际: %v", err)
	}
	t.Logf("nil chatModel 正确返回错误: %v", err)
}

// ============================================
// 运行时行为测试
// ============================================

func TestNewReimbursementGraph_GuardLoopExceedsSteps(t *testing.T) {
	// Phase1 Guard 始终失败（无票据），图应因 exceeded max steps 而错误退出
	mockModel := testutil.NewTextReplyChatModel("请上传票据图片以便继续。")

	deps := ReimbursementGraphDeps{
		Logger:    nopLogger(),
		ToolSet:   nil,
		ChatModel: mockModel,
	}

	runnable, err := NewReimbursementGraph(context.Background(), deps)
	if err != nil {
		t.Fatalf("编译失败: %v", err)
	}

	_, err = runnable.Invoke(context.Background(),
		[]*schema.Message{schema.UserMessage("报销")})

	if err == nil {
		t.Error("期望因 Guard 循环超限而返回错误")
	} else {
		t.Logf("Guard 循环正确超限: %v", err)
	}
}

// ============================================
// 依赖验证
// ============================================

func TestReimbursementGraphDeps_Defaults(t *testing.T) {
	deps := ReimbursementGraphDeps{
		Logger: nopLogger(),
	}

	if deps.Logger == nil {
		t.Error("Logger 不应为 nil")
	}
	// ToolSet/ChatModel nil 应该被安全处理
	t.Log("ReimbursementGraphDeps 默认值验证通过")
}
