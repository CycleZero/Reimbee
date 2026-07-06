// Package graph Graph 定义层 —— 构建 Eino compose.Graph 编译为 Runnable
//
// 本文件定义 ReAct 阶段构建器 buildReActPhase，
// 用于构建一个包含 ChatModel → ToolsNode → Branch 循环的标准 ReAct 子图。
// 图类型: [[]*schema.Message → *schema.Message]（与 Eino 官方 ReAct agent 一致）
// 每个报销阶段（Phase 1/2/3）均通过此构建器独立创建

package graph

import (
	"context"
	"fmt"

	"github.com/CycleZero/Reimbee/log"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"go.uber.org/zap"
)

// ============================================
// 阶段配置
// ============================================

// PhaseConfig 单个 ReAct 阶段的配置参数
// 每个阶段有独立的系统提示词和工具集，共享同一个 ChatModel 实例
type PhaseConfig struct {
	// Name 阶段名称，用于日志和 Graph 节点命名
	// 示例: "phase1_collect", "phase2_validate", "phase3_execute"
	Name string

	// SystemPrompt 该阶段的系统提示词，注入到 ChatModel 的 SystemMessage 中
	// 由 agent.BuildSystemPrompt() 生成
	SystemPrompt string

	// Tools 该阶段可用的工具列表
	// Phase1: [OCR, Compliance]  — 信息收集
	// Phase2: [Compliance, Budget] — 校验确认
	// Phase3: [PDF, Email, Progress] — 执行提交
	Tools []tool.BaseTool
}

// ============================================
// 阶段状态（仅本文件内使用）
// ============================================

// phaseState ReAct 循环中的消息历史累积状态
// 通过 compose.WithGenLocalState 注入到图中，
// 每次 ChatModel 产生的 assistant 消息和 ToolsNode 产生的 tool 消息
// 都会通过 StatePreHandler 追加到此状态中
type phaseState struct {
	Messages []*schema.Message // 对话消息历史（含 user/assistant/tool 消息）
}

// ============================================
// buildReActPhase — 核心构建器
// ============================================

// buildReActPhase 构建单个阶段的 ReAct 子图
//
// 图类型: [[]*schema.Message → *schema.Message]
// 这是 Eino 官方 ReAct agent (flow/agent/react/react.go) 使用的标准类型，
// 因为 ChatModel 需要 []*Message 输入，而最终输出为单个 *Message
//
// 内部拓扑:
//
//	START → ChatModelNode ──[Branch: 有 ToolCalls?]──→ ToolsNode
//	                            │                        │
//	                            ├──(无ToolCalls)→ END    └──(always)→ ChatModelNode (loop)
//
// 工具执行循环由 Eino Graph 引擎的 AddBranch 机制自动驱动：
//   - ChatModel 输出后 → StreamGraphBranch 检查输出中是否有 ToolCalls
//   - 有 ToolCalls → 路由到 ToolsNode，工具被执行，结果注入消息历史
//   - ToolsNode 输出后 → 路由回 ChatModelNode，LLM 根据工具结果继续推理
//   - 无 ToolCalls → 路由到 END，阶段结束，返回最终回复
//
// 防死循环: 由父图的 compose.WithMaxRunSteps() 全局限制
//
// 参数:
//   - ctx: 上下文（用于创建 ToolsNode 和 WithTools 调用）
//   - chatModel: ToolCallingChatModel 实例（通过 WithTools 绑定本阶段工具）
//   - logger: 结构化日志记录器
//   - config: 阶段配置
//
// 返回:
//   - *compose.Graph: 已构建但未编译的 ReAct 子图（待父图编译）
//   - error: 构建失败时返回
func buildReActPhase(
	ctx context.Context,
	chatModel model.ToolCallingChatModel,
	logger *log.Logger,
	config PhaseConfig,
) (*compose.Graph[[]*schema.Message, *schema.Message], error) {

	// ── 参数验证 ──
	if chatModel == nil {
		return nil, fmt.Errorf("创建ReAct阶段[%s]失败: chatModel 不能为 nil", config.Name)
	}

	logger.Debug("开始构建ReAct阶段",
		zap.String("阶段名", config.Name),
		zap.Int("工具数量", len(config.Tools)),
	)

	// ── Step 1: 提取 ToolInfo 列表 ──
	var toolInfos []*schema.ToolInfo
	if len(config.Tools) > 0 {
		toolInfos = make([]*schema.ToolInfo, 0, len(config.Tools))
		for i, t := range config.Tools {
			info, err := t.Info(ctx)
			if err != nil {
				return nil, fmt.Errorf("创建ReAct阶段[%s]失败: 获取第%d个工具Info失败: %w", config.Name, i+1, err)
			}
			toolInfos = append(toolInfos, info)
		}
	}

	// ── Step 2: 绑定工具到 ChatModel ──
	// WithTools 返回新实例（不修改原始 chatModel），
	// 确保不同阶段的工具集相互隔离
	var chatModelWithTools model.ToolCallingChatModel
	if len(toolInfos) > 0 {
		var err error
		chatModelWithTools, err = chatModel.WithTools(toolInfos)
		if err != nil {
			return nil, fmt.Errorf("创建ReAct阶段[%s]失败: ChatModel.WithTools失败: %w", config.Name, err)
		}
	} else {
		chatModelWithTools = chatModel
	}

	// ── Step 3: 创建 ToolsNode ──
	// ToolsNode 是 Eino 的标准工具执行器:
	//   输入: *schema.Message（包含 ToolCalls）
	//   输出: []*schema.Message（工具执行结果，每个 ToolCall 一条 ToolMessage）
	var toolsNode *compose.ToolsNode
	if len(config.Tools) > 0 {
		var err error
		toolsNode, err = compose.NewToolNode(ctx, &compose.ToolsNodeConfig{
			Tools: config.Tools,
		})
		if err != nil {
			return nil, fmt.Errorf("创建ReAct阶段[%s]失败: compose.NewToolNode失败: %w", config.Name, err)
		}
	}

	// ── Step 4: 创建 Graph（带消息历史状态） ──
	g := compose.NewGraph[[]*schema.Message, *schema.Message](
		compose.WithGenLocalState(func(ctx context.Context) *phaseState {
			return &phaseState{
				Messages: make([]*schema.Message, 0, 20),
			}
		}),
	)

	// ── Step 5: 添加 ChatModel 节点 ──
	// StatePreHandler: 在每次 ChatModel 被调用前注入系统提示词 + 累积消息历史
	// Graph 类型为 []*Message → *Message，StatePreHandler 的 I 与 ChatModel 输入匹配
	modelPreHandle := func(ctx context.Context, input []*schema.Message, ps *phaseState) ([]*schema.Message, error) {
		// 将新消息追加到历史
		ps.Messages = append(ps.Messages, input...)

		// 构建完整消息列表: 系统提示词 + 历史消息
		msgs := make([]*schema.Message, 0, len(ps.Messages)+1)
		msgs = append(msgs, schema.SystemMessage(config.SystemPrompt))
		msgs = append(msgs, ps.Messages...)

		return msgs, nil
	}

	err := g.AddChatModelNode("chat", chatModelWithTools,
		compose.WithStatePreHandler(modelPreHandle),
		compose.WithNodeName(config.Name+"_chat"),
	)
	if err != nil {
		return nil, fmt.Errorf("创建ReAct阶段[%s]失败: AddChatModelNode失败: %w", config.Name, err)
	}

	// ── Step 6: 添加 ToolsNode + 分支逻辑 ──
	if toolsNode != nil {
		// StatePreHandler: 将 assistant 的 tool_call 消息累积到历史
		toolsPreHandle := func(ctx context.Context, input *schema.Message, ps *phaseState) (*schema.Message, error) {
			if input != nil {
				ps.Messages = append(ps.Messages, input)
			}
			return input, nil
		}

		err = g.AddToolsNode("tools", toolsNode,
			compose.WithStatePreHandler(toolsPreHandle),
			compose.WithNodeName(config.Name+"_tools"),
		)
		if err != nil {
			return nil, fmt.Errorf("创建ReAct阶段[%s]失败: AddToolsNode失败: %w", config.Name, err)
		}

		// Branch 1: ChatModel 输出 → 检查是否有 ToolCalls
		//   - 有 ToolCalls → 路由到 ToolsNode 执行工具
		//   - 无 ToolCalls → 路由到 END（阶段结束，返回最终回复）
		err = g.AddBranch("chat", compose.NewStreamGraphBranch(
			func(ctx context.Context, sr *schema.StreamReader[*schema.Message]) (endNode string, err error) {
				defer sr.Close()
				for {
					msg, recvErr := sr.Recv()
					if recvErr != nil {
						// 流结束（EOF）→ 无 ToolCalls，阶段结束
						return compose.END, nil
					}
					if len(msg.ToolCalls) > 0 {
						// LLM 要求调用工具 → 路由到 ToolsNode
						return "tools", nil
					}
					if msg.Content != "" {
						// LLM 返回文本回复 → 阶段结束
						return compose.END, nil
					}
					// 空 chunk（流式中间帧）→ 继续读取
				}
			},
			map[string]bool{"tools": true, compose.END: true},
		))
		if err != nil {
			return nil, fmt.Errorf("创建ReAct阶段[%s]失败: AddBranch(chat)失败: %w", config.Name, err)
		}

		// Branch 2: ToolsNode 输出 → 循环回 ChatModel
		// 工具执行完成后，结果已通过 StatePreHandler 追加到消息历史，
		// 回到 ChatModel 让 LLM 根据工具结果继续推理
		err = g.AddBranch("tools", compose.NewStreamGraphBranch(
			func(ctx context.Context, sr *schema.StreamReader[[]*schema.Message]) (endNode string, err error) {
				// 关闭流释放资源——我们不需要消费工具输出，
				// StatePreHandler 已在调用 ToolsNode 前完成了状态累积
				sr.Close()
				return "chat", nil
			},
			map[string]bool{"chat": true},
		))
		if err != nil {
			return nil, fmt.Errorf("创建ReAct阶段[%s]失败: AddBranch(tools)失败: %w", config.Name, err)
		}

		// 连线: START → chat
		_ = g.AddEdge(compose.START, "chat")
	} else {
		// ── 无工具场景: ChatModel 直出 ──
		_ = g.AddEdge(compose.START, "chat")
		_ = g.AddEdge("chat", compose.END)
	}

	logger.Info("ReAct阶段构建完成",
		zap.String("阶段名", config.Name),
		zap.Int("工具数量", len(config.Tools)),
	)

	return g, nil
}
