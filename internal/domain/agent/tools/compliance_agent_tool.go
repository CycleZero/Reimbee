package tools

import (
	"github.com/CycleZero/Reimbee/log"
	"github.com/CycleZero/blades"
	blades_tools "github.com/CycleZero/blades/tools"
	"go.uber.org/zap"
)

// ComplianceAgentTool 合规审核 AgentTool
// 用 blades.NewAgent 创建子 Agent（持有 search_policy），通过 NewAgentTool 包装为 Tool
type ComplianceAgentTool struct{ blades_tools.Tool }

func NewComplianceAgentTool(
	complianceModel blades.ModelProvider,
	searchPolicyTool *SearchPolicyTool,
	logger *log.Logger,
) *ComplianceAgentTool {

	complianceAgent, err := blades.NewAgent("compliance_reviewer",
		blades.WithModel(complianceModel),
		blades.WithDescription("企业合规审核专家。接收票据信息，调用 search_policy 检索政策，判定合规性。"),
		blades.WithInstruction(complianceInstruction),
		blades.WithTools(searchPolicyTool),
		blades.WithMaxIterations(5),
	)
	if err != nil {
		logger.Error("创建合规审核Agent失败", zap.Error(err))
		panic("创建合规审核Agent失败: " + err.Error())
	}

	agentTool := blades.NewAgentTool(complianceAgent)
	logger.Info("合规审核AgentTool创建成功")
	return &ComplianceAgentTool{agentTool}
}

const complianceInstruction = `你是一个企业财务合规审核专家。

## 工作流程
1. 收到票据信息后，先调用 search_policy 检索相关报销政策
2. 根据检索到的政策原文，审核票据合规性
3. 如果一次检索未找到完整规则，换用不同关键词再次检索
4. 审核完成后，以 JSON 格式返回结果

## 输出格式（严格 JSON，不要输出额外内容）
{
  "result": "pass|warning|error",
  "message": "检查结果描述（中文，引用具体政策条款）",
  "rule_id": "触发的规则ID",
  "standard": "政策标准值（如 500元/晚，超标时必填）",
  "suggestion": "处理建议（超标时必填）",
  "reference": "引用的政策原文摘要"
}

## 判定标准
- pass: 票据完全符合所有相关政策标准
- warning: 金额为标准值的80%-100%，或日期距过期不足30天
- error: 金额超过标准值，或日期已过期，或类别不在允许报销范围内

## 注意事项
- 金额以"元"为单位展示和比较
- 日期格式为 YYYY-MM-DD，有效期通常为开票日期后90天内
- 尽可能引用具体政策编号（如 RULE-TRAVEL-001）`
