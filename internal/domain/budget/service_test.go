package budget_test

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
	itemRepo := reimbursement.NewItemRepo(data)
	receiptRepo := reimbursement.NewReceiptRepo(data)
	itemBiz := reimbursement.NewItemBiz(logger, itemRepo)
	reimbBiz := reimbursement.NewReimbursementBiz(logger, reimbRepo, itemBiz, receiptRepo, budgetBiz, approvalBiz, empBiz)
	reimbSvc := reimbursement.NewReimbursementService(reimbBiz, approvalBiz, nil, logger)

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
// Dashboard 测试
// ============================================================================

func TestBudgetService_Dashboard_Empty(t *testing.T) {
	engine, _, cleanup := setupEngine(t)
	defer cleanup()

	w := doJSON(http.MethodGet, "/api/budgets/dashboard", nil, engine)
	if w.Code != 200 {
		t.Fatalf("期望 200，实际 %d", w.Code)
	}

	var resp budget.DashboardResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp.Departments) != 0 {
		t.Errorf("期望空部门列表，实际 %d 条", len(resp.Departments))
	}
	if resp.Summary.TotalBudget != 0 {
		t.Errorf("期望总预算 0，实际 %d", resp.Summary.TotalBudget)
	}
}

func TestBudgetService_Dashboard_WithData(t *testing.T) {
	engine, data, cleanup := setupEngine(t)
	defer cleanup()

	d1 := testutil.SeedDepartment(data, "技术部")
	d2 := testutil.SeedDepartment(data, "财务部")
	testutil.SeedBudget(data, d1.ID, 2026, 100000)
	testutil.SeedBudget(data, d2.ID, 2026, 200000)

	w := doJSON(http.MethodGet, "/api/budgets/dashboard", nil, engine)
	if w.Code != 200 {
		t.Fatalf("期望 200，实际 %d", w.Code)
	}

	var resp budget.DashboardResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp.Departments) != 2 {
		t.Errorf("期望 2 个部门，实际 %d", len(resp.Departments))
	}
	if resp.Summary.TotalBudget != 300000 {
		t.Errorf("期望总预算 300000，实际 %d", resp.Summary.TotalBudget)
	}
}

func TestBudgetService_Dashboard_WithYear(t *testing.T) {
	engine, data, cleanup := setupEngine(t)
	defer cleanup()

	d := testutil.SeedDepartment(data, "技术部")
	testutil.SeedBudget(data, d.ID, 2025, 50000)
	testutil.SeedBudget(data, d.ID, 2026, 100000)

	w := doJSON(http.MethodGet, "/api/budgets/dashboard?year=2025", nil, engine)
	if w.Code != 200 {
		t.Fatalf("期望 200，实际 %d", w.Code)
	}

	var resp budget.DashboardResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Summary.TotalBudget != 50000 {
		t.Errorf("期望 2025 年总预算 50000，实际 %d", resp.Summary.TotalBudget)
	}
}

// ============================================================================
// GetByID 测试
// ============================================================================

func TestBudgetService_GetByID_Found(t *testing.T) {
	engine, data, cleanup := setupEngine(t)
	defer cleanup()

	d := testutil.SeedDepartment(data, "技术部")
	b := testutil.SeedBudget(data, d.ID, 2026, 100000)

	w := doJSON(http.MethodGet, fmt.Sprintf("/api/budgets/%d", b.ID), nil, engine)
	if w.Code != 200 {
		t.Fatalf("期望 200，实际 %d", w.Code)
	}

	var resp budget.BudgetResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.AnnualBudget != 100000 {
		t.Errorf("期望 100000，实际 %d", resp.AnnualBudget)
	}
	if resp.Remaining != 100000 {
		t.Errorf("期望剩余 100000，实际 %d", resp.Remaining)
	}
}

func TestBudgetService_GetByID_NotFound(t *testing.T) {
	engine, _, cleanup := setupEngine(t)
	defer cleanup()

	w := doJSON(http.MethodGet, "/api/budgets/99999", nil, engine)
	if w.Code != 404 {
		t.Fatalf("期望 404，实际 %d", w.Code)
	}

	var body map[string]string
	json.Unmarshal(w.Body.Bytes(), &body)
	if body["error"] != "预算记录不存在" {
		t.Errorf("期望 '预算记录不存在'，实际 %q", body["error"])
	}
}

func TestBudgetService_GetByID_BadFormat(t *testing.T) {
	engine, _, cleanup := setupEngine(t)
	defer cleanup()

	w := doJSON(http.MethodGet, "/api/budgets/abc", nil, engine)
	if w.Code != 400 {
		t.Fatalf("期望 400，实际 %d", w.Code)
	}
}

// ============================================================================
// Create 测试
// ============================================================================

func TestBudgetService_Create_Success(t *testing.T) {
	engine, data, cleanup := setupEngine(t)
	defer cleanup()

	d := testutil.SeedDepartment(data, "技术部")
	req := budget.CreateBudgetRequest{
		DepartmentID: d.ID,
		FiscalYear:   2026,
		AnnualBudget: 100000,
	}

	w := doJSON(http.MethodPost, "/api/budgets", req, engine)
	if w.Code != 201 {
		t.Fatalf("期望 201，实际 %d: %s", w.Code, w.Body.String())
	}

	var resp budget.BudgetResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.AnnualBudget != 100000 {
		t.Errorf("期望 100000，实际 %d", resp.AnnualBudget)
	}
}

func TestBudgetService_Create_DuplicateYearDept(t *testing.T) {
	engine, data, cleanup := setupEngine(t)
	defer cleanup()

	d := testutil.SeedDepartment(data, "技术部")
	testutil.SeedBudget(data, d.ID, 2026, 50000)

	req := budget.CreateBudgetRequest{
		DepartmentID: d.ID,
		FiscalYear:   2026,
		AnnualBudget: 100000,
	}
	w := doJSON(http.MethodPost, "/api/budgets", req, engine)
	if w.Code != 409 {
		t.Fatalf("期望 409，实际 %d", w.Code)
	}

	var body map[string]string
	json.Unmarshal(w.Body.Bytes(), &body)
	if body["error"] != "该部门2026年度预算已存在" {
		t.Errorf("期望 '该部门2026年度预算已存在'，实际 %q", body["error"])
	}
}

func TestBudgetService_Create_BadBody(t *testing.T) {
	engine, _, cleanup := setupEngine(t)
	defer cleanup()

	w := doJSON(http.MethodPost, "/api/budgets", map[string]string{}, engine)
	if w.Code != 400 {
		t.Fatalf("期望 400，实际 %d", w.Code)
	}
}

// ============================================================================
// Update 测试
// ============================================================================

func TestBudgetService_Update_Success(t *testing.T) {
	engine, data, cleanup := setupEngine(t)
	defer cleanup()

	d := testutil.SeedDepartment(data, "技术部")
	b := testutil.SeedBudget(data, d.ID, 2026, 50000)

	req := budget.UpdateBudgetRequest{AnnualBudget: 80000}
	w := doJSON(http.MethodPut, fmt.Sprintf("/api/budgets/%d", b.ID), req, engine)
	if w.Code != 200 {
		t.Fatalf("期望 200，实际 %d: %s", w.Code, w.Body.String())
	}

	var resp budget.BudgetResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.AnnualBudget != 80000 {
		t.Errorf("期望 80000，实际 %d", resp.AnnualBudget)
	}
}

func TestBudgetService_Update_NotFound(t *testing.T) {
	engine, _, cleanup := setupEngine(t)
	defer cleanup()

	req := budget.UpdateBudgetRequest{AnnualBudget: 50000}
	w := doJSON(http.MethodPut, "/api/budgets/99999", req, engine)
	if w.Code != 409 {
		t.Fatalf("期望 409，实际 %d", w.Code)
	}
}

func TestBudgetService_Update_BadID(t *testing.T) {
	engine, _, cleanup := setupEngine(t)
	defer cleanup()

	req := budget.UpdateBudgetRequest{AnnualBudget: 50000}
	w := doJSON(http.MethodPut, "/api/budgets/abc", req, engine)
	if w.Code != 400 {
		t.Fatalf("期望 400，实际 %d", w.Code)
	}
}

func TestBudgetService_Update_BadBody(t *testing.T) {
	engine, data, cleanup := setupEngine(t)
	defer cleanup()

	d := testutil.SeedDepartment(data, "技术部")
	b := testutil.SeedBudget(data, d.ID, 2026, 50000)

	w := doJSON(http.MethodPut, fmt.Sprintf("/api/budgets/%d", b.ID), map[string]string{}, engine)
	if w.Code != 400 {
		t.Fatalf("期望 400，实际 %d", w.Code)
	}
}

// ============================================================================
// Dashboard 含 usage 测试
// ============================================================================

func TestBudgetService_Create_DifferentYear(t *testing.T) {
	engine, data, cleanup := setupEngine(t)
	defer cleanup()

	d := testutil.SeedDepartment(data, "技术部")
	req := budget.CreateBudgetRequest{
		DepartmentID: d.ID,
		FiscalYear:   2025,
		AnnualBudget: 80000,
	}
	w := doJSON(http.MethodPost, "/api/budgets", req, engine)
	if w.Code != 201 {
		t.Fatalf("期望 201，实际 %d", w.Code)
	}
}

func TestBudgetService_Dashboard_UsageRate(t *testing.T) {
	engine, data, cleanup := setupEngine(t)
	defer cleanup()

	d := testutil.SeedDepartment(data, "技术部")
	// 直接通过 repo 扣减已用金额以验证 usage rate 计算
	b := testutil.SeedBudget(data, d.ID, 2026, 100000)
	data.DB.Model(b).Update("spent_amount", 30000)

	w := doJSON(http.MethodGet, "/api/budgets/dashboard", nil, engine)
	if w.Code != 200 {
		t.Fatalf("期望 200，实际 %d", w.Code)
	}

	var resp budget.DashboardResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Summary.TotalSpent != 30000 {
		t.Errorf("期望 spent=30000，实际 %d", resp.Summary.TotalSpent)
	}
	if resp.Summary.OverallUsage != 30.0 {
		t.Errorf("期望 usage=30%%，实际 %.2f", resp.Summary.OverallUsage)
	}
}
