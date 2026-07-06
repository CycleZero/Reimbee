// Package graph Graph 定义层——构建 Eino compose.Graph 编译为 Runnable
// 顶层 Root Graph 负责意图分类 → 工作流路由 → 子流程执行
// 每个子流程是独立的编译单元，通过 AddGraphNode 嵌套到 Root Graph 中
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

// intentOutput LLM 意图分类节点的输出 JSON 结构
type intentOutput struct {
	Intent     string            `json:"intent"`
	Entities   map[string]string `json:"entities"`
	Confidence float64           `json:"confidence"`
	Reason     string            `json:"reason"`
}

// ============================================
// Root Graph 构建
// ============================================

// RootGraphDeps Root Graph 构建所需的依赖
type RootGraphDeps struct {
	Logger                  *log.Logger
	ChatModel               model.ToolCallingChatModel                                       // 共享 ChatModel 实例，用于意图分类 + 通用对话
	ReimbursementRunnable   compose.Runnable[agent.ReimbursementState, *schema.Message]       // 报销子流程（已编译）
	ProgressGraph           *compose.Graph[agent.AgentInput, *schema.Message]                 // 进度查询子流程（未编译，由 Root Graph 统一编译）
	BudgetGraph             *compose.Graph[agent.AgentInput, *schema.Message]                 // 预算查询子流程（未编译）
	PolicyGraph             *compose.Graph[agent.AgentInput, *schema.Message]                 // 政策咨询子流程（未编译）
	ModifyGraph             *compose.Graph[agent.AgentInput, *schema.Message]                 // 修改报销子流程（未编译）
}

// NewRootGraph 构建顶层 Root Graph
// 拓扑：START → IntentClassifier(ChatModel) → IntentRouter(Lambda) → [子流程] → END
// 当 ChatModel 为 nil 时，跳过 IntentClassifier，直接使用关键词降级匹配
func NewRootGraph(ctx context.Context, deps RootGraphDeps) (compose.Runnable[agent.AgentInput, *schema.Message], error) {
	deps.Logger.Debug("开始构建顶层 Root Graph")

	g := compose.NewGraph[agent.AgentInput, *schema.Message]()

	// 意图分类节点：ChatModel（如有）或关键词降级
	if deps.ChatModel != nil {
		// ChatModel 模式：LLM 分析用户意图
		// 先通过 Lambda 构建 Prompt（AgentInput → []*schema.Message），再送入 ChatModel
		g.AddLambdaNode("build_intent_prompt", compose.InvokableLambda(func(ctx context.Context, input agent.AgentInput) ([]*schema.Message, error) {
			prompt := agent.BuildIntentClassifyPrompt(input.Message)
			return []*schema.Message{
				schema.SystemMessage("你是一个意图分类器。请分析用户输入并返回JSON格式的意图分类结果。"),
				schema.UserMessage(prompt),
			}, nil
		}))
		g.AddChatModelNode("intent_classifier", deps.ChatModel)
		g.AddEdge(compose.START, "build_intent_prompt")
		g.AddEdge("build_intent_prompt", "intent_classifier")

		// 意图路由 Lambda——解析 LLM JSON 输出
		g.AddLambdaNode("intent_router", compose.InvokableLambda(func(ctx context.Context, msg *schema.Message) (string, error) {
			return routeIntent(ctx, msg, deps.Logger), nil
		}))
		g.AddEdge("intent_classifier", "intent_router")
	} else {
		// 降级模式：直接关键词路由
		deps.Logger.Debug("ChatModel未配置，使用关键词匹配降级")
		g.AddLambdaNode("intent_router", compose.InvokableLambda(func(ctx context.Context, input agent.AgentInput) (string, error) {
			return classifyByKeywords(input.Message), nil
		}))
		g.AddEdge(compose.START, "intent_router")
	}

	// 条件分支：根据路由结果跳转到对应子流程
	g.AddBranch("intent_router", compose.NewGraphBranch(
		func(ctx context.Context, route string) (string, error) {
			return route, nil
		},
		map[string]bool{
			"new_reimbursement":    true,
			"query_progress":       true,
			"query_budget":         true,
			"policy_question":      true,
			"modify_reimbursement": true,
			"general_chat":         true,
		},
	))

	// 通用对话节点——ChatModel 或 Lambda 降级
	if deps.ChatModel != nil {
		g.AddLambdaNode("build_chat_prompt", compose.InvokableLambda(func(ctx context.Context, input agent.AgentInput) ([]*schema.Message, error) {
			prompt := agent.BuildGeneralChatPrompt()
			return []*schema.Message{
				schema.SystemMessage(prompt),
				schema.UserMessage(input.Message),
			}, nil
		}))
		g.AddChatModelNode("general_chat", deps.ChatModel)
		g.AddEdge("build_chat_prompt", "general_chat")
	} else {
		deps.Logger.Debug("通用对话降级为模板回复")
		g.AddLambdaNode("general_chat", compose.InvokableLambda(func(ctx context.Context, input agent.AgentInput) (*schema.Message, error) {
			return schema.AssistantMessage("您好！我是 Reimbee 报销助手。我可以帮您发起报销、查询进度、查看预算或解答政策问题。", nil), nil
		}))
	}

	g.AddEdge("general_chat", compose.END)

	// ============================================
	// 子流程 Graph 嵌套
	// 各子流程作为独立编译单元通过 AddGraphNode 嵌入 Root Graph
	// 当 ChatModel 为 nil 时，子流程内部自动降级为模板回复
	// ============================================

	if deps.ProgressGraph != nil {
		deps.Logger.Debug("挂载进度查询子流程")
		if err := g.AddGraphNode("query_progress", deps.ProgressGraph); err != nil {
			deps.Logger.Error("挂载进度查询子流程失败", zap.Error(err))
			return nil, fmt.Errorf("挂载进度查询子流程失败: %w", err)
		}
		g.AddEdge("query_progress", compose.END)
	} else {
		deps.Logger.Debug("进度查询子流程未提供，跳过挂载")
	}

	if deps.BudgetGraph != nil {
		deps.Logger.Debug("挂载预算查询子流程")
		if err := g.AddGraphNode("query_budget", deps.BudgetGraph); err != nil {
			deps.Logger.Error("挂载预算查询子流程失败", zap.Error(err))
			return nil, fmt.Errorf("挂载预算查询子流程失败: %w", err)
		}
		g.AddEdge("query_budget", compose.END)
	} else {
		deps.Logger.Debug("预算查询子流程未提供，跳过挂载")
	}

	if deps.PolicyGraph != nil {
		deps.Logger.Debug("挂载政策咨询子流程")
		if err := g.AddGraphNode("policy_question", deps.PolicyGraph); err != nil {
			deps.Logger.Error("挂载政策咨询子流程失败", zap.Error(err))
			return nil, fmt.Errorf("挂载政策咨询子流程失败: %w", err)
		}
		g.AddEdge("policy_question", compose.END)
	} else {
		deps.Logger.Debug("政策咨询子流程未提供，跳过挂载")
	}

	if deps.ModifyGraph != nil {
		deps.Logger.Debug("挂载修改报销子流程")
		if err := g.AddGraphNode("modify_reimbursement", deps.ModifyGraph); err != nil {
			deps.Logger.Error("挂载修改报销子流程失败", zap.Error(err))
			return nil, fmt.Errorf("挂载修改报销子流程失败: %w", err)
		}
		g.AddEdge("modify_reimbursement", compose.END)
	} else {
		deps.Logger.Debug("修改报销子流程未提供，跳过挂载")
	}

	// 编译
	runnable, err := g.Compile(ctx,
		compose.WithGraphName("reimbee_root"),
		compose.WithMaxRunSteps(50),
	)
	if err != nil {
		deps.Logger.Error("编译Root Graph失败", zap.Error(err))
		return nil, fmt.Errorf("编译Root Graph失败: %w", err)
	}

	deps.Logger.Info("Root Graph编译成功")
	return runnable, nil
}

// routeIntent 解析 LLM 意图分类输出，返回路由目标
// 简化版：基于关键词匹配（Phase D 将替换为 ChatModel 节点）
func routeIntent(ctx context.Context, msg *schema.Message, logger *log.Logger) string {
	if msg == nil {
		logger.Debug("消息为空，路由到通用对话")
		return "general_chat"
	}

	content := msg.Content

	// 尝试解析为 JSON 意图分类输出
	var intent intentOutput
	if err := json.Unmarshal([]byte(content), &intent); err == nil {
		logger.Debug("意图分类结果", zap.String("意图", intent.Intent), zap.Float64("置信度", intent.Confidence))

		if intent.Confidence < 0.7 {
			logger.Debug("意图置信度低于阈值，路由到通用对话")
			return "general_chat"
		}

		switch intent.Intent {
		case "new_reimbursement":
			return "new_reimbursement"
		case "query_progress":
			return "query_progress"
		case "query_budget":
			return "query_budget"
		case "policy_question":
			return "policy_question"
		case "modify_reimbursement":
			return "modify_reimbursement"
		default:
			return "general_chat"
		}
	}

	// JSON 解析失败——降级为关键词匹配
	logger.Debug("意图分类JSON解析失败，降级为关键词匹配", zap.String("内容", truncate(content, 50)))
	return classifyByKeywords(content)
}

// classifyByKeywords 基于关键词的意图分类（降级方案）
func classifyByKeywords(content string) string {
	content = truncate(content, 100)
	
	// 报销发起关键词
	if containsAny(content, "报销", "提交", "发票", "申请报销") {
		return "new_reimbursement"
	}
	// 进度查询关键词
	if containsAny(content, "进度", "到哪", "批了吗", "状态", "审批") {
		return "query_progress"
	}
	// 预算查询关键词
	if containsAny(content, "预算", "还剩", "余额", "够不够") {
		return "query_budget"
	}
	// 政策咨询关键词
	if containsAny(content, "标准", "规定", "多少", "可以报吗", "政策") {
		return "policy_question"
	}
	// 修改报销关键词
	if containsAny(content, "改", "修改", "重新提交", "驳回", "被退") {
		return "modify_reimbursement"
	}

	return "general_chat"
}

// ============================================
// 辅助函数
// ============================================

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
