package graph

import (
	"context"
	"strings"
	"testing"

	"github.com/CycleZero/Reimbee/internal/testutil"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

// ============================================
// TestBuildReActPhase_Compiles — 编译验证
// ============================================

// S1.1: 正常参数（有效 chatModel + 工具）→ 编译成功
func TestBuildReActPhase_CompilesWithTools(t *testing.T) {
	mockModel := testutil.NewTextReplyChatModel("收到您的报销请求")
	mockTool := testutil.NewNamedMockTool("ocr_tool", "识别成功: 金额500元")

	graph, err := buildReActPhase(
		context.Background(),
		mockModel,
		nopLogger(),
		PhaseConfig{
			Name:         "test_phase",
			SystemPrompt: "你是一个测试助手",
			Tools:        []tool.BaseTool{mockTool},
		},
	)

	if err != nil {
		t.Fatalf("buildReActPhase 编译失败: %v", err)
	}
	if graph == nil {
		t.Fatal("buildReActPhase 返回 nil graph")
	}
	t.Log("✅ buildReActPhase 带工具编译成功")
}

// S1.2: chatModel 为 nil → 返回错误
func TestBuildReActPhase_NilChatModel(t *testing.T) {
	graph, err := buildReActPhase(
		context.Background(),
		nil,
		nopLogger(),
		PhaseConfig{
			Name:         "nil_test",
			SystemPrompt: "你是一个测试助手",
			Tools:        nil,
		},
	)

	if err == nil {
		t.Fatal("期望返回错误（chatModel 为 nil），但错误为 nil")
	}
	if graph != nil {
		t.Error("期望 graph 为 nil")
	}
	if !strings.Contains(err.Error(), "chatModel") {
		t.Errorf("错误消息应包含 'chatModel'，实际: %v", err)
	}
	t.Logf("✅ nil chatModel 正确返回错误: %v", err)
}

// S1.3: 空工具列表 → 编译成功（ChatModel 直出模式）
func TestBuildReActPhase_NoTools(t *testing.T) {
	mockModel := testutil.NewTextReplyChatModel("纯文本回复")

	graph, err := buildReActPhase(
		context.Background(),
		mockModel,
		nopLogger(),
		PhaseConfig{
			Name:         "no_tools_test",
			SystemPrompt: "你是一个测试助手",
			Tools:        []tool.BaseTool{},
		},
	)

	if err != nil {
		t.Fatalf("buildReActPhase（无工具）编译失败: %v", err)
	}
	if graph == nil {
		t.Fatal("buildReActPhase（无工具）返回 nil graph")
	}
	t.Log("✅ buildReActPhase 无工具编译成功")
}

// S1.4: 工具列表为 nil → 编译成功（ChatModel 直出模式）
func TestBuildReActPhase_NilTools(t *testing.T) {
	mockModel := testutil.NewTextReplyChatModel("纯文本回复")

	graph, err := buildReActPhase(
		context.Background(),
		mockModel,
		nopLogger(),
		PhaseConfig{
			Name:         "nil_tools_test",
			SystemPrompt: "你是一个测试助手",
			Tools:        nil,
		},
	)

	if err != nil {
		t.Fatalf("buildReActPhase（nil 工具）编译失败: %v", err)
	}
	if graph == nil {
		t.Fatal("buildReActPhase（nil 工具）返回 nil graph")
	}
	t.Log("✅ buildReActPhase nil工具编译成功")
}

// ============================================
// TestBuildReActPhase_Runtime — 运行时验证
// ============================================

// S1.5: 图执行——ChatModel 返回直出文本（无 ToolCall）→ 正确返回文本回复
func TestBuildReActPhase_InvokeTextOnly(t *testing.T) {
	expectedReply := "您好！我是 Reimbee 报销助手，请上传您的票据图片。"
	mockModel := testutil.NewTextReplyChatModel(expectedReply)

	graph, err := buildReActPhase(
		context.Background(),
		mockModel,
		nopLogger(),
		PhaseConfig{
			Name:         "phase1_text",
			SystemPrompt: "你是 Reimbee 报销助手，请引导用户上传票据。",
			Tools:        []tool.BaseTool{},
		},
	)
	if err != nil {
		t.Fatalf("构建图失败: %v", err)
	}

	// 编译图
	runnable, err := graph.Compile(context.Background(),
		compose.WithGraphName("test_text_only"),
		compose.WithMaxRunSteps(10),
	)
	if err != nil {
		t.Fatalf("编译图失败: %v", err)
	}

	// 执行——传入用户消息
	input := []*schema.Message{schema.UserMessage("我要报销")}
	result, err := runnable.Invoke(context.Background(), input)
	if err != nil {
		t.Fatalf("图执行失败: %v", err)
	}
	if result == nil {
		t.Fatal("图返回 nil 结果")
	}
	if result.Content != expectedReply {
		t.Errorf("期望回复 '%s'，实际: '%s'", expectedReply, result.Content)
	}
	t.Logf("✅ 纯文本场景成功，回复: %s", result.Content)
}

// S1.6: 图执行——ChatModel 返回 ToolCall → ToolsNode 执行 → ChatModel 再返回文本
func TestBuildReActPhase_InvokeWithToolCall(t *testing.T) {
	// 工具: 返回固定结果
	ocrTool := testutil.NewNamedMockTool("ocr", "识别结果: 差旅-交通 ¥500.00, 置信度 0.95")

	// ChatModel: 第一轮返回 ToolCall，第二轮返回文本
	mockModel := testutil.NewMultiTurnChatModel([]*schema.Message{
		{
			Role:    schema.Assistant,
			Content: "",
			ToolCalls: []schema.ToolCall{
				{
					ID:   "call_001",
					Type: "function",
					Function: schema.FunctionCall{
						Name:      "ocr",
						Arguments: `{"input": "请识别发票"}`,
					},
				},
			},
		},
		schema.AssistantMessage("已识别到票据：差旅-交通 ¥500.00，请确认无误。", nil),
	})

	graph, err := buildReActPhase(
		context.Background(),
		mockModel,
		nopLogger(),
		PhaseConfig{
			Name:         "phase1_with_ocr",
			SystemPrompt: "你是 Reimbee 报销助手，帮助用户识别票据。",
			Tools:        []tool.BaseTool{ocrTool},
		},
	)
	if err != nil {
		t.Fatalf("构建图失败: %v", err)
	}

	runnable, err := graph.Compile(context.Background(),
		compose.WithGraphName("test_with_tool_call"),
		compose.WithMaxRunSteps(20),
	)
	if err != nil {
		t.Fatalf("编译图失败: %v", err)
	}

	input := []*schema.Message{schema.UserMessage("帮我识别这张发票")}
	result, err := runnable.Invoke(context.Background(), input)
	if err != nil {
		t.Fatalf("图执行失败: %v", err)
	}
	if result == nil {
		t.Fatal("图返回 nil 结果")
	}

	expectedSubstring := "已识别"
	if !strings.Contains(result.Content, expectedSubstring) {
		t.Errorf("期望回复包含 '%s'，实际: '%s'", expectedSubstring, result.Content)
	}
	if len(result.ToolCalls) > 0 {
		t.Error("最终回复不应包含 ToolCalls（应已被 ToolsNode 消费）")
	}
	t.Logf("✅ 工具调用场景成功，最终回复: %s", result.Content)
}

// S1.7: 图执行——连续3次 ToolCall 后得到最终回复
func TestBuildReActPhase_InvokeMultiTurnToolCalls(t *testing.T) {
	complianceTool := testutil.NewNamedMockTool("check_compliance", "合规检查通过")
	budgetTool := testutil.NewNamedMockTool("check_budget", "预算余额: ¥50,000, 使用率 30%")

	// ChatModel: 3轮交互 — 合规检查 → 预算检查 → 确认回复
	mockModel := testutil.NewMultiTurnChatModel([]*schema.Message{
		// 轮1: 调用合规检查
		{
			Role:    schema.Assistant,
			Content: "",
			ToolCalls: []schema.ToolCall{
				{ID: "c1", Type: "function", Function: schema.FunctionCall{Name: "check_compliance", Arguments: `{}`}},
			},
		},
		// 轮2: 调用预算检查
		{
			Role:    schema.Assistant,
			Content: "",
			ToolCalls: []schema.ToolCall{
				{ID: "c2", Type: "function", Function: schema.FunctionCall{Name: "check_budget", Arguments: `{}`}},
			},
		},
		// 轮3: 最终确认回复
		schema.AssistantMessage("合规检查通过，预算充足。总计 ¥500.00，请确认提交。", nil),
	})

	graph, err := buildReActPhase(
		context.Background(),
		mockModel,
		nopLogger(),
		PhaseConfig{
			Name:         "phase2_multi",
			SystemPrompt: "你是 Reimbee，执行合规检查和预算校验。",
			Tools:        []tool.BaseTool{complianceTool, budgetTool},
		},
	)
	if err != nil {
		t.Fatalf("构建图失败: %v", err)
	}

	runnable, err := graph.Compile(context.Background(),
		compose.WithGraphName("test_multi_turn"),
		compose.WithMaxRunSteps(30),
	)
	if err != nil {
		t.Fatalf("编译图失败: %v", err)
	}

	input := []*schema.Message{schema.UserMessage("校验这张票据")}
	result, err := runnable.Invoke(context.Background(), input)
	if err != nil {
		t.Fatalf("图执行失败: %v", err)
	}
	if result == nil {
		t.Fatal("图返回 nil 结果")
	}

	if !strings.Contains(result.Content, "确认提交") {
		t.Errorf("期望最终回复包含 '确认提交'，实际: '%s'", result.Content)
	}
	t.Logf("✅ 多轮工具调用成功，最终回复: %s", result.Content)
}

// ============================================
// TestBuildReActPhase_EdgeCases — 边界场景
// ============================================

// S1.8: 空消息输入应不 panic
func TestBuildReActPhase_EmptyInput(t *testing.T) {
	mockModel := testutil.NewTextReplyChatModel("请提供更多信息")

	graph, err := buildReActPhase(
		context.Background(),
		mockModel,
		nopLogger(),
		PhaseConfig{
			Name:         "empty_test",
			SystemPrompt: "你是一个助手",
			Tools:        nil,
		},
	)
	if err != nil {
		t.Fatalf("构建图失败: %v", err)
	}

	runnable, err := graph.Compile(context.Background(),
		compose.WithGraphName("test_empty"),
		compose.WithMaxRunSteps(5),
	)
	if err != nil {
		t.Fatalf("编译图失败: %v", err)
	}

	// 传入空消息列表——不应 panic
	result, err := runnable.Invoke(context.Background(), []*schema.Message{})
	if err != nil {
		t.Fatalf("空输入应返回错误或降级处理，实际: %v", err)
	}
	if result == nil {
		t.Fatal("空输入不应返回 nil")
	}
	t.Logf("✅ 空输入场景成功: %s", result.Content)
}

// S1.9: 验证状态——工具执行结果被正确累积到消息历史
func TestBuildReActPhase_StateAccumulation(t *testing.T) {
	// 自定义 GenerateFunc 以验证消息历史是否累积
	callCount := 0
	mockModel := &testutil.MockChatModel{
		GenerateFunc: func(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
			callCount++
			if callCount == 1 {
				// 第一轮: 返回 ToolCall
				return &schema.Message{
					Role:    schema.Assistant,
					Content: "",
					ToolCalls: []schema.ToolCall{
						{ID: "check", Type: "function", Function: schema.FunctionCall{Name: "my_tool", Arguments: `{}`}},
					},
				}, nil
			}
			// 第二轮: 验证 input 是否包含第一轮的 assistant 消息 + tool 结果
			// 至少应该有 3 条消息: system + user + assistant(含ToolCall) + tool_result
			if len(input) < 4 {
				t.Errorf("第二轮 Generate 输入消息数不足: 期望 >= 4, 实际 %d", len(input))
			}
			return schema.AssistantMessage("处理完成", nil), nil
		},
	}

	mockTool := testutil.NewNamedMockTool("my_tool", "执行成功")

	graph, err := buildReActPhase(
		context.Background(),
		mockModel,
		nopLogger(),
		PhaseConfig{
			Name:         "state_test",
			SystemPrompt: "系统提示词",
			Tools:        []tool.BaseTool{mockTool},
		},
	)
	if err != nil {
		t.Fatalf("构建图失败: %v", err)
	}

	runnable, err := graph.Compile(context.Background(),
		compose.WithGraphName("test_state"),
		compose.WithMaxRunSteps(20),
	)
	if err != nil {
		t.Fatalf("编译图失败: %v", err)
	}

	result, err := runnable.Invoke(context.Background(),
		[]*schema.Message{schema.UserMessage("帮助我")})
	if err != nil {
		t.Fatalf("图执行失败: %v", err)
	}

	if callCount != 2 {
		t.Errorf("期望 Generate 被调用 2 次，实际: %d", callCount)
	}
	if result.Content != "处理完成" {
		t.Errorf("期望最终回复 '处理完成'，实际: '%s'", result.Content)
	}
	t.Logf("✅ 状态累积验证成功 (Generate 调用次数: %d)", callCount)
}
