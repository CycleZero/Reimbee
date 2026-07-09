package tools

import (
	"github.com/CycleZero/Reimbee/log"
	"github.com/google/wire"
)

var ProviderSet = wire.NewSet(
	NewToolSet,
	NewOCRTool,
	NewBudgetTool,
	NewSearchDepartmentTool,
	NewSearchEmployeeTool,
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
	NewOrganizeItemsTool,
)

type ToolSet struct {
	OCR              *OCRTool
	Budget           *BudgetTool
	PDF              *PDFTool
	Email            *EmailTool
	Progress         *ProgressTool
	QueryRecords     *QueryTool
	SearchDepartment *SearchDepartmentTool
	SearchEmployee   *SearchEmployeeTool
	SearchPolicy     *SearchPolicyTool
	Compliance       *ComplianceAgentTool
	CreateReimb      *CreateReimbTool
	SubmitReimb      *SubmitReimbTool
	ApproveReimb     *ApproveTool
	RejectReimb      *RejectTool
	PendingList      *PendingTool
	CancelReimb      *CancelReimbTool
	DeptQuery        *DeptTool
	ReimbDetail      *ReimbDetailTool
	TestInterrupt    *TestInterruptTool
	OrganizeItems    *OrganizeItemsTool
}

func NewToolSet(
	ocr *OCRTool,
	budget *BudgetTool,
	pdf *PDFTool,
	email *EmailTool,
	progress *ProgressTool,
	query *QueryTool,
	searchDepartment *SearchDepartmentTool,
	searchEmployee *SearchEmployeeTool,
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
	organizeItems *OrganizeItemsTool,
	logger *log.Logger,
) *ToolSet {
	logger.Info("智能体工具集初始化完成（Blades，14个工具已启用）")
	return &ToolSet{
		OCR:              ocr,
		Budget:           budget,
		PDF:              pdf,
		Email:            email,
		Progress:         progress,
		QueryRecords:     query,
		SearchDepartment: searchDepartment,
		SearchEmployee:   searchEmployee,
		SearchPolicy:     searchPolicy,
		Compliance:       compliance,
		CreateReimb:      createReimb,
		SubmitReimb:      submitReimb,
		ApproveReimb:     approveReimb,
		RejectReimb:      rejectReimb,
		PendingList:      pendingList,
		CancelReimb:      cancelReimb,
		DeptQuery:        dept,
		ReimbDetail:      detail,
		TestInterrupt:    testInterrupt,
		OrganizeItems:    organizeItems,
	}
}
