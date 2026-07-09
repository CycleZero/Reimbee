package agent

import (
	"context"
	"fmt"
	"testing"

	"github.com/CycleZero/blades"
)

func TestPromptRender_EmployeeIdentity(t *testing.T) {
	// 模拟 JWT 中间件注入的身份信息
	state := blades.State{
		"employee_id":   "E001",
		"employee_name": "张三",
		"role":          "employee",
		"user_id":       uint(1),
	}

	ctx := WithAgentMeta(context.Background(), &AgentMeta{Role: "employee"})
	ctx = WithSessionState(ctx, state)

	provider := BuildInstruction()
	rendered, err := provider(ctx)
	if err != nil {
		t.Fatalf("渲染失败: %v", err)
	}

	fmt.Println("\n========== LLM 实际收到的系统提示词 ==========")
	fmt.Println(rendered)
	fmt.Println("========== 提示词结束 ==========")
	fmt.Printf("\n提示词长度: %d 字符\n", len(rendered))

	// 验证关键字段已渲染（不是模板占位符）
	if contains(rendered, "{{.employee_name}}") {
		t.Error("❌ employee_name 未被渲染！LLM 看到的是模板占位符 {{.employee_name}}")
	}
	if contains(rendered, "{{.employee_id}}") {
		t.Error("❌ employee_id 未被渲染！LLM 看到的是模板占位符 {{.employee_id}}")
	}
	if contains(rendered, "{{.state}}") {
		t.Error("❌ state 未被渲染！LLM 看到的是模板占位符 {{.state}}")
	}

	// 验证实际值已注入
	if !contains(rendered, "张三") {
		t.Error("❌ 提示词中不包含员工姓名 '张三'")
	}
	if !contains(rendered, "E001") {
		t.Error("❌ 提示词中不包含工号 'E001'")
	}
	if !contains(rendered, "员工") {
		t.Error("❌ 提示词中不包含角色信息")
	}

	fmt.Println("\n✅ 全部检查通过：员工身份信息已正确注入提示词")
}

func TestPromptRender_ApproverIdentity(t *testing.T) {
	state := blades.State{
		"employee_id":   "A001",
		"employee_name": "李主任",
		"role":          "approver",
		"user_id":       uint(2),
	}

	ctx := WithAgentMeta(context.Background(), &AgentMeta{Role: "approver"})
	ctx = WithSessionState(ctx, state)

	provider := BuildInstruction()
	rendered, err := provider(ctx)
	if err != nil {
		t.Fatalf("渲染失败: %v", err)
	}

	fmt.Println("\n========== 审批人提示词 ==========")
	fmt.Println(rendered)
	fmt.Println("========== 提示词结束 ==========")

	if contains(rendered, "{{.employee_name}}") {
		t.Error("❌ 审批人提示词中 employee_name 未被渲染")
	}
	if !contains(rendered, "李主任") {
		t.Error("❌ 审批人提示词中不包含姓名 '李主任'")
	}

	fmt.Println("\n✅ 审批人提示词检查通过")
}

func TestPromptRender_NoState_NoCrash(t *testing.T) {
	// 无 session state 时不崩溃
	ctx := WithAgentMeta(context.Background(), &AgentMeta{Role: "employee"})
	provider := BuildInstruction()
	rendered, err := provider(ctx)
	if err != nil {
		t.Fatalf("无 state 时渲染失败（不应该崩溃）: %v", err)
	}
	if rendered == "" {
		t.Error("无 state 时返回了空字符串")
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
