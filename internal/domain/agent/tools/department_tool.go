package tools

import (
	"context"
	"fmt"

	"github.com/CycleZero/Reimbee/internal/domain/employee"
	"github.com/CycleZero/Reimbee/log"
	"github.com/CycleZero/blades/tools"
	"go.uber.org/zap"
)

type DeptInput struct {
	EmployeeID string `json:"employee_id"`
}

type DeptOutput struct {
	DepartmentID uint   `json:"department_id"`
	Department   string `json:"department"`
}

type DeptTool struct{ tools.Tool }

func NewDeptTool(employeeBiz *employee.EmployeeBiz, logger *log.Logger) *DeptTool {
	t, err := tools.NewFunc[DeptInput, DeptOutput](
		"get_department_id",
		"根据员工工号查询所属部门ID和名称。调用 check_budget 前需先获取正确的部门ID。",
		func(ctx context.Context, input DeptInput) (DeptOutput, error) {
			emp, err := employeeBiz.GetByEmployeeID(input.EmployeeID)
			if err != nil {
				return DeptOutput{}, fmt.Errorf("查询员工失败: %w", err)
			}
			logger.Debug("部门查询完成", zap.String("工号", input.EmployeeID), zap.Uint("部门ID", emp.DepartmentID))
			return DeptOutput{
				DepartmentID: emp.DepartmentID,
				Department:   emp.Department.Name,
			}, nil
		},
	)
	if err != nil {
		panic("创建部门查询工具失败: " + err.Error())
	}
	return &DeptTool{t}
}
