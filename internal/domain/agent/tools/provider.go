// Package tools 智能体工具层，定义 Agent 可调用的全部工具（Eino InvokableTool）
// 每个工具封装一个现有的 infra 或 biz 层能力，通过 Eino 的 jsonschema 反射自动生成参数 Schema
// 按报销三阶段分组：Phase 1（信息收集）、Phase 2（校验确认）、Phase 3（执行提交）
//
// Wire 类型机制：每个工具使用独立的命名类型（OCRTool, ComplianceTool, ...），
// 避免 Wire 无法区分多个 tool.InvokableTool 接口返回值的类型冲突问题
package tools

import (
	"github.com/CycleZero/Reimbee/infra"
	"github.com/CycleZero/Reimbee/log"

	"github.com/cloudwego/eino/components/tool"
	"github.com/google/wire"
)

// ============================================
// 命名工具类型（Wire 依赖注入区分类键）
// ============================================

// OCRTool 票据识别工具类型
type OCRTool struct{ tool.InvokableTool }

// BudgetTool 预算检查工具类型
type BudgetTool struct{ tool.InvokableTool }

// PDFTool PDF 生成工具类型
type PDFTool struct{ tool.InvokableTool }

// EmailTool 邮件发送工具类型
type EmailTool struct{ tool.InvokableTool }

// ProgressTool 进度查询工具类型
type ProgressTool struct{ tool.InvokableTool }

// QueryTool 报销记录查询工具类型
type QueryTool struct{ tool.InvokableTool }

// ProviderSet 工具层的 Wire 依赖注入集合
var ProviderSet = wire.NewSet(
	NewToolSet,
	NewOCRTool,
	NewSearchPolicyTool,
	NewComplianceAgentTool,
	NewBudgetTool,
	NewPDFTool,
	NewEmailTool,
	NewProgressTool,
	NewQueryTool,
	NewCreateReimbTool,
	NewSubmitReimbTool,
)

type ToolSet struct {
	OCR          tool.InvokableTool
	Compliance   tool.BaseTool
	Budget       tool.InvokableTool
	PDF          tool.InvokableTool
	Email        tool.InvokableTool
	Progress     tool.InvokableTool
	QueryRecords tool.InvokableTool
	CreateReimb  tool.InvokableTool
	SubmitReimb  tool.InvokableTool
}

func NewToolSet(
	ocr *OCRTool,
	compliance *ComplianceAgentTool,
	budget *BudgetTool,
	pdf *PDFTool,
	email *EmailTool,
	progress *ProgressTool,
	query *QueryTool,
	createReimb *CreateReimbTool,
	submitReimb *SubmitReimbTool,
	store infra.SessionStore,
	logger *log.Logger,
) *ToolSet {
	logger.Info("智能体工具集初始化完成")
	return &ToolSet{
		OCR:         ocr.InvokableTool,
		Compliance:  compliance.BaseTool,
		Budget:      budget.InvokableTool,
		PDF:         pdf.InvokableTool,
		Email:       email.InvokableTool,
		Progress:    progress.InvokableTool,
		QueryRecords: query.InvokableTool,
		CreateReimb: createReimb.InvokableTool,
		SubmitReimb: submitReimb.InvokableTool,
	}
}


