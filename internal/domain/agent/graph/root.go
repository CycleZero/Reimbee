package graph

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/CycleZero/Reimbee/internal/domain/agent"
	"github.com/CycleZero/Reimbee/log"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"go.uber.org/zap"
)

type intentOutput struct {
	Intent     string            `json:"intent"`
	Entities   map[string]string `json:"entities"`
	Confidence float64           `json:"confidence"`
	Reason     string            `json:"reason"`
}

type RootGraphDeps struct {
	Logger                *log.Logger
	ChatModel             model.ToolCallingChatModel
	ReimbursementRunnable compose.Runnable[*schema.Message, *schema.Message]
	ProgressRunnable      compose.Runnable[*schema.Message, *schema.Message]
	BudgetRunnable        compose.Runnable[*schema.Message, *schema.Message]
	PolicyRunnable        compose.Runnable[*schema.Message, *schema.Message]
	ModifyRunnable        compose.Runnable[*schema.Message, *schema.Message]
}

// NewRootGraph 构建顶层 Root Graph
// 采用简单扁平拓扑：START → intent_router(Lambda) → END
// intent_router 内部完成意图分类 + 子流程调用 + 回复生成
// 不使用 AddBranch（其类型约束与多子流程路由不兼容）
func NewRootGraph(ctx context.Context, deps RootGraphDeps) (compose.Runnable[agent.AgentInput, *schema.Message], error) {
	deps.Logger.Debug("开始构建顶层 Root Graph")

	g := compose.NewGraph[agent.AgentInput, *schema.Message]()

	g.AddLambdaNode("dispatcher", compose.InvokableLambda(func(ctx context.Context, input agent.AgentInput) (*schema.Message, error) {
		return dispatchToWorkflow(ctx, input, deps), nil
	}))

	g.AddEdge(compose.START, "dispatcher")
	g.AddEdge("dispatcher", compose.END)

	runnable, err := g.Compile(ctx,
		compose.WithGraphName("reimbee_root"),
		compose.WithMaxRunSteps(50),
	)
	if err != nil {
		return nil, fmt.Errorf("编译Root Graph失败: %w", err)
	}

	deps.Logger.Info("Root Graph编译成功")
	return runnable, nil
}

// dispatchToWorkflow 意图分类 + 子流程路由 + 回复生成
func dispatchToWorkflow(ctx context.Context, input agent.AgentInput, deps RootGraphDeps) *schema.Message {
	deps.Logger.Debug("开始分发请求",
		zap.String("用户消息", truncate(input.Message, 50)))

	// 意图分类
	route := classifyIntent(ctx, input, deps)
	deps.Logger.Info("意图分类完成",
		zap.String("意图", route),
		zap.String("用户消息", truncate(input.Message, 30)))

	// 路由分发
	msg := schema.UserMessage(input.Message)

	switch route {
	case "new_reimbursement":
		if deps.ReimbursementRunnable != nil {
			deps.Logger.Debug("执行报销子流程")
			resp, err := deps.ReimbursementRunnable.Invoke(ctx, msg)
			if err != nil {
				deps.Logger.Error("报销子流程执行失败", zap.Error(err))
				return schema.AssistantMessage("抱歉，报销流程处理出错了，请稍后重试。", nil)
			}
			return resp
		}

	case "query_progress":
		if deps.ProgressRunnable != nil {
			deps.Logger.Debug("执行进度查询子流程")
			resp, err := deps.ProgressRunnable.Invoke(ctx, msg)
			if err != nil {
				deps.Logger.Error("进度查询执行失败", zap.Error(err))
				return schema.AssistantMessage("抱歉，查询进度出错了。", nil)
			}
			return resp
		}

	case "query_budget":
		if deps.BudgetRunnable != nil {
			deps.Logger.Debug("执行预算查询子流程")
			resp, err := deps.BudgetRunnable.Invoke(ctx, msg)
			if err != nil {
				deps.Logger.Error("预算查询执行失败", zap.Error(err))
				return schema.AssistantMessage("抱歉，查询预算出错了。", nil)
			}
			return resp
		}

	case "policy_question":
		if deps.PolicyRunnable != nil {
			resp, err := deps.PolicyRunnable.Invoke(ctx, msg)
			if err != nil {
				deps.Logger.Error("政策咨询执行失败", zap.Error(err))
			} else {
				return resp
			}
		}

	case "modify_reimbursement":
		if deps.ModifyRunnable != nil {
			resp, err := deps.ModifyRunnable.Invoke(ctx, msg)
			if err != nil {
				deps.Logger.Error("修改报销执行失败", zap.Error(err))
				return schema.AssistantMessage("抱歉，修改报销出错了。", nil)
			}
			return resp
		}
	}

	// 通用对话——ChatModel 直出
	if deps.ChatModel != nil {
		resp, err := deps.ChatModel.Generate(ctx,
			[]*schema.Message{
				schema.SystemMessage(agent.BuildGeneralChatPrompt()),
				schema.UserMessage(input.Message),
			})
		if err != nil {
			deps.Logger.Error("通用对话生成失败", zap.Error(err))
		} else if resp != nil {
			return resp
		}
	}

	return schema.AssistantMessage("您好！我是 Reimbee 报销助手。我可以帮您发起报销、查询进度、查看预算或解答政策问题。", nil)
}

// classifyIntent 意图分类（ChatModel 优先，关键词降级）
func classifyIntent(ctx context.Context, input agent.AgentInput, deps RootGraphDeps) string {
	if deps.ChatModel != nil {
		prompt := agent.BuildIntentClassifyPrompt(input.Message)
		resp, err := deps.ChatModel.Generate(ctx,
			[]*schema.Message{
				schema.SystemMessage("你是一个意图分类器。分析用户输入并返回JSON: {\"intent\":\"...\",\"confidence\":0.95}。可选意图: new_reimbursement, query_progress, query_budget, policy_question, modify_reimbursement, general_chat。"),
				schema.UserMessage(prompt),
			})
		if err == nil && resp != nil {
			var intent intentOutput
			if json.Unmarshal([]byte(resp.Content), &intent) == nil && intent.Confidence >= 0.7 {
				switch intent.Intent {
				case "new_reimbursement", "query_progress", "query_budget",
					"policy_question", "modify_reimbursement":
					return intent.Intent
				}
			}
		}
	}
	return classifyByKeywords(input.Message)
}

func classifyByKeywords(content string) string {
	content = truncate(content, 100)
	if containsAny(content, "报销", "提交", "发票", "申请报销") {
		return "new_reimbursement"
	}
	if containsAny(content, "进度", "到哪", "批了吗", "状态", "审批") {
		return "query_progress"
	}
	if containsAny(content, "预算", "还剩", "余额", "够不够") {
		return "query_budget"
	}
	if containsAny(content, "标准", "规定", "多少", "可以报吗", "政策") {
		return "policy_question"
	}
	if containsAny(content, "改", "修改", "重新提交", "驳回", "被退") {
		return "modify_reimbursement"
	}
	return "general_chat"
}

func containsAny(s string, keywords ...string) bool {
	for _, kw := range keywords {
		if len(s) >= len(kw) {
			for i := 0; i <= len(s)-len(kw); i++ {
				if s[i:i+len(kw)] == kw {
					return true
				}
			}
		}
	}
	return false
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// RouteIntent 导出供测试使用：解析 LLM JSON 输出 → 路由目标
func RouteIntent(msg *schema.Message, logger *log.Logger) string {
	if msg == nil {
		return "general_chat"
	}
	var intent intentOutput
	if err := json.Unmarshal([]byte(msg.Content), &intent); err == nil {
		if intent.Confidence >= 0.7 {
			switch intent.Intent {
			case "new_reimbursement", "query_progress", "query_budget",
				"policy_question", "modify_reimbursement":
				return intent.Intent
			}
		}
	}
	return classifyByKeywords(msg.Content)
}
