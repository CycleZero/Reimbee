// Package tools 智能体工具层，定义 Agent 可调用的全部工具（Blades tools.Tool）
// 每个工具封装一个现有的 infra 或 biz 层能力，通过 Blades tools.NewFunc 自动生成参数 Schema
// 按报销三阶段分组：Phase 1（信息收集）、Phase 2（校验确认）、Phase 3（执行提交）
//
// Wire 类型机制：每个工具使用独立的命名类型（OCRTool, ComplianceTool, ...），
// 避免 Wire 无法区分多个 tools.Tool 接口返回值的类型冲突问题
package tools

import (
	"github.com/CycleZero/Reimbee/log"
	"github.com/google/wire"
)

// ============================================
// Wire ProviderSet
// ============================================

// ProviderSet 工具层的 Wire 依赖注入集合
// TODO: 待 state 确认后添加 NewOCRTool, NewBudgetTool, NewComplianceAgentTool, NewSubmitReimbTool
var ProviderSet = wire.NewSet(
	NewToolSet,
	NewSearchPolicyTool,
	NewPDFTool,
	NewEmailTool,
	NewProgressTool,
	NewQueryTool,
	NewCreateReimbTool,
	NewTestInterruptTool,
)

// ============================================
// ToolSet 工具集
// 每个工具存储为命名指针类型（Wire 需要类型区分），外部使用 .Tool 字段获取 tools.Tool 接口
// ============================================

// ToolSet 聚合 Agent 可调用的全部工具实例（Wire 命名类型方式）
type ToolSet struct {
	PDF          *PDFTool
	Email        *EmailTool
	Progress     *ProgressTool
	QueryRecords *QueryTool
	SearchPolicy *SearchPolicyTool
	CreateReimb  *CreateReimbTool
	TestInterrupt *TestInterruptTool
}

// NewToolSet 创建工具集，聚合所有已启用的工具实例
func NewToolSet(
	pdf *PDFTool,
	email *EmailTool,
	progress *ProgressTool,
	query *QueryTool,
	searchPolicy *SearchPolicyTool,
	createReimb *CreateReimbTool,
	testInterrupt *TestInterruptTool,
	logger *log.Logger,
) *ToolSet {
	logger.Info("智能体工具集初始化完成（Blades，7个工具已启用）")
	return &ToolSet{
		PDF:          pdf,
		Email:        email,
		Progress:     progress,
		QueryRecords: query,
		SearchPolicy: searchPolicy,
		CreateReimb:  createReimb,
		TestInterrupt: testInterrupt,
	}
}
