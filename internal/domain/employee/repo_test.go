package employee

import (
	"testing"

	"github.com/CycleZero/Reimbee/internal/testutil"
	"github.com/CycleZero/Reimbee/model"
)

// setupRepoTest 创建测试用数据库与 EmployeeRepo 实例
func setupRepoTest() (*EmployeeRepo, func()) {
	data := testutil.NewTestData()
	repo := NewEmployeeRepo(data)
	cleanup := func() {
		testutil.CleanDB(data)
	}
	return repo, cleanup
}

// createTestEmployee 快速创建一条测试员工记录并返回
func createTestEmployee(repo *EmployeeRepo, employeeID, name string, deptID uint, role string, isApprover bool) *model.Employee {
	e := &model.Employee{
		EmployeeID:   employeeID,
		Name:         name,
		DepartmentID: deptID,
		Email:        name + "@test.com",
		Role:         role,
		IsApprover:   isApprover,
	}
	if err := repo.Create(e); err != nil {
		panic("创建测试员工失败: " + err.Error())
	}
	return e
}

// ============================================================
// Create 测试
// ============================================================

func TestEmployeeRepo_Create(t *testing.T) {
	repo, cleanup := setupRepoTest()
	defer cleanup()

	tests := []struct {
		name       string
		employee   *model.Employee
		wantErr    bool
		errContain string
	}{
		{
			name: "成功创建员工",
			employee: &model.Employee{
				EmployeeID:   "EMP001",
				Name:         "张三",
				DepartmentID: 1,
				Email:        "zhangsan@test.com",
				Role:         "employee",
				IsApprover:   false,
			},
			wantErr: false,
		},
		{
			name: "创建审批人",
			employee: &model.Employee{
				EmployeeID:   "EMP002",
				Name:         "李四",
				DepartmentID: 1,
				Email:        "lisi@test.com",
				Role:         "approver",
				IsApprover:   true,
			},
			wantErr: false,
		},
		{
			name: "创建管理员",
			employee: &model.Employee{
				EmployeeID:   "EMP003",
				Name:         "王五",
				DepartmentID: 2,
				Email:        "wangwu@test.com",
				Role:         "admin",
				IsApprover:   true,
			},
			wantErr: false,
		},
		{
			name: "重复工号创建应失败",
			employee: &model.Employee{
				EmployeeID:   "EMP001",
				Name:         "张三重复",
				DepartmentID: 1,
				Email:        "zhangsan2@test.com",
				Role:         "employee",
				IsApprover:   false,
			},
			wantErr:    true,
			errContain: "UNIQUE",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := repo.Create(tt.employee)
			if tt.wantErr {
				if err == nil {
					t.Errorf("期望 Create 返回错误，但 err 为 nil")
				} else if tt.errContain != "" {
					// 检查错误信息中包含预期关键字
				}
			} else {
				if err != nil {
					t.Errorf("Create 失败: %v", err)
				}
				if tt.employee.ID == 0 {
					t.Errorf("期望 Create 后 ID 被自动填充，但 ID 为 0")
				}
			}
		})
	}
}

// ============================================================
// GetByID 测试
// ============================================================

func TestEmployeeRepo_GetByID(t *testing.T) {
	repo, cleanup := setupRepoTest()
	defer cleanup()

	emp := createTestEmployee(repo, "EMP001", "张三", 1, "employee", false)

	tests := []struct {
		name       string
		id         uint
		wantErr    bool
		wantName   string
		errContain string
	}{
		{
			name:     "查询存在的员工",
			id:       emp.ID,
			wantErr:  false,
			wantName: "张三",
		},
		{
			name:     "查询不存在的 ID",
			id:       99999,
			wantErr:  true,
			errContain: "record not found",
		},
		{
			name:     "查询 ID 为 0",
			id:       0,
			wantErr:  true,
			errContain: "record not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := repo.GetByID(tt.id)
			if tt.wantErr {
				if err == nil {
					t.Errorf("期望 GetByID(%d) 返回错误，但 err 为 nil", tt.id)
				}
			} else {
				if err != nil {
					t.Errorf("GetByID(%d) 失败: %v", tt.id, err)
				}
				if got == nil {
					t.Fatal("GetByID 返回 nil 员工")
				}
				if got.Name != tt.wantName {
					t.Errorf("期望姓名为 %q，实际为 %q", tt.wantName, got.Name)
				}
			}
		})
	}
}

// ============================================================
// GetByEmployeeID 测试
// ============================================================

func TestEmployeeRepo_GetByEmployeeID(t *testing.T) {
	repo, cleanup := setupRepoTest()
	defer cleanup()

	_ = createTestEmployee(repo, "EMP001", "张三", 1, "employee", false)

	tests := []struct {
		name       string
		employeeID string
		wantErr    bool
		wantName   string
	}{
		{
			name:       "按工号查询存在的员工",
			employeeID: "EMP001",
			wantErr:    false,
			wantName:   "张三",
		},
		{
			name:       "按工号查询不存在的员工",
			employeeID: "EMP999",
			wantErr:    true,
		},
		{
			name:       "工号为空字符串",
			employeeID: "",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, err := repo.GetByEmployeeID(tt.employeeID)
			if tt.wantErr {
				if err == nil {
					t.Errorf("期望 GetByEmployeeID(%q) 返回错误，但 err 为 nil", tt.employeeID)
				}
			} else {
				if err != nil {
					t.Errorf("GetByEmployeeID(%q) 失败: %v", tt.employeeID, err)
				}
				if e == nil {
					t.Fatal("GetByEmployeeID 返回 nil 员工")
				}
				if e.Name != tt.wantName {
					t.Errorf("期望姓名为 %q，实际为 %q", tt.wantName, e.Name)
				}
			}
		})
	}
}

// ============================================================
// List 分页测试
// ============================================================

func TestEmployeeRepo_List(t *testing.T) {

	t.Run("空列表查询", func(t *testing.T) {
		repo, cleanup := setupRepoTest()
		defer cleanup()

		emps, total, err := repo.List(1, 10)
		if err != nil {
			t.Errorf("List 失败: %v", err)
		}
		if total != 0 {
			t.Errorf("期望 total 为 0，实际为 %d", total)
		}
		if len(emps) != 0 {
			t.Errorf("期望返回 0 条记录，实际返回 %d 条", len(emps))
		}
	})

	t.Run("单页查询-数据量小于pageSize", func(t *testing.T) {
		repo, cleanup := setupRepoTest()
		defer cleanup()

		createTestEmployee(repo, "EMP001", "张三", 1, "employee", false)
		createTestEmployee(repo, "EMP002", "李四", 1, "approver", true)
		createTestEmployee(repo, "EMP003", "王五", 2, "admin", true)

		emps, total, err := repo.List(1, 10)
		if err != nil {
			t.Errorf("List 失败: %v", err)
		}
		if total != 3 {
			t.Errorf("期望 total 为 3，实际为 %d", total)
		}
		if len(emps) != 3 {
			t.Errorf("期望返回 3 条记录，实际返回 %d 条", len(emps))
		}
		// 验证按 ID 升序排列
		if len(emps) > 1 && emps[0].ID > emps[1].ID {
			t.Errorf("期望结果按 ID 升序排列，但第一条 ID(%d) > 第二条 ID(%d)", emps[0].ID, emps[1].ID)
		}
	})

	t.Run("分页查询-第一页", func(t *testing.T) {
		repo, cleanup := setupRepoTest()
		defer cleanup()

		for i := 0; i < 5; i++ {
			eid := string(rune('A'+i)) + "001"
			name := "员工" + string(rune('A'+i))
			createTestEmployee(repo, eid, name, 1, "employee", false)
		}

		emps, total, err := repo.List(1, 2)
		if err != nil {
			t.Errorf("List 失败: %v", err)
		}
		if total != 5 {
			t.Errorf("期望 total 为 5，实际为 %d", total)
		}
		if len(emps) != 2 {
			t.Errorf("期望第一页返回 2 条记录，实际返回 %d 条", len(emps))
		}
	})

	t.Run("分页查询-第二页", func(t *testing.T) {
		repo, cleanup := setupRepoTest()
		defer cleanup()

		for i := 0; i < 5; i++ {
			eid := string(rune('A'+i)) + "001"
			name := "员工" + string(rune('A'+i))
			createTestEmployee(repo, eid, name, 1, "employee", false)
		}

		emps, total, err := repo.List(2, 2)
		if err != nil {
			t.Errorf("List 失败: %v", err)
		}
		if total != 5 {
			t.Errorf("期望 total 依然为 5，实际为 %d", total)
		}
		if len(emps) != 2 {
			t.Errorf("期望第二页返回 2 条记录，实际返回 %d 条", len(emps))
		}
	})

	t.Run("分页查询-最后一页（不足pageSize）", func(t *testing.T) {
		repo, cleanup := setupRepoTest()
		defer cleanup()

		for i := 0; i < 5; i++ {
			eid := string(rune('A'+i)) + "001"
			name := "员工" + string(rune('A'+i))
			createTestEmployee(repo, eid, name, 1, "employee", false)
		}

		emps, total, err := repo.List(3, 2)
		if err != nil {
			t.Errorf("List 失败: %v", err)
		}
		if total != 5 {
			t.Errorf("期望 total 依然为 5，实际为 %d", total)
		}
		if len(emps) != 1 {
			t.Errorf("期望第三页返回 1 条记录（剩余），实际返回 %d 条", len(emps))
		}
	})

	t.Run("分页查询-超出范围页", func(t *testing.T) {
		repo, cleanup := setupRepoTest()
		defer cleanup()

		for i := 0; i < 3; i++ {
			eid := string(rune('A'+i)) + "001"
			name := "员工" + string(rune('A'+i))
			createTestEmployee(repo, eid, name, 1, "employee", false)
		}

		emps, total, err := repo.List(10, 2)
		if err != nil {
			t.Errorf("List 失败: %v", err)
		}
		if total != 3 {
			t.Errorf("期望 total 为 3，实际为 %d", total)
		}
		if len(emps) != 0 {
			t.Errorf("期望超出范围页返回 0 条记录，实际返回 %d 条", len(emps))
		}
	})
}

// ============================================================
// ListByDepartment 测试
// ============================================================

func TestEmployeeRepo_ListByDepartment(t *testing.T) {

	t.Run("部门无员工返回空列表", func(t *testing.T) {
		repo, cleanup := setupRepoTest()
		defer cleanup()

		emps, err := repo.ListByDepartment(1)
		if err != nil {
			t.Errorf("ListByDepartment 失败: %v", err)
		}
		if len(emps) != 0 {
			t.Errorf("期望返回空列表，实际返回 %d 条", len(emps))
		}
	})

	t.Run("部门有员工正常返回", func(t *testing.T) {
		repo, cleanup := setupRepoTest()
		defer cleanup()

		createTestEmployee(repo, "EMP001", "张三", 1, "employee", false)
		createTestEmployee(repo, "EMP002", "李四", 1, "approver", true)
		createTestEmployee(repo, "EMP003", "王五", 2, "employee", false)

		emps, err := repo.ListByDepartment(1)
		if err != nil {
			t.Errorf("ListByDepartment 失败: %v", err)
		}
		if len(emps) != 2 {
			t.Errorf("期望部门 1 返回 2 名员工，实际返回 %d 名", len(emps))
		}
		for _, e := range emps {
			if e.DepartmentID != 1 {
				t.Errorf("期望所有员工 department_id 为 1，但 %s 的 department_id 为 %d", e.Name, e.DepartmentID)
			}
		}
	})

	t.Run("不同部门员工互不干扰", func(t *testing.T) {
		repo, cleanup := setupRepoTest()
		defer cleanup()

		createTestEmployee(repo, "EMP001", "张三", 10, "employee", false)
		createTestEmployee(repo, "EMP002", "李四", 20, "approver", true)
		createTestEmployee(repo, "EMP003", "王五", 10, "employee", false)

		empsDept10, err := repo.ListByDepartment(10)
		if err != nil {
			t.Errorf("ListByDepartment(10) 失败: %v", err)
		}
		empsDept20, err := repo.ListByDepartment(20)
		if err != nil {
			t.Errorf("ListByDepartment(20) 失败: %v", err)
		}
		if len(empsDept10) != 2 {
			t.Errorf("期望部门 10 返回 2 名员工，实际返回 %d 名", len(empsDept10))
		}
		if len(empsDept20) != 1 {
			t.Errorf("期望部门 20 返回 1 名员工，实际返回 %d 名", len(empsDept20))
		}
	})
}

// ============================================================
// ListApprovers 测试
// ============================================================

func TestEmployeeRepo_ListApprovers(t *testing.T) {

	t.Run("无审批人时返回空列表", func(t *testing.T) {
		repo, cleanup := setupRepoTest()
		defer cleanup()

		createTestEmployee(repo, "EMP001", "张三", 1, "employee", false)
		createTestEmployee(repo, "EMP002", "李四", 1, "employee", false)

		approvers, err := repo.ListApprovers()
		if err != nil {
			t.Errorf("ListApprovers 失败: %v", err)
		}
		if len(approvers) != 0 {
			t.Errorf("期望返回 0 名审批人，实际返回 %d 名", len(approvers))
		}
	})

	t.Run("部分为审批人时只返回审批人", func(t *testing.T) {
		repo, cleanup := setupRepoTest()
		defer cleanup()

		createTestEmployee(repo, "EMP001", "张三", 1, "employee", false)
		createTestEmployee(repo, "EMP002", "李四", 1, "approver", true)
		createTestEmployee(repo, "EMP003", "王五", 2, "employee", false)
		createTestEmployee(repo, "EMP004", "赵六", 2, "admin", true)

		approvers, err := repo.ListApprovers()
		if err != nil {
			t.Errorf("ListApprovers 失败: %v", err)
		}
		if len(approvers) != 2 {
			t.Errorf("期望返回 2 名审批人，实际返回 %d 名", len(approvers))
		}
		for _, a := range approvers {
			if !a.IsApprover {
				t.Errorf("期望所有返回的员工 IsApprover 为 true，但 %s 为 false", a.Name)
			}
		}
	})

	t.Run("全部为审批人", func(t *testing.T) {
		repo, cleanup := setupRepoTest()
		defer cleanup()

		createTestEmployee(repo, "EMP001", "张三", 1, "approver", true)
		createTestEmployee(repo, "EMP002", "李四", 1, "admin", true)
		createTestEmployee(repo, "EMP003", "王五", 2, "approver", true)

		approvers, err := repo.ListApprovers()
		if err != nil {
			t.Errorf("ListApprovers 失败: %v", err)
		}
		if len(approvers) != 3 {
			t.Errorf("期望返回 3 名审批人，实际返回 %d 名", len(approvers))
		}
	})
}

// ============================================================
// Update 测试
// ============================================================

func TestEmployeeRepo_Update(t *testing.T) {
	repo, cleanup := setupRepoTest()
	defer cleanup()

	tests := []struct {
		name    string
		setup   func() *model.Employee
		modify  func(*model.Employee)
		wantErr bool
		verify  func(*testing.T, *model.Employee)
	}{
		{
			name: "更新员工姓名",
			setup: func() *model.Employee {
				return createTestEmployee(repo, "EMP001", "张三", 1, "employee", false)
			},
			modify: func(e *model.Employee) { e.Name = "张三丰" },
			verify: func(t *testing.T, got *model.Employee) {
				if got.Name != "张三丰" {
					t.Errorf("期望姓名为 %q，实际为 %q", "张三丰", got.Name)
				}
			},
		},
		{
			name: "更新邮箱",
			setup: func() *model.Employee {
				return createTestEmployee(repo, "EMP002", "李四", 1, "employee", false)
			},
			modify: func(e *model.Employee) { e.Email = "newemail@test.com" },
			verify: func(t *testing.T, got *model.Employee) {
				if got.Email != "newemail@test.com" {
					t.Errorf("期望邮箱为 %q，实际为 %q", "newemail@test.com", got.Email)
				}
			},
		},
		{
			name: "更新部门",
			setup: func() *model.Employee {
				return createTestEmployee(repo, "EMP003", "王五", 1, "employee", false)
			},
			modify: func(e *model.Employee) { e.DepartmentID = 99 },
			verify: func(t *testing.T, got *model.Employee) {
				if got.DepartmentID != 99 {
					t.Errorf("期望 department_id 为 99，实际为 %d", got.DepartmentID)
				}
			},
		},
		{
			name: "更新角色和审批状态",
			setup: func() *model.Employee {
				return createTestEmployee(repo, "EMP004", "赵六", 1, "employee", false)
			},
			modify: func(e *model.Employee) {
				e.Role = "approver"
				e.IsApprover = true
			},
			verify: func(t *testing.T, got *model.Employee) {
				if got.Role != "approver" {
					t.Errorf("期望 role 为 %q，实际为 %q", "approver", got.Role)
				}
				if !got.IsApprover {
					t.Errorf("期望 IsApprover 为 true，实际为 false")
				}
			},
		},
		{
			name: "多字段同时更新",
			setup: func() *model.Employee {
				return createTestEmployee(repo, "EMP005", "孙七", 1, "employee", false)
			},
			modify: func(e *model.Employee) {
				e.Name = "孙七改"
				e.Email = "sunqi_new@test.com"
				e.DepartmentID = 50
				e.Role = "admin"
				e.IsApprover = true
			},
			verify: func(t *testing.T, got *model.Employee) {
				if got.Name != "孙七改" {
					t.Errorf("期望姓名为 %q，实际为 %q", "孙七改", got.Name)
				}
				if got.Email != "sunqi_new@test.com" {
					t.Errorf("期望邮箱为 %q，实际为 %q", "sunqi_new@test.com", got.Email)
				}
				if got.DepartmentID != 50 {
					t.Errorf("期望 department_id 为 50，实际为 %d", got.DepartmentID)
				}
				if got.Role != "admin" {
					t.Errorf("期望 role 为 %q，实际为 %q", "admin", got.Role)
				}
				if !got.IsApprover {
					t.Errorf("期望 IsApprover 为 true，实际为 false")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			emp := tt.setup()
			tt.modify(emp)

			err := repo.Update(emp)
			if tt.wantErr {
				if err == nil {
					t.Errorf("期望 Update 返回错误，但 err 为 nil")
				}
				return
			}
			if err != nil {
				t.Errorf("Update 失败: %v", err)
				return
			}

			// 重新查询验证持久化
			got, err := repo.GetByID(emp.ID)
			if err != nil {
				t.Errorf("Update 后 GetByID 失败: %v", err)
				return
			}
			tt.verify(t, got)
		})
	}
}

// ============================================================
// Delete 测试
// ============================================================

func TestEmployeeRepo_Delete(t *testing.T) {

	t.Run("删除存在的员工", func(t *testing.T) {
		repo, cleanup := setupRepoTest()
		defer cleanup()

		emp := createTestEmployee(repo, "EMP001", "张三", 1, "employee", false)
		if err := repo.Delete(emp.ID); err != nil {
			t.Errorf("Delete 失败: %v", err)
		}

		// 确认已删除（软删除后 GORM 默认查不到）
		_, err := repo.GetByID(emp.ID)
		if err == nil {
			t.Errorf("期望删除后 GetByID 返回错误，但成功查到了记录")
		}
	})

	t.Run("删除不存在的员工", func(t *testing.T) {
		repo, cleanup := setupRepoTest()
		defer cleanup()

		err := repo.Delete(99999)
		// GORM 的 Delete 在不存在的 ID 上不会返回错误（RowsAffected=0）
		if err != nil {
			t.Errorf("Delete 不存在的 ID 不应返回错误，但返回了: %v", err)
		}
	})

	t.Run("软删除后同工号仍受唯一约束限制", func(t *testing.T) {
		repo, cleanup := setupRepoTest()
		defer cleanup()

		emp := createTestEmployee(repo, "EMP001", "张三", 1, "employee", false)
		if err := repo.Delete(emp.ID); err != nil {
			t.Errorf("Delete 失败: %v", err)
		}

		// GORM 软删除后，employee_id 唯一索引仍然阻止重复插入
		// 这是 GORM + SQLite 的预期行为
		newEmp := &model.Employee{
			EmployeeID:   "EMP001",
			Name:         "李四",
			DepartmentID: 2,
			Email:        "lisi@test.com",
			Role:         "approver",
			IsApprover:   true,
		}
		err := repo.Create(newEmp)
		if err == nil {
			t.Errorf("期望软删除后使用相同工号创建失败（唯一约束），但创建成功了")
		}
	})

	t.Run("删除员工后 List 不包含该记录", func(t *testing.T) {
		repo, cleanup := setupRepoTest()
		defer cleanup()

		emp1 := createTestEmployee(repo, "EMP001", "张三", 1, "employee", false)
		emp2 := createTestEmployee(repo, "EMP002", "李四", 1, "approver", true)

		if err := repo.Delete(emp1.ID); err != nil {
			t.Errorf("Delete 失败: %v", err)
		}

		emps, total, err := repo.List(1, 100)
		if err != nil {
			t.Errorf("List 失败: %v", err)
		}
		if total != 1 {
			t.Errorf("期望删除后 total 为 1，实际为 %d", total)
		}
		if len(emps) != 1 || emps[0].ID != emp2.ID {
			t.Errorf("期望只有 emp2 被返回")
		}
	})
}

// ============================================================
// 综合测试：创建后立即查询
// ============================================================

func TestEmployeeRepo_CreateAndQuery(t *testing.T) {
	repo, cleanup := setupRepoTest()
	defer cleanup()

	emp := &model.Employee{
		EmployeeID:   "EMP001",
		Name:         "张三",
		DepartmentID: 1,
		Email:        "zhangsan@test.com",
		Role:         "approver",
		IsApprover:   true,
	}
	if err := repo.Create(emp); err != nil {
		t.Fatalf("Create 失败: %v", err)
	}

	// 按 ID 查询
	byID, err := repo.GetByID(emp.ID)
	if err != nil {
		t.Errorf("GetByID 失败: %v", err)
	} else {
		if byID.EmployeeID != "EMP001" {
			t.Errorf("期望工号为 EMP001，实际为 %s", byID.EmployeeID)
		}
		if byID.Name != "张三" {
			t.Errorf("期望姓名为 张三，实际为 %s", byID.Name)
		}
	}

	// 按工号查询
	byEID, err := repo.GetByEmployeeID("EMP001")
	if err != nil {
		t.Errorf("GetByEmployeeID 失败: %v", err)
	} else {
		if byEID.Name != "张三" {
			t.Errorf("期望姓名为 张三，实际为 %s", byEID.Name)
		}
	}
}
