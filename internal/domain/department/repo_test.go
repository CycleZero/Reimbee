package department

import (
	"strings"
	"sync"
	"testing"

	"github.com/CycleZero/Reimbee/infra"
	"github.com/CycleZero/Reimbee/internal/testutil"
	"github.com/CycleZero/Reimbee/model"
)

// setupRepoTest 创建测试用的 DepartmentRepo、infra.Data 和清理函数
func setupRepoTest(t *testing.T) (*DepartmentRepo, *infra.Data, func()) {
	t.Helper()
	data := testutil.NewTestData()
	testutil.CleanDB(data)
	repo := NewDepartmentRepo(data)
	return repo, data, func() {
		testutil.CleanDB(data)
	}
}

// TestDepartmentRepo_Create 测试部门的创建操作
func TestDepartmentRepo_Create(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(data *infra.Data)
		dept    *model.Department
		wantErr bool
		errMsg  string
	}{
		{
			name: "成功创建部门",
			dept: &model.Department{Name: "技术部"},
		},
		{
			name: "创建重名部门失败-唯一索引冲突",
			setup: func(data *infra.Data) {
				testutil.SeedDepartment(data, "财务部")
			},
			dept:    &model.Department{Name: "财务部"},
			wantErr: true,
			errMsg:  "UNIQUE",
		},
		{
			name: "创建空名称部门-允许空字符串",
			dept: &model.Department{Name: ""},
		},
		{
			name: "创建带主管ID的部门",
			dept: &model.Department{Name: "人事部", ManagerID: uintPtr(1)},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, data, cleanup := setupRepoTest(t)
			defer cleanup()

			if tt.setup != nil {
				tt.setup(data)
			}

			err := repo.Create(tt.dept)

			if tt.wantErr {
				if err == nil {
					t.Errorf("期望创建失败，但成功了")
				} else if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("错误信息不匹配，期望包含 %q，实际: %v", tt.errMsg, err)
				}
			} else {
				if err != nil {
					t.Errorf("创建失败: %v", err)
				} else if tt.dept.ID == 0 {
					t.Errorf("创建成功后 ID 应为非零值")
				}
			}
		})
	}
}

// TestDepartmentRepo_GetByID 测试根据主键 ID 查询部门
func TestDepartmentRepo_GetByID(t *testing.T) {
	tests := []struct {
		name    string
		seed    func(data *infra.Data) uint
		queryID uint
		wantErr bool
		errMsg  string
	}{
		{
			name: "根据存在的ID查询部门",
			seed: func(data *infra.Data) uint {
				return testutil.SeedDepartment(data, "测试部门").ID
			},
		},
		{
			name:    "根据不存在的ID查询部门",
			queryID: 99999,
			wantErr: true,
			errMsg:  "record not found",
		},
		{
			name:    "查询ID为0的部门",
			queryID: 0,
			wantErr: true,
			errMsg:  "record not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, data, cleanup := setupRepoTest(t)
			defer cleanup()

			queryID := tt.queryID
			if tt.seed != nil {
				queryID = tt.seed(data)
			}

			dept, err := repo.GetByID(queryID)

			if tt.wantErr {
				if err == nil {
					t.Errorf("期望查询失败，但成功了，返回: %+v", dept)
				} else if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("错误信息不匹配，期望包含 %q，实际: %v", tt.errMsg, err)
				}
			} else {
				if err != nil {
					t.Fatalf("查询失败: %v", err)
				}
				if dept == nil {
					t.Fatalf("查询返回了 nil 部门")
				}
				if dept.ID != queryID {
					t.Errorf("返回的部门 ID 不匹配，期望 %d，实际 %d", queryID, dept.ID)
				}
			}
		})
	}
}

// TestDepartmentRepo_GetByName 测试根据名称查询部门
func TestDepartmentRepo_GetByName(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(data *infra.Data)
		queryName string
		wantErr   bool
		errMsg    string
	}{
		{
			name: "根据名称查询存在的部门",
			setup: func(data *infra.Data) {
				testutil.SeedDepartment(data, "产品部")
			},
			queryName: "产品部",
		},
		{
			name:      "查询不存在的部门名称",
			queryName: "不存在的部门",
			wantErr:   true,
			errMsg:    "record not found",
		},
		{
			name: "查询空名称-无匹配应返回错误",
			setup: func(data *infra.Data) {
				testutil.SeedDepartment(data, "市场部")
			},
			queryName: "",
			wantErr:   true,
			errMsg:    "record not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, data, cleanup := setupRepoTest(t)
			defer cleanup()

			if tt.setup != nil {
				tt.setup(data)
			}

			dept, err := repo.GetByName(tt.queryName)

			if tt.wantErr {
				if err == nil {
					t.Errorf("期望查询失败，但成功了，返回: %+v", dept)
				} else if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("错误信息不匹配，期望包含 %q，实际: %v", tt.errMsg, err)
				}
			} else {
				if err != nil {
					t.Fatalf("查询失败: %v", err)
				}
				if dept == nil {
					t.Fatalf("查询返回了 nil 部门")
				}
				if dept.Name != tt.queryName {
					t.Errorf("返回的部门名称不匹配，期望 %q，实际 %q", tt.queryName, dept.Name)
				}
			}
		})
	}
}

// TestDepartmentRepo_List 测试分页查询部门列表
func TestDepartmentRepo_List(t *testing.T) {
	tests := []struct {
		name      string
		seedCount int
		page      int
		pageSize  int
		wantCount int
		wantTotal int64
		wantErr   bool
	}{
		{
			name: "空数据库分页查询",
			page: 1, pageSize: 10,
			wantCount: 0, wantTotal: 0,
		},
		{
			name: "单条数据分页查询",
			seedCount: 1, page: 1, pageSize: 10,
			wantCount: 1, wantTotal: 1,
		},
		{
			name: "多条数据第一页查询",
			seedCount: 10, page: 1, pageSize: 4,
			wantCount: 4, wantTotal: 10,
		},
		{
			name: "多条数据第二页查询",
			seedCount: 10, page: 2, pageSize: 4,
			wantCount: 4, wantTotal: 10,
		},
		{
			name: "多条数据最后一页查询",
			seedCount: 10, page: 3, pageSize: 4,
			wantCount: 2, wantTotal: 10,
		},
		{
			name: "超出范围的页码-返回空列表",
			seedCount: 5, page: 10, pageSize: 10,
			wantCount: 0, wantTotal: 5,
		},
		{
			name: "每页数量大于总数",
			seedCount: 3, page: 1, pageSize: 100,
			wantCount: 3, wantTotal: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, data, cleanup := setupRepoTest(t)
			defer cleanup()

			for i := 0; i < tt.seedCount; i++ {
				name := "部门" + string(rune('A'+i%26)) + string(rune('0'+i/26))
				testutil.SeedDepartment(data, name)
			}

			depts, total, err := repo.List(tt.page, tt.pageSize)

			if tt.wantErr {
				if err == nil {
					t.Errorf("期望查询失败，但成功了")
				}
				return
			}

			if err != nil {
				t.Fatalf("查询失败: %v", err)
			}

			if len(depts) != tt.wantCount {
				t.Errorf("返回条数不匹配，期望 %d，实际 %d", tt.wantCount, len(depts))
			}
			if total != tt.wantTotal {
				t.Errorf("总数不匹配，期望 %d，实际 %d", tt.wantTotal, total)
			}

			// 验证按 ID ASC 排序
			for i := 1; i < len(depts); i++ {
				if depts[i].ID <= depts[i-1].ID {
					t.Errorf("结果未按 ID 升序排列: depts[%d].ID=%d, depts[%d].ID=%d",
						i-1, depts[i-1].ID, i, depts[i].ID)
				}
			}
		})
	}
}

// TestDepartmentRepo_Update 测试更新部门信息
func TestDepartmentRepo_Update(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(data *infra.Data, repo *DepartmentRepo) *model.Department
		modify     func(d *model.Department)
		wantErr    bool
		errMsg     string
		verifyName string
	}{
		{
			name: "更新已存在部门的名称",
			setup: func(data *infra.Data, repo *DepartmentRepo) *model.Department {
				return testutil.SeedDepartment(data, "原名称")
			},
			modify: func(d *model.Department) {
				d.Name = "新名称"
			},
			verifyName: "新名称",
		},
		{
			name: "更新已存在部门的ManagerID",
			setup: func(data *infra.Data, repo *DepartmentRepo) *model.Department {
				return testutil.SeedDepartment(data, "设计部")
			},
			modify: func(d *model.Department) {
				d.ManagerID = uintPtr(42)
			},
		},
		{
			name: "将名称更新为另一部门已有的名称-违反唯一约束",
			setup: func(data *infra.Data, repo *DepartmentRepo) *model.Department {
				testutil.SeedDepartment(data, "已存在名称")
				return testutil.SeedDepartment(data, "待更新部门")
			},
			modify: func(d *model.Department) {
				d.Name = "已存在名称"
			},
			wantErr: true,
			errMsg:  "UNIQUE",
		},
		{
			name: "零值ID的Save会创建新记录而非更新",
			setup: func(data *infra.Data, repo *DepartmentRepo) *model.Department {
				return &model.Department{Name: "零ID部门"}
			},
			modify: func(d *model.Department) {
				d.Name = "修改后的名称"
			},
			verifyName: "修改后的名称",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, data, cleanup := setupRepoTest(t)
			defer cleanup()

			d := tt.setup(data, repo)
			tt.modify(d)

			err := repo.Update(d)

			if tt.wantErr {
				if err == nil {
					t.Errorf("期望更新失败，但成功了")
				} else if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("错误信息不匹配，期望包含 %q，实际: %v", tt.errMsg, err)
				}
			} else {
				if err != nil {
					t.Errorf("更新失败: %v", err)
				}
			}

			if tt.verifyName != "" && !tt.wantErr && d.ID > 0 {
				dept, getErr := repo.GetByID(d.ID)
				if getErr != nil {
					t.Fatalf("验证查询失败: %v", getErr)
				}
				if dept.Name != tt.verifyName {
					t.Errorf("更新后名称不匹配，期望 %q，实际 %q", tt.verifyName, dept.Name)
				}
			}
		})
	}
}

// TestDepartmentRepo_Delete 测试删除部门
func TestDepartmentRepo_Delete(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(data *infra.Data) uint
		wantErr    bool
		errMsg     string
		verifyGone bool
	}{
		{
			name: "成功删除无关联数据的部门",
			setup: func(data *infra.Data) uint {
				return testutil.SeedDepartment(data, "要删除的部门").ID
			},
			verifyGone: true,
		},
		{
			name: "删除有员工的部门被拒绝",
			setup: func(data *infra.Data) uint {
				d := testutil.SeedDepartment(data, "有员工的部门")
				testutil.SeedEmployee(data, "E001", "张三", d.ID, false)
				return d.ID
			},
			wantErr: true,
			errMsg:  "该部门下仍有员工，无法删除",
		},
		{
			name: "删除有预算的部门被拒绝",
			setup: func(data *infra.Data) uint {
				d := testutil.SeedDepartment(data, "有预算的部门")
				testutil.SeedBudget(data, d.ID, 2025, 100000)
				return d.ID
			},
			wantErr: true,
			errMsg:  "该部门下仍有预算记录，无法删除",
		},
		{
			name: "删除有员工和预算的部门-员工优先检测",
			setup: func(data *infra.Data) uint {
				d := testutil.SeedDepartment(data, "复杂部门")
				testutil.SeedEmployee(data, "E002", "李四", d.ID, false)
				testutil.SeedBudget(data, d.ID, 2025, 50000)
				return d.ID
			},
			wantErr: true,
			errMsg:  "该部门下仍有员工，无法删除",
		},
		{
			name: "删除不存在的ID-GORM不报错",
			setup: func(data *infra.Data) uint {
				return 99999
			},
		},
		{
			name: "删除ID为0-GORM不报错",
			setup: func(data *infra.Data) uint {
				return 0
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, data, cleanup := setupRepoTest(t)
			defer cleanup()

			deptID := tt.setup(data)

			err := repo.Delete(deptID)

			if tt.wantErr {
				if err == nil {
					t.Errorf("期望删除失败，但成功了")
				} else if err.Error() != tt.errMsg {
					t.Errorf("错误信息不匹配，期望 %q，实际: %v", tt.errMsg, err)
				}
			} else {
				if err != nil {
					t.Errorf("删除失败: %v", err)
				}
			}

			if tt.verifyGone {
				_, getErr := repo.GetByID(deptID)
				if getErr == nil {
					t.Errorf("删除后部门仍存在（ID=%d），GetByID 应返回错误", deptID)
				}
			}
		})
	}
}

// TestDepartmentRepo_GetByID_PreloadManager 测试 GetByID 预加载 Manager 关联
func TestDepartmentRepo_GetByID_PreloadManager(t *testing.T) {
	repo, data, cleanup := setupRepoTest(t)
	defer cleanup()

	d := testutil.SeedDepartment(data, "有主管的部门")
	manager := testutil.SeedEmployee(data, "M001", "王主管", d.ID, true)

	d.ManagerID = &manager.ID
	if err := repo.Update(d); err != nil {
		t.Fatalf("更新部门主管失败: %v", err)
	}

	dept, err := repo.GetByID(d.ID)
	if err != nil {
		t.Fatalf("查询部门失败: %v", err)
	}

	if dept.Manager == nil {
		t.Errorf("期望 Manager 被预加载，但为 nil")
	} else if dept.Manager.EmployeeID != "M001" {
		t.Errorf("Manager 工号不匹配，期望 'M001'，实际 %q", dept.Manager.EmployeeID)
	}
}

// TestDepartmentRepo_ConcurrentCreate 测试并发创建部门
// 注意：SQLite :memory: 模式中每个连接是独立数据库，需限制最大连接数为 1
// 以确保所有 goroutine 共享同一份数据。
func TestDepartmentRepo_ConcurrentCreate(t *testing.T) {
	repo, data, cleanup := setupRepoTest(t)
	defer cleanup()

	// SQLite :memory: 限制为单连接，避免每个 goroutine 操作独立的空数据库
	sqlDB, err := data.DB.DB()
	if err != nil {
		t.Fatalf("获取底层 sql.DB 失败: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)

	const goroutines = 10
	var wg sync.WaitGroup
	errCh := make(chan error, goroutines)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			name := "并发部门" + string(rune('A'+idx))
			d := &model.Department{Name: name}
			if err := repo.Create(d); err != nil {
				errCh <- err
			}
		}(i)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("并发创建失败: %v", err)
	}

	_, total, err := repo.List(1, 100)
	if err != nil {
		t.Fatalf("查询列表失败: %v", err)
	}
	if int(total) != goroutines {
		t.Errorf("并发创建后总数不匹配，期望 %d，实际 %d", goroutines, total)
	}
}

// TestDepartmentRepo_ConcurrentDelete 测试并发删除不同部门
func TestDepartmentRepo_ConcurrentDelete(t *testing.T) {
	repo, data, cleanup := setupRepoTest(t)
	defer cleanup()

	// SQLite :memory: 限制为单连接
	sqlDB, err := data.DB.DB()
	if err != nil {
		t.Fatalf("获取底层 sql.DB 失败: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)

	const goroutines = 8
	ids := make([]uint, goroutines)

	for i := 0; i < goroutines; i++ {
		name := "删除测试" + string(rune('A'+i))
		d := testutil.SeedDepartment(data, name)
		ids[i] = d.ID
	}

	var wg sync.WaitGroup
	errCh := make(chan error, goroutines)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id uint) {
			defer wg.Done()
			if err := repo.Delete(id); err != nil {
				errCh <- err
			}
		}(ids[i])
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("并发删除失败: %v", err)
	}

	_, total, err := repo.List(1, 100)
	if err != nil {
		t.Fatalf("查询列表失败: %v", err)
	}
	if total != 0 {
		t.Errorf("并发删除后仍有 %d 条记录残留", total)
	}
}

// uintPtr 返回 uint 指针，用于构造可选的 ManagerID
func uintPtr(v uint) *uint {
	return &v
}
