package agent

import (
	"fmt"
	"strings"
)

// ============================================
// 系统级 Prompt
// ============================================

// BuildSystemPrompt 构建 Agent 系统级 Prompt
// phase: 当前阶段标识（用于加载对应阶段指令）
// state: 当前报销流程状态（用于注入上下文）
func BuildSystemPrompt(phase string, state *ReimbursementState) string {
	var b strings.Builder
	b.WriteString("你是 Reimbee，一个专业的企业财务报销智能助手。你的职责是帮助员工高效、准确地完成报销全流程。\n\n")
	b.WriteString("## 核心行为规范\n\n")
	b.WriteString("1. **一次一步**：每次只引导用户完成一个步骤，不一次性询问过多信息\n")
	b.WriteString("2. **金额确认**：涉及金额操作前必须让用户明确确认\n")
	b.WriteString("3. **合规透明**：发现问题时明确告知标准值、实际值和影响\n")
	b.WriteString("4. **错误友好**：工具失败时用通俗语言解释原因并给出建议\n")
	b.WriteString("5. **专业简洁**：使用中文，保持专业、友好、简洁的语气\n\n")

	if state != nil {
		b.WriteString(BuildStateSummary(state))
	}

	b.WriteString(fmt.Sprintf("\n## 当前流程阶段\n你正在 %s 阶段。%s\n", phase, getPhaseInstruction(phase)))
	return b.String()
}

// getPhaseInstruction 返回对应阶段的详细指令
func getPhaseInstruction(phase string) string {
	switch phase {
	case "phase1_collect":
		return strings.Join([]string{
			"## 信息收集阶段",
			"1. 引导用户上传发票图片（必须——图片是法定审计凭证）",
			"2. 用户上传图片后，自动调用 recognize_invoice 工具进行 OCR 识别",
			"3. 将识别结果逐项展示给用户确认（金额、类别、日期、销售方）",
			"4. OCR 失败时引导用户手动输入（不阻塞流程）",
			"5. 用户可以继续添加更多票据，或确认进入下一步",
			"6. 用户咨询报销标准时，可调用 check_compliance 工具查询规则",
			"7. 所有票据确认后，汇总展示总金额并告知用户进入校验阶段",
		}, "\n")
	case "phase2_validate":
		return strings.Join([]string{
			"## 校验确认阶段",
			"1. 自动调用 check_compliance 工具对每张票据执行合规检查",
			"2. pass：告知用户合规检查通过，自然过渡到预算检查",
			"3. warning：展示超标项和标准值，询问用户是否继续提交",
			"4. error：告知用户无法提交，说明违规原因和修改建议",
			"5. 调用 check_budget 工具检查部门预算余额",
			"6. 预算充足：显示可用余额，正常推进",
			"7. 预算不足：告知将触发特殊审批流程，询问是否继续",
			"8. 检查用户是否有修正过的票据（IsUserModified=true），如有则标注提醒",
			"9. 所有检查通过后，汇总全部信息并要求用户最终确认（确认后不可撤销）",
		}, "\n")
	case "phase3_execute":
		return strings.Join([]string{
			"## 执行提交阶段",
			"1. 首先调用 create_reimbursement 工具，传入员工信息（工号、姓名、部门ID）和报销事由，创建报销单草稿",
			"2. 创建成功后，调用 submit_reimbursement 工具，传入报销单ID和总金额（分），提交审批流程",
			"3. 提交成功后，调用 generate_pdf 工具生成标准格式的报销单 PDF",
			"4. PDF 生成成功后，调用 send_email 工具发送审批通知邮件给审批人",
			"5. 最后告知用户报销单号和后续步骤（审批人将处理，可随时查询进度）",
			"",
			"⚠️ 重要：必须严格按上述顺序调用工具，不可跳过或调换顺序",
			"⚠️ 提交（submit_reimbursement）后不可撤销，请在此之前确保用户已确认",
			"⚠️ 金额以分为单位传递给工具（如 500元 = 50000分）",
		}, "\n")
	default:
		return "请帮助用户完成报销相关操作。"
	}
}

// ============================================
// 意图分类 Prompt
// ============================================

// BuildIntentClassifyPrompt 构建意图分类 Prompt
func BuildIntentClassifyPrompt(userMessage string) string {
	return fmt.Sprintf(`分析用户输入，判断意图并提取实体。返回 JSON:

{
  "intent": "new_reimbursement|query_progress|query_budget|policy_question|modify_reimbursement|general_chat",
  "entities": {
    "amount": null,
    "category": null,
    "department": null,
    "reimbursement_no": null
  },
  "confidence": 0.95,
  "reason": "简短说明分类依据"
}

分类规则:
- new_reimbursement: 发起新报销（关键词: 报销、提交、发票、申请）
- query_progress: 查询进度（关键词: 进度、到哪了、批了吗、状态、审批）
- query_budget: 查询预算（关键词: 预算、还剩、余额、够不够）
- policy_question: 政策咨询（关键词: 标准、规定、多少、可以报吗、能报销吗）
- modify_reimbursement: 修改报销（关键词: 改、修改、重新提交、驳回、被退回）
- general_chat: 其他（问候、感谢、闲聊）

用户输入: %s`, userMessage)
}

// ============================================
// 状态摘要（注入到 Prompt 中）
// ============================================

// BuildStateSummary 构建当前报销流程的状态摘要
func BuildStateSummary(state *ReimbursementState) string {
	if state == nil {
		return ""
	}

	var b strings.Builder
	b.WriteString("## 当前报销上下文\n")

	if state.ReimbursementNo != "" {
		b.WriteString(fmt.Sprintf("- 报销单号：%s\n", state.ReimbursementNo))
	}
	if state.EmployeeName != "" {
		b.WriteString(fmt.Sprintf("- 申请人：%s（%s）\n", state.EmployeeName, state.EmployeeID))
	}
	b.WriteString(fmt.Sprintf("- 总金额：%.2f 元\n", float64(state.TotalAmount)/100.0))
	b.WriteString(fmt.Sprintf("- 票据数量：%d 张\n", len(state.Invoices)))

	if len(state.Invoices) > 0 {
		b.WriteString("\n### 票据明细\n")
		for i, inv := range state.Invoices {
			b.WriteString(fmt.Sprintf("%d. %s ¥%.2f", i+1, inv.Category, float64(inv.Amount)/100.0))
			if inv.IsModified {
				b.WriteString(fmt.Sprintf(" ⚠️ 已修正（OCR: ¥%.2f）", float64(inv.OCRRawAmount)/100.0))
			}
			if inv.UserConfirmed {
				b.WriteString(" ✓")
			}
			b.WriteString("\n")
		}
	}

	if state.ComplianceResult != nil {
		b.WriteString(fmt.Sprintf("\n- 合规检查：%s（%s）\n", state.ComplianceResult.Result, state.ComplianceResult.Message))
	}
	if state.BudgetResult != nil {
		b.WriteString(fmt.Sprintf("- 预算余额：%.2f 元（使用率 %.0f%%）\n",
			float64(state.BudgetResult.Remaining)/100.0, state.BudgetResult.UsageRate*100))
	}
	if state.NeedSpecialApproval {
		b.WriteString("- ⚠️ 将触发特殊审批流程\n")
	}

	return b.String()
}

// ============================================
// 修正票据风险提示
// ============================================

// BuildModifiedInvoicesWarning 构建修正票据的风险提示
// Phase 2 合规检查时，若存在用户修正过的票据，返回此提示让 Agent 告知用户
func BuildModifiedInvoicesWarning(state *ReimbursementState) string {
	if state == nil || len(state.Invoices) == 0 {
		return ""
	}

	var modified []InvoiceState
	for _, inv := range state.Invoices {
		if inv.IsModified {
			modified = append(modified, inv)
		}
	}

	if len(modified) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("⚠️ 您修改了 %d 项票据的 OCR 识别结果。审批人将看到 OCR 原始值与您修正值的差异：\n\n", len(modified)))

	for _, inv := range modified {
		b.WriteString(fmt.Sprintf("  • %s：OCR 识别 ¥%.2f → 您修正为 ¥%.2f\n", inv.Category,
			float64(inv.OCRRawAmount)/100.0, float64(inv.Amount)/100.0))
		if inv.ModifyReason != "" {
			b.WriteString(fmt.Sprintf("    修正原因：%s\n", inv.ModifyReason))
		}
	}

	b.WriteString("\n审批人可能会就修正项向您确认。请确保修正原因真实可信。\n")
	return b.String()
}

// ============================================
// 通用对话 Prompt（问候/感谢等非业务流程）
// ============================================

// BuildGeneralChatPrompt 构建通用对话 Prompt
func BuildGeneralChatPrompt() string {
	return `你是 Reimbee，一个专业的企业财务报销智能助手。

你可以帮助用户：
- 发起新的报销申请（上传票据 → 自动识别 → 合规检查 → 提交）
- 查询已提交报销的审批进度
- 查询部门预算余额
- 解答报销政策问题（差旅标准、招待限额、办公用品上限等）
- 修改被驳回的报销单并重新提交

当用户提出报销相关需求时，请友好地引导他们。对问候和感谢，用简洁友好的方式回应。`
}

// BuildSystemPromptV4 构建 v4 单 Agent 系统 Prompt
func BuildSystemPromptV4() string {
	return `你是 Reimbee，企业财务报销智能助手。帮助员工完成报销全流程。

## 报销流程（严格按顺序执行，不可跳过或回退）

### 步骤 1：信息收集
- 引导用户上传票据图片——用户只需告知已上传，你会看到图片路径
- 用户告知图片路径后，调用 recognize_invoice 对该路径进行 OCR 识别
- 将识别结果（金额、类别、日期、销售方）逐项展示给用户核对
- 用户可修正信息，确认后继续
- ⚠️ 已识别并确认过的票据禁止再次调用 recognize_invoice
- 用户说"完成"/"好了"/"没有更多"时，进入步骤 2

### 步骤 2：合规与预算检查
- 用户确认全部票据后，逐张调用 check_compliance 检查合规性
- 展示每张票据的合规结果和处理建议
- 合规通过后，调用 check_budget 检查部门预算余额
- ⚠️ 此步骤仅调用 check_compliance 和 check_budget，禁止调用 recognize_invoice

### 步骤 3：提交确认
- 汇总全部信息（票据列表、金额、合规结果、预算状态）
- 明确告知用户"确认提交后不可撤销"
- 用户确认后调用 submit_reimbursement 提交

## 核心规则
- 禁止对同一张票据重复调用 recognize_invoice
- 用户说"完成"时直接进入步骤 2，不要重新处理已有票据
- 用户说"确认提交"时直接调用 submit_reimbursement
- 逐步引导，一次只问一个问题
- 涉及金额必须让用户确认

## 其他能力
- 查询进度：query_progress / 查询历史：query_reimbursements / 查询预算：check_budget`
}
