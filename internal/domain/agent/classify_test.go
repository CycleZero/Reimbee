// Package agent 智能体层 — classifyByKeywords / selectPhaseAgent 单元测试
// 内部测试（package agent）可访问未导出函数，直接测试状态机逻辑
package agent

import (
	"context"
	"testing"

	"github.com/CycleZero/Reimbee/internal/testutil"
	"github.com/cloudwego/eino/adk"
)

// ============================================
// 辅助函数
// ============================================

// createTestAgent 使用 testutil.MockChatModel 创建最小 ChatModelAgent
// 仅用于 selectPhaseAgent 测试中区分不同 Agent 实例
func createTestAgent(t *testing.T, name string) *adk.ChatModelAgent {
	t.Helper()
	mock := testutil.NewTextReplyChatModel("test reply")
	agent, err := adk.NewChatModelAgent(context.Background(), &adk.ChatModelAgentConfig{
		Name:        name,
		Description: "测试用Agent",
		Model:       mock,
	})
	if err != nil {
		t.Fatalf("创建测试Agent(%s)失败: %v", name, err)
	}
	return agent
}

// buildTestLoopManager 构建最小 LoopManager，仅设置 4 个 Phase Agent 字段
// 用于 selectPhaseAgent 测试
func buildTestLoopManager(t *testing.T) *LoopManager {
	t.Helper()
	return &LoopManager{
		phase1Agent: createTestAgent(t, "phase1_collect"),
		phase2Agent: createTestAgent(t, "phase2_validate"),
		phase3Agent: createTestAgent(t, "phase3_execute"),
		chatAgent:   createTestAgent(t, "general_chat"),
		loops:       make(map[string]*SessionLoop),
	}
}

// ============================================
// classifyByKeywords 测试（6 种意图 × 边界条件）
// ============================================

func TestClassifyByKeywords(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		// ── 新报销意图 ──
		{
			name:    "报销关键词_我要报销",
			content: "我要报销差旅费",
			want:    "new_reimbursement",
		},
		{
			name:    "提交关键词_提交发票",
			content: "提交发票申请",
			want:    "new_reimbursement",
		},
		{
			name:    "发票关键词_发票报销",
			content: "这张发票可以报销吗",
			want:    "new_reimbursement",
		},
		{
			name:    "申请报销关键词",
			content: "申请报销办公用品",
			want:    "new_reimbursement",
		},

		// ── 查询进度意图 ──
		{
			name:    "进度关键词_进度到哪了",
			content: "我的报销进度到哪了",
			want:    "query_progress",
		},
		{
			name:    "审批关键词_批了吗",
			content: "上次那个报销批了吗",
			want:    "query_progress",
		},
		{
			name:    "状态关键词_报销状态",
			content: "帮我查一下报销状态",
			want:    "query_progress",
		},
		{
			name:    "到哪关键词_审批到哪了",
			content: "审批到哪了",
			want:    "query_progress",
		},

		// ── 查询预算意图 ──
		{
			name:    "预算关键词_还剩多少预算",
			content: "我们这个季度还剩多少预算",
			want:    "query_budget",
		},
		{
			name:    "余额关键词_余额够不够",
			content: "预算余额够不够报销这笔",
			want:    "query_budget",
		},
		{
			name:    "还剩关键词_还剩多少",
			content: "还剩多少钱",
			want:    "query_budget",
		},
		{
			name:    "够不够关键词",
			content: "这个金额够不够报",
			want:    "query_budget",
		},

		// ── 政策咨询意图 ──
		{
			name:    "标准关键词_差旅标准",
			content: "差旅住宿标准是什么",
			want:    "policy_question",
		},
		{
			name:    "规定关键词_报销规定",
			content: "公司的报销规定有哪些",
			want:    "policy_question",
		},
		{
			name:    "可以报吗关键词_可以报销吗",
			content: "伙食费可以报吗",
			want:    "policy_question",
		},
		{
			name:    "政策关键词_报销政策",
			content: "最新的报销政策是什么",
			want:    "policy_question",
		},
		{
			name:    "多少关键词_报多少",
			content: "交通费每天能报多少",
			want:    "policy_question",
		},

		// ── 修改报销意图 ──
		{
			name:    "修改关键词_我要修改报销单",
			content: "我要修改报销单",
			want:    "modify_reimbursement",
		},
		{
			name:    "驳回关键词_被退回来了",
			content: "报销被退回来了怎么办",
			want:    "modify_reimbursement",
		},
		{
			name:    "重新提交关键词",
			content: "怎么重新提交报销",
			want:    "modify_reimbursement",
		},
		{
			name:    "改关键词_改一下",
			content: "帮我把金额改一下",
			want:    "modify_reimbursement",
		},

		// ── 通用对话 ──
		{
			name:    "问候语_你好",
			content: "你好",
			want:    "general_chat",
		},
		{
			name:    "感谢语_谢谢",
			content: "谢谢你的帮助",
			want:    "general_chat",
		},
		{
			name:    "空消息",
			content: "",
			want:    "general_chat",
		},
		{
			name:    "无关键词消息",
			content: "今天天气不错",
			want:    "general_chat",
		},

		// ── 边界条件 — 超长消息截断 ──
		{
			name:    "超长消息含报销关键词",
			content: "这是一段很长的前缀文本" +
				"这是一段很长的前缀文本这是一段很长的前缀文本这是一段很长的前缀文本" +
				"这是一段很长的前缀文本这是一段很长的前缀文本这是一段很长的前缀文本" +
				"报销差旅费",
			want: "new_reimbursement",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyByKeywords(tt.content)
			if got != tt.want {
				t.Errorf("classifyByKeywords(%q) = %q, want %q",
					tt.content, got, tt.want)
			}
		})
	}
}

// ============================================
// selectPhaseAgent 测试（4 分支 + 优先级验证）
// ============================================

func TestSelectPhaseAgent(t *testing.T) {
	mgr := buildTestLoopManager(t)

	tests := []struct {
		name string
		rs   ReimbursementState
		want string
	}{
		{
			name: "Phase1_默认分支_空状态",
			rs: ReimbursementState{
				// UserConfirmed=false, FinalConfirmed=false, ReimbursementNo=""
			},
			want: "phase1_collect",
		},
		{
			name: "Phase2_票据已确认",
			rs: ReimbursementState{
				UserConfirmed: true,
				// FinalConfirmed=false, ReimbursementNo=""
			},
			want: "phase2_validate",
		},
		{
			name: "Phase3_用户最终确认",
			rs: ReimbursementState{
				FinalConfirmed: true,
				// ReimbursementNo=""
			},
			want: "phase3_execute",
		},
		{
			name: "Chat_已存在报销单号",
			rs: ReimbursementState{
				ReimbursementNo: "REIMB-001",
			},
			want: "general_chat",
		},
		{
			name: "优先级_报销单号优先于最终确认",
			rs: ReimbursementState{
				ReimbursementNo: "REIMB-001",
				FinalConfirmed:  true,
				UserConfirmed:   true,
			},
			want: "general_chat", // ReimbursementNo 非空优先于所有其他条件
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mgr.selectPhaseAgent(&tt.rs)
			gotName := got.Name(context.Background())
			if gotName != tt.want {
				t.Errorf("selectPhaseAgent() = %q, want %q\n状态: ReimbursementNo=%q FinalConfirmed=%v UserConfirmed=%v",
					gotName, tt.want,
					tt.rs.ReimbursementNo, tt.rs.FinalConfirmed, tt.rs.UserConfirmed)
			}
		})
	}
}

// ============================================
// containsAnyStr 测试（关键词匹配引擎）
// ============================================

func TestContainsAnyStr(t *testing.T) {
	tests := []struct {
		name     string
		s        string
		keywords []string
		want     bool
	}{
		{
			name:     "单关键词命中",
			s:        "我要报销差旅费",
			keywords: []string{"报销"},
			want:     true,
		},
		{
			name:     "多关键词命中第二个",
			s:        "帮我提交报销",
			keywords: []string{"删除", "提交", "取消"},
			want:     true,
		},
		{
			name:     "关键词不匹配",
			s:        "你好世界",
			keywords: []string{"报销", "提交", "发票"},
			want:     false,
		},
		{
			name:     "空关键词列表",
			s:        "任何内容",
			keywords: []string{},
			want:     false,
		},
		{
			name:     "空字符串_不匹配",
			s:        "",
			keywords: []string{"报销"},
			want:     false,
		},
		{
			name:     "关键词比字符串长",
			s:        "报",
			keywords: []string{"报销"},
			want:     false,
		},
		{
			name:     "中文字符精确匹配",
			s:        "批了吗",
			keywords: []string{"批了吗"},
			want:     true,
		},
		{
			name:     "部分匹配不命中",
			s:        "报销",
			keywords: []string{"报销单"},
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := containsAnyStr(tt.s, tt.keywords...)
			if got != tt.want {
				t.Errorf("containsAnyStr(%q, %v) = %v, want %v",
					tt.s, tt.keywords, got, tt.want)
			}
		})
	}
}
