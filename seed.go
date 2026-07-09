package main

import (
	"time"

	"github.com/CycleZero/Reimbee/model"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// SeedDemoData 预置演示数据：部门、员工、预算、报销单、票据、审批记录
func SeedDemoData(db *gorm.DB) error {
	// 密码统一用 "123456"
	hash, _ := bcrypt.GenerateFromPassword([]byte("123456"), bcrypt.DefaultCost)
	pw := string(hash)

	// ===== 部门 =====
	depts := []model.Department{
		{Name: "计算机科学与技术学院"},
		{Name: "经济管理学院"},
		{Name: "党政办公室"},
	}
	for i := range depts {
		db.Create(&depts[i])
	}

	// ===== 员工（12人，含普通员工6人、审批人4人、管理员2人） =====
	employees := []model.Employee{
		{EmployeeID: "E001", Name: "王小明", DepartmentID: depts[0].ID, Email: "wangxm@reimbee.cn", Role: "employee", PasswordHash: pw},
		{EmployeeID: "E002", Name: "李芳", DepartmentID: depts[0].ID, Email: "lifang@reimbee.cn", Role: "employee", PasswordHash: pw},
		{EmployeeID: "E003", Name: "张主任", DepartmentID: depts[0].ID, Email: "zhangzr@reimbee.cn", Role: "approver", IsApprover: true, PasswordHash: pw},
		{EmployeeID: "E004", Name: "刘教授", DepartmentID: depts[0].ID, Email: "liujs@reimbee.cn", Role: "approver", IsApprover: true, PasswordHash: pw},
		{EmployeeID: "E005", Name: "赵运维", DepartmentID: depts[0].ID, Email: "zhaoyw@reimbee.cn", Role: "admin", IsApprover: true, PasswordHash: pw},
		{EmployeeID: "E006", Name: "孙会计", DepartmentID: depts[1].ID, Email: "sunkj@reimbee.cn", Role: "employee", PasswordHash: pw},
		{EmployeeID: "E007", Name: "周老师", DepartmentID: depts[1].ID, Email: "zhouls@reimbee.cn", Role: "employee", PasswordHash: pw},
		{EmployeeID: "E008", Name: "吴院长", DepartmentID: depts[1].ID, Email: "wuyz@reimbee.cn", Role: "approver", IsApprover: true, PasswordHash: pw},
		{EmployeeID: "E009", Name: "郑秘书", DepartmentID: depts[1].ID, Email: "zhengms@reimbee.cn", Role: "employee", PasswordHash: pw},
		{EmployeeID: "E010", Name: "钱处长", DepartmentID: depts[2].ID, Email: "qiancz@reimbee.cn", Role: "approver", IsApprover: true, PasswordHash: pw},
		{EmployeeID: "E011", Name: "冯科员", DepartmentID: depts[2].ID, Email: "fengky@reimbee.cn", Role: "employee", PasswordHash: pw},
		{EmployeeID: "E012", Name: "陈管理", DepartmentID: depts[2].ID, Email: "chengl@reimbee.cn", Role: "admin", IsApprover: true, PasswordHash: pw},
	}
	for i := range employees {
		db.Create(&employees[i])
	}

	// ===== 部门预算（3条，不同使用率） =====
	budgets := []model.DepartmentBudget{
		{DepartmentID: depts[0].ID, FiscalYear: 2026, AnnualBudget: 50000000, SpentAmount: 15000000, FrozenAmount: 5000000},  // 40% 正常
		{DepartmentID: depts[1].ID, FiscalYear: 2026, AnnualBudget: 30000000, SpentAmount: 24000000, FrozenAmount: 3000000},  // 90% 预警
		{DepartmentID: depts[2].ID, FiscalYear: 2026, AnnualBudget: 8000000, SpentAmount: 2000000, FrozenAmount: 0},          // 25% 正常
	}
	for i := range budgets {
		db.Create(&budgets[i])
	}

	// ===== 报销单（6条，不同状态） =====
	reimbs := []model.Reimbursement{
		{ReimbursementNo: "REIMB-2026-0001", EmployeeID: "E001", EmployeeName: "王小明", DepartmentID: depts[0].ID, TotalAmount: 320000, Status: "approved", SubmitNote: "参加 IJCAI 2026 会议差旅费"},
		{ReimbursementNo: "REIMB-2026-0002", EmployeeID: "E001", EmployeeName: "王小明", DepartmentID: depts[0].ID, TotalAmount: 150000, Status: "pending", SubmitNote: "购置办公用品"},
		{ReimbursementNo: "REIMB-2026-0003", EmployeeID: "E002", EmployeeName: "李芳", DepartmentID: depts[0].ID, TotalAmount: 85000, Status: "reviewing", SubmitNote: "市内交通费报销"},
		{ReimbursementNo: "REIMB-2026-0004", EmployeeID: "E006", EmployeeName: "孙会计", DepartmentID: depts[1].ID, TotalAmount: 450000, Status: "rejected", SubmitNote: "参加学术会议差旅"},
		{ReimbursementNo: "REIMB-2026-0005", EmployeeID: "E007", EmployeeName: "周老师", DepartmentID: depts[1].ID, TotalAmount: 280000, Status: "approved", SubmitNote: "教材印刷费"},
		{ReimbursementNo: "REIMB-2026-0006", EmployeeID: "E001", EmployeeName: "王小明", DepartmentID: depts[0].ID, TotalAmount: 198000, Status: "draft", SubmitNote: "实验室耗材采购"},
	}
	for i := range reimbs {
		db.Create(&reimbs[i])
	}

	// ===== 报销明细 + 票据 =====
	seedItems := []struct {
		reimbIdx int
		category string
		amount   int64
		desc     string
		receipts []struct {
			code, number, date, seller string
			amount                     int64
		}
	}{
		{0, "差旅-交通", 180000, "北京→上海往返机票", []struct{ code, number, date, seller string; amount int64 }{{"031002200111", "98765432", "2026-06-20", "中国国际航空", 180000}}},
		{0, "差旅-住宿", 90000, "3晚酒店住宿", []struct{ code, number, date, seller string; amount int64 }{{"044002300222", "87654321", "2026-06-22", "上海国际大酒店", 90000}}},
		{0, "差旅-交通", 50000, "市内交通", []struct{ code, number, date, seller string; amount int64 }{{"", "", "2026-06-21", "出租车", 15000}, {"", "", "2026-06-22", "出租车", 18000}, {"", "", "2026-06-23", "出租车", 17000}}},
		{1, "办公用品", 150000, "打印纸+墨盒+文件夹", []struct{ code, number, date, seller string; amount int64 }{{"055003500333", "76543210", "2026-07-01", "晨光文具", 150000}}},
		{2, "差旅-交通", 85000, "市内出租车", []struct{ code, number, date, seller string; amount int64 }{{"", "", "2026-07-03", "出租车", 35000}, {"", "", "2026-07-03", "出租车", 28000}, {"", "", "2026-07-04", "网约车", 22000}}},
		{3, "差旅-交通", 250000, "往返机票+火车票", []struct{ code, number, date, seller string; amount int64 }{{"066006600666", "65432109", "2026-05-15", "南方航空", 180000}, {"088008800888", "54321098", "2026-05-15", "中国铁路", 70000}}},
		{3, "差旅-住宿", 200000, "4晚住宿", []struct{ code, number, date, seller string; amount int64 }{{"099009900999", "43210987", "2026-05-18", "广州国际会议中心", 200000}}},
		{4, "印刷费", 280000, "课程讲义印刷1000册", []struct{ code, number, date, seller string; amount int64 }{{"011001100111", "32109876", "2026-06-10", "学校印刷厂", 280000}}},
		{5, "办公用品", 198000, "实验室耗材", []struct{ code, number, date, seller string; amount int64 }{{"022002200222", "21098765", "2026-07-05", "实验器材公司", 198000}}},
	}
	for _, si := range seedItems {
		item := &model.ReimbursementItem{
			ReimbursementID: reimbs[si.reimbIdx].ID,
			Category:        si.category,
			Amount:          si.amount,
			Description:     si.desc,
		}
		db.Create(item)
		for _, r := range si.receipts {
			db.Create(&model.Receipt{
				ItemID:        item.ID,
				InvoiceCode:   r.code,
				InvoiceNumber: r.number,
				Amount:        r.amount,
				InvoiceDate:   r.date,
				SellerName:    r.seller,
				Category:      si.category,
			})
		}
	}

	// ===== 审批记录（≥10条） =====
	approvals := []model.ApprovalRecord{
		{ReimbursementID: reimbs[0].ID, ApproverName: "张主任", ApproverEmail: "zhangzr@reimbee.cn", Action: "approved", ActionAt: timePtr("2026-06-25 09:30:00"), Comment: "同意报销"},
		{ReimbursementID: reimbs[0].ID, ApproverName: "刘教授", ApproverEmail: "liujs@reimbee.cn", Action: "approved", ActionAt: timePtr("2026-06-25 14:00:00"), Comment: "批准"},
		{ReimbursementID: reimbs[1].ID, ApproverName: "张主任", ApproverEmail: "zhangzr@reimbee.cn", Action: "pending", Comment: ""},
		{ReimbursementID: reimbs[1].ID, ApproverName: "刘教授", ApproverEmail: "liujs@reimbee.cn", Action: "pending", Comment: ""},
		{ReimbursementID: reimbs[2].ID, ApproverName: "张主任", ApproverEmail: "zhangzr@reimbee.cn", Action: "approved", ActionAt: timePtr("2026-07-04 10:00:00"), Comment: "同意"},
		{ReimbursementID: reimbs[2].ID, ApproverName: "刘教授", ApproverEmail: "liujs@reimbee.cn", Action: "pending", Comment: ""},
		{ReimbursementID: reimbs[3].ID, ApproverName: "吴院长", ApproverEmail: "wuyz@reimbee.cn", Action: "rejected", ActionAt: timePtr("2026-05-20 15:30:00"), Comment: "住宿费用超标,请核实后重新提交"},
		{ReimbursementID: reimbs[3].ID, ApproverName: "钱处长", ApproverEmail: "qiancz@reimbee.cn", Action: "pending", Comment: ""},
		{ReimbursementID: reimbs[4].ID, ApproverName: "吴院长", ApproverEmail: "wuyz@reimbee.cn", Action: "approved", ActionAt: timePtr("2026-06-12 08:30:00"), Comment: "教材印刷属教学必要支出,同意"},
		{ReimbursementID: reimbs[4].ID, ApproverName: "钱处长", ApproverEmail: "qiancz@reimbee.cn", Action: "approved", ActionAt: timePtr("2026-06-12 16:00:00"), Comment: "批准"},
		{ReimbursementID: reimbs[5].ID, ApproverName: "张主任", ApproverEmail: "zhangzr@reimbee.cn", Action: "pending", Comment: ""},
		{ReimbursementID: reimbs[5].ID, ApproverName: "刘教授", ApproverEmail: "liujs@reimbee.cn", Action: "pending", Comment: ""},
	}
	for i := range approvals {
		db.Create(&approvals[i])
	}

	return nil
}

func timePtr(s string) *time.Time {
	t, _ := time.Parse("2006-01-02 15:04:05", s)
	return &t
}
