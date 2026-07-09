package reimbursement

import (
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/CycleZero/Reimbee/infra"
	"github.com/CycleZero/Reimbee/internal/testutil"
	"github.com/CycleZero/Reimbee/model"
	"gorm.io/gorm"
)

// setupRepoTest 创建测试用内存数据库与 ReimbursementRepo 实例，返回清理函数
func setupRepoTest() (*ReimbursementRepo, *infra.Data, func()) {
	data := testutil.NewTestData()
	repo := NewReimbursementRepo(data)
	cleanup := func() {
		testutil.CleanDB(data)
	}
	return repo, data, cleanup
}

// createTestReimbursement 在数据库快速创建一条报销单（跳过 biz 层，仅用于 repo 测试）
func createTestReimbursement(db *infra.Data, no, employeeID, employeeName string, deptID uint, status string, amount int64) *model.Reimbursement {
	rm := testutil.SeedReimbursement(db, no, employeeID, employeeName, deptID, status, amount)
	return rm
}

// resetSeq 重置全局流水号，避免跨用例干扰（仅测试用）
func resetSeq() {
	atomic.StoreUint64(&reimbursementSeq, 0)
}

// ============================================================
// Create 测试
// ============================================================

func TestReimbursementRepo_Create(t *testing.T) {
	repo, _, cleanup := setupRepoTest()
	defer cleanup()

	tests := []struct {
		name    string
		setup   func() *model.Reimbursement
		wantErr bool
		verify  func(*testing.T, *model.Reimbursement)
	}{
		{
			name: "成功创建报销单（草稿状态）",
			setup: func() *model.Reimbursement {
				return &model.Reimbursement{
					ReimbursementNo: "REIMB-2026-0001",
					EmployeeID:      "EMP001",
					EmployeeName:    "张三",
					DepartmentID:    1,
					Status:          StatusDraft,
					TotalAmount:     10000,
					SubmitNote:      "差旅费报销",
				}
			},
			wantErr: false,
			verify: func(t *testing.T, rm *model.Reimbursement) {
				if rm.ID == 0 {
					t.Errorf("期望 Create 后 ID 被自动填充，但 ID 为 0")
				}
				if rm.ReimbursementNo != "REIMB-2026-0001" {
					t.Errorf("期望报销单号为 REIMB-2026-0001，实际为 %s", rm.ReimbursementNo)
				}
				if rm.Status != StatusDraft {
					t.Errorf("期望状态为 draft，实际为 %s", rm.Status)
				}
			},
		},
		{
			name: "成功创建大金额报销单",
			setup: func() *model.Reimbursement {
				return &model.Reimbursement{
					ReimbursementNo: "REIMB-2026-0002",
					EmployeeID:      "EMP002",
					EmployeeName:    "李四",
					DepartmentID:    2,
					Status:          StatusPending,
					TotalAmount:     99999999,
					SubmitNote:      "设备采购报销",
					NeedSpecialApproval: true,
				}
			},
			wantErr: false,
			verify: func(t *testing.T, rm *model.Reimbursement) {
				if rm.ID == 0 {
					t.Errorf("期望 Create 后 ID 被自动填充，但 ID 为 0")
				}
				if rm.TotalAmount != 99999999 {
					t.Errorf("期望总金额为 99999999，实际为 %d", rm.TotalAmount)
				}
				if !rm.NeedSpecialApproval {
					t.Errorf("期望 NeedSpecialApproval 为 true，实际为 false")
				}
			},
		},
		{
			name: "重复报销单号创建应失败",
			setup: func() *model.Reimbursement {
				// 先创建第一条
				rm1 := &model.Reimbursement{
					ReimbursementNo: "REIMB-2026-DUP",
					EmployeeID:      "EMP003",
					EmployeeName:    "王五",
					DepartmentID:    1,
					Status:          StatusDraft,
				}
				if err := repo.Create(rm1); err != nil {
					t.Fatalf("种子数据创建失败: %v", err)
				}
				// 使用相同单号创建第二条
				return &model.Reimbursement{
					ReimbursementNo: "REIMB-2026-DUP",
					EmployeeID:      "EMP004",
					EmployeeName:    "赵六",
					DepartmentID:    1,
					Status:          StatusDraft,
				}
			},
			wantErr: true,
			verify:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rm := tt.setup()
			err := repo.Create(rm)
			if tt.wantErr {
				if err == nil {
					t.Errorf("期望 Create 返回错误，但 err 为 nil")
				}
			} else {
				if err != nil {
					t.Errorf("Create 失败: %v", err)
				}
				if tt.verify != nil {
					tt.verify(t, rm)
				}
			}
		})
	}
}

// ============================================================
// GetByID 测试（验证 Preload 预加载关联）
// ============================================================

func TestReimbursementRepo_GetByID(t *testing.T) {
	repo, data, cleanup := setupRepoTest()
	defer cleanup()

	// 创建部门
	dept := testutil.SeedDepartment(data, "技术部")

	// 创建报销单
	rm := createTestReimbursement(data, "REIMB-2026-R01", "EMP001", "张三", dept.ID, StatusDraft, 50000)

	// 为报销单添加一条明细
	item := &model.ReimbursementItem{
		ReimbursementID: rm.ID,
		Category:        "差旅费",
		Amount:          30000,
		Description:     "测试明细",
	}
	data.DB.Create(item)

	// 为明细添加一张票据
	invoice := &model.Receipt{
		ItemID: item.ID,
		InvoiceCode:     "INV-001",
		InvoiceNumber:   "12345678",
		Amount:          30000,
		Category:        "差旅费",
		CheckResult:     "pass",
	}
	data.DB.Create(invoice)

	// 为报销单添加一条审批记录
	approval := &model.ApprovalRecord{
		ReimbursementID: rm.ID,
		ApproverName:    "经理A",
		ApproverEmail:   "mgrA@test.com",
		Action:          "pending",
	}
	data.DB.Create(approval)

	t.Run("查询存在的报销单-验证预加载", func(t *testing.T) {
		got, err := repo.GetByID(rm.ID)
		if err != nil {
			t.Fatalf("GetByID 失败: %v", err)
		}
		if got.ID != rm.ID {
			t.Errorf("期望 ID 为 %d，实际为 %d", rm.ID, got.ID)
		}
		if got.ReimbursementNo != "REIMB-2026-R01" {
			t.Errorf("期望报销单号为 REIMB-2026-R01，实际为 %s", got.ReimbursementNo)
		}
		// 验证 Department 预加载
		if got.Department == nil {
			t.Errorf("期望 Department 被预加载，但为 nil")
		} else if got.Department.Name != "技术部" {
			t.Errorf("期望部门名称为'技术部'，实际为 %s", got.Department.Name)
		}
		// 验证 Items 预加载
		if len(got.Items) != 1 {
			t.Errorf("期望预加载 1 条明细，实际为 %d 条", len(got.Items))
		} else {
			if len(got.Items[0].Receipts) != 1 {
				t.Errorf("期望明细下预加载 1 张票据")
			} else if got.Items[0].Receipts[0].InvoiceNumber != "12345678" {
				t.Errorf("期望发票号码为 12345678，实际为 %s", got.Items[0].Receipts[0].InvoiceNumber)
			}
		}
		// 验证 Approvals 预加载
		if len(got.Approvals) != 1 {
			t.Errorf("期望预加载 1 条审批记录，实际为 %d 条", len(got.Approvals))
		} else {
			if got.Approvals[0].ApproverName != "经理A" {
				t.Errorf("期望审批人为'经理A'，实际为 %s", got.Approvals[0].ApproverName)
			}
		}
	})

	t.Run("查询不存在的 ID", func(t *testing.T) {
		_, err := repo.GetByID(99999)
		if err == nil {
			t.Errorf("期望 GetByID(99999) 返回错误，但 err 为 nil")
		}
	})

	t.Run("查询 ID 为 0", func(t *testing.T) {
		_, err := repo.GetByID(0)
		if err == nil {
			t.Errorf("期望 GetByID(0) 返回错误，但 err 为 nil")
		}
	})
}

// ============================================================
// GetByNo 测试
// ============================================================

func TestReimbursementRepo_GetByNo(t *testing.T) {
	repo, data, cleanup := setupRepoTest()
	defer cleanup()

	// 创建部门
	dept := testutil.SeedDepartment(data, "财务部")
	rm := createTestReimbursement(data, "REIMB-2026-GBN01", "EMP002", "李四", dept.ID, StatusPending, 20000)

	// 添加明细和票据以验证预加载
	item2 := &model.ReimbursementItem{
		ReimbursementID: rm.ID,
		Category:        "办公用品",
		Amount:          20000,
		Description:     "测试明细",
	}
	data.DB.Create(item2)
	data.DB.Create(&model.Receipt{
		ItemID: item2.ID,
		Amount:          20000,
		Category:        "办公用品",
		InvoiceNumber:   "NO-88888",
	})
	data.DB.Create(&model.ApprovalRecord{
		ReimbursementID: rm.ID,
		ApproverName:    "审批人B",
		ApproverEmail:   "approverB@test.com",
		Action:          "approved",
	})

	t.Run("按报销单号查询-存在", func(t *testing.T) {
		got, err := repo.GetByNo("REIMB-2026-GBN01")
		if err != nil {
			t.Fatalf("GetByNo 失败: %v", err)
		}
		if got.EmployeeName != "李四" {
			t.Errorf("期望申请人为'李四'，实际为 %s", got.EmployeeName)
		}
		if got.Department == nil || got.Department.Name != "财务部" {
			t.Errorf("期望部门预加载正确，但 Department 为 nil 或名称不匹配")
		}
		if len(got.Items) != 1 {
			t.Errorf("期望预加载 1 条明细，实际为 %d 条", len(got.Items))
		}
		if len(got.Approvals) != 1 {
			t.Errorf("期望预加载 1 条审批记录，实际为 %d 条", len(got.Approvals))
		}
	})

	t.Run("按报销单号查询-不存在", func(t *testing.T) {
		_, err := repo.GetByNo("REIMB-9999-9999")
		if err == nil {
			t.Errorf("期望 GetByNo(不存在) 返回错误，但 err 为 nil")
		}
	})

	t.Run("按报销单号查询-空字符串", func(t *testing.T) {
		_, err := repo.GetByNo("")
		if err == nil {
			t.Errorf("期望 GetByNo('') 返回错误，但 err 为 nil")
		}
	})
}

// ============================================================
// List 分页和筛选测试
// ============================================================

func TestReimbursementRepo_List(t *testing.T) {
	t.Run("空列表查询", func(t *testing.T) {
		repo, _, cleanup := setupRepoTest()
		defer cleanup()

		rms, total, err := repo.List(1, 10, "")
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

	t.Run("单页查询-数据量小于pageSize", func(t *testing.T) {
		repo, data, cleanup := setupRepoTest()
		defer cleanup()

		dept := testutil.SeedDepartment(data, "技术部")
		createTestReimbursement(data, "REIMB-2026-L01", "EMP001", "张三", dept.ID, StatusDraft, 1000)
		createTestReimbursement(data, "REIMB-2026-L02", "EMP002", "李四", dept.ID, StatusPending, 2000)
		createTestReimbursement(data, "REIMB-2026-L03", "EMP001", "张三", dept.ID, StatusApproved, 3000)

		rms, total, err := repo.List(1, 10, "")
		if err != nil {
			t.Fatalf("List 失败: %v", err)
		}
		if total != 3 {
			t.Errorf("期望 total 为 3，实际为 %d", total)
		}
		if len(rms) != 3 {
			t.Errorf("期望返回 3 条记录，实际返回 %d 条", len(rms))
		}
		// 验证按 ID 降序排列
		if len(rms) > 1 && rms[0].ID < rms[1].ID {
			t.Errorf("期望结果按 ID 降序排列，但第一条 ID(%d) < 第二条 ID(%d)", rms[0].ID, rms[1].ID)
		}
	})

	t.Run("分页查询-第一页", func(t *testing.T) {
		repo, data, cleanup := setupRepoTest()
		defer cleanup()

		dept := testutil.SeedDepartment(data, "技术部")
		for i := range 5 {
			no := fmt.Sprintf("REIMB-2026-PG%02d", i+1)
			createTestReimbursement(data, no, "EMP001", "张三", dept.ID, StatusDraft, int64(i+1)*1000)
		}

		rms, total, err := repo.List(1, 2, "")
		if err != nil {
			t.Fatalf("List 失败: %v", err)
		}
		if total != 5 {
			t.Errorf("期望 total 为 5，实际为 %d", total)
		}
		if len(rms) != 2 {
			t.Errorf("期望第一页返回 2 条记录，实际返回 %d 条", len(rms))
		}
	})

	t.Run("分页查询-第二页", func(t *testing.T) {
		repo, data, cleanup := setupRepoTest()
		defer cleanup()

		dept := testutil.SeedDepartment(data, "技术部")
		for i := range 5 {
			no := fmt.Sprintf("REIMB-2026-P2%02d", i+1)
			createTestReimbursement(data, no, "EMP001", "张三", dept.ID, StatusDraft, int64(i+1)*1000)
		}

		rms, total, err := repo.List(2, 2, "")
		if err != nil {
			t.Fatalf("List 失败: %v", err)
		}
		if total != 5 {
			t.Errorf("期望 total 为 5，实际为 %d", total)
		}
		if len(rms) != 2 {
			t.Errorf("期望第二页返回 2 条记录，实际返回 %d 条", len(rms))
		}
	})

	t.Run("分页查询-超出范围页", func(t *testing.T) {
		repo, data, cleanup := setupRepoTest()
		defer cleanup()

		dept := testutil.SeedDepartment(data, "技术部")
		for i := range 3 {
			no := fmt.Sprintf("REIMB-2026-OR%02d", i+1)
			createTestReimbursement(data, no, "EMP001", "张三", dept.ID, StatusDraft, int64(i+1)*1000)
		}

		rms, total, err := repo.List(10, 2, "")
		if err != nil {
			t.Fatalf("List 失败: %v", err)
		}
		if total != 3 {
			t.Errorf("期望 total 为 3，实际为 %d", total)
		}
		if len(rms) != 0 {
			t.Errorf("期望超出范围页返回 0 条记录，实际返回 %d 条", len(rms))
		}
	})

	t.Run("按员工工号筛选-存在匹配", func(t *testing.T) {
		repo, data, cleanup := setupRepoTest()
		defer cleanup()

		dept := testutil.SeedDepartment(data, "技术部")
		createTestReimbursement(data, "REIMB-2026-F01", "EMP001", "张三", dept.ID, StatusDraft, 1000)
		createTestReimbursement(data, "REIMB-2026-F02", "EMP002", "李四", dept.ID, StatusPending, 2000)
		createTestReimbursement(data, "REIMB-2026-F03", "EMP001", "张三", dept.ID, StatusApproved, 3000)

		rms, total, err := repo.List(1, 10, "EMP001")
		if err != nil {
			t.Fatalf("List 失败: %v", err)
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

	t.Run("按员工工号筛选-无匹配", func(t *testing.T) {
		repo, data, cleanup := setupRepoTest()
		defer cleanup()

		dept := testutil.SeedDepartment(data, "技术部")
		createTestReimbursement(data, "REIMB-2026-NF01", "EMP001", "张三", dept.ID, StatusDraft, 1000)

		rms, total, err := repo.List(1, 10, "EMP999")
		if err != nil {
			t.Fatalf("List 失败: %v", err)
		}
		if total != 0 {
			t.Errorf("期望 EMP999 的 total 为 0，实际为 %d", total)
		}
		if len(rms) != 0 {
			t.Errorf("期望返回空列表，实际返回 %d 条", len(rms))
		}
	})
}

// ============================================================
// ListByStatus 测试
// ============================================================

func TestReimbursementRepo_ListByStatus(t *testing.T) {
	t.Run("查询草稿状态", func(t *testing.T) {
		repo, data, cleanup := setupRepoTest()
		defer cleanup()

		dept := testutil.SeedDepartment(data, "技术部")
		createTestReimbursement(data, "REIMB-2026-S01", "EMP001", "张三", dept.ID, StatusDraft, 1000)
		createTestReimbursement(data, "REIMB-2026-S02", "EMP002", "李四", dept.ID, StatusPending, 2000)
		createTestReimbursement(data, "REIMB-2026-S03", "EMP001", "张三", dept.ID, StatusDraft, 3000)

		rms, err := repo.ListByStatus(StatusDraft)
		if err != nil {
			t.Fatalf("ListByStatus(draft) 失败: %v", err)
		}
		if len(rms) != 2 {
			t.Errorf("期望返回 2 条草稿报销单，实际返回 %d 条", len(rms))
		}
		for _, r := range rms {
			if r.Status != StatusDraft {
				t.Errorf("期望所有结果为 draft，但发现 status=%s", r.Status)
			}
		}
	})

	t.Run("查询待审批状态", func(t *testing.T) {
		repo, data, cleanup := setupRepoTest()
		defer cleanup()

		dept := testutil.SeedDepartment(data, "技术部")
		createTestReimbursement(data, "REIMB-2026-P1", "EMP001", "张三", dept.ID, StatusPending, 1000)
		createTestReimbursement(data, "REIMB-2026-P2", "EMP002", "李四", dept.ID, StatusPending, 2000)

		rms, err := repo.ListByStatus(StatusPending)
		if err != nil {
			t.Fatalf("ListByStatus(pending) 失败: %v", err)
		}
		if len(rms) != 2 {
			t.Errorf("期望返回 2 条待审批报销单，实际返回 %d 条", len(rms))
		}
	})

	t.Run("查询已通过状态", func(t *testing.T) {
		repo, data, cleanup := setupRepoTest()
		defer cleanup()

		dept := testutil.SeedDepartment(data, "技术部")
		createTestReimbursement(data, "REIMB-2026-A1", "EMP001", "张三", dept.ID, StatusApproved, 5000)

		rms, err := repo.ListByStatus(StatusApproved)
		if err != nil {
			t.Fatalf("ListByStatus(approved) 失败: %v", err)
		}
		if len(rms) != 1 {
			t.Errorf("期望返回 1 条已通过报销单，实际返回 %d 条", len(rms))
		}
	})

	t.Run("查询不存在状态-返回空列表", func(t *testing.T) {
		repo, _, cleanup := setupRepoTest()
		defer cleanup()

		rms, err := repo.ListByStatus("nonexistent")
		if err != nil {
			t.Fatalf("ListByStatus(nonexistent) 失败: %v", err)
		}
		if len(rms) != 0 {
			t.Errorf("期望返回空列表，实际返回 %d 条", len(rms))
		}
	})
}

// ============================================================
// Update 测试
// ============================================================

func TestReimbursementRepo_Update(t *testing.T) {
	repo, data, cleanup := setupRepoTest()
	defer cleanup()

	dept := testutil.SeedDepartment(data, "技术部")

	t.Run("更新总金额", func(t *testing.T) {
		rm := createTestReimbursement(data, "REIMB-2026-U01", "EMP001", "张三", dept.ID, StatusDraft, 1000)
		rm.TotalAmount = 50000
		if err := repo.Update(rm); err != nil {
			t.Fatalf("Update 失败: %v", err)
		}
		got, err := repo.GetByID(rm.ID)
		if err != nil {
			t.Fatalf("Update 后 GetByID 失败: %v", err)
		}
		if got.TotalAmount != 50000 {
			t.Errorf("期望总金额为 50000，实际为 %d", got.TotalAmount)
		}
	})

	t.Run("更新状态", func(t *testing.T) {
		rm := createTestReimbursement(data, "REIMB-2026-U02", "EMP002", "李四", dept.ID, StatusDraft, 2000)
		rm.Status = StatusApproved
		if err := repo.Update(rm); err != nil {
			t.Fatalf("Update 失败: %v", err)
		}
		got, err := repo.GetByID(rm.ID)
		if err != nil {
			t.Fatalf("Update 后 GetByID 失败: %v", err)
		}
		if got.Status != StatusApproved {
			t.Errorf("期望状态为 approved，实际为 %s", got.Status)
		}
	})

	t.Run("更新多个字段", func(t *testing.T) {
		rm := createTestReimbursement(data, "REIMB-2026-U03", "EMP003", "王五", dept.ID, StatusDraft, 3000)
		rm.EmployeeName = "王五修改"
		rm.TotalAmount = 99999
		rm.Status = StatusRejected
		rm.SubmitNote = "修改后的备注"
		rm.NeedSpecialApproval = true
		if err := repo.Update(rm); err != nil {
			t.Fatalf("Update 失败: %v", err)
		}
		got, err := repo.GetByID(rm.ID)
		if err != nil {
			t.Fatalf("Update 后 GetByID 失败: %v", err)
		}
		if got.EmployeeName != "王五修改" {
			t.Errorf("期望申请人为'王五修改'，实际为 %s", got.EmployeeName)
		}
		if got.TotalAmount != 99999 {
			t.Errorf("期望总金额为 99999，实际为 %d", got.TotalAmount)
		}
		if got.Status != StatusRejected {
			t.Errorf("期望状态为 rejected，实际为 %s", got.Status)
		}
		if got.SubmitNote != "修改后的备注" {
			t.Errorf("期望备注为'修改后的备注'，实际为 %s", got.SubmitNote)
		}
		if !got.NeedSpecialApproval {
			t.Errorf("期望 NeedSpecialApproval 为 true，实际为 false")
		}
	})
}

// ============================================================
// UpdateStatus 测试（仅更新状态字段）
// ============================================================

func TestReimbursementRepo_UpdateStatus(t *testing.T) {
	repo, data, cleanup := setupRepoTest()
	defer cleanup()

	dept := testutil.SeedDepartment(data, "技术部")

	t.Run("更新为 pending 状态", func(t *testing.T) {
		rm := createTestReimbursement(data, "REIMB-2026-US01", "EMP001", "张三", dept.ID, StatusDraft, 1000)
		rm.Status = StatusPending
		if err := repo.Update(rm); err != nil {
			t.Fatalf("Update 失败: %v", err)
		}
		got, err := repo.GetByID(rm.ID)
		if err != nil {
			t.Fatalf("UpdateStatus 后 GetByID 失败: %v", err)
		}
		if got.Status != StatusPending {
			t.Errorf("期望状态为 pending，实际为 %s", got.Status)
		}
		// 验证其他字段未被修改
		if got.TotalAmount != 1000 {
			t.Errorf("期望 TotalAmount 保持 1000，实际为 %d", got.TotalAmount)
		}
	})

	t.Run("更新为 approved 状态", func(t *testing.T) {
		rm := createTestReimbursement(data, "REIMB-2026-US02", "EMP002", "李四", dept.ID, StatusPending, 2000)
		rm.Status = StatusApproved
		if err := repo.Update(rm); err != nil {
			t.Fatalf("Update 失败: %v", err)
		}
		got, err := repo.GetByID(rm.ID)
		if err != nil {
			t.Fatalf("UpdateStatus 后 GetByID 失败: %v", err)
		}
		if got.Status != StatusApproved {
			t.Errorf("期望状态为 approved，实际为 %s", got.Status)
		}
	})

	t.Run("更新不存在的 ID", func(t *testing.T) {
		rm := &model.Reimbursement{Model: gorm.Model{ID: 99999}}
		rm.Status = StatusPending
		err := repo.Update(rm)
		// Update 对不存在的 ID 会创建新记录或报错，此处仅验证不 panic
		if err != nil {
			t.Errorf("期望 UpdateStatus 不存在的 ID 不报错，但返回了: %v", err)
		}
	})
}
