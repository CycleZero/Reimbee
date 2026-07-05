package employee_test

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
		return func(c *gin.Context) { c.Next() }
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

func TestEmployeeService_List_Empty(t *testing.T) {
	engine, _, cleanup := setupEngine(t)
	defer cleanup()

	w := doJSON(http.MethodGet, "/api/employees", nil, engine)
	if w.Code != 200 {
		t.Fatalf("期望 200，实际 %d", w.Code)
	}

	var resp employee.ListEmployeeResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Total != 0 {
		t.Errorf("空列表 total 应为 0，实际 %d", resp.Total)
	}
}

func TestEmployeeService_List_WithData(t *testing.T) {
	engine, data, cleanup := setupEngine(t)
	defer cleanup()

	d := testutil.SeedDepartment(data, "技术部")
	testutil.SeedEmployee(data, "E001", "张三", d.ID, false)
	testutil.SeedEmployee(data, "E002", "李四", d.ID, true)

	w := doJSON(http.MethodGet, "/api/employees", nil, engine)
	if w.Code != 200 {
		t.Fatalf("期望 200，实际 %d", w.Code)
	}

	var resp employee.ListEmployeeResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Total != 2 {
		t.Errorf("期望 total=2，实际 %d", resp.Total)
	}
}

func TestEmployeeService_List_Pagination(t *testing.T) {
	engine, data, cleanup := setupEngine(t)
	defer cleanup()

	d := testutil.SeedDepartment(data, "研发部")
	for i := 0; i < 10; i++ {
		testutil.SeedEmployee(data, fmt.Sprintf("EP%02d", i), fmt.Sprintf("员工%c", 'A'+i), d.ID, false)
	}

	w := doJSON(http.MethodGet, "/api/employees?page=1&page_size=4", nil, engine)
	var resp employee.ListEmployeeResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Total != 10 {
		t.Errorf("期望 total=10，实际 %d", resp.Total)
	}
	if len(resp.List) != 4 {
		t.Errorf("第一页应有 4 条，实际 %d 条", len(resp.List))
	}
}

func TestEmployeeService_List_PageTwo(t *testing.T) {
	engine, data, cleanup := setupEngine(t)
	defer cleanup()

	d := testutil.SeedDepartment(data, "研发部")
	for i := 0; i < 6; i++ {
		testutil.SeedEmployee(data, fmt.Sprintf("EX%02d", i), fmt.Sprintf("员工%c", 'A'+i), d.ID, false)
	}

	w := doJSON(http.MethodGet, "/api/employees?page=2&page_size=2", nil, engine)
	var resp employee.ListEmployeeResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Total != 6 {
		t.Errorf("期望 total=6，实际 %d", resp.Total)
	}
	if len(resp.List) != 2 {
		t.Errorf("第二页应有 2 条，实际 %d 条", len(resp.List))
	}
}

// ============================================================================
// GetByID 测试
// ============================================================================

func TestEmployeeService_GetByID_Found(t *testing.T) {
	engine, data, cleanup := setupEngine(t)
	defer cleanup()

	d := testutil.SeedDepartment(data, "技术部")
	e := testutil.SeedEmployee(data, "E101", "张三", d.ID, false)

	w := doJSON(http.MethodGet, fmt.Sprintf("/api/employees/%d", e.ID), nil, engine)
	if w.Code != 200 {
		t.Fatalf("期望 200，实际 %d", w.Code)
	}

	var resp employee.EmployeeResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Name != "张三" {
		t.Errorf("期望 '张三'，实际 %q", resp.Name)
	}
}

func TestEmployeeService_GetByID_NotFound(t *testing.T) {
	engine, _, cleanup := setupEngine(t)
	defer cleanup()

	w := doJSON(http.MethodGet, "/api/employees/99999", nil, engine)
	if w.Code != 404 {
		t.Fatalf("期望 404，实际 %d", w.Code)
	}

	var body map[string]string
	json.Unmarshal(w.Body.Bytes(), &body)
	if body["error"] != "员工不存在" {
		t.Errorf("期望 '员工不存在'，实际 %q", body["error"])
	}
}

func TestEmployeeService_GetByID_BadFormat(t *testing.T) {
	engine, _, cleanup := setupEngine(t)
	defer cleanup()

	w := doJSON(http.MethodGet, "/api/employees/abc", nil, engine)
	if w.Code != 400 {
		t.Fatalf("期望 400，实际 %d", w.Code)
	}
}

// ============================================================================
// ListApprovers 测试
// ============================================================================

func TestEmployeeService_ListApprovers_Empty(t *testing.T) {
	engine, _, cleanup := setupEngine(t)
	defer cleanup()

	w := doJSON(http.MethodGet, "/api/employees/approvers", nil, engine)
	if w.Code != 200 {
		t.Fatalf("期望 200，实际 %d", w.Code)
	}

	var resp []employee.EmployeeResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp) != 0 {
		t.Errorf("期望空列表，实际 %d 条", len(resp))
	}
}

func TestEmployeeService_ListApprovers_WithApprovers(t *testing.T) {
	engine, data, cleanup := setupEngine(t)
	defer cleanup()

	d := testutil.SeedDepartment(data, "技术部")
	testutil.SeedEmployee(data, "E101", "张三", d.ID, false)
	testutil.SeedEmployee(data, "E102", "李四", d.ID, true)
	testutil.SeedEmployee(data, "E103", "王五", d.ID, true)

	w := doJSON(http.MethodGet, "/api/employees/approvers", nil, engine)
	if w.Code != 200 {
		t.Fatalf("期望 200，实际 %d", w.Code)
	}

	var resp []employee.EmployeeResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp) != 2 {
		t.Errorf("期望 2 个审批人，实际 %d", len(resp))
	}
}

// ============================================================================
// Create 测试
// ============================================================================

func TestEmployeeService_Create_Success(t *testing.T) {
	engine, data, cleanup := setupEngine(t)
	defer cleanup()

	d := testutil.SeedDepartment(data, "技术部")
	req := employee.CreateEmployeeRequest{
		EmployeeID:   "E201",
		Name:         "赵六",
		Email:        "zhaoliu@test.com",
		DepartmentID: d.ID,
		Role:         "employee",
	}

	w := doJSON(http.MethodPost, "/api/employees", req, engine)
	if w.Code != 201 {
		t.Fatalf("期望 201，实际 %d: %s", w.Code, w.Body.String())
	}

	var resp employee.EmployeeResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.EmployeeID != "E201" {
		t.Errorf("期望工号 'E201'，实际 %q", resp.EmployeeID)
	}
	if resp.IsApprover {
		t.Error("普通员工 is_approver 应为 false")
	}
}

func TestEmployeeService_Create_Approver(t *testing.T) {
	engine, data, cleanup := setupEngine(t)
	defer cleanup()

	d := testutil.SeedDepartment(data, "技术部")
	req := createEmployeeReq("E301", "审批人", "approver@test.com", d.ID, "approver")

	w := doJSON(http.MethodPost, "/api/employees", req, engine)
	if w.Code != 201 {
		t.Fatalf("期望 201，实际 %d: %s", w.Code, w.Body.String())
	}

	var resp employee.EmployeeResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if !resp.IsApprover {
		t.Error("审批人 is_approver 应为 true")
	}
	if resp.Role != "approver" {
		t.Errorf("审批人 role 应为 'approver'，实际 %q", resp.Role)
	}
}

func TestEmployeeService_Create_DuplicateEmployeeID(t *testing.T) {
	engine, data, cleanup := setupEngine(t)
	defer cleanup()

	d := testutil.SeedDepartment(data, "技术部")
	testutil.SeedEmployee(data, "E401", "钱七", d.ID, false)

	req := createEmployeeReq("E401", "孙八", "sunba@test.com", d.ID, "employee")
	w := doJSON(http.MethodPost, "/api/employees", req, engine)
	if w.Code != 409 {
		t.Fatalf("期望 409，实际 %d", w.Code)
	}

	var body map[string]string
	json.Unmarshal(w.Body.Bytes(), &body)
	if body["error"] != "工号'E401'已被使用" {
		t.Errorf("期望 '工号'E401'已被使用'，实际 %q", body["error"])
	}
}

func TestEmployeeService_Create_BadBody(t *testing.T) {
	engine, _, cleanup := setupEngine(t)
	defer cleanup()

	w := doJSON(http.MethodPost, "/api/employees", map[string]string{}, engine)
	if w.Code != 400 {
		t.Fatalf("期望 400，实际 %d", w.Code)
	}
}

// ============================================================================
// Update 测试
// ============================================================================

func TestEmployeeService_Update_Success(t *testing.T) {
	engine, data, cleanup := setupEngine(t)
	defer cleanup()

	d := testutil.SeedDepartment(data, "技术部")
	e := testutil.SeedEmployee(data, "E501", "周九", d.ID, false)

	req := employee.UpdateEmployeeRequest{
		Name:         "周十",
		Email:        "zhoushi@test.com",
		DepartmentID: d.ID,
		Role:         "employee",
	}
	w := doJSON(http.MethodPut, fmt.Sprintf("/api/employees/%d", e.ID), req, engine)
	if w.Code != 200 {
		t.Fatalf("期望 200，实际 %d: %s", w.Code, w.Body.String())
	}

	var resp employee.EmployeeResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Name != "周十" {
		t.Errorf("期望 '周十'，实际 %q", resp.Name)
	}
}

func TestEmployeeService_Update_NotFound(t *testing.T) {
	engine, _, cleanup := setupEngine(t)
	defer cleanup()

	req := employee.UpdateEmployeeRequest{
		Name:         "不存在",
		DepartmentID: 1,
	}
	w := doJSON(http.MethodPut, "/api/employees/99999", req, engine)
	if w.Code != 409 {
		t.Fatalf("期望 409，实际 %d", w.Code)
	}
}

func TestEmployeeService_Update_Role(t *testing.T) {
	engine, data, cleanup := setupEngine(t)
	defer cleanup()

	d := testutil.SeedDepartment(data, "技术部")
	e := testutil.SeedEmployee(data, "E601", "员工", d.ID, false)

	req := employee.UpdateEmployeeRequest{
		Name:         "员工",
		DepartmentID: d.ID,
		Role:         "approver",
	}
	w := doJSON(http.MethodPut, fmt.Sprintf("/api/employees/%d", e.ID), req, engine)
	if w.Code != 200 {
		t.Fatalf("期望 200，实际 %d", w.Code)
	}

	var resp employee.EmployeeResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if !resp.IsApprover {
		t.Error("角色变为 approver 后 is_approver 应为 true")
	}
}

func TestEmployeeService_Update_BadID(t *testing.T) {
	engine, _, cleanup := setupEngine(t)
	defer cleanup()

	req := employee.UpdateEmployeeRequest{Name: "测试", DepartmentID: 1}
	w := doJSON(http.MethodPut, "/api/employees/abc", req, engine)
	if w.Code != 400 {
		t.Fatalf("期望 400，实际 %d", w.Code)
	}
}

// ============================================================================
// Delete 测试
// ============================================================================

func TestEmployeeService_Delete_Success(t *testing.T) {
	engine, data, cleanup := setupEngine(t)
	defer cleanup()

	d := testutil.SeedDepartment(data, "技术部")
	e := testutil.SeedEmployee(data, "E701", "待删", d.ID, false)

	w := doJSON(http.MethodDelete, fmt.Sprintf("/api/employees/%d", e.ID), nil, engine)
	if w.Code != 200 {
		t.Fatalf("期望 200，实际 %d: %s", w.Code, w.Body.String())
	}

	var body map[string]string
	json.Unmarshal(w.Body.Bytes(), &body)
	if body["message"] != "员工删除成功" {
		t.Errorf("期望 '员工删除成功'，实际 %q", body["message"])
	}
}

func TestEmployeeService_Delete_NotFound(t *testing.T) {
	engine, _, cleanup := setupEngine(t)
	defer cleanup()

	w := doJSON(http.MethodDelete, "/api/employees/99999", nil, engine)
	if w.Code != 409 {
		t.Fatalf("期望 409，实际 %d", w.Code)
	}
}

func TestEmployeeService_Delete_BadID(t *testing.T) {
	engine, _, cleanup := setupEngine(t)
	defer cleanup()

	w := doJSON(http.MethodDelete, "/api/employees/abc", nil, engine)
	if w.Code != 400 {
		t.Fatalf("期望 400，实际 %d", w.Code)
	}
}

// createEmployeeReq 快捷创建员工请求
func createEmployeeReq(id, name, email string, deptID uint, role string) employee.CreateEmployeeRequest {
	return employee.CreateEmployeeRequest{
		EmployeeID:   id,
		Name:         name,
		Email:        email,
		DepartmentID: deptID,
		Role:         role,
	}
}
