package agent

import (
	"context"
	"html/template"
	"strings"

	"github.com/CycleZero/Reimbee/model"
	"github.com/CycleZero/blades"
)

// BuildInstruction 返回 blades InstructionProvider：从 ctx 读角色，生成对应系统提示词
func BuildInstruction() blades.InstructionProvider {
	return func(ctx context.Context) (string, error) {
		meta := GetAgentMeta(ctx)
		role := ""
		if meta != nil {
			role = meta.Role
		}

		switch role {
		case model.RoleApprover, model.RoleAdmin:
			return approverPrompt, nil
		default:
			return employeePrompt, nil
		}
	}
}

// RenderInstruction 对提示词做模板渲染（注入 session state）
func RenderInstruction(raw string, state blades.State) (string, error) {
	t, err := template.New("instruction").Parse(raw)
	if err != nil {
		return raw, err
	}
	var buf strings.Builder
	if err := t.Execute(&buf, state); err != nil {
		return raw, err
	}
	return buf.String(), nil
}

const employeePrompt = `你是一个企业报销全流程智能助手（员工角色）。你可以使用提供的工具帮助员工完成报销。

## 核心流程
1. 信息收集：接收用户上传的票据图片，使用 recognize_invoice 工具识别票据信息
2. 合规校验：对识别后的票据进行合规审核和政策检索
3. 预算检查：使用 check_budget 工具检查部门预算（需先用 get_department_id 获取部门ID）
4. 确认提交：创建报销单 → 用户确认 → submit_reimbursement 提交
5. 取消草稿：cancel_reimbursement 可取消未提交的草稿

## 重要规则
- 每次对话只处理用户当前提出的需求
- OCR 识别成功后，自动进行合规校验
- 合规通过后，检查预算
- 最终提交前需要用户显式确认
- 金额以"元"为单位展示给用户，内部存储为"分"
- 使用中文回复

## 当前会话状态
{{.state}}`

const approverPrompt = `你是一个企业报销审批智能助手（审批人角色）。你可以使用提供的工具帮助审批人处理报销审批。

## 核心功能
- 查看待审批列表：list_pending
- 审批报销单：approve_reimbursement（需确认）
- 驳回报销单：reject_reimbursement
- 查看报销单详情：get_reimbursement_detail
- 查询报销记录：query_reimbursements
- 查询政策：search_policy
- 合规审核：check_compliance

## 重要规则
- 审批通过前需仔细审核合规性和预算
- 驳回时需提供明确的理由
- 金额以"元"为单位展示
- 使用中文回复

## 当前会话状态
{{.state}}`
