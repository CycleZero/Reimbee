package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/CycleZero/Reimbee/internal/domain/compliance"
	"github.com/CycleZero/Reimbee/log"
	"github.com/CycleZero/blades"
	blades_tools "github.com/CycleZero/blades/tools"
	"go.uber.org/zap"
)

// ComplianceAgentTool 合规审核工具
// 内部使用子 Agent（search_policy RAG + 推理判断），外部用 NewFunc 封装，主 LLM 看到的是干净的输入/输出
type ComplianceAgentTool struct{ blades_tools.Tool }

func NewComplianceAgentTool(
	complianceModel blades.ModelProvider,
	searchPolicyTool *SearchPolicyTool,
	logger *log.Logger,
) *ComplianceAgentTool {
	subAgent := newComplianceSubAgent(complianceModel, searchPolicyTool, logger)
	runner := blades.NewRunner(subAgent)

	t, err := blades_tools.NewFunc[compliance.ComplianceInput, complianceOutput](
		ToolCheckCompliance,
		"合规审核票据。检索公司报销政策库，根据费用类别、金额、开票日期判定合规性，返回 pass/warning/error 三级结果。",
		func(ctx context.Context, input compliance.ComplianceInput) (complianceOutput, error) {
			return runComplianceCheck(ctx, runner, input, logger)
		},
	)
	if err != nil {
		panic("创建合规审核工具失败: " + err.Error())
	}
	logger.Info("合规审核工具创建成功（子Agent+Func封装）")
	return &ComplianceAgentTool{t}
}

// complianceOutput 内部输出类型（匹配子 Agent 返回的 JSON）
type complianceOutput struct {
	Result     string `json:"result"`
	Level      string `json:"level"`
	Message    string `json:"message"`
	RuleID     string `json:"rule_id"`
	Standard   string `json:"standard"`
	Suggestion string `json:"suggestion"`
	Reference  string `json:"reference"`
}

// newComplianceSubAgent 创建内部合规审核子 Agent
func newComplianceSubAgent(
	model blades.ModelProvider,
	searchPolicyTool *SearchPolicyTool,
	logger *log.Logger,
) blades.Agent {
	agent, err := blades.NewAgent("compliance_reviewer",
		blades.WithModel(model),
		blades.WithDescription("企业合规审核专家。调用 search_policy 检索政策，根据金额、类别、日期判定合规性。"),
		blades.WithInstruction(complianceReviewerInstruction),
		blades.WithTools(searchPolicyTool),
		blades.WithMaxIterations(8),
	)
	if err != nil {
		logger.Error("创建合规审核子Agent失败", zap.Error(err))
		panic("创建合规审核子Agent失败: " + err.Error())
	}
	return agent
}

// runComplianceCheck 运行子 Agent，支持多明细多票据审核
func runComplianceCheck(
	ctx context.Context,
	runner *blades.Runner,
	input compliance.ComplianceInput,
	logger *log.Logger,
) (complianceOutput, error) {
	// 使用 GetItems() 兼容单张/多张模式
	items := input.GetItems()
	if len(items) == 0 {
		return complianceOutput{Result: "pass", Message: "无待审核票据", RuleID: "no-input"}, nil
	}

	// 构建多票据审核 prompt
	var promptLines []string
	promptLines = append(promptLines, fmt.Sprintf("请审核以下%d条报销明细的票据：", len(items)))
	totalReceipts := 0
	for i, item := range items {
		promptLines = append(promptLines, fmt.Sprintf("\n明细%d: %s, 申请金额 %.2f元", i+1, item.Category, float64(item.Amount)/100.0))
		for j, rct := range item.Receipts {
			promptLines = append(promptLines, fmt.Sprintf("  票据%d: 票面%.2f元, 日期%s",
				j+1, float64(rct.Amount)/100.0, rct.InvoiceDate))
			totalReceipts++
		}
	}
	promptLines = append(promptLines, "\n先调用 search_policy 检索相关报销政策，然后输出 JSON 格式审核结果。")
	prompt := strings.Join(promptLines, "\n")

	logger.Debug("开始合规审核（子Agent）",
		zap.Int("明细数", len(items)),
		zap.Int("票据数", totalReceipts))

	stream := runner.RunStream(ctx, blades.UserMessage(prompt))

	var responseText string
	for msg, err := range stream {
		if err != nil {
			logger.Error("合规审核子Agent执行失败", zap.Error(err))
			return complianceOutput{
				Result:  "error",
				Level:   "error",
				Message: fmt.Sprintf("合规审核执行失败: %v", err),
				RuleID:  "agent-error",
			}, nil
		}
		if msg.Role == blades.RoleAssistant && msg.Status == blades.StatusCompleted {
			responseText = msg.Text()
		}
	}

	if responseText == "" {
		logger.Warn("合规审核子Agent未返回结果")
		return complianceOutput{
			Result:  "pass",
			Level:   "pass",
			Message: "合规审核未返回结果，默认通过",
			RuleID:  "no-response",
		}, nil
	}

	var out complianceOutput
	if err := extractJSON(responseText, &out); err != nil {
		logger.Warn("合规审核子Agent返回非JSON文本",
			zap.String("响应", responseText),
			zap.Error(err))
		return complianceOutput{
			Result:  "pass",
			Level:   "pass",
			Message: responseText,
			RuleID:  "text-response",
		}, nil
	}

	logger.Info("合规审核完成",
		zap.String("结果", out.Result),
		zap.String("规则ID", out.RuleID))

	return out, nil
}

// extractJSON 从可能包含额外文本的响应中提取 JSON
func extractJSON(text string, v interface{}) error {
	text = strings.TrimSpace(text)

	if idx := strings.Index(text, "```json"); idx >= 0 {
		start := idx + 7
		end := strings.Index(text[start:], "```")
		if end >= 0 {
			text = strings.TrimSpace(text[start : start+end])
		}
	}

	if idx := strings.Index(text, "{"); idx >= 0 {
		text = text[idx:]
	}

	return json.Unmarshal([]byte(text), v)
}

const complianceReviewerInstruction = "你是一个企业财务合规审核专家。" +
	"\n\n## 工作流程" +
	"\n1. 调用 search_policy 检索相关报销政策（使用费用类别+金额作为查询词，一次即可）" +
	"\n2. 根据检索到的政策原文，直接判定合规性" +
	"\n3. 以 JSON 格式返回结果（不要额外文字）" +
	"\n\n## 输出格式（严格 JSON，不要输出额外内容）" +
	"\n{\n  \"result\": \"pass|warning|error\"," +
	"\n  \"level\": \"pass|warning|error\"," +
	"\n  \"message\": \"检查结果描述（中文，引用具体政策条款）\"," +
	"\n  \"rule_id\": \"触发的规则ID\"," +
	"\n  \"standard\": \"政策标准值（超标时必填）\"," +
	"\n  \"suggestion\": \"处理建议（超标时必填）\"," +
	"\n  \"reference\": \"引用的政策原文摘要\"\n}" +
	"\n\n## 判定标准" +
	"\n- pass: 票据完全符合所有相关政策标准" +
	"\n- warning: 金额为标准值的80%-100%，或日期距过期不足30天" +
	"\n- error: 金额超过标准值，或日期已过期，或类别不在允许报销范围内" +
	"\n\n## 注意事项" +
	"\n- 金额以\"元\"为单位展示和比较" +
	"\n- 日期格式为 YYYY-MM-DD，有效期通常为开票日期后90天内" +
	"\n- 尽可能引用具体政策编号（如 RULE-TRAVEL-001）"
