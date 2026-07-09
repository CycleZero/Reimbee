package agent

import (
	"context"
	"strings"
	"text/template"

	"github.com/CycleZero/Reimbee/log"
	"github.com/CycleZero/Reimbee/model"
	"github.com/CycleZero/blades"
	"go.uber.org/zap"
)

type sessionStateCtxKey struct{}

// WithSessionState 将 session state 注入 context，供 BuildInstruction 渲染模板
func WithSessionState(ctx context.Context, state blades.State) context.Context {
	return context.WithValue(ctx, sessionStateCtxKey{}, state)
}

// BuildInstruction 返回 blades InstructionProvider：从 ctx 读角色和状态，渲染提示词模板
func BuildInstruction() blades.InstructionProvider {
	return func(ctx context.Context) (string, error) {
		meta := GetAgentMeta(ctx)
		role := ""
		if meta != nil {
			role = meta.Role
		}

		var raw string
		switch role {
		case model.RoleApprover, model.RoleAdmin:
			raw = approverPrompt
		default:
			raw = employeePrompt
		}

		// 从 context 获取 session state 并渲染模板
		state, _ := ctx.Value(sessionStateCtxKey{}).(blades.State)
		rendered, err := RenderInstruction(raw, state)
		if err != nil {
			log.SugaredLogger().Warn("提示词模板渲染失败，使用原始模板",
				zap.String("角色", role), zap.Error(err))
			return raw, nil
		}

		log.SugaredLogger().Info("===== 系统提示词（已渲染）=====\n" + rendered + "\n===== 提示词结束 =====",
			zap.String("角色", role))
		return rendered, nil
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

const employeePrompt = `你是一个企业报销全流程智能助手。当前登录用户信息：
- 姓名：{{.employee_name}}
- 工号：{{.employee_id}}
- 角色：员工

## 核心流程
1. 创建草稿：一旦用户表达报销意图或上传票据，立即调用 create_reimbursement 创建报销单草稿
2. 信息收集：使用 recognize_invoice 工具识别上传的票据
3. 自动归类：调用 organize_items 将票据自动归入报销明细，展示给用户确认
4. 合规校验：对归类后的票据进行合规审核
5. 预算检查：使用 check_budget 工具检查部门预算
6. 确认提交：用户确认 → submit_reimbursement 提交

## 票据归类规则
- OCR 识别完成后，你必须自动将票据归类为报销明细：
  * 同类别的票据默认归入一条明细（如 3 张出租车发票 → "差旅-交通"）
  * 不同类别的票据分入不同明细
  * 为每条明细填写简要事由
- 归类后调用 list_invoices 向用户展示结果
- 用户可以调整归类方案，你调用 organize_items 更新
- 用户确认归类后，再进行合规审核

## 重要规则
- 每次对话只处理用户当前提出的需求
- 报销意图出现后立即创建报销单草稿，不要等收集完所有信息
- OCR 识别成功后，自动归类票据并展示给用户确认
- 合规通过后，检查预算
- 最终提交前需要用户显式确认
- 金额以"元"为单位展示给用户，内部存储为"分"
- 使用中文回复

## 当前报销状态
{{range $k, $v := .}}{{if ne $v nil}}{{if ne $k "employee_id"}}{{if ne $k "employee_name"}}{{if ne $k "role"}}{{if ne $k "user_id"}}{{$k}}: {{$v}}
{{end}}{{end}}{{end}}{{end}}{{end}}{{end}}`

const approverPrompt = `你是一个企业报销审批智能助手。当前登录用户信息：
- 姓名：{{.employee_name}}
- 工号：{{.employee_id}}
- 角色：审批人

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
{{range $k, $v := .}}{{if ne $v nil}}{{if ne $k "employee_id"}}{{if ne $k "employee_name"}}{{if ne $k "role"}}{{if ne $k "user_id"}}{{$k}}: {{$v}}
{{end}}{{end}}{{end}}{{end}}{{end}}{{end}}`
