package tools

import (
	"context"
	"fmt"

	"github.com/CycleZero/Reimbee/internal/domain/employee"
	"github.com/CycleZero/Reimbee/log"
	"github.com/CycleZero/blades/tools"
	"go.uber.org/zap"
)

// SearchEmployeeInput 模糊搜索员工的输入参数
type SearchEmployeeInput struct {
	Name string `json:"name"`
}

// EmployeeItem 员工搜索结果项
type EmployeeItem struct {
	ID         uint   `json:"id"`
	EmployeeID string `json:"employee_id"`
	Name       string `json:"name"`
	Department string `json:"department"`
	Role       string `json:"role"`
	IsApprover bool   `json:"is_approver"`
	Email      string `json:"email"`
}

// SearchEmployeeOutput 模糊搜索员工的输出
type SearchEmployeeOutput struct {
	Employees []EmployeeItem `json:"employees"`
}

// SearchEmployeeTool 按姓名模糊搜索员工工具
type SearchEmployeeTool struct{ tools.Tool }

// NewSearchEmployeeTool 创建模糊搜索员工工具
func NewSearchEmployeeTool(employeeBiz *employee.EmployeeBiz, logger *log.Logger) *SearchEmployeeTool {
	t, err := tools.NewFunc[SearchEmployeeInput, SearchEmployeeOutput](
		"search_employee",
		"按姓名模糊搜索员工。支持部分匹配（如输入'张'可匹配'张三'）。返回员工ID、工号、姓名、所属部门、角色、是否为审批人、邮箱。",
		func(ctx context.Context, input SearchEmployeeInput) (SearchEmployeeOutput, error) {
			emps, err := employeeBiz.SearchByName(input.Name)
			if err != nil {
				return SearchEmployeeOutput{}, fmt.Errorf("搜索员工失败: %w", err)
			}
			items := make([]EmployeeItem, 0, len(emps))
			for _, emp := range emps {
				item := EmployeeItem{
					ID:         emp.ID,
					EmployeeID: emp.EmployeeID,
					Name:       emp.Name,
					Role:       emp.Role,
					IsApprover: emp.IsApprover,
					Email:      emp.Email,
				}
				if emp.Department != nil {
					item.Department = emp.Department.Name
				}
				items = append(items, item)
			}
			logger.Info("员工模糊搜索完成", zap.String("关键词", input.Name), zap.Int("结果数", len(items)))
			return SearchEmployeeOutput{Employees: items}, nil
		},
	)
	if err != nil {
		panic("创建员工搜索工具失败: " + err.Error())
	}
	return &SearchEmployeeTool{t}
}
