package reimbursement_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/CycleZero/Reimbee/infra"
	"github.com/CycleZero/Reimbee/internal/domain"
	"github.com/CycleZero/Reimbee/internal/domain/approval"
	"github.com/CycleZero/Reimbee/internal/domain/budget"
	"github.com/CycleZero/Reimbee/internal/domain/department"
	"github.com/CycleZero/Reimbee/internal/domain/employee"
	"github.com/CycleZero/Reimbee/internal/domain/reimbursement"
	"github.com/CycleZero/Reimbee/internal/router"
	"github.com/CycleZero/Reimbee/internal/router/middleware"
	"github.com/CycleZero/Reimbee/internal/testutil"
	zaplog "github.com/CycleZero/Reimbee/log"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func setupEngine(t *testing.T) (*gin.Engine, *infra.Data, func()) {
	t.Helper()
	data := testutil.NewTestData()
	testutil.CleanDB(data)
	logger := &zaplog.Logger{Logger: zap.NewNop()}

	deptRepo := department.NewDepartmentRepo(data)
	deptBiz := department.NewDepartmentBiz(logger, deptRepo)
	deptSvc := department.NewDepartmentService(deptBiz, logger)

	empRepo := employee.NewEmployeeRepo(data)
	empBiz := employee.NewEmployeeBiz(logger, empRepo)
	empSvc := employee.NewEmployeeService(empBiz, logger)

	budgetRepo := budget.NewBudgetRepo(data)
	budgetBiz := budget.NewBudgetBiz(logger, budgetRepo)
	budgetSvc := budget.NewBudgetService(budgetBiz, logger)

	approvalRepo := approval.NewApprovalRepo(data)
	approvalBiz := approval.NewApprovalBiz(logger, approvalRepo)
	approvalSvc := approval.NewApprovalService(approvalBiz, logger)

	reimbRepo := reimbursement.NewReimbursementRepo(data)
	reimbBiz := reimbursement.NewReimbursementBiz(logger, reimbRepo, budgetBiz, approvalBiz, empBiz)
	reimbSvc := reimbursement.NewReimbursementService(reimbBiz, approvalBiz, logger)

	hub := &domain.ServiceHub{
		DepartmentService:    deptSvc,
		EmployeeService:      empSvc,
		BudgetService:        budgetSvc,
		ApprovalService:      approvalSvc,
		ReimbursementService: reimbSvc,
	}

	middleware.IsMiddleWireRegisterFinished = true
	middleware.AuthMiddleWire = func(optional bool) gin.HandlerFunc {
		return func(c *gin.Context) {
			c.Set("user_id", uint(1))
			c.Set("employee_id", "E001")
			c.Set("role", "admin")
			c.Next()
		}
	}

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	router.RegisterRouter(engine, hub)

	return engine, data, func() { testutil.CleanDB(data) }
}

func doJSON(method, url string, body any, engine *gin.Engine) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	if body != nil {
		json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, url, &buf)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	return w
}

// ============================================================================
// List 测试
// ============================================================================

func TestReimbursementService_List_Empty(t *testing.T) {
	engine, _, cleanup := setupEngine(t)
	defer cleanup()

	w := doJSON(http.MethodGet, "/api/reimbursements", nil, engine)
	if w.Code != 200 {
		t.Fatalf("期望 200，实际 %d", w.Code)
	}

	var resp reimbursement.ListReimbursementResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Total != 0 {
		t.Errorf("空列表 total 应为 0，实际 %d", resp.Total)
	}
}

func TestReimbursementService_List_WithData(t *testing.T) {
	engine, data, cleanup := setupEngine(t)
	defer cleanup()

	d := testutil.SeedDepartment(data, "技术部")
	testutil.SeedReimbursement(data, "RM-001", "E001", "张三", d.ID, "draft", 0)
	testutil.SeedReimbursement(data, "RM-002", "E002", "李四", d.ID, "draft", 0)

	w := doJSON(http.MethodGet, "/api/reimbursements", nil, engine)
	if w.Code != 200 {
		t.Fatalf("期望 200，实际 %d", w.Code)
	}

	var resp reimbursement.ListReimbursementResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Total != 2 {
		t.Errorf("期望 total=2，实际 %d", resp.Total)
	}
}

func TestReimbursementService_List_EmployeeFilter(t *testing.T) {
	engine, data, cleanup := setupEngine(t)
	defer cleanup()

	d := testutil.SeedDepartment(data, "技术部")
	testutil.SeedReimbursement(data, "RM-101", "EE01", "张三", d.ID, "draft", 0)
	testutil.SeedReimbursement(data, "RM-102", "EE02", "李四", d.ID, "draft", 0)

	w := doJSON(http.MethodGet, "/api/reimbursements?employee_id=EE01", nil, engine)
	if w.Code != 200 {
		t.Fatalf("期望 200，实际 %d", w.Code)
	}

	var resp reimbursement.ListReimbursementResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Total != 1 {
		t.Errorf("期望 total=1（过滤 EE01），实际 %d", resp.Total)
	}
}

func TestReimbursementService_List_Pagination(t *testing.T) {
	engine, data, cleanup := setupEngine(t)
	defer cleanup()

	d := testutil.SeedDepartment(data, "技术部")
	for i := 0; i < 15; i++ {
		testutil.SeedReimbursement(data, fmt.Sprintf("RM-P%02d", i), "EE01", "张三", d.ID, "draft", 0)
	}

	w := doJSON(http.MethodGet, "/api/reimbursements?page=1&page_size=5", nil, engine)
	var resp reimbursement.ListReimbursementResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Total != 15 {
		t.Errorf("期望 total=15，实际 %d", resp.Total)
	}
	if len(resp.List) != 5 {
		t.Errorf("第一页应有 5 条，实际 %d 条", len(resp.List))
	}
}

// ============================================================================
// ListPending 测试
// ============================================================================

func TestReimbursementService_ListPending_Empty(t *testing.T) {
	engine, _, cleanup := setupEngine(t)
	defer cleanup()

	w := doJSON(http.MethodGet, "/api/reimbursements/pending", nil, engine)
	if w.Code != 200 {
		t.Fatalf("期望 200，实际 %d", w.Code)
	}

	var resp []reimbursement.ReimbursementResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp) != 0 {
		t.Errorf("期望空列表，实际 %d 条", len(resp))
	}
}

func TestReimbursementService_ListPending_WithData(t *testing.T) {
	engine, data, cleanup := setupEngine(t)
	defer cleanup()

	d := testutil.SeedDepartment(data, "技术部")
	testutil.SeedReimbursement(data, "RM-PD01", "EE01", "张三", d.ID, "pending", 5000)
	testutil.SeedReimbursement(data, "RM-PD02", "EE01", "张三", d.ID, "draft", 0)

	w := doJSON(http.MethodGet, "/api/reimbursements/pending", nil, engine)
	if w.Code != 200 {
		t.Fatalf("期望 200，实际 %d", w.Code)
	}

	var resp []reimbursement.ReimbursementResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp) != 1 {
		t.Errorf("期望 1 条待审批，实际 %d 条", len(resp))
	}
	if resp[0].ReimbursementNo != "RM-PD01" {
		t.Errorf("期望单号 'RM-PD01'，实际 %q", resp[0].ReimbursementNo)
	}
}

// ============================================================================
// GetByID 测试
// ============================================================================

func TestReimbursementService_GetByID_Found(t *testing.T) {
	engine, data, cleanup := setupEngine(t)
	defer cleanup()

	d := testutil.SeedDepartment(data, "技术部")
	rm := testutil.SeedReimbursement(data, "RM-GB01", "EE01", "张三", d.ID, "draft", 0)

	w := doJSON(http.MethodGet, fmt.Sprintf("/api/reimbursements/%d", rm.ID), nil, engine)
	if w.Code != 200 {
		t.Fatalf("期望 200，实际 %d", w.Code)
	}

	var resp reimbursement.ReimbursementResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.ReimbursementNo != "RM-GB01" {
		t.Errorf("期望单号 'RM-GB01'，实际 %q", resp.ReimbursementNo)
	}
}

func TestReimbursementService_GetByID_NotFound(t *testing.T) {
	engine, _, cleanup := setupEngine(t)
	defer cleanup()

	w := doJSON(http.MethodGet, "/api/reimbursements/99999", nil, engine)
	if w.Code != 404 {
		t.Fatalf("期望 404，实际 %d", w.Code)
	}

	var body map[string]string
	json.Unmarshal(w.Body.Bytes(), &body)
	if body["error"] != "报销单不存在" {
		t.Errorf("期望 '报销单不存在'，实际 %q", body["error"])
	}
}

func TestReimbursementService_GetByID_BadFormat(t *testing.T) {
	engine, _, cleanup := setupEngine(t)
	defer cleanup()

	w := doJSON(http.MethodGet, "/api/reimbursements/abc", nil, engine)
	if w.Code != 400 {
		t.Fatalf("期望 400，实际 %d", w.Code)
	}
}

// ============================================================================
// GetByNo 测试
// ============================================================================

func TestReimbursementService_GetByNo_Found(t *testing.T) {
	engine, data, cleanup := setupEngine(t)
	defer cleanup()

	d := testutil.SeedDepartment(data, "技术部")
	testutil.SeedReimbursement(data, "RM-GN01", "EE01", "张三", d.ID, "draft", 0)

	w := doJSON(http.MethodGet, "/api/reimbursements/no/RM-GN01", nil, engine)
	if w.Code != 200 {
		t.Fatalf("期望 200，实际 %d: %s", w.Code, w.Body.String())
	}

	var resp reimbursement.ReimbursementResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.ReimbursementNo != "RM-GN01" {
		t.Errorf("期望单号 'RM-GN01'，实际 %q", resp.ReimbursementNo)
	}
}

func TestReimbursementService_GetByNo_NotFound(t *testing.T) {
	engine, _, cleanup := setupEngine(t)
	defer cleanup()

	w := doJSON(http.MethodGet, "/api/reimbursements/no/NONEXIST", nil, engine)
	if w.Code != 404 {
		t.Fatalf("期望 404，实际 %d", w.Code)
	}
}

// ============================================================================
// Create 测试
// ============================================================================

func TestReimbursementService_Create_Success(t *testing.T) {
	engine, data, cleanup := setupEngine(t)
	defer cleanup()

	d := testutil.SeedDepartment(data, "技术部")
	req := reimbursement.CreateReimbursementRequest{
		EmployeeID:   "EE01",
		EmployeeName: "张三",
		DepartmentID: d.ID,
		SubmitNote:   "差旅费报销",
	}

	w := doJSON(http.MethodPost, "/api/reimbursements", req, engine)
	if w.Code != 201 {
		t.Fatalf("期望 201，实际 %d: %s", w.Code, w.Body.String())
	}

	var resp reimbursement.ReimbursementResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Status != "draft" {
		t.Errorf("期望 draft，实际 %q", resp.Status)
	}
	if resp.ReimbursementNo == "" {
		t.Error("报销单号不应为空")
	}
}

func TestReimbursementService_Create_BadBody(t *testing.T) {
	engine, _, cleanup := setupEngine(t)
	defer cleanup()

	w := doJSON(http.MethodPost, "/api/reimbursements", map[string]string{}, engine)
	if w.Code != 400 {
		t.Fatalf("期望 400，实际 %d", w.Code)
	}
}

// ============================================================================
// Submit 测试
// ============================================================================

func TestReimbursementService_Submit_Success(t *testing.T) {
	engine, data, cleanup := setupEngine(t)
	defer cleanup()

	d := testutil.SeedDepartment(data, "技术部")
	testutil.SeedBudget(data, d.ID, 2026, 500000)
	testutil.SeedEmployee(data, "AP01", "审批人", d.ID, true)

	rm := testutil.SeedReimbursement(data, "RM-S01", "EE01", "张三", d.ID, "draft", 0)

	req := reimbursement.SubmitReimbursementRequest{TotalAmount: 10000}
	w := doJSON(http.MethodPost, fmt.Sprintf("/api/reimbursements/%d/submit", rm.ID), req, engine)
	if w.Code != 200 {
		t.Fatalf("期望 200，实际 %d: %s", w.Code, w.Body.String())
	}

	var resp reimbursement.ReimbursementResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Status != "pending" {
		t.Errorf("期望 pending，实际 %q", resp.Status)
	}
	if resp.TotalAmount != 10000 {
		t.Errorf("期望总金额 10000，实际 %d", resp.TotalAmount)
	}
}

func TestReimbursementService_Submit_WrongStatus(t *testing.T) {
	engine, data, cleanup := setupEngine(t)
	defer cleanup()

	d := testutil.SeedDepartment(data, "技术部")
	testutil.SeedBudget(data, d.ID, 2026, 500000)

	rm := testutil.SeedReimbursement(data, "RM-S02", "EE01", "张三", d.ID, "approved", 5000)

	req := reimbursement.SubmitReimbursementRequest{TotalAmount: 5000}
	w := doJSON(http.MethodPost, fmt.Sprintf("/api/reimbursements/%d/submit", rm.ID), req, engine)
	if w.Code != 409 {
		t.Fatalf("期望 409，实际 %d", w.Code)
	}
}

func TestReimbursementService_Submit_ZeroAmount(t *testing.T) {
	engine, data, cleanup := setupEngine(t)
	defer cleanup()

	d := testutil.SeedDepartment(data, "技术部")
	testutil.SeedBudget(data, d.ID, 2026, 500000)

	rm := testutil.SeedReimbursement(data, "RM-S03", "EE01", "张三", d.ID, "draft", 0)

	// binding:"required" 对 int64 的零值会返回 400，先于 biz 校验
	req := reimbursement.SubmitReimbursementRequest{TotalAmount: 0}
	w := doJSON(http.MethodPost, fmt.Sprintf("/api/reimbursements/%d/submit", rm.ID), req, engine)
	if w.Code != 400 {
		t.Fatalf("期望 400（Gin binding 拒绝零值），实际 %d", w.Code)
	}
}

func TestReimbursementService_Submit_InsufficientBudget(t *testing.T) {
	engine, data, cleanup := setupEngine(t)
	defer cleanup()

	d := testutil.SeedDepartment(data, "技术部")
	testutil.SeedBudget(data, d.ID, 2026, 10000) // 只有 10000 预算
	testutil.SeedEmployee(data, "AP01", "审批人", d.ID, true)

	rm := testutil.SeedReimbursement(data, "RM-S04", "EE01", "张三", d.ID, "draft", 0)

	req := reimbursement.SubmitReimbursementRequest{TotalAmount: 20000} // 超额
	w := doJSON(http.MethodPost, fmt.Sprintf("/api/reimbursements/%d/submit", rm.ID), req, engine)
	if w.Code != 200 {
		t.Fatalf("期望 200（超额触发特殊审批），实际 %d: %s", w.Code, w.Body.String())
	}

	var resp reimbursement.ReimbursementResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if !resp.NeedSpecialApproval {
		t.Error("超额提交应标记 need_special_approval=true")
	}
}

func TestReimbursementService_Submit_BadID(t *testing.T) {
	engine, _, cleanup := setupEngine(t)
	defer cleanup()

	req := reimbursement.SubmitReimbursementRequest{TotalAmount: 1000}
	w := doJSON(http.MethodPost, "/api/reimbursements/abc/submit", req, engine)
	if w.Code != 400 {
		t.Fatalf("期望 400，实际 %d", w.Code)
	}
}

// ============================================================================
// Approve 测试（报销单级别强制审批）
// ============================================================================

func TestReimbursementService_Approve_Success(t *testing.T) {
	engine, data, cleanup := setupEngine(t)
	defer cleanup()

	d := testutil.SeedDepartment(data, "技术部")
	testutil.SeedBudget(data, d.ID, 2026, 500000)
	testutil.SeedEmployee(data, "AP01", "审批人", d.ID, true)

	rm := testutil.SeedReimbursement(data, "RM-A01", "EE01", "张三", d.ID, "pending", 30000)
	testutil.SeedApprovalRecord(data, rm.ID, "审批人", "pending")

	w := doJSON(http.MethodPost, fmt.Sprintf("/api/reimbursements/%d/approve", rm.ID), nil, engine)
	if w.Code != 200 {
		t.Fatalf("期望 200，实际 %d: %s", w.Code, w.Body.String())
	}

	var resp reimbursement.ReimbursementResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Status != "approved" {
		t.Errorf("期望 approved，实际 %q", resp.Status)
	}
}

func TestReimbursementService_Approve_WrongStatus(t *testing.T) {
	engine, data, cleanup := setupEngine(t)
	defer cleanup()

	d := testutil.SeedDepartment(data, "技术部")
	rm := testutil.SeedReimbursement(data, "RM-A02", "EE01", "张三", d.ID, "draft", 0)

	w := doJSON(http.MethodPost, fmt.Sprintf("/api/reimbursements/%d/approve", rm.ID), nil, engine)
	if w.Code != 409 {
		t.Fatalf("期望 409，实际 %d", w.Code)
	}
}

// ============================================================================
// Reject 测试（报销单级别强制驳回）
// ============================================================================

func TestReimbursementService_Reject_Success(t *testing.T) {
	engine, data, cleanup := setupEngine(t)
	defer cleanup()

	d := testutil.SeedDepartment(data, "技术部")
	testutil.SeedBudget(data, d.ID, 2026, 500000)
	testutil.SeedEmployee(data, "AP01", "审批人", d.ID, true)

	rm := testutil.SeedReimbursement(data, "RM-R01", "EE01", "张三", d.ID, "pending", 30000)
	testutil.SeedApprovalRecord(data, rm.ID, "审批人", "pending")

	w := doJSON(http.MethodPost, fmt.Sprintf("/api/reimbursements/%d/reject", rm.ID), nil, engine)
	if w.Code != 200 {
		t.Fatalf("期望 200，实际 %d: %s", w.Code, w.Body.String())
	}

	var resp reimbursement.ReimbursementResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Status != "rejected" {
		t.Errorf("期望 rejected，实际 %q", resp.Status)
	}
}

func TestReimbursementService_Reject_WrongStatus(t *testing.T) {
	engine, data, cleanup := setupEngine(t)
	defer cleanup()

	d := testutil.SeedDepartment(data, "技术部")
	rm := testutil.SeedReimbursement(data, "RM-R02", "EE01", "张三", d.ID, "draft", 0)

	w := doJSON(http.MethodPost, fmt.Sprintf("/api/reimbursements/%d/reject", rm.ID), nil, engine)
	if w.Code != 409 {
		t.Fatalf("期望 409，实际 %d", w.Code)
	}
}
