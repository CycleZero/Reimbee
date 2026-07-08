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
	NewComplianceAgentTool,
	NewPDFTool,
	NewEmailTool,
	NewProgressTool,
	NewQueryTool,
	NewCreateReimbTool,
	NewSubmitReimbTool,
	NewDeptTool,
	NewReimbDetailTool,
	NewCancelReimbTool,
	NewTestInterruptTool,
)

type ToolSet struct {
	OCR           *OCRTool
	Budget         *BudgetTool
	PDF            *PDFTool
	Email          *EmailTool
	Progress       *ProgressTool
	QueryRecords   *QueryTool
	SearchPolicy   *SearchPolicyTool
	Compliance     *ComplianceAgentTool
	CreateReimb    *CreateReimbTool
	SubmitReimb    *SubmitReimbTool
	CancelReimb    *CancelReimbTool
	DeptQuery      *DeptTool
	ReimbDetail    *ReimbDetailTool
	TestInterrupt  *TestInterruptTool
}

func NewToolSet(
	ocr *OCRTool,
	budget *BudgetTool,
	pdf *PDFTool,
	email *EmailTool,
	progress *ProgressTool,
	query *QueryTool,
	searchPolicy *SearchPolicyTool,
	compliance *ComplianceAgentTool,
	createReimb *CreateReimbTool,
	submitReimb *SubmitReimbTool,
	cancelReimb *CancelReimbTool,
	dept *DeptTool,
	detail *ReimbDetailTool,
	testInterrupt *TestInterruptTool,
	logger *log.Logger,
) *ToolSet {
	logger.Info("智能体工具集初始化完成（Blades，11个工具已启用）")
	return &ToolSet{
		OCR:           ocr,
		Budget:        budget,
		PDF:           pdf,
		Email:         email,
		Progress:      progress,
		QueryRecords:  query,
		SearchPolicy:  searchPolicy,
		Compliance:    compliance,
		CreateReimb:   createReimb,
		SubmitReimb:   submitReimb,
		CancelReimb:   cancelReimb,
		DeptQuery:     dept,
		ReimbDetail:   detail,
		TestInterrupt: testInterrupt,
	}
}
