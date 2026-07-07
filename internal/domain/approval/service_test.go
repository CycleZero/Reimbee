package approval_test

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
// GetProgress 测试
// ============================================================================

func TestApprovalService_GetProgress_Empty(t *testing.T) {
	engine, _, cleanup := setupEngine(t)
	defer cleanup()

	w := doJSON(http.MethodGet, "/api/reimbursements/1/approvals", nil, engine)
	if w.Code != 200 {
		t.Fatalf("期望 200，实际 %d", w.Code)
	}

	var resp []approval.ApprovalRecordResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp) != 0 {
		t.Errorf("期望空列表，实际 %d 条", len(resp))
	}
}

func TestApprovalService_GetProgress_WithRecords(t *testing.T) {
	engine, data, cleanup := setupEngine(t)
	defer cleanup()

	d := testutil.SeedDepartment(data, "技术部")
	testutil.SeedBudget(data, d.ID, 2026, 500000)
	testutil.SeedEmployee(data, "AP01", "审批人A", d.ID, true)

	rm := testutil.SeedReimbursement(data, "REIMB-TEST-001", "E001", "张三", d.ID, "pending", 50000)
	testutil.SeedApprovalRecord(data, rm.ID, "审批人A", "pending")

	w := doJSON(http.MethodGet, fmt.Sprintf("/api/reimbursements/%d/approvals", rm.ID), nil, engine)
	if w.Code != 200 {
		t.Fatalf("期望 200，实际 %d", w.Code)
	}

	var resp []approval.ApprovalRecordResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp) != 1 {
		t.Errorf("期望 1 条记录，实际 %d", len(resp))
	}
	if resp[0].Action != "pending" {
		t.Errorf("期望 action=pending，实际 %q", resp[0].Action)
	}
}

func TestApprovalService_GetProgress_BadReimbursementID(t *testing.T) {
	engine, _, cleanup := setupEngine(t)
	defer cleanup()

	w := doJSON(http.MethodGet, "/api/reimbursements/abc/approvals", nil, engine)
	if w.Code != 400 {
		t.Fatalf("期望 400，实际 %d", w.Code)
	}
}

// ============================================================================
// Approve 测试
// ============================================================================

func TestApprovalService_Approve_Success(t *testing.T) {
	engine, data, cleanup := setupEngine(t)
	defer cleanup()

	d := testutil.SeedDepartment(data, "技术部")
	testutil.SeedBudget(data, d.ID, 2026, 500000)
	testutil.SeedEmployee(data, "AP01", "审批人A", d.ID, true)

	rm := testutil.SeedReimbursement(data, "REIMB-TEST-002", "E001", "张三", d.ID, "pending", 50000)
	ar := testutil.SeedApprovalRecord(data, rm.ID, "审批人A", "pending")

	w := doJSON(http.MethodPost, fmt.Sprintf("/api/approvals/%d/approve", ar.ID),
		map[string]string{"comment": "同意"}, engine)
	if w.Code != 200 {
		t.Fatalf("期望 200，实际 %d: %s", w.Code, w.Body.String())
	}

	var body map[string]string
	json.Unmarshal(w.Body.Bytes(), &body)
	if body["message"] != "审批已通过" {
		t.Errorf("期望 '审批已通过'，实际 %q", body["message"])
	}
}

func TestApprovalService_Approve_DoubleApprove(t *testing.T) {
	engine, data, cleanup := setupEngine(t)
	defer cleanup()

	d := testutil.SeedDepartment(data, "技术部")
	testutil.SeedBudget(data, d.ID, 2026, 500000)
	testutil.SeedEmployee(data, "AP01", "审批人A", d.ID, true)

	rm := testutil.SeedReimbursement(data, "REIMB-TEST-003", "E001", "张三", d.ID, "pending", 50000)
	ar := testutil.SeedApprovalRecord(data, rm.ID, "审批人A", "approved")

	// 再次审批已通过的记录
	w := doJSON(http.MethodPost, fmt.Sprintf("/api/approvals/%d/approve", ar.ID),
		map[string]string{"comment": "再次同意"}, engine)
	if w.Code != 409 {
		t.Fatalf("期望 409，实际 %d", w.Code)
	}

	var body map[string]string
	json.Unmarshal(w.Body.Bytes(), &body)
	if body["error"] != "该审批已处理（当前状态: approved），不可重复操作" {
		t.Errorf("期望重复操作错误，实际 %q", body["error"])
	}
}

func TestApprovalService_Approve_NotFound(t *testing.T) {
	engine, _, cleanup := setupEngine(t)
	defer cleanup()

	w := doJSON(http.MethodPost, "/api/approvals/99999/approve",
		map[string]string{"comment": "同意"}, engine)
	if w.Code != 409 {
		t.Fatalf("期望 409，实际 %d", w.Code)
	}
}

func TestApprovalService_Approve_BadID(t *testing.T) {
	engine, _, cleanup := setupEngine(t)
	defer cleanup()

	w := doJSON(http.MethodPost, "/api/approvals/abc/approve",
		map[string]string{"comment": "同意"}, engine)
	if w.Code != 400 {
		t.Fatalf("期望 400，实际 %d", w.Code)
	}
}

// ============================================================================
// Reject 测试
// ============================================================================

func TestApprovalService_Reject_Success(t *testing.T) {
	engine, data, cleanup := setupEngine(t)
	defer cleanup()

	d := testutil.SeedDepartment(data, "技术部")
	testutil.SeedBudget(data, d.ID, 2026, 500000)
	testutil.SeedEmployee(data, "AP01", "审批人A", d.ID, true)

	rm := testutil.SeedReimbursement(data, "REIMB-TEST-004", "E001", "张三", d.ID, "pending", 50000)
	ar := testutil.SeedApprovalRecord(data, rm.ID, "审批人A", "pending")

	w := doJSON(http.MethodPost, fmt.Sprintf("/api/approvals/%d/reject", ar.ID),
		map[string]string{"reason": "金额超标"}, engine)
	if w.Code != 200 {
		t.Fatalf("期望 200，实际 %d: %s", w.Code, w.Body.String())
	}

	var body map[string]string
	json.Unmarshal(w.Body.Bytes(), &body)
	if body["message"] != "审批已驳回" {
		t.Errorf("期望 '审批已驳回'，实际 %q", body["message"])
	}
}

func TestApprovalService_Reject_NoReason(t *testing.T) {
	engine, data, cleanup := setupEngine(t)
	defer cleanup()

	d := testutil.SeedDepartment(data, "技术部")
	testutil.SeedBudget(data, d.ID, 2026, 500000)
	testutil.SeedEmployee(data, "AP01", "审批人A", d.ID, true)

	rm := testutil.SeedReimbursement(data, "REIMB-TEST-005", "E001", "张三", d.ID, "pending", 50000)
	ar := testutil.SeedApprovalRecord(data, rm.ID, "审批人A", "pending")

	w := doJSON(http.MethodPost, fmt.Sprintf("/api/approvals/%d/reject", ar.ID),
		map[string]string{}, engine)
	if w.Code != 400 {
		t.Fatalf("期望 400，实际 %d", w.Code)
	}

	var body map[string]string
	json.Unmarshal(w.Body.Bytes(), &body)
	if body["error"] != "驳回原因不能为空" {
		t.Errorf("期望 '驳回原因不能为空'，实际 %q", body["error"])
	}
}

func TestApprovalService_Reject_DoubleReject(t *testing.T) {
	engine, data, cleanup := setupEngine(t)
	defer cleanup()

	d := testutil.SeedDepartment(data, "技术部")
	testutil.SeedBudget(data, d.ID, 2026, 500000)
	testutil.SeedEmployee(data, "AP01", "审批人A", d.ID, true)

	rm := testutil.SeedReimbursement(data, "REIMB-TEST-006", "E001", "张三", d.ID, "pending", 50000)
	ar := testutil.SeedApprovalRecord(data, rm.ID, "审批人A", "rejected")

	// 再次驳回
	w := doJSON(http.MethodPost, fmt.Sprintf("/api/approvals/%d/reject", ar.ID),
		map[string]string{"reason": "再次驳回"}, engine)
	if w.Code != 409 {
		t.Fatalf("期望 409，实际 %d", w.Code)
	}

	var body map[string]string
	json.Unmarshal(w.Body.Bytes(), &body)
	if body["error"] != "该审批已处理（当前状态: rejected），不可重复操作" {
		t.Errorf("期望重复操作错误，实际 %q", body["error"])
	}
}

func TestApprovalService_GetProgress_MultipleRecords(t *testing.T) {
	engine, data, cleanup := setupEngine(t)
	defer cleanup()

	d := testutil.SeedDepartment(data, "技术部")
	rm := testutil.SeedReimbursement(data, "RM-PR01", "E001", "张三", d.ID, "pending", 50000)
	testutil.SeedApprovalRecord(data, rm.ID, "审批人A", "pending")
	testutil.SeedApprovalRecord(data, rm.ID, "审批人B", "approved")

	w := doJSON(http.MethodGet, fmt.Sprintf("/api/reimbursements/%d/approvals", rm.ID), nil, engine)
	if w.Code != 200 {
		t.Fatalf("期望 200，实际 %d", w.Code)
	}

	var resp []approval.ApprovalRecordResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp) != 2 {
		t.Errorf("期望 2 条记录，实际 %d", len(resp))
	}
}

func TestApprovalService_Approve_WithoutComment(t *testing.T) {
	engine, data, cleanup := setupEngine(t)
	defer cleanup()

	d := testutil.SeedDepartment(data, "技术部")
	testutil.SeedBudget(data, d.ID, 2026, 500000)
	testutil.SeedEmployee(data, "AP02", "审批人B", d.ID, true)

	rm := testutil.SeedReimbursement(data, "RM-AC01", "E001", "张三", d.ID, "pending", 30000)
	ar := testutil.SeedApprovalRecord(data, rm.ID, "审批人B", "pending")

	w := doJSON(http.MethodPost, fmt.Sprintf("/api/approvals/%d/approve", ar.ID), nil, engine)
	if w.Code != 200 {
		t.Fatalf("期望 200（审批意见可选），实际 %d: %s", w.Code, w.Body.String())
	}
}

func TestApprovalService_Reject_BadID(t *testing.T) {
	engine, _, cleanup := setupEngine(t)
	defer cleanup()

	w := doJSON(http.MethodPost, "/api/approvals/abc/reject",
		map[string]string{"reason": "格式错误"}, engine)
	if w.Code != 400 {
		t.Fatalf("期望 400，实际 %d", w.Code)
	}
}

func TestApprovalService_GetProgress_NonExistentReimbursement(t *testing.T) {
	engine, _, cleanup := setupEngine(t)
	defer cleanup()

	// 不存在的报销单——返回空列表（200）
	w := doJSON(http.MethodGet, "/api/reimbursements/99999/approvals", nil, engine)
	if w.Code != 200 {
		t.Fatalf("期望 200，实际 %d", w.Code)
	}
}

func TestApprovalService_Reject_NotFound(t *testing.T) {
	engine, _, cleanup := setupEngine(t)
	defer cleanup()

	w := doJSON(http.MethodPost, "/api/approvals/99999/reject",
		map[string]string{"reason": "不存在"}, engine)
	if w.Code != 409 {
		t.Fatalf("期望 409，实际 %d", w.Code)
	}
}
