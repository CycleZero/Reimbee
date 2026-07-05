package budget

import (
	"strings"
	"testing"

	"github.com/CycleZero/Reimbee/infra"
	"github.com/CycleZero/Reimbee/internal/testutil"
	"github.com/CycleZero/Reimbee/log"
	"github.com/CycleZero/Reimbee/model"
	"go.uber.org/zap"
)

// 辅助函数：创建无操作日志器
func newTestLogger() *log.Logger {
	return &log.Logger{Logger: zap.NewNop()}
}

// 辅助函数：创建 BudgetBiz 和基础测试数据
func setupBizTest(t *testing.T) (*BudgetBiz, *infra.Data, *model.Department, func()) {
	t.Helper()
	data := testutil.NewTestData()
	repo := NewBudgetRepo(data)
	biz := NewBudgetBiz(newTestLogger(), repo)
	dept := testutil.SeedDepartment(data, "研发部")

	cleanup := func() {
		testutil.CleanDB(data)
	}
	return biz, data, dept, cleanup
}

// 辅助函数：创建已有一个预算记录的 Biz 测试环境
func setupBizWithBudget(t *testing.T, annualBudget int64) (*BudgetBiz, *infra.Data, *model.Department, *model.DepartmentBudget, func()) {
	t.Helper()
	biz, data, dept, cleanup := setupBizTest(t)
	budget := testutil.SeedBudget(data, dept.ID, 2026, annualBudget)
	return biz, data, dept, budget, cleanup
}

// ============================================================
// Create 测试
// ============================================================
func TestBudgetBiz_Create(t *testing.T) {
	t.Run("正常创建预算记录", func(t *testing.T) {
		biz, _, dept, cleanup := setupBizTest(t)
		defer cleanup()

		b, err := biz.Create(dept.ID, 2026, 5000000)
		if err != nil {
			t.Fatalf("创建预算失败: %v", err)
		}
		if b.ID == 0 {
			t.Fatal("创建后预算 ID 应为非零值")
		}
		if b.DepartmentID != dept.ID {
			t.Errorf("部门ID不匹配: 期望 %d, 实际 %d", dept.ID, b.DepartmentID)
		}
		if b.FiscalYear != 2026 {
			t.Errorf("财年不匹配: 期望 2026, 实际 %d", b.FiscalYear)
		}
		if b.AnnualBudget != 5000000 {
			t.Errorf("年度预算不匹配: 期望 5000000, 实际 %d", b.AnnualBudget)
		}
		if b.SpentAmount != 0 || b.FrozenAmount != 0 {
			t.Errorf("新建预算的已结算和冻结金额均应为 0")
		}
	})

	t.Run("同一部门同一年重复创建应报错", func(t *testing.T) {
		biz, data, dept, cleanup := setupBizTest(t)
		defer cleanup()

		// 先创建一条
		testutil.SeedBudget(data, dept.ID, 2026, 5000000)

		// 相同部门同年再创建
		_, err := biz.Create(dept.ID, 2026, 8000000)
		if err == nil {
			t.Fatal("期望重复创建报错, 实际无错误")
		}
		if !strings.Contains(err.Error(), "已存在") {
			t.Errorf("错误消息应包含'已存在', 实际: %v", err)
		}
	})

	t.Run("同一部门不同财年可以创建多条", func(t *testing.T) {
		biz, _, dept, cleanup := setupBizTest(t)
		defer cleanup()

		b1, err := biz.Create(dept.ID, 2025, 3000000)
		if err != nil {
			t.Fatalf("创建2025年预算失败: %v", err)
		}

		b2, err := biz.Create(dept.ID, 2026, 5000000)
		if err != nil {
			t.Fatalf("创建2026年预算失败: %v", err)
		}

		if b1.ID == b2.ID {
			t.Error("不同财年的预算 ID 不应相同")
		}
		if b1.FiscalYear != 2025 {
			t.Errorf("第一条财年应为2025, 实际 %d", b1.FiscalYear)
		}
		if b2.FiscalYear != 2026 {
			t.Errorf("第二条财年应为2026, 实际 %d", b2.FiscalYear)
		}
	})

	t.Run("不同部门同一年可以创建多条", func(t *testing.T) {
		biz, data, _, cleanup := setupBizTest(t)
		defer cleanup()

		dept2 := testutil.SeedDepartment(data, "市场部")

		b1, err := biz.Create(dept2.ID, 2026, 3000000)
		if err != nil {
			t.Fatalf("创建市场部预算失败: %v", err)
		}
		if b1.DepartmentID != dept2.ID {
			t.Errorf("部门ID不匹配")
		}
	})
}

// ============================================================
// GetDashboard 测试
// ============================================================
func TestBudgetBiz_GetDashboard(t *testing.T) {
	t.Run("获取空看板返回空列表不报错", func(t *testing.T) {
		biz, _, _, cleanup := setupBizTest(t)
		defer cleanup()

		budgets, err := biz.GetDashboard(2026)
		if err != nil {
			t.Fatalf("获取空看板失败: %v", err)
		}
		if len(budgets) != 0 {
			t.Errorf("期望空列表, 实际 %d 条", len(budgets))
		}
	})

	t.Run("获取多个部门预算看板并验证汇总数据", func(t *testing.T) {
		biz, data, _, cleanup := setupBizTest(t)
		defer cleanup()

		dept1 := testutil.SeedDepartment(data, "研发部")
		dept2 := testutil.SeedDepartment(data, "市场部")
		dept3 := testutil.SeedDepartment(data, "财务部")

		testutil.SeedBudget(data, dept1.ID, 2026, 10000000) // 研发: 10万
		testutil.SeedBudget(data, dept2.ID, 2026, 8000000)  // 市场: 8万
		testutil.SeedBudget(data, dept3.ID, 2026, 5000000)  // 财务: 5万

		budgets, err := biz.GetDashboard(2026)
		if err != nil {
			t.Fatalf("获取看板失败: %v", err)
		}
		if len(budgets) != 3 {
			t.Fatalf("期望 3 条记录, 实际 %d", len(budgets))
		}

		// 验证汇总计算
		var totalBudget, totalSpent, totalRemaining int64
		for _, b := range budgets {
			totalBudget += b.AnnualBudget
			totalSpent += b.SpentAmount
			remaining := b.AnnualBudget - b.SpentAmount - b.FrozenAmount
			totalRemaining += remaining
		}
		if totalBudget != 23000000 {
			t.Errorf("总预算应为 23000000, 实际 %d", totalBudget)
		}
		if totalSpent != 0 {
			t.Errorf("总已结算应为 0, 实际 %d", totalSpent)
		}
		if totalRemaining != 23000000 {
			t.Errorf("总剩余应为 23000000, 实际 %d", totalRemaining)
		}
	})

	t.Run("部分部门有已结算和冻结金额时看板数据正确", func(t *testing.T) {
		biz, data, dept, _, cleanup := setupBizWithBudget(t, 10000000)
		defer cleanup()

		// 手动设置 dept 的已结算和冻结
		biz.repo.Freeze(dept.ID, 2026, 2000000)
		biz.repo.Deduct(dept.ID, 2026, 1500000)

		// 添加第二个部门
		dept2 := testutil.SeedDepartment(data, "行政部")
		testutil.SeedBudget(data, dept2.ID, 2026, 5000000)

		budgets, err := biz.GetDashboard(2026)
		if err != nil {
			t.Fatalf("获取看板失败: %v", err)
		}

		for _, b := range budgets {
			if b.DepartmentID == dept.ID {
				if b.SpentAmount != 1500000 {
					t.Errorf("研发部已结算应为 1500000, 实际 %d", b.SpentAmount)
				}
				// 冻结: 2000000 - 1500000 = 500000
				if b.FrozenAmount != 500000 {
					t.Errorf("研发部冻结金额应为 500000, 实际 %d", b.FrozenAmount)
				}
			}
		}
	})
}

// ============================================================
// CheckBudget 测试
// ============================================================
func TestBudgetBiz_CheckBudget(t *testing.T) {
	t.Run("预算充足时返回剩余金额且不需要特殊审批", func(t *testing.T) {
		biz, _, dept, _, cleanup := setupBizWithBudget(t, 10000000) // 10万
		defer cleanup()

		remaining, needApproval, err := biz.CheckBudget(dept.ID, 500000) // 申请5千
		if err != nil {
			t.Fatalf("检查预算失败: %v", err)
		}
		if remaining != 10000000 {
			t.Errorf("可用余额应为 10000000, 实际 %d", remaining)
		}
		if needApproval {
			t.Error("预算充足时不应需要特殊审批")
		}
	})

	t.Run("申请金额等于可用余额时不需要特殊审批", func(t *testing.T) {
		biz, _, dept, _, cleanup := setupBizWithBudget(t, 10000000)
		defer cleanup()

		remaining, needApproval, err := biz.CheckBudget(dept.ID, 10000000) // 申请=全部预算
		if err != nil {
			t.Fatalf("检查预算失败: %v", err)
		}
		if remaining != 10000000 {
			t.Errorf("可用余额应为 10000000, 实际 %d", remaining)
		}
		if needApproval {
			t.Error("申请金额等于可用余额时不应需要特殊审批")
		}
	})

	t.Run("申请金额超过可用余额时触发特殊审批", func(t *testing.T) {
		biz, _, dept, _, cleanup := setupBizWithBudget(t, 10000000)
		defer cleanup()

		remaining, needApproval, err := biz.CheckBudget(dept.ID, 15000000) // 申请15万 > 10万
		if err != nil {
			t.Fatalf("检查预算失败: %v", err)
		}
		if remaining != 10000000 {
			t.Errorf("可用余额应为 10000000, 实际 %d", remaining)
		}
		if !needApproval {
			t.Error("超额申请应触发特殊审批")
		}
	})

	t.Run("有冻结金额时可用余额减少", func(t *testing.T) {
		biz, _, dept, _, cleanup := setupBizWithBudget(t, 10000000)
		defer cleanup()

		// 冻结 3000000
		biz.repo.Freeze(dept.ID, 2026, 3000000)

		remaining, _, err := biz.CheckBudget(dept.ID, 8000000) // 申请8万
		if err != nil {
			t.Fatalf("检查预算失败: %v", err)
		}
		// remaining = 10,000,000 - 0 - 3,000,000 = 7,000,000
		if remaining != 7000000 {
			t.Errorf("有冻结时可用余额应为 7000000, 实际 %d", remaining)
		}
	})

	t.Run("有已结算金额时可用余额减少", func(t *testing.T) {
		biz, _, dept, _, cleanup := setupBizWithBudget(t, 10000000)
		defer cleanup()

		// 已结算 4000000
		biz.repo.Deduct(dept.ID, 2026, 4000000)

		remaining, _, err := biz.CheckBudget(dept.ID, 5000000) // 申请5万
		if err != nil {
			t.Fatalf("检查预算失败: %v", err)
		}
		// remaining = 10,000,000 - 4,000,000 - 0 = 6,000,000
		if remaining != 6000000 {
			t.Errorf("有已结算时可用余额应为 6000000, 实际 %d", remaining)
		}
	})

	t.Run("既有已结算又有冻结时可用余额正确计算", func(t *testing.T) {
		biz, _, dept, _, cleanup := setupBizWithBudget(t, 10000000)
		defer cleanup()

		biz.repo.Freeze(dept.ID, 2026, 2000000)  // 冻结 200万
		biz.repo.Deduct(dept.ID, 2026, 3000000)  // 结算 300万

		remaining, needApproval, err := biz.CheckBudget(dept.ID, 6000000) // 申请6万
		if err != nil {
			t.Fatalf("检查预算失败: %v", err)
		}
		// 剩余 = 10,000,000 - 3,000,000 - (2,000,000-3,000,000=0) = 7,000,000
		if remaining != 7000000 {
			t.Errorf("可用余额应为 7000000, 实际 %d", remaining)
		}
		// 6,000,000 < 7,000,000 不需特殊审批
		if needApproval {
			t.Error("预算充足时不应触发特殊审批")
		}
	})

	t.Run("部门无预算记录时返回错误", func(t *testing.T) {
		biz, _, dept, cleanup := setupBizTest(t)
		defer cleanup()

		_, _, err := biz.CheckBudget(dept.ID, 100000)
		if err == nil {
			t.Fatal("无预算记录时应返回错误")
		}
		if !strings.Contains(err.Error(), "未设置") {
			t.Errorf("错误消息应包含'未设置', 实际: %v", err)
		}
	})

	t.Run("预算恰好为零时申请任何金额都需要特殊审批", func(t *testing.T) {
		biz, _, dept, _, cleanup := setupBizWithBudget(t, 0)
		defer cleanup()

		remaining, needApproval, err := biz.CheckBudget(dept.ID, 1)
		if err != nil {
			t.Fatalf("检查预算失败: %v", err)
		}
		if remaining != 0 {
			t.Errorf("零预算时可用余额应为 0, 实际 %d", remaining)
		}
		if !needApproval {
			t.Error("零预算时申请任何金额都应触发特殊审批")
		}
	})
}

// ============================================================
// Freeze 测试
// ============================================================
func TestBudgetBiz_Freeze(t *testing.T) {
	t.Run("正常冻结一笔金额", func(t *testing.T) {
		biz, _, dept, _, cleanup := setupBizWithBudget(t, 10000000)
		defer cleanup()

		if err := biz.Freeze(dept.ID, 500000); err != nil {
			t.Fatalf("冻结失败: %v", err)
		}

		b, _ := biz.repo.GetByDepartmentAndYear(dept.ID, 2026)
		if b.FrozenAmount != 500000 {
			t.Errorf("冻结后冻结金额应为 500000, 实际 %d", b.FrozenAmount)
		}
	})

	t.Run("零金额冻结应报错", func(t *testing.T) {
		biz, _, dept, _, cleanup := setupBizWithBudget(t, 10000000)
		defer cleanup()

		err := biz.Freeze(dept.ID, 0)
		if err == nil {
			t.Fatal("零金额冻结应返回错误")
		}
		if !strings.Contains(err.Error(), "大于零") {
			t.Errorf("错误消息应包含'大于零', 实际: %v", err)
		}
	})

	t.Run("负金额冻结应报错", func(t *testing.T) {
		biz, _, dept, _, cleanup := setupBizWithBudget(t, 10000000)
		defer cleanup()

		err := biz.Freeze(dept.ID, -100000)
		if err == nil {
			t.Fatal("负金额冻结应返回错误")
		}
		if !strings.Contains(err.Error(), "大于零") {
			t.Errorf("错误消息应包含'大于零', 实际: %v", err)
		}
	})
}

// ============================================================
// Deduct 测试
// ============================================================
func TestBudgetBiz_Deduct(t *testing.T) {
	t.Run("正常扣减预算增加已结算并减少冻结", func(t *testing.T) {
		biz, _, dept, _, cleanup := setupBizWithBudget(t, 10000000)
		defer cleanup()

		// 先冻结
		biz.repo.Freeze(dept.ID, 2026, 500000)

		if err := biz.Deduct(dept.ID, 300000); err != nil {
			t.Fatalf("扣减失败: %v", err)
		}

		b, _ := biz.repo.GetByDepartmentAndYear(dept.ID, 2026)
		if b.SpentAmount != 300000 {
			t.Errorf("扣减后已结算应为 300000, 实际 %d", b.SpentAmount)
		}
		if b.FrozenAmount != 200000 {
			t.Errorf("扣减后冻结金额应为 200000, 实际 %d", b.FrozenAmount)
		}
	})

	t.Run("零金额扣减应报错", func(t *testing.T) {
		biz, _, dept, _, cleanup := setupBizWithBudget(t, 10000000)
		defer cleanup()

		err := biz.Deduct(dept.ID, 0)
		if err == nil {
			t.Fatal("零金额扣减应返回错误")
		}
		if !strings.Contains(err.Error(), "大于零") {
			t.Errorf("错误消息应包含'大于零', 实际: %v", err)
		}
	})

	t.Run("负金额扣减应报错", func(t *testing.T) {
		biz, _, dept, _, cleanup := setupBizWithBudget(t, 10000000)
		defer cleanup()

		err := biz.Deduct(dept.ID, -500000)
		if err == nil {
			t.Fatal("负金额扣减应返回错误")
		}
		if !strings.Contains(err.Error(), "大于零") {
			t.Errorf("错误消息应包含'大于零', 实际: %v", err)
		}
	})
}

// ============================================================
// Unfreeze 测试
// ============================================================
func TestBudgetBiz_Unfreeze(t *testing.T) {
	t.Run("正常解冻部分冻结金额", func(t *testing.T) {
		biz, _, dept, _, cleanup := setupBizWithBudget(t, 10000000)
		defer cleanup()

		biz.repo.Freeze(dept.ID, 2026, 500000)

		if err := biz.Unfreeze(dept.ID, 200000); err != nil {
			t.Fatalf("解冻失败: %v", err)
		}

		b, _ := biz.repo.GetByDepartmentAndYear(dept.ID, 2026)
		if b.FrozenAmount != 300000 {
			t.Errorf("解冻后冻结金额应为 300000, 实际 %d", b.FrozenAmount)
		}
	})

	t.Run("零金额解冻应报错", func(t *testing.T) {
		biz, _, dept, _, cleanup := setupBizWithBudget(t, 10000000)
		defer cleanup()

		err := biz.Unfreeze(dept.ID, 0)
		if err == nil {
			t.Fatal("零金额解冻应返回错误")
		}
		if !strings.Contains(err.Error(), "大于零") {
			t.Errorf("错误消息应包含'大于零', 实际: %v", err)
		}
	})

	t.Run("负金额解冻应报错", func(t *testing.T) {
		biz, _, dept, _, cleanup := setupBizWithBudget(t, 10000000)
		defer cleanup()

		err := biz.Unfreeze(dept.ID, -200000)
		if err == nil {
			t.Fatal("负金额解冻应返回错误")
		}
		if !strings.Contains(err.Error(), "大于零") {
			t.Errorf("错误消息应包含'大于零', 实际: %v", err)
		}
	})
}

// ============================================================
// Update 测试
// ============================================================
func TestBudgetBiz_Update(t *testing.T) {
	t.Run("正常更新预算金额", func(t *testing.T) {
		biz, _, _, budget, cleanup := setupBizWithBudget(t, 5000000)
		defer cleanup()

		updated, err := biz.Update(budget.ID, 12000000)
		if err != nil {
			t.Fatalf("更新预算失败: %v", err)
		}
		if updated.AnnualBudget != 12000000 {
			t.Errorf("更新后年度预算应为 12000000, 实际 %d", updated.AnnualBudget)
		}
		if updated.ID != budget.ID {
			t.Errorf("ID 不应改变: 期望 %d, 实际 %d", budget.ID, updated.ID)
		}

		// 验证数据库值
		got, _ := biz.repo.GetByID(budget.ID)
		if got.AnnualBudget != 12000000 {
			t.Errorf("数据库中年度预算应为 12000000, 实际 %d", got.AnnualBudget)
		}
	})

	t.Run("更新不存在的预算记录报错", func(t *testing.T) {
		biz, _, _, cleanup := setupBizTest(t)
		defer cleanup()

		_, err := biz.Update(99999, 10000000)
		if err == nil {
			t.Fatal("更新不存在的记录应返回错误")
		}
		if !strings.Contains(err.Error(), "不存在") {
			t.Errorf("错误消息应包含'不存在', 实际: %v", err)
		}
	})

	t.Run("更新为零预算", func(t *testing.T) {
		biz, _, _, budget, cleanup := setupBizWithBudget(t, 5000000)
		defer cleanup()

		updated, err := biz.Update(budget.ID, 0)
		if err != nil {
			t.Fatalf("更新为零预算失败: %v", err)
		}
		if updated.AnnualBudget != 0 {
			t.Errorf("更新后年度预算应为 0, 实际 %d", updated.AnnualBudget)
		}
	})

	t.Run("更新后其他字段保持不变", func(t *testing.T) {
		biz, _, dept, budget, cleanup := setupBizWithBudget(t, 5000000)
		defer cleanup()

		// 先做一些操作改变冻结和结算
		biz.repo.Freeze(dept.ID, 2026, 1000000)
		biz.repo.Deduct(dept.ID, 2026, 600000)

		// 更新 budget
		updated, err := biz.Update(budget.ID, 8000000)
		if err != nil {
			t.Fatalf("更新失败: %v", err)
		}

		// 验证冻结和已结算未受影响
		if updated.SpentAmount != 600000 {
			t.Errorf("更新后已结算不应改变: 期望 600000, 实际 %d", updated.SpentAmount)
		}
		if updated.FrozenAmount != 400000 {
			t.Errorf("更新后冻结金额不应改变: 期望 400000, 实际 %d", updated.FrozenAmount)
		}
		if updated.DepartmentID != dept.ID {
			t.Errorf("更新后部门ID不应改变")
		}
	})
}

// ============================================================
// GetByID 测试（Biz 层封装）
// ============================================================
func TestBudgetBiz_GetByID(t *testing.T) {
	t.Run("通过ID查询已存在的预算", func(t *testing.T) {
		biz, _, _, budget, cleanup := setupBizWithBudget(t, 15000000)
		defer cleanup()

		got, err := biz.GetByID(budget.ID)
		if err != nil {
			t.Fatalf("查询失败: %v", err)
		}
		if got.AnnualBudget != 15000000 {
			t.Errorf("年度预算不匹配")
		}
	})

	t.Run("查询不存在的ID返回错误", func(t *testing.T) {
		biz, _, _, cleanup := setupBizTest(t)
		defer cleanup()

		_, err := biz.GetByID(99999)
		if err == nil {
			t.Fatal("查询不存在的记录应返回错误")
		}
		if !strings.Contains(err.Error(), "不存在") {
			t.Errorf("错误消息应包含'不存在', 实际: %v", err)
		}
	})
}
