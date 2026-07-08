package tools

import (
	"context"
	"fmt"

	"github.com/CycleZero/Reimbee/internal/domain/department"
	"github.com/CycleZero/Reimbee/log"
	"github.com/CycleZero/blades/tools"
	"go.uber.org/zap"
)

// SearchDepartmentInput 模糊搜索部门的输入参数
type SearchDepartmentInput struct {
	Name string `json:"name"`
}

// DepartmentItem 部门搜索结果项
type DepartmentItem struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
}

// SearchDepartmentOutput 模糊搜索部门的输出
type SearchDepartmentOutput struct {
	Departments []DepartmentItem `json:"departments"`
}

// SearchDepartmentTool 按名称模糊搜索部门工具
type SearchDepartmentTool struct{ tools.Tool }

// NewSearchDepartmentTool 创建模糊搜索部门工具
func NewSearchDepartmentTool(departmentBiz *department.DepartmentBiz, logger *log.Logger) *SearchDepartmentTool {
	t, err := tools.NewFunc[SearchDepartmentInput, SearchDepartmentOutput](
		"search_department",
		"按名称模糊搜索部门。支持部分匹配（如输入'计算机'可匹配'计算机科学与技术学院'）。返回部门ID和名称。",
		func(ctx context.Context, input SearchDepartmentInput) (SearchDepartmentOutput, error) {
			depts, err := departmentBiz.SearchByName(input.Name)
			if err != nil {
				return SearchDepartmentOutput{}, fmt.Errorf("搜索部门失败: %w", err)
			}
			items := make([]DepartmentItem, 0, len(depts))
			for _, dept := range depts {
				items = append(items, DepartmentItem{
					ID:   dept.ID,
					Name: dept.Name,
				})
			}
			logger.Info("部门模糊搜索完成", zap.String("关键词", input.Name), zap.Int("结果数", len(items)))
			return SearchDepartmentOutput{Departments: items}, nil
		},
	)
	if err != nil {
		panic("创建部门搜索工具失败: " + err.Error())
	}
	return &SearchDepartmentTool{t}
}
