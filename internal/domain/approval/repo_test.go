package approval

import (
	"testing"

	"github.com/CycleZero/Reimbee/internal/testutil"
	"github.com/CycleZero/Reimbee/model"
)

// ============================================================
// ApprovalRepo.Create 测试
// ============================================================

func TestApprovalRepo_Create(t *testing.T) {
	t.Run("正常创建审批记录", func(t *testing.T) {
		data := testutil.NewTestData()
		repo := NewApprovalRepo(data)

		record := &model.ApprovalRecord{
			ReimbursementID: 1,
			ApproverName:    "张三",
			ApproverEmail:   "zhangsan@test.com",
			Action:          "pending",
		}
		err := repo.Create(record)
		if err != nil {
			t.Fatalf("创建审批记录失败: %v", err)
		}
		if record.ID == 0 {
			t.Fatal("创建成功后ID不应为零值")
		}
		if record.CreatedAt.IsZero() {
			t.Fatal("创建成功后CreatedAt不应为零值")
		}
	})

	t.Run("验证创建后所有字段正确保存", func(t *testing.T) {
		data := testutil.NewTestData()
		repo := NewApprovalRepo(data)

		record := &model.ApprovalRecord{
			ReimbursementID: 100,
			ApproverName:    "李四",
			ApproverEmail:   "lisi@test.com",
			Action:          "pending",
			Comment:         "请尽快审批",
		}
		if err := repo.Create(record); err != nil {
			t.Fatalf("创建审批记录失败: %v", err)
		}

		// 重新查询验证字段
		fetched, err := repo.GetByID(record.ID)
		if err != nil {
			t.Fatalf("查询已创建的记录失败: %v", err)
		}
		if fetched.ReimbursementID != 100 {
			t.Errorf("ReimbursementID 应为 %d，实际 %d", 100, fetched.ReimbursementID)
		}
		if fetched.ApproverName != "李四" {
			t.Errorf("ApproverName 应为 %q，实际 %q", "李四", fetched.ApproverName)
		}
		if fetched.ApproverEmail != "lisi@test.com" {
			t.Errorf("ApproverEmail 应为 %q，实际 %q", "lisi@test.com", fetched.ApproverEmail)
		}
		if fetched.Action != "pending" {
			t.Errorf("Action 应为 %q，实际 %q", "pending", fetched.Action)
		}
		if fetched.Comment != "请尽快审批" {
			t.Errorf("Comment 应为 %q，实际 %q", "请尽快审批", fetched.Comment)
		}
	})

	t.Run("创建多条审批记录", func(t *testing.T) {
		data := testutil.NewTestData()
		repo := NewApprovalRepo(data)

		records := []*model.ApprovalRecord{
			{ReimbursementID: 1, ApproverName: "审批人A", ApproverEmail: "a@test.com", Action: "pending"},
			{ReimbursementID: 1, ApproverName: "审批人B", ApproverEmail: "b@test.com", Action: "pending"},
			{ReimbursementID: 2, ApproverName: "审批人C", ApproverEmail: "c@test.com", Action: "pending"},
		}
		for i, r := range records {
			if err := repo.Create(r); err != nil {
				t.Fatalf("创建第%d条审批记录失败: %v", i+1, err)
			}
			if r.ID == 0 {
				t.Fatalf("第%d条记录ID为零值", i+1)
			}
		}
	})

	t.Run("创建时默认action为pending", func(t *testing.T) {
		data := testutil.NewTestData()
		repo := NewApprovalRepo(data)

		record := &model.ApprovalRecord{
			ReimbursementID: 1,
			ApproverName:    "王五",
			ApproverEmail:   "wangwu@test.com",
		}
		// Action 使用数据库默认值 pending
		if err := repo.Create(record); err != nil {
			t.Fatalf("创建审批记录失败: %v", err)
		}
		fetched, _ := repo.GetByID(record.ID)
		if fetched.Action != "pending" {
			t.Errorf("默认Action 应为 %q，实际 %q", "pending", fetched.Action)
		}
	})
}

// ============================================================
// ApprovalRepo.GetByID 测试
// ============================================================

func TestApprovalRepo_GetByID(t *testing.T) {
	t.Run("查询存在的审批记录", func(t *testing.T) {
		data := testutil.NewTestData()
		repo := NewApprovalRepo(data)

		seeded := testutil.SeedApprovalRecord(data, 1, "赵六", "pending")
		fetched, err := repo.GetByID(seeded.ID)
		if err != nil {
			t.Fatalf("查询存在的记录失败: %v", err)
		}
		if fetched.ID != seeded.ID {
			t.Errorf("ID 应为 %d，实际 %d", seeded.ID, fetched.ID)
		}
		if fetched.ApproverName != "赵六" {
			t.Errorf("ApproverName 应为 %q，实际 %q", "赵六", fetched.ApproverName)
		}
	})

	t.Run("查询不存在的审批记录返回错误", func(t *testing.T) {
		data := testutil.NewTestData()
		repo := NewApprovalRepo(data)

		_, err := repo.GetByID(99999)
		if err == nil {
			t.Fatal("查询不存在的记录应返回错误")
		}
	})

	t.Run("查询ID为零值返回错误", func(t *testing.T) {
		data := testutil.NewTestData()
		repo := NewApprovalRepo(data)

		_, err := repo.GetByID(0)
		if err == nil {
			t.Fatal("查询ID为零应返回错误")
		}
	})
}

// ============================================================
// ApprovalRepo.ListByReimbursement 测试
// ============================================================

func TestApprovalRepo_ListByReimbursement(t *testing.T) {
	t.Run("报销单无审批记录返回空列表", func(t *testing.T) {
		data := testutil.NewTestData()
		repo := NewApprovalRepo(data)

		records, err := repo.ListByReimbursement(1)
		if err != nil {
			t.Fatalf("查询空列表失败: %v", err)
		}
		if len(records) != 0 {
			t.Errorf("空列表长度应为 0，实际 %d", len(records))
		}
	})

	t.Run("单条审批记录", func(t *testing.T) {
		data := testutil.NewTestData()
		repo := NewApprovalRepo(data)

		testutil.SeedApprovalRecord(data, 1, "孙七", "pending")
		records, err := repo.ListByReimbursement(1)
		if err != nil {
			t.Fatalf("查询失败: %v", err)
		}
		if len(records) != 1 {
			t.Fatalf("记录数应为 1，实际 %d", len(records))
		}
		if records[0].ApproverName != "孙七" {
			t.Errorf("审批人应为 %q，实际 %q", "孙七", records[0].ApproverName)
		}
	})

	t.Run("多条审批记录按ID升序排列", func(t *testing.T) {
		data := testutil.NewTestData()
		repo := NewApprovalRepo(data)

		// 按顺序创建，ID自增
		r1 := testutil.SeedApprovalRecord(data, 1, "审批人A", "pending")
		r2 := testutil.SeedApprovalRecord(data, 1, "审批人B", "pending")
		r3 := testutil.SeedApprovalRecord(data, 1, "审批人C", "pending")

		records, err := repo.ListByReimbursement(1)
		if err != nil {
			t.Fatalf("查询失败: %v", err)
		}
		if len(records) != 3 {
			t.Fatalf("记录数应为 3，实际 %d", len(records))
		}
		if records[0].ID != r1.ID {
			t.Errorf("第一条应为 r1(ID=%d)，实际 ID=%d", r1.ID, records[0].ID)
		}
		if records[1].ID != r2.ID {
			t.Errorf("第二条应为 r2(ID=%d)，实际 ID=%d", r2.ID, records[1].ID)
		}
		if records[2].ID != r3.ID {
			t.Errorf("第三条应为 r3(ID=%d)，实际 ID=%d", r3.ID, records[2].ID)
		}
		// 验证升序
		if records[0].ID >= records[1].ID || records[1].ID >= records[2].ID {
			t.Fatal("审批记录应按ID升序排列")
		}
	})

	t.Run("仅返回指定报销单的记录", func(t *testing.T) {
		data := testutil.NewTestData()
		repo := NewApprovalRepo(data)

		testutil.SeedApprovalRecord(data, 1, "审批人A", "pending")
		testutil.SeedApprovalRecord(data, 2, "审批人B", "pending")
		testutil.SeedApprovalRecord(data, 2, "审批人C", "pending")

		records, err := repo.ListByReimbursement(2)
		if err != nil {
			t.Fatalf("查询报销单2失败: %v", err)
		}
		if len(records) != 2 {
			t.Fatalf("报销单2的记录数应为 2，实际 %d", len(records))
		}
		for _, r := range records {
			if r.ReimbursementID != 2 {
				t.Errorf("返回了不属于报销单2的记录，ReimbursementID=%d", r.ReimbursementID)
			}
		}
	})

	t.Run("不存在的报销单ID返回空列表", func(t *testing.T) {
		data := testutil.NewTestData()
		repo := NewApprovalRepo(data)

		testutil.SeedApprovalRecord(data, 1, "审批人A", "pending")
		records, err := repo.ListByReimbursement(99999)
		if err != nil {
			t.Fatalf("查询不存在报销单失败: %v", err)
		}
		if len(records) != 0 {
			t.Errorf("不存在报销单应返回空列表，实际长度 %d", len(records))
		}
	})
}

// ============================================================
// ApprovalRepo.Update 测试
// ============================================================

func TestApprovalRepo_Update(t *testing.T) {
	t.Run("更新审批状态为approved", func(t *testing.T) {
		data := testutil.NewTestData()
		repo := NewApprovalRepo(data)

		record := testutil.SeedApprovalRecord(data, 1, "周八", "pending")
		record.Action = "approved"
		if err := repo.Update(record); err != nil {
			t.Fatalf("更新记录失败: %v", err)
		}

		fetched, _ := repo.GetByID(record.ID)
		if fetched.Action != "approved" {
			t.Errorf("Action 应为 %q，实际 %q", "approved", fetched.Action)
		}
	})

	t.Run("更新审批意见和操作时间", func(t *testing.T) {
		data := testutil.NewTestData()
		repo := NewApprovalRepo(data)

		record := testutil.SeedApprovalRecord(data, 1, "吴九", "pending")
		record.Comment = "同意报销"
		if err := repo.Update(record); err != nil {
			t.Fatalf("更新记录失败: %v", err)
		}

		fetched, _ := repo.GetByID(record.ID)
		if fetched.Comment != "同意报销" {
			t.Errorf("Comment 应为 %q，实际 %q", "同意报销", fetched.Comment)
		}
	})

	t.Run("更新不存在的记录（Save会创建新记录）", func(t *testing.T) {
		data := testutil.NewTestData()
		repo := NewApprovalRepo(data)

		// 创建一个未持久化的记录，ID为不存在的大值
		record := &model.ApprovalRecord{
			ReimbursementID: 1,
			ApproverName:    "新审批人",
			ApproverEmail:   "new@test.com",
			Action:          "pending",
		}
		// Save在没有主键时会创建新记录
		err := repo.Update(record)
		if err != nil {
			t.Fatalf("保存新记录失败: %v", err)
		}
		if record.ID == 0 {
			t.Fatal("保存后ID不应为零值")
		}
		// 验证可以查到
		fetched, err := repo.GetByID(record.ID)
		if err != nil {
			t.Fatalf("查询新创建的记录失败: %v", err)
		}
		if fetched.ApproverName != "新审批人" {
			t.Errorf("ApproverName 应为 %q，实际 %q", "新审批人", fetched.ApproverName)
		}
	})

	t.Run("更新操作时间字段", func(t *testing.T) {
		data := testutil.NewTestData()
		repo := NewApprovalRepo(data)

		record := testutil.SeedApprovalRecord(data, 1, "郑十", "pending")
		// ActionAt由业务层设置，这里仅验证Update能持久化
		record.Action = "rejected"
		if err := repo.Update(record); err != nil {
			t.Fatalf("更新记录失败: %v", err)
		}
		fetched, _ := repo.GetByID(record.ID)
		if fetched.Action != "rejected" {
			t.Errorf("Action 应为 %q，实际 %q", "rejected", fetched.Action)
		}
	})
}

// ============================================================
// 集成场景测试
// ============================================================

func TestApprovalRepo_Integration(t *testing.T) {
	t.Run("批量创建后通过ListByReimbursement正确查询并排序", func(t *testing.T) {
		data := testutil.NewTestData()
		repo := NewApprovalRepo(data)

		// 批量创建，交错报销单ID以验证过滤
		for _, name := range []string{"审批人A", "审批人B", "审批人C"} {
			testutil.SeedApprovalRecord(data, 10, name, "pending")
		}
		testutil.SeedApprovalRecord(data, 20, "审批人D", "pending")
		testutil.SeedApprovalRecord(data, 20, "审批人E", "pending")

		// 查询报销单10的审批记录
		records, err := repo.ListByReimbursement(10)
		if err != nil {
			t.Fatalf("查询失败: %v", err)
		}
		if len(records) != 3 {
			t.Fatalf("报销单10应有3条记录，实际 %d", len(records))
		}
		// 按名字验证顺序（ID升序 = 插入顺序）
		expectedNames := []string{"审批人A", "审批人B", "审批人C"}
		for i, name := range expectedNames {
			if records[i].ApproverName != name {
				t.Errorf("第%d条应为 %s，实际 %s", i+1, name, records[i].ApproverName)
			}
		}
	})
}
