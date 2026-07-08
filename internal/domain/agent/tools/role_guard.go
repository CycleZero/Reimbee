// Package tools 角色守卫工具包装器
//
// RoleGuard 包裹一个 Tool，在执行前检查请求上下文中的用户角色。
// 采用 fail-closed 策略：无法获取身份或角色不匹配时拒绝执行。
//
// 使用方式：
//
//	baseTool, _ := tools.NewFunc[I, O]("name", "desc", handler)
//	roleGuardedTool := NewRoleGuard(baseTool, model.RoleApprover, model.RoleAdmin)
//
// RoleGuard 应作为最外层包装器，在任何中断/确认检查之前拦截请求。
package tools

import (
	"context"

	"github.com/CycleZero/Reimbee/internal/common"
	"github.com/CycleZero/blades/tools"
	"github.com/google/jsonschema-go/jsonschema"
)

// RoleGuard 包裹一个 Tool，在 Handle 执行前验证请求上下文中的角色。
// 若用户角色不在 allowed 列表中，返回 forbidden 响应，不执行 inner 逻辑。
type RoleGuard struct {
	inner   tools.Tool
	allowed []string
}

// NewRoleGuard 创建角色守卫工具包装器。
// allowed 参数为允许执行的角色列表（如 model.RoleEmployee, model.RoleApprover）。
func NewRoleGuard(inner tools.Tool, allowed ...string) *RoleGuard {
	return &RoleGuard{inner: inner, allowed: allowed}
}

// ── tools.Tool 接口（全部委托给 inner）──

func (g *RoleGuard) Name() string                { return g.inner.Name() }
func (g *RoleGuard) Description() string          { return g.inner.Description() }
func (g *RoleGuard) InputSchema() *jsonschema.Schema  { return g.inner.InputSchema() }
func (g *RoleGuard) OutputSchema() *jsonschema.Schema { return g.inner.OutputSchema() }

// Handle 在执行 inner 之前检查角色权限。
// fail-closed：无法获取身份或角色不匹配时直接返回 forbidden 状态。
func (g *RoleGuard) Handle(ctx context.Context, input string) (string, error) {
	meta := common.GetRequestMetadata(ctx)
	if meta == nil {
		// 无法获取用户身份信息 → 拒绝（fail-closed）
		return mustJSON(map[string]any{
			"status":  "forbidden",
			"message": "无法获取用户身份信息",
		}), nil
	}

	if !g.hasRole(meta.Role) {
		// 当前角色不在允许列表中 → 拒绝
		return mustJSON(map[string]any{
			"status":  "forbidden",
			"message": "当前角色无权执行此操作",
		}), nil
	}

	// 角色验证通过 → 委托执行
	return g.inner.Handle(ctx, input)
}

// hasRole 检查 role 是否在允许列表中。
func (g *RoleGuard) hasRole(role string) bool {
	for _, a := range g.allowed {
		if a == role {
			return true
		}
	}
	return false
}
