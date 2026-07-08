package tools

import (
	"github.com/CycleZero/Reimbee/log"
	"github.com/google/wire"
)

var ProviderSet = wire.NewSet(
	NewToolSet,
	NewOCRTool,
	NewBudgetTool,
	NewSearchPolicyTool,
	NewPDFTool,
	NewEmailTool,
	NewProgressTool,
	NewQueryTool,
	NewCreateReimbTool,
	NewTestInterruptTool,
)

type ToolSet struct {
	OCR           *OCRTool
	Budget        *BudgetTool
	PDF           *PDFTool
	Email         *EmailTool
	Progress      *ProgressTool
	QueryRecords  *QueryTool
	SearchPolicy  *SearchPolicyTool
	CreateReimb   *CreateReimbTool
	TestInterrupt *TestInterruptTool
}

func NewToolSet(
	ocr *OCRTool,
	budget *BudgetTool,
	pdf *PDFTool,
	email *EmailTool,
	progress *ProgressTool,
	query *QueryTool,
	searchPolicy *SearchPolicyTool,
	createReimb *CreateReimbTool,
	testInterrupt *TestInterruptTool,
	logger *log.Logger,
) *ToolSet {
	logger.Info("智能体工具集初始化完成（Blades，9个工具已启用）")
	return &ToolSet{
		OCR:           ocr,
		Budget:        budget,
		PDF:           pdf,
		Email:         email,
		Progress:      progress,
		QueryRecords:  query,
		SearchPolicy:  searchPolicy,
		CreateReimb:   createReimb,
		TestInterrupt: testInterrupt,
	}
}
