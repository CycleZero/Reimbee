package agent

import (
	"context"

	"github.com/CycleZero/Reimbee/internal/common"
	"github.com/CycleZero/Reimbee/model"
	"github.com/CycleZero/blades/tools"
)

// Resolver 按角色动态返回工具清单（读取 common.RequestMetadata.Role）
type Resolver struct {
	employeeTools []tools.Tool
	approverTools []tools.Tool
}

func NewResolver(shared, employee, approver []tools.Tool) *Resolver {
	emp := make([]tools.Tool, 0, len(shared)+len(employee))
	emp = append(emp, shared...)
	emp = append(emp, employee...)

	app := make([]tools.Tool, 0, len(shared)+len(approver))
	app = append(app, shared...)
	app = append(app, approver...)

	return &Resolver{employeeTools: emp, approverTools: app}
}

func (r *Resolver) Resolve(ctx context.Context) ([]tools.Tool, error) {
	meta := common.Meta(ctx)
	if meta != nil && (meta.Role == model.RoleApprover || meta.Role == model.RoleAdmin) {
		return r.approverTools, nil
	}
	return r.employeeTools, nil
}
