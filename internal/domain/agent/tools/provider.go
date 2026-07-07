// Package tools 智能体工具层，定义 Agent 可调用的全部工具（Eino InvokableTool）
// 每个工具封装一个现有的 infra 或 biz 层能力，通过 Eino 的 jsonschema 反射自动生成参数 Schema
// 按报销三阶段分组：Phase 1（信息收集）、Phase 2（校验确认）、Phase 3（执行提交）
//
// Wire 类型机制：每个工具使用独立的命名类型（OCRTool, ComplianceTool, ...），
// 避免 Wire 无法区分多个 tool.InvokableTool 接口返回值的类型冲突问题
package tools

import (
	"github.com/CycleZero/Reimbee/log"

	"github.com/cloudwego/eino/components/tool"
	"github.com/google/wire"
)

// ============================================
// 命名工具类型（Wire 依赖注入区分类键）
// ============================================

// OCRTool 票据识别工具类型——Wire 通过此命名类型区分 OCR 工具与其他 tool.InvokableTool 返回
type OCRTool struct{ tool.InvokableTool }

// ComplianceTool 合规检查工具类型
type ComplianceTool struct{ tool.InvokableTool }

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

// ============================================
// Wire ProviderSet
// ============================================

// ProviderSet 工具层的 Wire 依赖注入集合
var ProviderSet = wire.NewSet(
	NewToolSet,
	NewOCRTool,
	NewComplianceTool,
	NewBudgetTool,
	NewPDFTool,
	NewEmailTool,
	NewProgressTool,
	NewQueryTool,
	NewCreateReimbTool,
	NewSubmitReimbTool,
)

// ============================================
// ToolSet 聚合结构
// ============================================

// ToolSet 聚合所有 Agent 可用工具，按报销三阶段分组提供
type ToolSet struct {
	OCR          tool.InvokableTool
	Compliance   tool.InvokableTool
	Budget       tool.InvokableTool
	PDF          tool.InvokableTool
	Email        tool.InvokableTool
	Progress     tool.InvokableTool
	QueryRecords tool.InvokableTool
	CreateReimb  tool.InvokableTool
	SubmitReimb  tool.InvokableTool
}

// NewToolSet 创建工具集聚合实例
func NewToolSet(
	ocr *OCRTool,
	compliance *ComplianceTool,
	budget *BudgetTool,
	pdf *PDFTool,
	email *EmailTool,
	progress *ProgressTool,
	query *QueryTool,
	createReimb *CreateReimbTool,
	submitReimb *SubmitReimbTool,
	logger *log.Logger,
) *ToolSet {
	logger.Debug("智能体工具集初始化完成")
	return &ToolSet{
		OCR:          ocr.InvokableTool,
		Compliance:   compliance.InvokableTool,
		Budget:       budget.InvokableTool,
		PDF:          pdf.InvokableTool,
		Email:        email.InvokableTool,
		Progress:     progress.InvokableTool,
		QueryRecords: query.InvokableTool,
		CreateReimb:  createReimb.InvokableTool,
		SubmitReimb:  submitReimb.InvokableTool,
	}
}

// GetPhase1Tools 返回 Phase 1（信息收集）阶段可用的工具
// Phase 1 Agent 可调用 OCR 自动识别票据 + 合规工具查询报销标准
func (ts *ToolSet) GetPhase1Tools() []tool.InvokableTool {
	return []tool.InvokableTool{ts.OCR, ts.Compliance}
}

// GetPhase2Tools 返回 Phase 2（校验确认）阶段可用的工具
// Phase 2 Agent 可调用合规检查（校验模式）+ 预算检查
func (ts *ToolSet) GetPhase2Tools() []tool.InvokableTool {
	return []tool.InvokableTool{ts.Compliance, ts.Budget}
}

// GetPhase3Tools 返回 Phase 3（执行提交）阶段可用的工具
// Phase 3 Agent 依次调用：创建报销单 → 提交审批 → 生成 PDF → 发送邮件 → 进度告知
func (ts *ToolSet) GetPhase3Tools() []tool.InvokableTool {
	return []tool.InvokableTool{ts.CreateReimb, ts.SubmitReimb, ts.PDF, ts.Email, ts.Progress}
}

// GetAllTools 返回全部工具（用于通用子流程如进度查询、预算查询）
func (ts *ToolSet) GetAllTools() []tool.InvokableTool {
	return []tool.InvokableTool{
		ts.OCR, ts.Compliance, ts.Budget,
		ts.PDF, ts.Email, ts.Progress, ts.QueryRecords,
		ts.CreateReimb, ts.SubmitReimb,
	}
}

// ============================================
// BaseTool 方法（用于 compose.AddToolsNode 的 ReAct 模式）
// tool.InvokableTool 继承 tool.BaseTool，直接转型即可
// ============================================

// GetPhase1BaseTools 返回 Phase 1 的工具（[]tool.BaseTool）
func (ts *ToolSet) GetPhase1BaseTools() []tool.BaseTool {
	return []tool.BaseTool{ts.OCR, ts.Compliance}
}

// GetPhase2BaseTools 返回 Phase 2 的工具（[]tool.BaseTool）
func (ts *ToolSet) GetPhase2BaseTools() []tool.BaseTool {
	return []tool.BaseTool{ts.Compliance, ts.Budget}
}

// GetPhase3BaseTools 返回 Phase 3 的工具（[]tool.BaseTool）
func (ts *ToolSet) GetPhase3BaseTools() []tool.BaseTool {
	return []tool.BaseTool{ts.CreateReimb, ts.SubmitReimb, ts.PDF, ts.Email, ts.Progress}
}

// GetProgressBaseTools 返回进度查询相关工具
func (ts *ToolSet) GetProgressBaseTools() []tool.BaseTool {
	return []tool.BaseTool{ts.Progress, ts.QueryRecords}
}

// GetBudgetBaseTools 返回预算查询相关工具
func (ts *ToolSet) GetBudgetBaseTools() []tool.BaseTool {
	return []tool.BaseTool{ts.Budget}
}


