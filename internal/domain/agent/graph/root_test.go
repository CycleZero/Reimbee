package graph

import (
	"testing"

	"github.com/CycleZero/Reimbee/log"
	"github.com/cloudwego/eino/schema"
	"go.uber.org/zap"
)

// nopLogger 创建一个静默的 logger 用于测试
func nopLogger() *log.Logger {
	return &log.Logger{Logger: zap.NewNop()}
}

// ============================================
// classifyByKeywords 测试
// ============================================

func TestClassifyByKeywords_NewReimbursement(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"报销发票", "我要报销一张发票", "new_reimbursement"},
		{"提交报销", "提交报销", "new_reimbursement"},
		{"申请发票报销", "申请发票报销", "new_reimbursement"},
		{"报销差旅费", "帮我报销差旅费", "new_reimbursement"},
		{"上传发票", "上传发票", "new_reimbursement"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyByKeywords(tt.input)
			if got != tt.want {
				t.Errorf("classifyByKeywords(%q) = %q，期望 %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestClassifyByKeywords_QueryProgress(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"查询进度", "进度怎么样了", "query_progress"},
		{"批了吗", "批了吗", "query_progress"},
		{"审批状态", "审批状态", "query_progress"},
		{"走到哪了", "审批走到哪了", "query_progress"},
		{"到哪一步了", "到哪一步了", "query_progress"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyByKeywords(tt.input)
			if got != tt.want {
				t.Errorf("classifyByKeywords(%q) = %q，期望 %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestClassifyByKeywords_QueryBudget(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"剩余预算", "还剩多少预算", "query_budget"},
		{"余额是否充足", "余额够不够", "query_budget"},
		{"部门预算", "部门预算还有多少", "query_budget"},
		{"预算查询", "查询预算", "query_budget"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyByKeywords(tt.input)
			if got != tt.want {
				t.Errorf("classifyByKeywords(%q) = %q，期望 %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestClassifyByKeywords_PolicyQuestion(t *testing.T) {
	// 注意：包含"报销"关键词的输入会优先匹配 new_reimbursement，
	// 因为 classifyByKeywords 按优先级顺序依次检查关键词
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"住宿标准", "住宿标准是多少", "policy_question"},
		{"什么规定", "公司什么规定", "policy_question"},
		{"公司政策", "公司政策咨询", "policy_question"},
		{"最多报多少", "出差最多报多少", "policy_question"},
		{"可以报吗", "这项可以报吗", "policy_question"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyByKeywords(tt.input)
			if got != tt.want {
				t.Errorf("classifyByKeywords(%q) = %q，期望 %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestClassifyByKeywords_ModifyReimbursement(t *testing.T) {
	// 注意：包含"报销""提交""发票"等关键词会优先匹配 new_reimbursement
	// 只有不含前序关键词但包含"改""驳回"等的才会匹配 modify_reimbursement
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"被退回修改", "被退回修改了", "modify_reimbursement"},
		{"系统驳回", "系统驳回了", "modify_reimbursement"},
		{"改一下金额", "改一下金额", "modify_reimbursement"},
		{"单字驳回", "驳回", "modify_reimbursement"},          // 不含前序关键词，含"驳回"
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyByKeywords(tt.input)
			if got != tt.want {
				t.Errorf("classifyByKeywords(%q) = %q，期望 %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestClassifyByKeywords_KeywordOrder 验证关键词优先级：
// 当输入同时包含多个类别的关键词时，优先匹配先检查的类别
func TestClassifyByKeywords_KeywordOrder(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string // 实际行为（按优先级）
	}{
		{"报销关键词优先于修改", "修改报销单", "new_reimbursement"}, // "报销"先于"修改"
		{"报销关键词优先于政策", "可以报销吗", "new_reimbursement"},   // "报销"先于"可以报吗"
		{"报销关键词优先于进度", "报销状态", "new_reimbursement"},     // "报销"先于"状态"
		{"提交关键词优先于修改", "驳回的重新提交", "new_reimbursement"}, // "提交"先于"驳回"
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyByKeywords(tt.input)
			if got != tt.want {
				t.Errorf("classifyByKeywords(%q) = %q，期望 %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestClassifyByKeywords_GeneralChat(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"问候", "你好", "general_chat"},
		{"感谢", "谢谢", "general_chat"},
		{"无关内容", "火星探险", "general_chat"},
		{"自我介绍", "你是谁", "general_chat"},
		{"今天天气", "今天天气怎么样", "general_chat"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyByKeywords(tt.input)
			if got != tt.want {
				t.Errorf("classifyByKeywords(%q) = %q，期望 %q", tt.input, got, tt.want)
			}
		})
	}
}

// ============================================
// containsAny 测试
// ============================================

func TestContainsAny_Match(t *testing.T) {
	tests := []struct {
		name     string
		s        string
		keywords []string
		want     bool
	}{
		{"匹配单个关键词", "我要报销", []string{"报销"}, true},
		{"匹配多个关键词之一", "hello world", []string{"报销", "world"}, true},
		{"不匹配任何关键词", "查询进度", []string{"报销"}, false},
		{"精确匹配", "abc", []string{"abc"}, true},
		{"子串匹配", "abcde", []string{"bcd"}, true},
		{"空字符串", "", []string{"报销"}, false},
		{"空关键词列表", "测试", []string{}, false},
		{"单字符匹配", "a", []string{"a"}, true},
		{"单字符不匹配", "a", []string{"b"}, false},
		{"关键词比字符串长", "a", []string{"ab"}, false},
		{"中文关键词匹配", "提交报销申请", []string{"申请"}, true},
		{"中文关键词不匹配", "你好", []string{"报销"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := containsAny(tt.s, tt.keywords...)
			if got != tt.want {
				t.Errorf("containsAny(%q, %v) = %v，期望 %v", tt.s, tt.keywords, got, tt.want)
			}
		})
	}
}

func TestContainsAny_Unicode(t *testing.T) {
	// 中文精确匹配测试
	if !containsAny("你好世界", "世界") {
		t.Errorf("containsAny(%q, %q) = false，期望 true", "你好世界", "世界")
	}
	// 中文偏移测试（关键：验证不是按字节而是按 UTF-8 字符匹配）
	if !containsAny("报销发票", "发票") {
		t.Errorf("containsAny(%q, %q) = false，期望 true（UTF-8 偏移测试）", "报销发票", "发票")
	}
}

// ============================================
// truncate 测试
// ============================================

func TestTruncate(t *testing.T) {
	tests := []struct {
		name   string
		s      string
		maxLen int
		want   string
	}{
		{"短字符串不变", "hello", 100, "hello"},
		{"长字符串截断", "hello world this is a long message", 5, "hello..."},
		{"精确长度不变", "hello", 5, "hello"},
		{"零长度截断", "hello", 0, "..."},
		{"空字符串", "", 10, ""},
		{"中文截断按字符", "你好世界测试数据", 3, "你好世..."},          // truncate 改用 rune-safe，按字符截断
		{"中文短于上限不变", "你好世界", 12, "你好世界"},                    // 4个中文字符，maxLen=12 不截断
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncate(tt.s, tt.maxLen)
			if got != tt.want {
				t.Errorf("truncate(%q, %d) = %q，期望 %q", tt.s, tt.maxLen, got, tt.want)
			}
		})
	}
}

// ============================================
// RouteIntent 测试
// ============================================

func TestRouteIntent_NilMessage(t *testing.T) {
	logger := nopLogger()
	got := RouteIntent(nil, logger)
	if got != "general_chat" {
		t.Errorf("RouteIntent(nil) = %q，期望 general_chat（nil 消息应路由到通用对话）", got)
	}
}

func TestRouteIntent_ValidJSON_HighConfidence(t *testing.T) {
	logger := nopLogger()
	msg := &schema.Message{
		Content: `{"intent":"new_reimbursement","entities":{},"confidence":0.95,"reason":"用户明确发起了报销请求"}`,
	}
	got := RouteIntent(msg, logger)
	if got != "new_reimbursement" {
		t.Errorf("RouteIntent(高置信度JSON) = %q，期望 new_reimbursement", got)
	}
}

func TestRouteIntent_ValidJSON_LowConfidence(t *testing.T) {
	logger := nopLogger()
	msg := &schema.Message{
		Content: `{"intent":"new_reimbursement","entities":{},"confidence":0.5,"reason":"置信度较低"}`,
	}
	got := RouteIntent(msg, logger)
	if got != "general_chat" {
		t.Errorf("RouteIntent(低置信度JSON, 0.5) = %q，期望 general_chat", got)
	}
}

func TestRouteIntent_ValidJSON_EdgeConfidence(t *testing.T) {
	logger := nopLogger()
	// 边界：confidence == 0.7 刚好等于阈值，应该通过
	msg := &schema.Message{
		Content: `{"intent":"query_progress","confidence":0.7,"reason":"用户查询进度"}`,
	}
	got := RouteIntent(msg, logger)
	if got != "query_progress" {
		t.Errorf("RouteIntent(置信度=0.7) = %q，期望 query_progress", got)
	}
}

func TestRouteIntent_ValidJSON_AllIntents(t *testing.T) {
	logger := nopLogger()
	tests := []struct {
		intent string
		route  string
	}{
		{"new_reimbursement", "new_reimbursement"},
		{"query_progress", "query_progress"},
		{"query_budget", "query_budget"},
		{"policy_question", "policy_question"},
		{"modify_reimbursement", "modify_reimbursement"},
		{"unknown_intent", "general_chat"}, // 未知意图
	}
	for _, tt := range tests {
		t.Run(tt.intent, func(t *testing.T) {
			msg := &schema.Message{
				Content: `{"intent":"` + tt.intent + `","entities":{},"confidence":0.95,"reason":"test"}`,
			}
			got := RouteIntent(msg, logger)
			if got != tt.route {
				t.Errorf("RouteIntent(intent=%q) = %q，期望 %q", tt.intent, got, tt.route)
			}
		})
	}
}

func TestRouteIntent_InvalidJSON_FallbackToKeywords(t *testing.T) {
	logger := nopLogger()
	// JSON 解析失败时，降级为关键词匹配
	msg := &schema.Message{
		Content: "我要报销一张发票",
	}
	got := RouteIntent(msg, logger)
	if got != "new_reimbursement" {
		t.Errorf("RouteIntent(非JSON消息) = %q，期望 new_reimbursement（降级关键词匹配）", got)
	}
}

func TestRouteIntent_EmptyContent(t *testing.T) {
	logger := nopLogger()
	msg := &schema.Message{
		Content: "",
	}
	got := RouteIntent(msg, logger)
	if got != "general_chat" {
		t.Errorf("RouteIntent(空内容) = %q，期望 general_chat", got)
	}
}
