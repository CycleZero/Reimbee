package reimbursement

import (
	"regexp"
	"sync/atomic"
	"testing"

	zaplog "github.com/CycleZero/Reimbee/log"

	"go.uber.org/zap"

	"github.com/CycleZero/Reimbee/infra"
	"github.com/CycleZero/Reimbee/internal/domain/approval"
	"github.com/CycleZero/Reimbee/internal/domain/budget"
	"github.com/CycleZero/Reimbee/internal/domain/employee"
	"github.com/CycleZero/Reimbee/internal/testutil"
	"github.com/CycleZero/Reimbee/model"
)

// newTestLogger 创建一个适用于测试的 Logger 实例
func newTestLogger() *zaplog.Logger {
	zapLogger, _ := zap.NewDevelopment()
	return &zaplog.Logger{Logger: zapLogger}
}

// setupBizTest 创建 ReimbursementBiz 及其所有跨域依赖（BudgetBiz、ApprovalBiz、EmployeeBiz），
// 返回 biz 实例、原始 Data 引用和清理函数
func setupBizTest() (*ReimbursementBiz, *infra.Data, func()) {
	data := testutil.NewTestData()
	logger := newTestLogger()

	reimbursementRepo := NewReimbursementRepo(data)
	budgetRepo := budget.NewBudgetRepo(data)
	approvalRepo := approval.NewApprovalRepo(data)
	employeeRepo := employee.NewEmployeeRepo(data)

	budgetBiz := budget.NewBudgetBiz(logger, budgetRepo)
	approvalBiz := approval.NewApprovalBiz(logger, approvalRepo)
	employeeBiz := employee.NewEmployeeBiz(logger, employeeRepo)

	biz := NewReimbursementBiz(logger, reimbursementRepo, budgetBiz, approvalBiz, employeeBiz)

	cleanup := func() {
		testutil.CleanDB(data)
	}
	return biz, data, cleanup
}

// resetBizSeq 重置报销单全局流水号，避免跨测试干扰
func resetBizSeq() {
	atomic.StoreUint64(&reimbursementSeq, 0)
}

// ============================================================
// Create 测试
// ============================================================

func TestReimbursementBiz_Create(t *testing.T) {
	t.Run("创建报销单成功-验证草稿状态和单号格式", func(t *testing.T) {
		resetBizSeq()
		biz, _, cleanup := setupBizTest()
		defer cleanup()

		rm, err := biz.Create("EMP001", "张三", 1, "差旅费报销")
		if err != nil {
			t.Fatalf("Create 失败: %v", err)
		}
		if rm.ID == 0 {
			t.Errorf("期望 ID 被填充，但为 0")
		}
		if rm.Status != StatusDraft {
			t.Errorf("期望状态为 draft，实际为 %s", rm.Status)
		}
		if rm.EmployeeID != "EMP001" {
			t.Errorf("期望工号为 EMP001，实际为 %s", rm.EmployeeID)
		}
		if rm.EmployeeName != "张三" {
			t.Errorf("期望姓名为'张三'，实际为 %s", rm.EmployeeName)
		}
		if rm.DepartmentID != 1 {
			t.Errorf("期望部门 ID 为 1，实际为 %d", rm.DepartmentID)
		}
		if rm.SubmitNote != "差旅费报销" {
			t.Errorf("期望备注为'差旅费报销'，实际为 %s", rm.SubmitNote)
		}
		// 验证报销单号格式 REIMB-YYYY-NNNN
		matched, _ := regexp.MatchString(`^REIMB-\d{4}-\d{4}$`, rm.ReimbursementNo)
		if !matched {
			t.Errorf("期望报销单号格式为 REIMB-YYYY-NNNN，实际为 %s", rm.ReimbursementNo)
		}
		// 验证 TotalAmount 默认值为 0
		if rm.TotalAmount != 0 {
			t.Errorf("期望新创建的单 TotalAmount 为 0，实际为 %d", rm.TotalAmount)
		}
	})

	t.Run("创建第二张报销单-单号递增不重复", func(t *testing.T) {
		resetBizSeq()
		biz, _, cleanup := setupBizTest()
		defer cleanup()

		rm1, err := biz.Create("EMP001", "张三", 1, "第一张")
		if err != nil {
			t.Fatalf("Create rm1 失败: %v", err)
		}
		rm2, err := biz.Create("EMP002", "李四", 2, "第二张")
		if err != nil {
			t.Fatalf("Create rm2 失败: %v", err)
		}
		if rm1.ReimbursementNo == rm2.ReimbursementNo {
			t.Errorf("期望两张报销单号不同，但都是 %s", rm1.ReimbursementNo)
		}
		if rm1.DepartmentID != 1 {
			t.Errorf("期望 rm1 部门为 1，实际为 %d", rm1.DepartmentID)
		}
		if rm2.DepartmentID != 2 {
			t.Errorf("期望 rm2 部门为 2，实际为 %d", rm2.DepartmentID)
		}
	})
}

// ============================================================
// Submit 测试（跨域编排：预算检查→冻结→审批链创建→状态更新）
// ============================================================

func TestReimbursementBiz_Submit(t *testing.T) {
	t.Run("成功提交草稿单-验证状态、预算冻结、审批链创建", func(t *testing.T) {
		resetBizSeq()
		biz, data, cleanup := setupBizTest()
		defer cleanup()

		// 准备：部门 + 预算 + 审批人
		dept := testutil.SeedDepartment(data, "技术部")
		testutil.SeedBudget(data, dept.ID, 2026, 100000) // 预算 100000 分
		testutil.SeedEmployee(data, "APPR01", "审批人甲", dept.ID, true)
		testutil.SeedEmployee(data, "APPR02", "审批人乙", dept.ID, true)

		// 创建报销单
		rm, err := biz.Create("EMP001", "张三", dept.ID, "测试提交")
		if err != nil {
			t.Fatalf("Create 失败: %v", err)
		}

		// 提交
		submitted, err := biz.Submit(rm.ID, 50000)
		if err != nil {
			t.Fatalf("Submit 失败: %v", err)
		}
		if submitted.Status != StatusPending {
			t.Errorf("期望状态为 pending，实际为 %s", submitted.Status)
		}
		if submitted.TotalAmount != 50000 {
			t.Errorf("期望 TotalAmount 为 50000，实际为 %d", submitted.TotalAmount)
		}

		// 验证预算已冻结
		var budget model.DepartmentBudget
		if err := data.DB.Where("department_id = ?", dept.ID).First(&budget).Error; err != nil {
			t.Fatalf("查询预算失败: %v", err)
		}
		if budget.FrozenAmount != 50000 {
			t.Errorf("期望冻结金额为 50000，实际为 %d", budget.FrozenAmount)
		}

		// 验证审批链已创建
		var approvals []model.ApprovalRecord
		data.DB.Where("reimbursement_id = ?", rm.ID).Find(&approvals)
		if len(approvals) != 2 {
			t.Errorf("期望创建 2 条审批记录，实际为 %d 条", len(approvals))
		}
		for _, a := range approvals {
			if a.Action != "pending" {
				t.Errorf("期望审批记录状态为 pending，实际为 %s", a.Action)
			}
		}

		// 验证 NeedSpecialApproval 默认 false（预算充足）
		if submitted.NeedSpecialApproval {
			t.Errorf("预算充足时 NeedSpecialApproval 应为 false")
		}
	})

	t.Run("从已驳回状态重新提交", func(t *testing.T) {
		resetBizSeq()
		biz, data, cleanup := setupBizTest()
		defer cleanup()

		dept := testutil.SeedDepartment(data, "技术部")
		testutil.SeedBudget(data, dept.ID, 2026, 100000)
		testutil.SeedEmployee(data, "APPR01", "审批人甲", dept.ID, true)

		// 直接创建一条已驳回的报销单
		rm := testutil.SeedReimbursement(data, "REIMB-2026-RJCT", "EMP001", "张三", dept.ID, StatusRejected, 30000)

		submitted, err := biz.Submit(rm.ID, 30000)
		if err != nil {
			t.Fatalf("从已驳回状态 Submit 失败: %v", err)
		}
		if submitted.Status != StatusPending {
			t.Errorf("期望状态从 rejected 变为 pending，实际为 %s", submitted.Status)
		}

		// 验证预算已冻结
		var budget model.DepartmentBudget
		data.DB.Where("department_id = ?", dept.ID).First(&budget)
		if budget.FrozenAmount != 30000 {
			t.Errorf("期望冻结金额为 30000，实际为 %d", budget.FrozenAmount)
		}
	})

	t.Run("状态为pending时提交应失败", func(t *testing.T) {
		resetBizSeq()
		biz, data, cleanup := setupBizTest()
		defer cleanup()

		dept := testutil.SeedDepartment(data, "技术部")
		testutil.SeedBudget(data, dept.ID, 2026, 100000)
		testutil.SeedEmployee(data, "APPR01", "审批人甲", dept.ID, true)

		rm := testutil.SeedReimbursement(data, "REIMB-2026-ALREADY", "EMP001", "张三", dept.ID, StatusPending, 10000)

		_, err := biz.Submit(rm.ID, 10000)
		if err == nil {
			t.Errorf("期望 pending 状态提交失败，但成功了")
		}
	})

	t.Run("状态为approved时提交应失败", func(t *testing.T) {
		resetBizSeq()
		biz, data, cleanup := setupBizTest()
		defer cleanup()

		dept := testutil.SeedDepartment(data, "技术部")
		testutil.SeedBudget(data, dept.ID, 2026, 100000)
		testutil.SeedEmployee(data, "APPR01", "审批人甲", dept.ID, true)

		rm := testutil.SeedReimbursement(data, "REIMB-2026-APPRV", "EMP001", "张三", dept.ID, StatusApproved, 10000)

		_, err := biz.Submit(rm.ID, 10000)
		if err == nil {
			t.Errorf("期望 approved 状态提交失败，但成功了")
		}
	})

	t.Run("金额为0时提交应失败", func(t *testing.T) {
		resetBizSeq()
		biz, data, cleanup := setupBizTest()
		defer cleanup()

		dept := testutil.SeedDepartment(data, "技术部")
		testutil.SeedBudget(data, dept.ID, 2026, 100000)
		testutil.SeedEmployee(data, "APPR01", "审批人甲", dept.ID, true)

		rm, err := biz.Create("EMP001", "张三", dept.ID, "零金额测试")
		if err != nil {
			t.Fatalf("Create 失败: %v", err)
		}

		_, err = biz.Submit(rm.ID, 0)
		if err == nil {
			t.Errorf("期望金额为 0 时 Submit 返回错误，但成功了")
		}
	})

	t.Run("金额为负数时提交应失败", func(t *testing.T) {
		resetBizSeq()
		biz, data, cleanup := setupBizTest()
		defer cleanup()

		dept := testutil.SeedDepartment(data, "技术部")
		testutil.SeedBudget(data, dept.ID, 2026, 100000)
		testutil.SeedEmployee(data, "APPR01", "审批人甲", dept.ID, true)

		rm, err := biz.Create("EMP001", "张三", dept.ID, "负数金额测试")
		if err != nil {
			t.Fatalf("Create 失败: %v", err)
		}

		_, err = biz.Submit(rm.ID, -100)
		if err == nil {
			t.Errorf("期望金额为负数时 Submit 返回错误，但成功了")
		}
	})

	t.Run("报销单不存在时提交应失败", func(t *testing.T) {
		resetBizSeq()
		biz, _, cleanup := setupBizTest()
		defer cleanup()

		_, err := biz.Submit(99999, 10000)
		if err == nil {
			t.Errorf("期望报销单不存在时返回错误，但成功了")
		}
	})

	t.Run("无预算记录时提交应失败", func(t *testing.T) {
		resetBizSeq()
		biz, data, cleanup := setupBizTest()
		defer cleanup()

		dept := testutil.SeedDepartment(data, "技术部")
		// 不创建预算记录
		testutil.SeedEmployee(data, "APPR01", "审批人甲", dept.ID, true)

		rm, err := biz.Create("EMP001", "张三", dept.ID, "无预算测试")
		if err != nil {
			t.Fatalf("Create 失败: %v", err)
		}

		_, err = biz.Submit(rm.ID, 50000)
		if err == nil {
			t.Errorf("期望无预算记录时返回错误，但成功了")
		}
	})

	t.Run("无审批人时提交应失败-验证预算回滚解冻", func(t *testing.T) {
		resetBizSeq()
		biz, data, cleanup := setupBizTest()
		defer cleanup()

		dept := testutil.SeedDepartment(data, "技术部")
		testutil.SeedBudget(data, dept.ID, 2026, 100000)
		// 不创建任何审批人

		rm, err := biz.Create("EMP001", "张三", dept.ID, "无审批人测试")
		if err != nil {
			t.Fatalf("Create 失败: %v", err)
		}

		_, err = biz.Submit(rm.ID, 50000)
		if err == nil {
			t.Errorf("期望无审批人时返回错误，但成功了")
		}

		// 验证预算已解冻（回滚）
		var budget model.DepartmentBudget
		data.DB.Where("department_id = ?", dept.ID).First(&budget)
		if budget.FrozenAmount != 0 {
			t.Errorf("期望回滚后冻结金额为 0，实际为 %d", budget.FrozenAmount)
		}
	})

	t.Run("预算不足时设置need_special_approval为true", func(t *testing.T) {
		resetBizSeq()
		biz, data, cleanup := setupBizTest()
		defer cleanup()

		dept := testutil.SeedDepartment(data, "技术部")
		// 年度预算仅 10000，可用余额不足
		testutil.SeedBudget(data, dept.ID, 2026, 10000)
		testutil.SeedEmployee(data, "APPR01", "审批人甲", dept.ID, true)
		testutil.SeedEmployee(data, "APPR02", "审批人乙", dept.ID, true)

		rm, err := biz.Create("EMP001", "张三", dept.ID, "超额预算测试")
		if err != nil {
			t.Fatalf("Create 失败: %v", err)
		}

		submitted, err := biz.Submit(rm.ID, 50000) // 超过 10000 预算
		if err != nil {
			t.Fatalf("Submit 失败（即使超额也应成功提交）: %v", err)
		}
		if !submitted.NeedSpecialApproval {
			t.Errorf("期望 NeedSpecialApproval 为 true（预算不足），实际为 false")
		}
		if submitted.TotalAmount != 50000 {
			t.Errorf("期望金额为 50000，实际为 %d", submitted.TotalAmount)
		}

		// 预算仍然被冻结（超额也冻结）
		var budget model.DepartmentBudget
		data.DB.Where("department_id = ?", dept.ID).First(&budget)
		if budget.FrozenAmount != 50000 {
			t.Errorf("期望冻结金额为 50000，实际为 %d", budget.FrozenAmount)
		}
	})
}

// ============================================================
// Approve 测试（强制审批通过模式）
// ============================================================

func TestReimbursementBiz_Approve(t *testing.T) {
	t.Run("审批通过-状态变为approved且预算已扣减", func(t *testing.T) {
		resetBizSeq()
		biz, data, cleanup := setupBizTest()
		defer cleanup()

		dept := testutil.SeedDepartment(data, "技术部")
		testutil.SeedBudget(data, dept.ID, 2026, 100000)
		testutil.SeedEmployee(data, "APPR01", "审批人甲", dept.ID, true)
		testutil.SeedEmployee(data, "APPR02", "审批人乙", dept.ID, true)

		rm, err := biz.Create("EMP001", "张三", dept.ID, "测试审批")
		if err != nil {
			t.Fatalf("Create 失败: %v", err)
		}
		submitted, err := biz.Submit(rm.ID, 50000)
		if err != nil {
			t.Fatalf("Submit 失败: %v", err)
		}
		if submitted.Status != StatusPending {
			t.Fatalf("期望 Submit 后为 pending，实际为 %s", submitted.Status)
		}

		// 审批通过
		approved, err := biz.Approve(submitted.ID)
		if err != nil {
			t.Fatalf("Approve 失败: %v", err)
		}
		if approved.Status != StatusApproved {
			t.Errorf("期望状态为 approved，实际为 %s", approved.Status)
		}

		// 验证预算已扣减（冻结→实际支出）
		var budget model.DepartmentBudget
		if err := data.DB.Where("department_id = ?", dept.ID).First(&budget).Error; err != nil {
			t.Fatalf("查询预算失败: %v", err)
		}
		if budget.SpentAmount != 50000 {
			t.Errorf("期望已结算金额为 50000，实际为 %d", budget.SpentAmount)
		}
		if budget.FrozenAmount != 0 {
			t.Errorf("期望冻结金额被释放为 0，实际为 %d", budget.FrozenAmount)
		}

		// 验证所有审批记录已标记为 approved
		var approvalRecords []model.ApprovalRecord
		data.DB.Where("reimbursement_id = ?", submitted.ID).Find(&approvalRecords)
		for _, a := range approvalRecords {
			if a.Action != "approved" {
				t.Errorf("期望审批记录 %d 状态为 approved，实际为 %s", a.ID, a.Action)
			}
		}
	})

	t.Run("报销单不存在时审批应失败", func(t *testing.T) {
		resetBizSeq()
		biz, _, cleanup := setupBizTest()
		defer cleanup()

		_, err := biz.Approve(99999)
		if err == nil {
			t.Errorf("期望报销单不存在时返回错误，但成功了")
		}
	})

	t.Run("草稿状态不可审批", func(t *testing.T) {
		resetBizSeq()
		biz, data, cleanup := setupBizTest()
		defer cleanup()

		dept := testutil.SeedDepartment(data, "技术部")
		rm := testutil.SeedReimbursement(data, "REIMB-2026-DRAFT", "EMP001", "张三", dept.ID, StatusDraft, 10000)

		_, err := biz.Approve(rm.ID)
		if err == nil {
			t.Errorf("期望草稿状态审批失败，但成功了")
		}
	})

	t.Run("已通过状态不可重复审批", func(t *testing.T) {
		resetBizSeq()
		biz, data, cleanup := setupBizTest()
		defer cleanup()

		dept := testutil.SeedDepartment(data, "技术部")
		rm := testutil.SeedReimbursement(data, "REIMB-2026-DUPAP", "EMP001", "张三", dept.ID, StatusApproved, 10000)

		_, err := biz.Approve(rm.ID)
		if err == nil {
			t.Errorf("期望已通过状态不可重复审批，但成功了")
		}
	})

	t.Run("已驳回状态不可审批", func(t *testing.T) {
		resetBizSeq()
		biz, data, cleanup := setupBizTest()
		defer cleanup()

		dept := testutil.SeedDepartment(data, "技术部")
		rm := testutil.SeedReimbursement(data, "REIMB-2026-RJCTAP", "EMP001", "张三", dept.ID, StatusRejected, 10000)

		_, err := biz.Approve(rm.ID)
		if err == nil {
			t.Errorf("期望已驳回状态不可审批，但成功了")
		}
	})
}

// ============================================================
// Reject 测试（驳回+解冻预算）
// ============================================================

func TestReimbursementBiz_Reject(t *testing.T) {
	t.Run("驳回成功-状态变为rejected且预算已解冻", func(t *testing.T) {
		resetBizSeq()
		biz, data, cleanup := setupBizTest()
		defer cleanup()

		dept := testutil.SeedDepartment(data, "技术部")
		testutil.SeedBudget(data, dept.ID, 2026, 100000)
		testutil.SeedEmployee(data, "APPR01", "审批人甲", dept.ID, true)

		rm, err := biz.Create("EMP001", "张三", dept.ID, "测试驳回")
		if err != nil {
			t.Fatalf("Create 失败: %v", err)
		}
		submitted, err := biz.Submit(rm.ID, 30000)
		if err != nil {
			t.Fatalf("Submit 失败: %v", err)
		}

		// 驳回
		rejected, err := biz.Reject(submitted.ID, "测试驳回")
		if err != nil {
			t.Fatalf("Reject 失败: %v", err)
		}
		if rejected.Status != StatusRejected {
			t.Errorf("期望状态为 rejected，实际为 %s", rejected.Status)
		}

		// 验证预算已解冻
		var budget model.DepartmentBudget
		data.DB.Where("department_id = ?", dept.ID).First(&budget)
		if budget.FrozenAmount != 0 {
			t.Errorf("期望解冻后冻结金额为 0，实际为 %d", budget.FrozenAmount)
		}
	})

	t.Run("报销单不存在时驳回应失败", func(t *testing.T) {
		resetBizSeq()
		biz, _, cleanup := setupBizTest()
		defer cleanup()

		_, err := biz.Reject(99999, "测试")
		if err == nil {
			t.Errorf("期望报销单不存在时返回错误，但成功了")
		}
	})

	t.Run("草稿状态不可驳回", func(t *testing.T) {
		resetBizSeq()
		biz, data, cleanup := setupBizTest()
		defer cleanup()

		dept := testutil.SeedDepartment(data, "技术部")
		rm := testutil.SeedReimbursement(data, "REIMB-2026-DRFT", "EMP001", "张三", dept.ID, StatusDraft, 10000)

		_, err := biz.Reject(rm.ID, "测试")
		if err == nil {
			t.Errorf("期望草稿状态驳回失败，但成功了")
		}
	})

	t.Run("已通过状态不可驳回", func(t *testing.T) {
		resetBizSeq()
		biz, data, cleanup := setupBizTest()
		defer cleanup()

		dept := testutil.SeedDepartment(data, "技术部")
		rm := testutil.SeedReimbursement(data, "REIMB-2026-APRV", "EMP001", "张三", dept.ID, StatusApproved, 10000)

		_, err := biz.Reject(rm.ID, "测试")
		if err == nil {
			t.Errorf("期望已通过状态驳回失败，但成功了")
		}
	})

	t.Run("已驳回状态不可重复驳回", func(t *testing.T) {
		resetBizSeq()
		biz, data, cleanup := setupBizTest()
		defer cleanup()

		dept := testutil.SeedDepartment(data, "技术部")
		rm := testutil.SeedReimbursement(data, "REIMB-2026-DUPRJ", "EMP001", "张三", dept.ID, StatusRejected, 10000)

		_, err := biz.Reject(rm.ID, "测试")
		if err == nil {
			t.Errorf("期望已驳回状态不可重复驳回，但成功了")
		}
	})
}

// ============================================================
// GetByID 测试
// ============================================================

func TestReimbursementBiz_GetByID(t *testing.T) {
	t.Run("查询存在的报销单", func(t *testing.T) {
		resetBizSeq()
		biz, _, cleanup := setupBizTest()
		defer cleanup()

		rm, err := biz.Create("EMP001", "张三", 1, "测试查询")
		if err != nil {
			t.Fatalf("Create 失败: %v", err)
		}

		got, err := biz.GetByID(rm.ID)
		if err != nil {
			t.Fatalf("GetByID 失败: %v", err)
		}
		if got.ID != rm.ID {
			t.Errorf("期望 ID 为 %d，实际为 %d", rm.ID, got.ID)
		}
		if got.EmployeeID != "EMP001" {
			t.Errorf("期望工号为 EMP001，实际为 %s", got.EmployeeID)
		}
	})

	t.Run("查询不存在的报销单", func(t *testing.T) {
		resetBizSeq()
		biz, _, cleanup := setupBizTest()
		defer cleanup()

		_, err := biz.GetByID(99999)
		if err == nil {
			t.Errorf("期望不存在时返回错误，但成功了")
		}
	})
}

// ============================================================
// GetByNo 测试
// ============================================================

func TestReimbursementBiz_GetByNo(t *testing.T) {
	t.Run("按单号查询存在", func(t *testing.T) {
		resetBizSeq()
		biz, _, cleanup := setupBizTest()
		defer cleanup()

		rm, err := biz.Create("EMP001", "张三", 1, "测试单号查询")
		if err != nil {
			t.Fatalf("Create 失败: %v", err)
		}

		got, err := biz.GetByNo(rm.ReimbursementNo)
		if err != nil {
			t.Fatalf("GetByNo 失败: %v", err)
		}
		if got.ReimbursementNo != rm.ReimbursementNo {
			t.Errorf("期望单号为 %s，实际为 %s", rm.ReimbursementNo, got.ReimbursementNo)
		}
	})

	t.Run("按单号查询不存在", func(t *testing.T) {
		resetBizSeq()
		biz, _, cleanup := setupBizTest()
		defer cleanup()

		_, err := biz.GetByNo("REIMB-9999-9999")
		if err == nil {
			t.Errorf("期望不存在时返回错误，但成功了")
		}
	})
}

// ============================================================
// List 测试
// ============================================================

func TestReimbursementBiz_List(t *testing.T) {
	t.Run("空列表查询", func(t *testing.T) {
		resetBizSeq()
		biz, _, cleanup := setupBizTest()
		defer cleanup()

		rms, total, err := biz.List(1, 10, "")
		if err != nil {
			t.Fatalf("List 失败: %v", err)
		}
		if total != 0 {
			t.Errorf("期望 total 为 0，实际为 %d", total)
		}
		if len(rms) != 0 {
			t.Errorf("期望返回空列表，实际返回 %d 条", len(rms))
		}
	})

	t.Run("不带筛选返回所有数据", func(t *testing.T) {
		resetBizSeq()
		biz, data, cleanup := setupBizTest()
		defer cleanup()

		dept := testutil.SeedDepartment(data, "技术部")
		testutil.SeedReimbursement(data, "REIMB-2026-L01", "EMP001", "张三", dept.ID, StatusDraft, 1000)
		testutil.SeedReimbursement(data, "REIMB-2026-L02", "EMP002", "李四", dept.ID, StatusPending, 2000)

		rms, total, err := biz.List(1, 10, "")
		if err != nil {
			t.Fatalf("List 失败: %v", err)
		}
		if total != 2 {
			t.Errorf("期望 total 为 2，实际为 %d", total)
		}
		if len(rms) != 2 {
			t.Errorf("期望返回 2 条，实际返回 %d 条", len(rms))
		}
	})

	t.Run("按工号筛选", func(t *testing.T) {
		resetBizSeq()
		biz, data, cleanup := setupBizTest()
		defer cleanup()

		dept := testutil.SeedDepartment(data, "技术部")
		testutil.SeedReimbursement(data, "REIMB-2026-F1", "EMP001", "张三", dept.ID, StatusDraft, 1000)
		testutil.SeedReimbursement(data, "REIMB-2026-F2", "EMP002", "李四", dept.ID, StatusPending, 2000)
		testutil.SeedReimbursement(data, "REIMB-2026-F3", "EMP001", "张三", dept.ID, StatusApproved, 3000)

		rms, total, err := biz.List(1, 10, "EMP001")
		if err != nil {
			t.Fatalf("List(EMP001) 失败: %v", err)
		}
		if total != 2 {
			t.Errorf("期望 EMP001 的 total 为 2，实际为 %d", total)
		}
		for _, r := range rms {
			if r.EmployeeID != "EMP001" {
				t.Errorf("期望所有结果的 EmployeeID 为 EMP001，但发现 %s", r.EmployeeID)
			}
		}
	})
}

// ============================================================
// ListPending 测试
// ============================================================

func TestReimbursementBiz_ListPending(t *testing.T) {
	t.Run("无待审批单时返回空列表", func(t *testing.T) {
		resetBizSeq()
		biz, _, cleanup := setupBizTest()
		defer cleanup()

		rms, err := biz.ListPending()
		if err != nil {
			t.Fatalf("ListPending 失败: %v", err)
		}
		if len(rms) != 0 {
			t.Errorf("期望返回空列表，实际返回 %d 条", len(rms))
		}
	})

	t.Run("有待审批单时正常返回", func(t *testing.T) {
		resetBizSeq()
		biz, data, cleanup := setupBizTest()
		defer cleanup()

		dept := testutil.SeedDepartment(data, "技术部")
		testutil.SeedReimbursement(data, "REIMB-2026-PD1", "EMP001", "张三", dept.ID, StatusDraft, 1000)
		testutil.SeedReimbursement(data, "REIMB-2026-PD2", "EMP002", "李四", dept.ID, StatusPending, 2000)
		testutil.SeedReimbursement(data, "REIMB-2026-PD3", "EMP003", "王五", dept.ID, StatusPending, 3000)
		testutil.SeedReimbursement(data, "REIMB-2026-PD4", "EMP004", "赵六", dept.ID, StatusApproved, 4000)

		rms, err := biz.ListPending()
		if err != nil {
			t.Fatalf("ListPending 失败: %v", err)
		}
		if len(rms) != 2 {
			t.Errorf("期望返回 2 条待审批单，实际返回 %d 条", len(rms))
		}
		for _, r := range rms {
			if r.Status != StatusPending {
				t.Errorf("期望所有结果为 pending，但发现 status=%s", r.Status)
			}
		}
	})
}
