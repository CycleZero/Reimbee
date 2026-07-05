package employee

import (
	"testing"

	"github.com/CycleZero/Reimbee/internal/testutil"
	"github.com/CycleZero/Reimbee/log"
	"go.uber.org/zap"
)

// newTestBiz 创建测试用 EmployeeBiz 实例及清理函数
func newTestBiz() (*EmployeeBiz, func()) {
	data := testutil.NewTestData()
	repo := NewEmployeeRepo(data)
	logger := &log.Logger{Logger: zap.NewNop()}
	biz := NewEmployeeBiz(logger, repo)
	cleanup := func() {
		testutil.CleanDB(data)
	}
	return biz, cleanup
}

// ============================================================
// Create 测试
// ============================================================

func TestEmployeeBiz_Create(t *testing.T) {

	t.Run("创建成功-普通员工", func(t *testing.T) {
		biz, cleanup := newTestBiz()
		defer cleanup()

		emp, err := biz.Create("EMP001", "张三", "zhangsan@test.com", 1, "employee")
		if err != nil {
			t.Fatalf("Create 失败: %v", err)
		}
		if emp.EmployeeID != "EMP001" {
			t.Errorf("期望工号为 EMP001，实际为 %s", emp.EmployeeID)
		}
		if emp.Name != "张三" {
			t.Errorf("期望姓名为 张三，实际为 %s", emp.Name)
		}
		if emp.Role != "employee" {
			t.Errorf("期望 role 为 employee，实际为 %s", emp.Role)
		}
		if emp.IsApprover {
			t.Errorf("普通员工 IsApprover 应为 false，实际为 true")
		}
	})

	t.Run("创建成功-审批人角色自动设置IsApprover", func(t *testing.T) {
		biz, cleanup := newTestBiz()
		defer cleanup()

		emp, err := biz.Create("EMP002", "李四", "lisi@test.com", 1, "approver")
		if err != nil {
			t.Fatalf("Create 失败: %v", err)
		}
		if emp.Role != "approver" {
			t.Errorf("期望 role 为 approver，实际为 %s", emp.Role)
		}
		if !emp.IsApprover {
			t.Errorf("approver 角色的 IsApprover 应为 true，实际为 false")
		}
	})

	t.Run("创建成功-管理员角色自动设置IsApprover", func(t *testing.T) {
		biz, cleanup := newTestBiz()
		defer cleanup()

		emp, err := biz.Create("EMP003", "王五", "wangwu@test.com", 2, "admin")
		if err != nil {
			t.Fatalf("Create 失败: %v", err)
		}
		if emp.Role != "admin" {
			t.Errorf("期望 role 为 admin，实际为 %s", emp.Role)
		}
		if !emp.IsApprover {
			t.Errorf("admin 角色的 IsApprover 应为 true，实际为 false")
		}
	})

	t.Run("创建失败-工号重复", func(t *testing.T) {
		biz, cleanup := newTestBiz()
		defer cleanup()

		_, err := biz.Create("EMP001", "张三", "zhangsan@test.com", 1, "employee")
		if err != nil {
			t.Fatalf("首次 Create 失败: %v", err)
		}

		_, err = biz.Create("EMP001", "张三重复", "zhangsan2@test.com", 2, "employee")
		if err == nil {
			t.Errorf("期望重复工号创建返回错误，但 err 为 nil")
		}
	})

	t.Run("创建后可通过ID查询", func(t *testing.T) {
		biz, cleanup := newTestBiz()
		defer cleanup()

		created, err := biz.Create("EMP004", "赵六", "zhaoliu@test.com", 1, "employee")
		if err != nil {
			t.Fatalf("Create 失败: %v", err)
		}

		fetched, err := biz.GetByID(created.ID)
		if err != nil {
			t.Fatalf("GetByID 失败: %v", err)
		}
		if fetched.EmployeeID != "EMP004" {
			t.Errorf("期望工号为 EMP004，实际为 %s", fetched.EmployeeID)
		}
	})
}

// ============================================================
// Role → IsApprover 自动设置测试（表格驱动）
// ============================================================

func TestEmployeeBiz_RoleAutoSetsIsApprover(t *testing.T) {
	tests := []struct {
		name            string
		role            string
		wantIsApprover  bool
	}{
		{
			name:           "角色为approver自动设为审批人",
			role:           "approver",
			wantIsApprover: true,
		},
		{
			name:           "角色为admin自动设为审批人",
			role:           "admin",
			wantIsApprover: true,
		},
		{
			name:           "角色为employee不设为审批人",
			role:           "employee",
			wantIsApprover: false,
		},
		{
			name:           "角色为空字符串不设为审批人",
			role:           "",
			wantIsApprover: false,
		},
		{
			name:           "角色为任意未知值不设为审批人",
			role:           "manager",
			wantIsApprover: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			biz, cleanup := newTestBiz()
			defer cleanup()

			eid := "EMP_" + tt.role
			emp, err := biz.Create(eid, "测试", "test@test.com", 1, tt.role)
			if err != nil {
				t.Fatalf("Create 失败: %v", err)
			}
			if emp.IsApprover != tt.wantIsApprover {
				t.Errorf("角色 %q 期望 IsApprover=%v，实际=%v", tt.role, tt.wantIsApprover, emp.IsApprover)
			}
		})
	}
}

// ============================================================
// GetByID 测试
// ============================================================

func TestEmployeeBiz_GetByID(t *testing.T) {

	t.Run("查询存在的员工", func(t *testing.T) {
		biz, cleanup := newTestBiz()
		defer cleanup()

		created, err := biz.Create("EMP001", "张三", "zhangsan@test.com", 1, "employee")
		if err != nil {
			t.Fatalf("Create 失败: %v", err)
		}

		emp, err := biz.GetByID(created.ID)
		if err != nil {
			t.Errorf("GetByID 失败: %v", err)
		}
		if emp.Name != "张三" {
			t.Errorf("期望姓名为 张三，实际为 %s", emp.Name)
		}
	})

	t.Run("查询不存在的员工", func(t *testing.T) {
		biz, cleanup := newTestBiz()
		defer cleanup()

		_, err := biz.GetByID(99999)
		if err == nil {
			t.Errorf("期望 GetByID 不存在的员工返回错误，但 err 为 nil")
		}
	})

	t.Run("查询ID为0", func(t *testing.T) {
		biz, cleanup := newTestBiz()
		defer cleanup()

		_, err := biz.GetByID(0)
		if err == nil {
			t.Errorf("期望 GetByID(0) 返回错误，但 err 为 nil")
		}
	})
}

// ============================================================
// GetByEmployeeID 测试
// ============================================================

func TestEmployeeBiz_GetByEmployeeID(t *testing.T) {

	t.Run("按工号查询存在的员工", func(t *testing.T) {
		biz, cleanup := newTestBiz()
		defer cleanup()

		_, err := biz.Create("EMP001", "张三", "zhangsan@test.com", 1, "employee")
		if err != nil {
			t.Fatalf("Create 失败: %v", err)
		}

		emp, err := biz.GetByEmployeeID("EMP001")
		if err != nil {
			t.Errorf("GetByEmployeeID 失败: %v", err)
		}
		if emp.Name != "张三" {
			t.Errorf("期望姓名为 张三，实际为 %s", emp.Name)
		}
	})

	t.Run("按工号查询不存在的员工", func(t *testing.T) {
		biz, cleanup := newTestBiz()
		defer cleanup()

		_, err := biz.GetByEmployeeID("EMP999")
		if err == nil {
			t.Errorf("期望 GetByEmployeeID 不存在的工号返回错误，但 err 为 nil")
		}
	})
}

// ============================================================
// List 分页测试
// ============================================================

func TestEmployeeBiz_List(t *testing.T) {

	t.Run("空列表分页查询", func(t *testing.T) {
		biz, cleanup := newTestBiz()
		defer cleanup()

		emps, total, err := biz.List(1, 10)
		if err != nil {
			t.Errorf("List 失败: %v", err)
		}
		if total != 0 {
			t.Errorf("期望 total 为 0，实际为 %d", total)
		}
		if len(emps) != 0 {
			t.Errorf("期望返回空列表，实际返回 %d 条", len(emps))
		}
	})

	t.Run("分页查询5条数据-pageSize=2", func(t *testing.T) {
		biz, cleanup := newTestBiz()
		defer cleanup()

		for i := 0; i < 5; i++ {
			eid := string(rune('A'+i)) + "001"
			name := "员工" + string(rune('A'+i))
			_, err := biz.Create(eid, name, name+"@test.com", 1, "employee")
			if err != nil {
				t.Fatalf("Create(%s) 失败: %v", eid, err)
			}
		}

		// 第一页
		emps1, total1, err := biz.List(1, 2)
		if err != nil {
			t.Errorf("List(1,2) 失败: %v", err)
		}
		if total1 != 5 {
			t.Errorf("期望 total 为 5，实际为 %d", total1)
		}
		if len(emps1) != 2 {
			t.Errorf("期望第一页返回 2 条，实际返回 %d 条", len(emps1))
		}

		// 第三页（最后一页，只剩1条）
		emps3, total3, err := biz.List(3, 2)
		if err != nil {
			t.Errorf("List(3,2) 失败: %v", err)
		}
		if total3 != 5 {
			t.Errorf("期望 total 为 5，实际为 %d", total3)
		}
		if len(emps3) != 1 {
			t.Errorf("期望第三页返回 1 条，实际返回 %d 条", len(emps3))
		}
	})
}

// ============================================================
// ListByDepartment 测试
// ============================================================

func TestEmployeeBiz_ListByDepartment(t *testing.T) {

	t.Run("查询有员工的部门", func(t *testing.T) {
		biz, cleanup := newTestBiz()
		defer cleanup()

		biz.Create("EMP001", "张三", "zhangsan@test.com", 1, "employee")
		biz.Create("EMP002", "李四", "lisi@test.com", 1, "approver")
		biz.Create("EMP003", "王五", "wangwu@test.com", 2, "employee")

		emps, err := biz.ListByDepartment(1)
		if err != nil {
			t.Errorf("ListByDepartment 失败: %v", err)
		}
		if len(emps) != 2 {
			t.Errorf("期望部门 1 返回 2 名员工，实际返回 %d 名", len(emps))
		}
		for _, e := range emps {
			if e.DepartmentID != 1 {
				t.Errorf("期望 department_id 为 1，实际 %s 的为 %d", e.Name, e.DepartmentID)
			}
		}
	})

	t.Run("查询无员工的部门返回空列表", func(t *testing.T) {
		biz, cleanup := newTestBiz()
		defer cleanup()

		biz.Create("EMP001", "张三", "zhangsan@test.com", 1, "employee")

		emps, err := biz.ListByDepartment(999)
		if err != nil {
			t.Errorf("ListByDepartment 失败: %v", err)
		}
		if len(emps) != 0 {
			t.Errorf("期望返回空列表，实际返回 %d 条", len(emps))
		}
	})
}

// ============================================================
// ListApprovers 测试
// ============================================================

func TestEmployeeBiz_ListApprovers(t *testing.T) {

	t.Run("只返回审批人", func(t *testing.T) {
		biz, cleanup := newTestBiz()
		defer cleanup()

		biz.Create("EMP001", "张三", "zhangsan@test.com", 1, "employee")
		biz.Create("EMP002", "李四", "lisi@test.com", 1, "approver")
		biz.Create("EMP003", "王五", "wangwu@test.com", 2, "employee")
		biz.Create("EMP004", "赵六", "zhaoliu@test.com", 2, "admin")

		approvers, err := biz.ListApprovers()
		if err != nil {
			t.Errorf("ListApprovers 失败: %v", err)
		}
		if len(approvers) != 2 {
			t.Errorf("期望返回 2 名审批人，实际返回 %d 名", len(approvers))
		}
		for _, a := range approvers {
			if !a.IsApprover {
				t.Errorf("期望所有返回员工 IsApprover=true，但 %s 为 false", a.Name)
			}
		}
	})

	t.Run("无审批人时返回空列表", func(t *testing.T) {
		biz, cleanup := newTestBiz()
		defer cleanup()

		biz.Create("EMP001", "张三", "zhangsan@test.com", 1, "employee")
		biz.Create("EMP002", "李四", "lisi@test.com", 1, "employee")

		approvers, err := biz.ListApprovers()
		if err != nil {
			t.Errorf("ListApprovers 失败: %v", err)
		}
		if len(approvers) != 0 {
			t.Errorf("期望返回 0 名审批人，实际返回 %d 名", len(approvers))
		}
	})

	t.Run("全部为审批人", func(t *testing.T) {
		biz, cleanup := newTestBiz()
		defer cleanup()

		biz.Create("EMP001", "张三", "zhangsan@test.com", 1, "approver")
		biz.Create("EMP002", "李四", "lisi@test.com", 1, "admin")

		approvers, err := biz.ListApprovers()
		if err != nil {
			t.Errorf("ListApprovers 失败: %v", err)
		}
		if len(approvers) != 2 {
			t.Errorf("期望返回 2 名审批人，实际返回 %d 名", len(approvers))
		}
	})
}

// ============================================================
// Update 测试
// ============================================================

func TestEmployeeBiz_Update(t *testing.T) {

	t.Run("更新成功-基本信息", func(t *testing.T) {
		biz, cleanup := newTestBiz()
		defer cleanup()

		created, err := biz.Create("EMP001", "张三", "zhangsan@test.com", 1, "employee")
		if err != nil {
			t.Fatalf("Create 失败: %v", err)
		}

		updated, err := biz.Update(created.ID, "张三丰", "zhangsanfeng@test.com", 2, "employee")
		if err != nil {
			t.Errorf("Update 失败: %v", err)
		}
		if updated.Name != "张三丰" {
			t.Errorf("期望姓名为 张三丰，实际为 %s", updated.Name)
		}
		if updated.Email != "zhangsanfeng@test.com" {
			t.Errorf("期望邮箱为 zhangsanfeng@test.com，实际为 %s", updated.Email)
		}
		if updated.DepartmentID != 2 {
			t.Errorf("期望部门 ID 为 2，实际为 %d", updated.DepartmentID)
		}
	})

	t.Run("更新成功-角色变更触发IsApprover自动更新", func(t *testing.T) {
		biz, cleanup := newTestBiz()
		defer cleanup()

		created, err := biz.Create("EMP002", "李四", "lisi@test.com", 1, "employee")
		if err != nil {
			t.Fatalf("Create 失败: %v", err)
		}
		if created.IsApprover {
			t.Errorf("创建时 IsApprover 期望为 false，实际为 true")
		}

		// 提升为审批人
		updated, err := biz.Update(created.ID, "李四", "lisi@test.com", 1, "approver")
		if err != nil {
			t.Errorf("Update 失败: %v", err)
		}
		if !updated.IsApprover {
			t.Errorf("角色改为 approver 后 IsApprover 应为 true，实际为 false")
		}

		// 降级为普通员工
		updated2, err := biz.Update(created.ID, "李四", "lisi@test.com", 1, "employee")
		if err != nil {
			t.Errorf("第二次 Update 失败: %v", err)
		}
		if updated2.IsApprover {
			t.Errorf("角色改回 employee 后 IsApprover 应为 false，实际为 true")
		}
	})

	t.Run("更新成功-升为管理员自动设置IsApprover", func(t *testing.T) {
		biz, cleanup := newTestBiz()
		defer cleanup()

		created, err := biz.Create("EMP003", "王五", "wangwu@test.com", 1, "employee")
		if err != nil {
			t.Fatalf("Create 失败: %v", err)
		}

		updated, err := biz.Update(created.ID, "王五", "wangwu@test.com", 1, "admin")
		if err != nil {
			t.Errorf("Update 失败: %v", err)
		}
		if !updated.IsApprover {
			t.Errorf("角色改为 admin 后 IsApprover 应为 true，实际为 false")
		}
	})

	t.Run("更新失败-员工不存在", func(t *testing.T) {
		biz, cleanup := newTestBiz()
		defer cleanup()

		_, err := biz.Update(99999, "不存在", "no@test.com", 1, "employee")
		if err == nil {
			t.Errorf("期望 Update 不存在的员工返回错误，但 err 为 nil")
		}
	})
}

// ============================================================
// Delete 测试
// ============================================================

func TestEmployeeBiz_Delete(t *testing.T) {

	t.Run("删除成功-存在的员工", func(t *testing.T) {
		biz, cleanup := newTestBiz()
		defer cleanup()

		created, err := biz.Create("EMP001", "张三", "zhangsan@test.com", 1, "employee")
		if err != nil {
			t.Fatalf("Create 失败: %v", err)
		}

		if err := biz.Delete(created.ID); err != nil {
			t.Errorf("Delete 失败: %v", err)
		}

		// 确认已删除无法查询
		_, err = biz.GetByID(created.ID)
		if err == nil {
			t.Errorf("期望删除后 GetByID 返回错误，但查到了记录")
		}
	})

	t.Run("删除失败-员工不存在", func(t *testing.T) {
		biz, cleanup := newTestBiz()
		defer cleanup()

		err := biz.Delete(99999)
		if err == nil {
			t.Errorf("期望 Delete 不存在的员工返回错误，但 err 为 nil")
		}
	})

	t.Run("删除后List总数为0", func(t *testing.T) {
		biz, cleanup := newTestBiz()
		defer cleanup()

		created, err := biz.Create("EMP001", "张三", "zhangsan@test.com", 1, "employee")
		if err != nil {
			t.Fatalf("Create 失败: %v", err)
		}

		if err := biz.Delete(created.ID); err != nil {
			t.Errorf("Delete 失败: %v", err)
		}

		_, total, err := biz.List(1, 10)
		if err != nil {
			t.Errorf("List 失败: %v", err)
		}
		if total != 0 {
			t.Errorf("期望删除后 total 为 0，实际为 %d", total)
		}
	})
}

// ============================================================
// 综合流程测试：完整 CRUD 流程
// ============================================================

func TestEmployeeBiz_FullCRUDFlow(t *testing.T) {
	biz, cleanup := newTestBiz()
	defer cleanup()

	// 1. 创建三名员工
	e1, _ := biz.Create("EMP001", "张三", "zhangsan@test.com", 1, "employee")
	e2, _ := biz.Create("EMP002", "李四", "lisi@test.com", 1, "approver")
	biz.Create("EMP003", "王五", "wangwu@test.com", 2, "admin")

	// 2. 列表查询
	emps, total, _ := biz.List(1, 10)
	if total != 3 {
		t.Errorf("期望 total 为 3，实际为 %d", total)
	}
	if len(emps) != 3 {
		t.Errorf("期望返回 3 条，实际为 %d", len(emps))
	}

	// 3. 按部门查询
	dept1Emps, _ := biz.ListByDepartment(1)
	if len(dept1Emps) != 2 {
		t.Errorf("期望部门 1 有 2 人，实际为 %d", len(dept1Emps))
	}

	// 4. 审批人查询
	approvers, _ := biz.ListApprovers()
	if len(approvers) != 2 {
		t.Errorf("期望 2 名审批人，实际为 %d 名", len(approvers))
	}

	// 5. 更新张三
	updated, _ := biz.Update(e1.ID, "张三丰", "zsf@test.com", 3, "approver")
	if updated.Name != "张三丰" || !updated.IsApprover {
		t.Errorf("更新结果不符合预期: name=%s, IsApprover=%v", updated.Name, updated.IsApprover)
	}

	// 6. 更新后审批人应增至 3
	approvers2, _ := biz.ListApprovers()
	if len(approvers2) != 3 {
		t.Errorf("更新后期望 3 名审批人，实际为 %d 名", len(approvers2))
	}

	// 7. 删除李四
	_ = biz.Delete(e2.ID)

	// 8. 删除后审批人应减至 2
	approvers3, _ := biz.ListApprovers()
	if len(approvers3) != 2 {
		t.Errorf("删除后期望 2 名审批人，实际为 %d 名", len(approvers3))
	}

	// 9. 列表应剩 2 条
	_, total2, _ := biz.List(1, 10)
	if total2 != 2 {
		t.Errorf("期望删除后 total 为 2，实际为 %d", total2)
	}
}
