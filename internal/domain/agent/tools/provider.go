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
	NewApproveTool,
	NewRejectTool,
	NewPendingTool,
	NewCancelReimbTool,
	NewDeptTool,
	NewReimbDetailTool,
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
	ApproveReimb   *ApproveTool
	RejectReimb    *RejectTool
	PendingList    *PendingTool
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
	approveReimb *ApproveTool,
	rejectReimb *RejectTool,
	pendingList *PendingTool,
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
		ApproveReimb:  approveReimb,
		RejectReimb:   rejectReimb,
		PendingList:   pendingList,
		CancelReimb:   cancelReimb,
		DeptQuery:     dept,
		ReimbDetail:   detail,
		TestInterrupt: testInterrupt,
	}
}
