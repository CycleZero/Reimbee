package budget

import (
	"fmt"
	"sync"
	"testing"

	"github.com/CycleZero/Reimbee/infra"
	"github.com/CycleZero/Reimbee/internal/testutil"
	"github.com/CycleZero/Reimbee/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// 辅助函数：创建 BudgetRepo（带 infra.Data），返回 repo、data、第一个部门、清理函数
func setupRepoTest(t *testing.T) (*BudgetRepo, *infra.Data, *model.Department, func()) {
	t.Helper()
	data := testutil.NewTestData()
	repo := NewBudgetRepo(data)
	dept := testutil.SeedDepartment(data, "测试部门")

	cleanup := func() {
		testutil.CleanDB(data)
	}
	return repo, data, dept, cleanup
}

// 辅助函数：创建已有一个预算记录的测试环境
func setupRepoWithBudget(t *testing.T, annualBudget int64) (*BudgetRepo, *infra.Data, *model.Department, *model.DepartmentBudget, func()) {
	t.Helper()
	repo, data, dept, cleanup := setupRepoTest(t)
	budget := testutil.SeedBudget(data, dept.ID, 2026, annualBudget)
	return repo, data, dept, budget, cleanup
}

// ============================================================
// Create 测试
// ============================================================
func TestBudgetRepo_Create(t *testing.T) {
	t.Run("正常创建预算记录", func(t *testing.T) {
		repo, _, dept, cleanup := setupRepoTest(t)
		defer cleanup()

		b := &model.DepartmentBudget{
			DepartmentID: dept.ID,
			FiscalYear:   2026,
			AnnualBudget: 10000000, // 10万元(分)
		}
		err := repo.Create(b)
		if err != nil {
			t.Fatalf("创建预算失败: %v", err)
		}
		if b.ID == 0 {
			t.Fatal("创建后预算 ID 应为非零值")
		}

		got, err := repo.GetByID(b.ID)
		if err != nil {
			t.Fatalf("查询已创建的预算失败: %v", err)
		}
		if got.AnnualBudget != 10000000 {
			t.Errorf("年度预算不匹配: 期望 %d, 实际 %d", 10000000, got.AnnualBudget)
		}
		if got.SpentAmount != 0 {
			t.Errorf("新建预算的已结算金额应为 0, 实际 %d", got.SpentAmount)
		}
		if got.FrozenAmount != 0 {
			t.Errorf("新建预算的冻结金额应为 0, 实际 %d", got.FrozenAmount)
		}
	})

	t.Run("创建预算时SpentAmount和FrozenAmount默认为零", func(t *testing.T) {
		repo, _, dept, cleanup := setupRepoTest(t)
		defer cleanup()

		b := &model.DepartmentBudget{
			DepartmentID: dept.ID,
			FiscalYear:   2025,
			AnnualBudget: 5000000,
		}
		if err := repo.Create(b); err != nil {
			t.Fatalf("创建预算失败: %v", err)
		}
		got, _ := repo.GetByID(b.ID)
		if got.SpentAmount != 0 || got.FrozenAmount != 0 {
			t.Errorf("SpentAmount=%d, FrozenAmount=%d, 期望均为 0", got.SpentAmount, got.FrozenAmount)
		}
	})
}

// ============================================================
// GetByID 测试
// ============================================================
func TestBudgetRepo_GetByID(t *testing.T) {
	t.Run("通过ID查询存在的预算记录并验证Department预加载", func(t *testing.T) {
		repo, _, _, budget, cleanup := setupRepoWithBudget(t, 20000000)
		defer cleanup()

		got, err := repo.GetByID(budget.ID)
		if err != nil {
			t.Fatalf("查询预算失败: %v", err)
		}
		if got.ID != budget.ID {
			t.Errorf("ID 不匹配: 期望 %d, 实际 %d", budget.ID, got.ID)
		}
		if got.Department == nil {
			t.Error("预加载的 Department 不应为 nil")
		} else if got.Department.Name != "测试部门" {
			t.Errorf("部门名称不匹配: 期望 '测试部门', 实际 '%s'", got.Department.Name)
		}
	})

	t.Run("通过不存在的ID查询返回错误", func(t *testing.T) {
		repo, _, _, cleanup := setupRepoTest(t)
		defer cleanup()

		_, err := repo.GetByID(99999)
		if err == nil {
			t.Fatal("查询不存在的记录应返回错误")
		}
	})
}

// ============================================================
// GetByDepartmentAndYear 测试
// ============================================================
func TestBudgetRepo_GetByDepartmentAndYear(t *testing.T) {
	tests := []struct {
		name       string
		queryYear  int
		wantErr    bool
		wantBudget int64
	}{
		{
			name:       "查询存在的部门+财年组合",
			queryYear:  2026,
			wantErr:    false,
			wantBudget: 15000000,
		},
		{
			name:       "查询不存在的财年",
			queryYear:  2025,
			wantErr:    true,
			wantBudget: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, _, dept, _, cleanup := setupRepoWithBudget(t, 15000000)
			defer cleanup()

			got, err := repo.GetByDepartmentAndYear(dept.ID, tt.queryYear)
			if tt.wantErr {
				if err == nil {
					t.Fatal("期望返回错误, 实际无错误")
				}
				return
			}
			if err != nil {
				t.Fatalf("查询失败: %v", err)
			}
			if got.AnnualBudget != tt.wantBudget {
				t.Errorf("年度预算不匹配: 期望 %d, 实际 %d", tt.wantBudget, got.AnnualBudget)
			}
		})
	}

	t.Run("查询不存在的部门应返回错误", func(t *testing.T) {
		repo, _, _, cleanup := setupRepoTest(t)
		defer cleanup()

		_, err := repo.GetByDepartmentAndYear(99999, 2026)
		if err == nil {
			t.Fatal("查询不存在的部门应返回错误")
		}
	})
}

// ============================================================
// ListByYear 测试
// ============================================================
func TestBudgetRepo_ListByYear(t *testing.T) {
	t.Run("按财年列出多个部门预算", func(t *testing.T) {
		repo, data, dept, _, cleanup := setupRepoWithBudget(t, 10000000)
		defer cleanup()

		// 创建同部门的第二个财年预算（不应出现在2026查询结果中）
		testutil.SeedBudget(data, dept.ID, 2027, 20000000)

		// 创建另一个部门的2026预算
		dept2 := testutil.SeedDepartment(data, "财务部")
		testutil.SeedBudget(data, dept2.ID, 2026, 30000000)

		budgets, err := repo.ListByYear(2026)
		if err != nil {
			t.Fatalf("查询失败: %v", err)
		}
		if len(budgets) != 2 {
			t.Errorf("期望 2 条记录, 实际 %d", len(budgets))
		}
		for _, b := range budgets {
			if b.FiscalYear != 2026 {
				t.Errorf("预计所有记录都是 2026 年, 但发现 %d", b.FiscalYear)
			}
			if b.Department == nil {
				t.Errorf("预算 ID=%d 的 Department 预加载失败", b.ID)
			}
		}
	})

	t.Run("查询没有预算记录的财年返回空列表", func(t *testing.T) {
		repo, _, _, cleanup := setupRepoTest(t)
		defer cleanup()

		budgets, err := repo.ListByYear(2099)
		if err != nil {
			t.Fatalf("查询失败: %v", err)
		}
		if len(budgets) != 0 {
			t.Errorf("期望空列表, 实际 %d 条", len(budgets))
		}
	})
}

// ============================================================
// Update 测试
// ============================================================
func TestBudgetRepo_Update(t *testing.T) {
	t.Run("更新预算年度预算金额", func(t *testing.T) {
		repo, _, _, budget, cleanup := setupRepoWithBudget(t, 5000000)
		defer cleanup()

		budget.AnnualBudget = 8000000
		if err := repo.Update(budget); err != nil {
			t.Fatalf("更新预算失败: %v", err)
		}

		got, _ := repo.GetByID(budget.ID)
		if got.AnnualBudget != 8000000 {
			t.Errorf("更新后年度预算不匹配: 期望 %d, 实际 %d", 8000000, got.AnnualBudget)
		}
	})

	t.Run("更新已结算和冻结金额", func(t *testing.T) {
		repo, _, _, budget, cleanup := setupRepoWithBudget(t, 5000000)
		defer cleanup()

		budget.SpentAmount = 1000000
		budget.FrozenAmount = 500000
		if err := repo.Update(budget); err != nil {
			t.Fatalf("更新失败: %v", err)
		}

		got, _ := repo.GetByID(budget.ID)
		if got.SpentAmount != 1000000 {
			t.Errorf("已结算金额不匹配: 期望 %d, 实际 %d", 1000000, got.SpentAmount)
		}
		if got.FrozenAmount != 500000 {
			t.Errorf("冻结金额不匹配: 期望 %d, 实际 %d", 500000, got.FrozenAmount)
		}
	})
}

// ============================================================
// Deduct 测试（原子操作: spent_amount+ 同时 frozen_amount-）
// ============================================================
func TestBudgetRepo_Deduct(t *testing.T) {
	t.Run("扣减时已结算增加且冻结金额减少", func(t *testing.T) {
		repo, _, dept, _, cleanup := setupRepoWithBudget(t, 10000000)
		defer cleanup()

		if err := repo.Freeze(dept.ID, 2026, 300000); err != nil {
			t.Fatalf("冻结失败: %v", err)
		}

		if err := repo.Deduct(dept.ID, 2026, 200000); err != nil {
			t.Fatalf("扣减失败: %v", err)
		}

		b, _ := repo.GetByDepartmentAndYear(dept.ID, 2026)
		if b.SpentAmount != 200000 {
			t.Errorf("扣减后已结算金额应为 200000, 实际 %d", b.SpentAmount)
		}
		if b.FrozenAmount != 100000 {
			t.Errorf("扣减后冻结金额应为 100000 (300000-200000), 实际 %d", b.FrozenAmount)
		}
	})

	t.Run("扣减金额超过冻结金额时冻结归零不出现负数", func(t *testing.T) {
		repo, _, dept, _, cleanup := setupRepoWithBudget(t, 10000000)
		defer cleanup()

		if err := repo.Freeze(dept.ID, 2026, 100000); err != nil {
			t.Fatalf("冻结失败: %v", err)
		}

		if err := repo.Deduct(dept.ID, 2026, 200000); err != nil {
			t.Fatalf("扣减失败: %v", err)
		}

		b, _ := repo.GetByDepartmentAndYear(dept.ID, 2026)
		if b.FrozenAmount < 0 {
			t.Errorf("冻结金额不应为负数, 实际 %d", b.FrozenAmount)
		}
		if b.FrozenAmount != 0 {
			t.Errorf("扣减超过冻结后冻结金额应为 0, 实际 %d", b.FrozenAmount)
		}
		if b.SpentAmount != 200000 {
			t.Errorf("扣减后已结算金额应为 200000, 实际 %d", b.SpentAmount)
		}
	})

	t.Run("无冻结时扣减只增加已结算", func(t *testing.T) {
		repo, _, dept, _, cleanup := setupRepoWithBudget(t, 10000000)
		defer cleanup()

		if err := repo.Deduct(dept.ID, 2026, 500000); err != nil {
			t.Fatalf("扣减失败: %v", err)
		}

		b, _ := repo.GetByDepartmentAndYear(dept.ID, 2026)
		if b.SpentAmount != 500000 {
			t.Errorf("扣减后已结算金额应为 500000, 实际 %d", b.SpentAmount)
		}
		if b.FrozenAmount != 0 {
			t.Errorf("无冻结时扣减不应影响冻结金额, 实际 %d", b.FrozenAmount)
		}
	})

	t.Run("多次扣减累计已结算正确", func(t *testing.T) {
		repo, _, dept, _, cleanup := setupRepoWithBudget(t, 50000000)
		defer cleanup()

		amounts := []int64{500000, 300000, 200000}
		for _, amt := range amounts {
			if err := repo.Deduct(dept.ID, 2026, amt); err != nil {
				t.Fatalf("扣减 %d 失败: %v", amt, err)
			}
		}

		b, _ := repo.GetByDepartmentAndYear(dept.ID, 2026)
		expectedSpent := int64(1000000)
		if b.SpentAmount != expectedSpent {
			t.Errorf("累计已结算应为 %d, 实际 %d", expectedSpent, b.SpentAmount)
		}
	})
}

// ============================================================
// Freeze 测试
// ============================================================
func TestBudgetRepo_Freeze(t *testing.T) {
	t.Run("冻结一笔金额", func(t *testing.T) {
		repo, _, dept, _, cleanup := setupRepoWithBudget(t, 10000000)
		defer cleanup()

		if err := repo.Freeze(dept.ID, 2026, 500000); err != nil {
			t.Fatalf("冻结失败: %v", err)
		}

		b, _ := repo.GetByDepartmentAndYear(dept.ID, 2026)
		if b.FrozenAmount != 500000 {
			t.Errorf("冻结金额应为 500000, 实际 %d", b.FrozenAmount)
		}
	})

	t.Run("多次冻结累计正确", func(t *testing.T) {
		repo, _, dept, _, cleanup := setupRepoWithBudget(t, 10000000)
		defer cleanup()

		amounts := []int64{100000, 200000, 300000}
		for _, amt := range amounts {
			if err := repo.Freeze(dept.ID, 2026, amt); err != nil {
				t.Fatalf("冻结 %d 失败: %v", amt, err)
			}
		}

		b, _ := repo.GetByDepartmentAndYear(dept.ID, 2026)
		if b.FrozenAmount != 600000 {
			t.Errorf("累计冻结应为 600000, 实际 %d", b.FrozenAmount)
		}
	})

	t.Run("负金额冻结会导致冻结金额减少", func(t *testing.T) {
		repo, _, dept, _, cleanup := setupRepoWithBudget(t, 10000000)
		defer cleanup()

		if err := repo.Freeze(dept.ID, 2026, -100000); err != nil {
			t.Fatalf("负金额冻结应成功执行 SQL: %v", err)
		}

		b, _ := repo.GetByDepartmentAndYear(dept.ID, 2026)
		if b.FrozenAmount != -100000 {
			t.Errorf("负金额冻结后冻结金额应为 -100000(分), 实际 %d(分)", b.FrozenAmount)
		}
	})
}

// ============================================================
// Unfreeze 测试
// ============================================================
func TestBudgetRepo_Unfreeze(t *testing.T) {
	t.Run("正常解冻部分金额", func(t *testing.T) {
		repo, _, dept, _, cleanup := setupRepoWithBudget(t, 10000000)
		defer cleanup()

		repo.Freeze(dept.ID, 2026, 500000)

		if err := repo.Unfreeze(dept.ID, 2026, 200000); err != nil {
			t.Fatalf("解冻失败: %v", err)
		}

		b, _ := repo.GetByDepartmentAndYear(dept.ID, 2026)
		if b.FrozenAmount != 300000 {
			t.Errorf("解冻后冻结金额应为 300000, 实际 %d", b.FrozenAmount)
		}
	})

	t.Run("解冻金额超过已冻结金额时不出现负数", func(t *testing.T) {
		repo, _, dept, _, cleanup := setupRepoWithBudget(t, 10000000)
		defer cleanup()

		repo.Freeze(dept.ID, 2026, 100000)

		if err := repo.Unfreeze(dept.ID, 2026, 200000); err != nil {
			t.Fatalf("解冻失败: %v", err)
		}

		b, _ := repo.GetByDepartmentAndYear(dept.ID, 2026)
		if b.FrozenAmount < 0 {
			t.Errorf("解冻后冻结金额不应为负数, 实际 %d", b.FrozenAmount)
		}
		if b.FrozenAmount != 0 {
			t.Errorf("过度解冻后冻结金额应为 0, 实际 %d", b.FrozenAmount)
		}
	})

	t.Run("无冻结时解冻保持为零", func(t *testing.T) {
		repo, _, dept, _, cleanup := setupRepoWithBudget(t, 10000000)
		defer cleanup()

		if err := repo.Unfreeze(dept.ID, 2026, 500000); err != nil {
			t.Fatalf("解冻失败: %v", err)
		}

		b, _ := repo.GetByDepartmentAndYear(dept.ID, 2026)
		if b.FrozenAmount != 0 {
			t.Errorf("无冻结时解冻后冻结金额应为 0, 实际 %d", b.FrozenAmount)
		}
	})
}

// ============================================================
// Delete 测试
// ============================================================
func TestBudgetRepo_Delete(t *testing.T) {
	t.Run("删除存在的预算记录后查询返回错误", func(t *testing.T) {
		repo, _, _, budget, cleanup := setupRepoWithBudget(t, 10000000)
		defer cleanup()

		if err := repo.Delete(budget.ID); err != nil {
			t.Fatalf("删除失败: %v", err)
		}

		_, err := repo.GetByID(budget.ID)
		if err == nil {
			t.Fatal("删除后查询应返回错误, 实际无错误")
		}
	})

	t.Run("删除不存在的记录不报错", func(t *testing.T) {
		repo, _, _, cleanup := setupRepoTest(t)
		defer cleanup()

		if err := repo.Delete(99999); err != nil {
			t.Fatalf("删除不存在的记录不应报错: %v", err)
		}
	})
}

// newSharedData 创建使用共享缓存+WAL模式的内存 SQLite
// 设置 MaxOpenConns(1) 使所有并发 goroutine 串行化写入,避免 SQLite 锁冲突
func newSharedData() *infra.Data {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&_journal_mode=WAL"), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
		Logger:                                   logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		panic("创建共享内存测试数据库失败: " + err.Error())
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	if err := db.AutoMigrate(
		&model.Department{},
		&model.DepartmentBudget{},
	); err != nil {
		panic("迁移测试数据库失败: " + err.Error())
	}
	return &infra.Data{DB: db}
}

// ============================================================
// 并发安全测试: 多 goroutine 同时 Freeze
// ============================================================
func TestBudgetRepo_ConcurrentFreeze(t *testing.T) {
	t.Run("10个goroutine并发冻结各100分验证原子性", func(t *testing.T) {
		data := newSharedData()
		repo := NewBudgetRepo(data)
		dept := testutil.SeedDepartment(data, "并发测试部门")
		testutil.SeedBudget(data, dept.ID, 2026, 10000000)
		defer testutil.CleanDB(data)

		const numGoroutines = 10
		const amountPerCall = 100
		expectedTotal := int64(numGoroutines * amountPerCall)

		var wg sync.WaitGroup
		errCh := make(chan error, numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if err := repo.Freeze(dept.ID, 2026, amountPerCall); err != nil {
					errCh <- fmt.Errorf("goroutine冻结失败: %w", err)
				}
			}()
		}
		wg.Wait()
		close(errCh)

		for err := range errCh {
			t.Fatal(err)
		}

		b, _ := repo.GetByDepartmentAndYear(dept.ID, 2026)
		if b.FrozenAmount != expectedTotal {
			t.Errorf("并发冻结后冻结金额应为 %d (10×100), 实际 %d (丢失 %d)",
				expectedTotal, b.FrozenAmount, expectedTotal-b.FrozenAmount)
		}
	})

	t.Run("50个goroutine并发冻结验证高并发原子性", func(t *testing.T) {
		data := newSharedData()
		repo := NewBudgetRepo(data)
		dept := testutil.SeedDepartment(data, "高并发测试部门")
		testutil.SeedBudget(data, dept.ID, 2026, 10000000)
		defer testutil.CleanDB(data)

		const numGoroutines = 50
		const amountPerCall = 200
		expectedTotal := int64(numGoroutines * amountPerCall)

		var wg sync.WaitGroup
		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_ = repo.Freeze(dept.ID, 2026, amountPerCall)
			}()
		}
		wg.Wait()

		b, _ := repo.GetByDepartmentAndYear(dept.ID, 2026)
		if b.FrozenAmount != expectedTotal {
			t.Errorf("50并发后冻结金额应为 %d, 实际 %d (丢失 %d)",
				expectedTotal, b.FrozenAmount, expectedTotal-b.FrozenAmount)
		}
	})
}

// ============================================================
// GORM Expr 原子操作完整流程验证
// ============================================================
func TestBudgetRepo_AtomicOperations(t *testing.T) {
	t.Run("冻结后立即等额解冻回到零", func(t *testing.T) {
		repo, _, dept, _, cleanup := setupRepoWithBudget(t, 10000000)
		defer cleanup()

		repo.Freeze(dept.ID, 2026, 500000)
		repo.Unfreeze(dept.ID, 2026, 500000)

		b, _ := repo.GetByDepartmentAndYear(dept.ID, 2026)
		if b.FrozenAmount != 0 {
			t.Errorf("冻结再等额解冻后冻结金额应为 0, 实际 %d", b.FrozenAmount)
		}
	})

	t.Run("冻结-扣减-解冻完整流程验证状态一致性", func(t *testing.T) {
		repo, _, dept, _, cleanup := setupRepoWithBudget(t, 10000000)
		defer cleanup()

		// 步骤1: 冻结 1000000
		repo.Freeze(dept.ID, 2026, 1000000)
		b, _ := repo.GetByDepartmentAndYear(dept.ID, 2026)
		if b.FrozenAmount != 1000000 {
			t.Fatalf("步骤1 冻结后冻结金额应为 1000000, 实际 %d", b.FrozenAmount)
		}

		// 步骤2: 扣减 600000（冻结自动减 600000）
		repo.Deduct(dept.ID, 2026, 600000)
		b, _ = repo.GetByDepartmentAndYear(dept.ID, 2026)
		if b.SpentAmount != 600000 {
			t.Errorf("步骤2 已结算应为 600000, 实际 %d", b.SpentAmount)
		}
		if b.FrozenAmount != 400000 {
			t.Errorf("步骤2 冻结金额应为 400000, 实际 %d", b.FrozenAmount)
		}

		// 步骤3: 解冻 400000
		repo.Unfreeze(dept.ID, 2026, 400000)
		b, _ = repo.GetByDepartmentAndYear(dept.ID, 2026)
		if b.FrozenAmount != 0 {
			t.Errorf("步骤3 解冻后冻结金额应为 0, 实际 %d", b.FrozenAmount)
		}
		if b.SpentAmount != 600000 {
			t.Errorf("步骤3 解冻不应影响已结算, 期望 600000, 实际 %d", b.SpentAmount)
		}
	})
}
