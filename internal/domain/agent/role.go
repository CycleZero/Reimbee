// Package agent 角色元数据与动态工具解析
package agent

import (
	"context"

	"github.com/CycleZero/Reimbee/model"
	"github.com/CycleZero/blades/tools"
)

// AgentMeta 注入 context 的代理元数据
type AgentMeta struct {
	Role string
}

type agentMetaKey struct{}

// WithAgentMeta 将代理元数据注入 context
func WithAgentMeta(ctx context.Context, meta *AgentMeta) context.Context {
	return context.WithValue(ctx, agentMetaKey{}, meta)
}

// GetAgentMeta 从 context 读取代理元数据
func GetAgentMeta(ctx context.Context) *AgentMeta {
	v, _ := ctx.Value(agentMetaKey{}).(*AgentMeta)
	return v
}

// Resolver 按角色动态返回工具清单
type Resolver struct {
	employeeTools []tools.Tool
	approverTools []tools.Tool
}

// NewResolver 创建工具解析器
// shared: 所有角色共用的工具（静态加载）
// employee: employee 专属工具
// approver: approver 专属工具
func NewResolver(shared, employee, approver []tools.Tool) *Resolver {
	emp := make([]tools.Tool, 0, len(shared)+len(employee))
	emp = append(emp, shared...)
	emp = append(emp, employee...)

	app := make([]tools.Tool, 0, len(shared)+len(approver))
	app = append(app, shared...)
	app = append(app, approver...)

	return &Resolver{employeeTools: emp, approverTools: app}
}

// Resolve 实现 tools.Resolver 接口：从 ctx 读角色，返回对应工具清单
func (r *Resolver) Resolve(ctx context.Context) ([]tools.Tool, error) {
	meta := GetAgentMeta(ctx)
	if meta != nil && (meta.Role == model.RoleApprover || meta.Role == model.RoleAdmin) {
		return r.approverTools, nil
	}
	return r.employeeTools, nil
}
