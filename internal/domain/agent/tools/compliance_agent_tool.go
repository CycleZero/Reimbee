// Package tools 合规审核 AgentTool
//
// 将合规审核逻辑封装为一个 ChatModelAgent（持有 search_policy 工具），
// 通过 adk.NewAgentTool 包装为可被主 Agent 调用的 Tool。
//
// 调用链:
//
//	主 Agent → 调用 check_compliance → ComplianceMiniAgent.ReAct →
//	  search_policy(检索政策) → LLM 审核 → 返回 JSON（pass/warning/error）
package tools

import (
	"context"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"

	"github.com/CycleZero/Reimbee/log"
	"go.uber.org/zap"
)

// ComplianceAgentTool 合规审核 AgentTool（Wire DI 区分类键）
// AgentTool 返回 tool.BaseTool（非 InvokableTool），故嵌入 BaseTool
type ComplianceAgentTool struct{ tool.BaseTool }

// NewComplianceAgentTool 创建合规审核 AgentTool
//
// 依赖:
//   - complianceModel: 审核专用 LLM（可与主 Agent 共用或独立配置）
//   - searchPolicyTool: RAG 政策检索工具
func NewComplianceAgentTool(
	complianceModel model.ToolCallingChatModel,
	searchPolicyTool *SearchPolicyTool,
	logger *log.Logger,
) *ComplianceAgentTool {
	ctx := context.Background()

	complianceAgent, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:        "compliance_reviewer",
		Description: "企业合规审核专家。接收票据信息（金额、类别、日期），调用 search_policy 检索相关政策文档，然后判定合规性并给出处理建议。",
		Instruction: `你是一个企业财务合规审核专家。

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
- 尽可能引用具体政策编号（如 RULE-TRAVEL-001）`,
		Model: complianceModel,
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools: []tool.BaseTool{searchPolicyTool},
			},
		},
		MaxIterations: 5, // search → review → done 通常需 2-3 轮
	})
	if err != nil {
		logger.Error("创建合规审核Agent失败", zap.Error(err))
		panic("创建合规审核Agent失败: " + err.Error())
	}

	agentTool := adk.NewAgentTool(ctx, complianceAgent)
	logger.Info("合规审核AgentTool创建成功")
	return &ComplianceAgentTool{BaseTool: agentTool}
}
