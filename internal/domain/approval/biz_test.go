package approval

import (
	"testing"

	"github.com/CycleZero/Reimbee/infra"
	"github.com/CycleZero/Reimbee/internal/testutil"
	"github.com/CycleZero/Reimbee/log"
	"github.com/CycleZero/Reimbee/model"
	"go.uber.org/zap"
)

type testEnv struct {
	biz  *ApprovalBiz
	repo *ApprovalRepo
	data *infra.Data
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	data := testutil.NewTestData()
	repo := NewApprovalRepo(data)
	logger := &log.Logger{Logger: zap.NewNop()}
	biz := NewApprovalBiz(logger, repo)
	return &testEnv{biz: biz, repo: repo, data: data}
}

func TestApprovalBiz_CreateApprovalChain(t *testing.T) {
	t.Run("单个审批人创建成功", func(t *testing.T) {
		env := newTestEnv(t)

		approvers := []*model.Employee{
			{Name: "张三", Email: "zhangsan@test.com"},
		}
		err := env.biz.CreateApprovalChain(1, approvers)
		if err != nil {
			t.Fatalf("创建审批链失败: %v", err)
		}

		records, err := env.repo.ListByReimbursement(1)
		if err != nil {
			t.Fatalf("查询审批链失败: %v", err)
		}
		if len(records) != 1 {
			t.Fatalf("审批记录数应为 1，实际 %d", len(records))
		}
		if records[0].ApproverName != "张三" {
			t.Errorf("审批人应为 %q，实际 %q", "张三", records[0].ApproverName)
		}
		if records[0].Action != "pending" {
			t.Errorf("初始状态应为 %q，实际 %q", "pending", records[0].Action)
		}
	})

	t.Run("多个审批人创建成功", func(t *testing.T) {
		env := newTestEnv(t)

		approvers := []*model.Employee{
			{Name: "张三", Email: "zhangsan@test.com"},
			{Name: "李四", Email: "lisi@test.com"},
			{Name: "王五", Email: "wangwu@test.com"},
		}
		err := env.biz.CreateApprovalChain(2, approvers)
		if err != nil {
			t.Fatalf("创建审批链失败: %v", err)
		}

		records, err := env.repo.ListByReimbursement(2)
		if err != nil {
			t.Fatalf("查询审批链失败: %v", err)
		}
		if len(records) != 3 {
			t.Fatalf("审批记录数应为 3，实际 %d", len(records))
		}
		expectedNames := []string{"张三", "李四", "王五"}
		for i, name := range expectedNames {
			if records[i].ApproverName != name {
				t.Errorf("第%d条审批人应为 %q，实际 %q", i+1, name, records[i].ApproverName)
			}
			if records[i].ReimbursementID != 2 {
				t.Errorf("第%d条ReimbursementID应为 %d，实际 %d", i+1, 2, records[i].ReimbursementID)
			}
		}
	})

	t.Run("空审批人列表被拒绝", func(t *testing.T) {
		env := newTestEnv(t)

		err := env.biz.CreateApprovalChain(3, nil)
		if err == nil {
			t.Fatal("空审批人列表应返回错误")
		}
		if err.Error() != "至少需要指定一位审批人" {
			t.Errorf("错误信息应为 %q，实际 %q", "至少需要指定一位审批人", err.Error())
		}
	})
}

func TestApprovalBiz_Approve(t *testing.T) {
	t.Run("正常审批通过", func(t *testing.T) {
		env := newTestEnv(t)
		record := testutil.SeedApprovalRecord(env.data, 1, "赵六", "pending")

		err := env.biz.Approve(record.ID, "同意报销，金额合理")
		if err != nil {
			t.Fatalf("审批通过失败: %v", err)
		}

		fetched, _ := env.repo.GetByID(record.ID)
		if fetched.Action != "approved" {
			t.Errorf("状态应为 %q，实际 %q", "approved", fetched.Action)
		}
		if fetched.Comment != "同意报销，金额合理" {
			t.Errorf("审批意见应为 %q，实际 %q", "同意报销，金额合理", fetched.Comment)
		}
		if fetched.ActionAt == nil {
			t.Fatal("操作时间不应为nil")
		}
	})

	t.Run("审批评论为空也能通过", func(t *testing.T) {
		env := newTestEnv(t)
		record := testutil.SeedApprovalRecord(env.data, 1, "孙七", "pending")

		err := env.biz.Approve(record.ID, "")
		if err != nil {
			t.Fatalf("无评论审批通过失败: %v", err)
		}

		fetched, _ := env.repo.GetByID(record.ID)
		if fetched.Action != "approved" {
			t.Errorf("状态应为 %q，实际 %q", "approved", fetched.Action)
		}
		if fetched.Comment != "" {
			t.Errorf("审批意见应为空，实际 %q", fetched.Comment)
		}
	})

	t.Run("重复审批被拒绝——状态机保护", func(t *testing.T) {
		env := newTestEnv(t)
		record := testutil.SeedApprovalRecord(env.data, 1, "周八", "pending")

		if err := env.biz.Approve(record.ID, "通过"); err != nil {
			t.Fatalf("第一次审批失败: %v", err)
		}
		err := env.biz.Approve(record.ID, "再次通过")
		if err == nil {
			t.Fatal("重复审批应返回错误")
		}
		expectedErr := "该审批已处理（当前状态: approved），不可重复操作"
		if err.Error() != expectedErr {
			t.Errorf("错误信息应为 %q，实际 %q", expectedErr, err.Error())
		}
		fetched, _ := env.repo.GetByID(record.ID)
		if fetched.Comment != "通过" {
			t.Errorf("审批意见应保持 %q，实际 %q", "通过", fetched.Comment)
		}
	})

	t.Run("审批不存在的记录返回错误", func(t *testing.T) {
		env := newTestEnv(t)

		err := env.biz.Approve(99999, "通过")
		if err == nil {
			t.Fatal("审批不存在的记录应返回错误")
		}
		if err.Error() != "审批记录不存在" {
			t.Errorf("错误信息应为 %q，实际 %q", "审批记录不存在", err.Error())
		}
	})
}

func TestApprovalBiz_Reject(t *testing.T) {
	t.Run("正常驳回成功", func(t *testing.T) {
		env := newTestEnv(t)
		record := testutil.SeedApprovalRecord(env.data, 1, "吴九", "pending")

		err := env.biz.Reject(record.ID, "金额超标，请重新提交")
		if err != nil {
			t.Fatalf("驳回失败: %v", err)
		}

		fetched, _ := env.repo.GetByID(record.ID)
		if fetched.Action != "rejected" {
			t.Errorf("状态应为 %q，实际 %q", "rejected", fetched.Action)
		}
		if fetched.Comment != "金额超标，请重新提交" {
			t.Errorf("驳回原因应为 %q，实际 %q", "金额超标，请重新提交", fetched.Comment)
		}
		if fetched.ActionAt == nil {
			t.Fatal("操作时间不应为nil")
		}
	})

	t.Run("重复驳回被拒绝——状态机保护", func(t *testing.T) {
		env := newTestEnv(t)
		record := testutil.SeedApprovalRecord(env.data, 1, "郑十", "pending")

		if err := env.biz.Reject(record.ID, "金额不合理"); err != nil {
			t.Fatalf("第一次驳回失败: %v", err)
		}
		err := env.biz.Reject(record.ID, "再次驳回")
		if err == nil {
			t.Fatal("重复驳回应返回错误")
		}
		expectedErr := "该审批已处理（当前状态: rejected），不可重复操作"
		if err.Error() != expectedErr {
			t.Errorf("错误信息应为 %q，实际 %q", expectedErr, err.Error())
		}
		fetched, _ := env.repo.GetByID(record.ID)
		if fetched.Comment != "金额不合理" {
			t.Errorf("驳回原因应保持 %q，实际 %q", "金额不合理", fetched.Comment)
		}
	})

	t.Run("驳回原因不能为空", func(t *testing.T) {
		env := newTestEnv(t)
		record := testutil.SeedApprovalRecord(env.data, 1, "冯十一", "pending")

		err := env.biz.Reject(record.ID, "")
		if err == nil {
			t.Fatal("空驳回原因应返回错误")
		}
		if err.Error() != "驳回时必须填写驳回原因" {
			t.Errorf("错误信息应为 %q，实际 %q", "驳回时必须填写驳回原因", err.Error())
		}
		fetched, _ := env.repo.GetByID(record.ID)
		if fetched.Action != "pending" {
			t.Errorf("状态应保持 %q，实际 %q", "pending", fetched.Action)
		}
	})

	t.Run("驳回不存在的记录返回错误", func(t *testing.T) {
		env := newTestEnv(t)

		err := env.biz.Reject(99999, "原因")
		if err == nil {
			t.Fatal("驳回不存在的记录应返回错误")
		}
		if err.Error() != "审批记录不存在" {
			t.Errorf("错误信息应为 %q，实际 %q", "审批记录不存在", err.Error())
		}
	})
}

func TestApprovalBiz_IsAllApproved(t *testing.T) {
	t.Run("全部pending返回false", func(t *testing.T) {
		env := newTestEnv(t)
		testutil.SeedApprovalRecord(env.data, 1, "审批人A", "pending")
		testutil.SeedApprovalRecord(env.data, 1, "审批人B", "pending")

		allApproved, err := env.biz.IsAllApproved(1)
		if err != nil {
			t.Fatalf("查询失败: %v", err)
		}
		if allApproved {
			t.Fatal("全部pending时应返回false")
		}
	})

	t.Run("全部approved返回true", func(t *testing.T) {
		env := newTestEnv(t)
		testutil.SeedApprovalRecord(env.data, 1, "审批人A", "approved")
		testutil.SeedApprovalRecord(env.data, 1, "审批人B", "approved")

		allApproved, err := env.biz.IsAllApproved(1)
		if err != nil {
			t.Fatalf("查询失败: %v", err)
		}
		if !allApproved {
			t.Fatal("全部approved时应返回true")
		}
	})

	t.Run("有rejected返回false", func(t *testing.T) {
		env := newTestEnv(t)
		testutil.SeedApprovalRecord(env.data, 1, "审批人A", "approved")
		testutil.SeedApprovalRecord(env.data, 1, "审批人B", "rejected")
		testutil.SeedApprovalRecord(env.data, 1, "审批人C", "approved")

		allApproved, err := env.biz.IsAllApproved(1)
		if err != nil {
			t.Fatalf("查询失败: %v", err)
		}
		if allApproved {
			t.Fatal("存在rejected时应返回false")
		}
	})

	t.Run("还有一个pending返回false", func(t *testing.T) {
		env := newTestEnv(t)
		testutil.SeedApprovalRecord(env.data, 1, "审批人A", "approved")
		testutil.SeedApprovalRecord(env.data, 1, "审批人B", "pending")

		allApproved, err := env.biz.IsAllApproved(1)
		if err != nil {
			t.Fatalf("查询失败: %v", err)
		}
		if allApproved {
			t.Fatal("存在pending时应返回false")
		}
	})

	t.Run("空审批链返回true（无pending即全通过）", func(t *testing.T) {
		env := newTestEnv(t)

		allApproved, err := env.biz.IsAllApproved(999)
		if err != nil {
			t.Fatalf("查询空审批链失败: %v", err)
		}
		if !allApproved {
			t.Fatal("空审批链应返回true（没有pending和rejected）")
		}
	})
}

func TestApprovalBiz_IsAnyRejected(t *testing.T) {
	t.Run("无拒绝返回false", func(t *testing.T) {
		env := newTestEnv(t)
		testutil.SeedApprovalRecord(env.data, 1, "审批人A", "pending")
		testutil.SeedApprovalRecord(env.data, 1, "审批人B", "pending")

		rejected, reason, err := env.biz.IsAnyRejected(1)
		if err != nil {
			t.Fatalf("查询失败: %v", err)
		}
		if rejected {
			t.Fatal("无拒绝时应返回false")
		}
		if reason != "" {
			t.Errorf("无拒绝时原因应为空，实际 %q", reason)
		}
	})

	t.Run("有拒绝返回true并返回原因", func(t *testing.T) {
		env := newTestEnv(t)
		testutil.SeedApprovalRecord(env.data, 1, "审批人A", "approved")
		seeded := testutil.SeedApprovalRecord(env.data, 1, "审批人B", "rejected")
		seeded.Comment = "金额超标"
		env.repo.Update(seeded)

		rejected, reason, err := env.biz.IsAnyRejected(1)
		if err != nil {
			t.Fatalf("查询失败: %v", err)
		}
		if !rejected {
			t.Fatal("有拒绝时应返回true")
		}
		if reason != "金额超标" {
			t.Errorf("拒绝原因应为 %q，实际 %q", "金额超标", reason)
		}
	})

	t.Run("全部approved返回false", func(t *testing.T) {
		env := newTestEnv(t)
		testutil.SeedApprovalRecord(env.data, 1, "审批人A", "approved")
		testutil.SeedApprovalRecord(env.data, 1, "审批人B", "approved")
		testutil.SeedApprovalRecord(env.data, 1, "审批人C", "approved")

		rejected, reason, err := env.biz.IsAnyRejected(1)
		if err != nil {
			t.Fatalf("查询失败: %v", err)
		}
		if rejected {
			t.Fatal("全部approved时应返回false")
		}
		if reason != "" {
			t.Errorf("全部approved时原因应为空，实际 %q", reason)
		}
	})

	t.Run("空审批链返回false", func(t *testing.T) {
		env := newTestEnv(t)

		rejected, reason, err := env.biz.IsAnyRejected(999)
		if err != nil {
			t.Fatalf("查询空审批链失败: %v", err)
		}
		if rejected {
			t.Fatal("空审批链应返回false")
		}
		if reason != "" {
			t.Errorf("空审批链时原因应为空，实际 %q", reason)
		}
	})
}

func TestApprovalBiz_GetProgress(t *testing.T) {
	t.Run("空审批链返回空列表", func(t *testing.T) {
		env := newTestEnv(t)

		records, err := env.biz.GetProgress(999)
		if err != nil {
			t.Fatalf("查询空审批链失败: %v", err)
		}
		if len(records) != 0 {
			t.Errorf("空审批链应返回空列表，实际长度 %d", len(records))
		}
	})

	t.Run("多条审批记录按ID升序返回", func(t *testing.T) {
		env := newTestEnv(t)

		r1 := testutil.SeedApprovalRecord(env.data, 1, "审批人A", "approved")
		r2 := testutil.SeedApprovalRecord(env.data, 1, "审批人B", "pending")
		r3 := testutil.SeedApprovalRecord(env.data, 1, "审批人C", "pending")

		records, err := env.biz.GetProgress(1)
		if err != nil {
			t.Fatalf("查询进度失败: %v", err)
		}
		if len(records) != 3 {
			t.Fatalf("记录数应为 3，实际 %d", len(records))
		}
		if records[0].ID != r1.ID {
			t.Errorf("第1条应为审批人A(ID=%d)，实际 ID=%d", r1.ID, records[0].ID)
		}
		if records[1].ID != r2.ID {
			t.Errorf("第2条应为审批人B(ID=%d)，实际 ID=%d", r2.ID, records[1].ID)
		}
		if records[2].ID != r3.ID {
			t.Errorf("第3条应为审批人C(ID=%d)，实际 ID=%d", r3.ID, records[2].ID)
		}
	})
}

func TestApprovalBiz_StateMachine(t *testing.T) {
	t.Run("approve后reject被拒绝", func(t *testing.T) {
		env := newTestEnv(t)
		record := testutil.SeedApprovalRecord(env.data, 1, "审批人A", "pending")

		if err := env.biz.Approve(record.ID, "通过"); err != nil {
			t.Fatalf("审批通过失败: %v", err)
		}
		err := env.biz.Reject(record.ID, "反悔了")
		if err == nil {
			t.Fatal("approved后reject应被拒绝")
		}
		expectedErr := "该审批已处理（当前状态: approved），不可重复操作"
		if err.Error() != expectedErr {
			t.Errorf("错误信息应为 %q，实际 %q", expectedErr, err.Error())
		}
		fetched, _ := env.repo.GetByID(record.ID)
		if fetched.Action != "approved" {
			t.Errorf("最终状态应为 %q，实际 %q", "approved", fetched.Action)
		}
	})

	t.Run("reject后approve被拒绝", func(t *testing.T) {
		env := newTestEnv(t)
		record := testutil.SeedApprovalRecord(env.data, 1, "审批人B", "pending")

		if err := env.biz.Reject(record.ID, "金额不合理"); err != nil {
			t.Fatalf("驳回失败: %v", err)
		}
		err := env.biz.Approve(record.ID, "重新考虑通过")
		if err == nil {
			t.Fatal("rejected后approve应被拒绝")
		}
		expectedErr := "该审批已处理（当前状态: rejected），不可重复操作"
		if err.Error() != expectedErr {
			t.Errorf("错误信息应为 %q，实际 %q", expectedErr, err.Error())
		}
		fetched, _ := env.repo.GetByID(record.ID)
		if fetched.Action != "rejected" {
			t.Errorf("最终状态应为 %q，实际 %q", "rejected", fetched.Action)
		}
	})

	t.Run("完整审批流程——全部通过", func(t *testing.T) {
		env := newTestEnv(t)

		approvers := []*model.Employee{
			{Name: "经理A", Email: "a@test.com"},
			{Name: "总监B", Email: "b@test.com"},
			{Name: "财务C", Email: "c@test.com"},
		}
		if err := env.biz.CreateApprovalChain(100, approvers); err != nil {
			t.Fatalf("创建审批链失败: %v", err)
		}

		records, _ := env.repo.ListByReimbursement(100)
		if len(records) != 3 {
			t.Fatalf("应有3条审批记录，实际 %d", len(records))
		}

		allApproved, _ := env.biz.IsAllApproved(100)
		if allApproved {
			t.Fatal("初始状态IsAllApproved应为false")
		}

		for _, r := range records {
			if err := env.biz.Approve(r.ID, "同意"); err != nil {
				t.Fatalf("审批通过失败(记录%d): %v", r.ID, err)
			}
		}

		allApproved, _ = env.biz.IsAllApproved(100)
		if !allApproved {
			t.Fatal("全部通过后IsAllApproved应为true")
		}
		rejected, _, _ := env.biz.IsAnyRejected(100)
		if rejected {
			t.Fatal("全部通过后IsAnyRejected应为false")
		}
	})
}
