package employee

// CreateEmployeeRequest 创建员工请求
type CreateEmployeeRequest struct {
	EmployeeID   string `json:"employee_id" binding:"required"`   // 工号
	Name         string `json:"name" binding:"required"`          // 姓名
	Email        string `json:"email"`                            // 邮箱
	DepartmentID uint   `json:"department_id" binding:"required"`  // 所属部门ID
	Role         string `json:"role"`                             // 角色，默认 employee
}

// UpdateEmployeeRequest 更新员工请求
type UpdateEmployeeRequest struct {
	Name         string `json:"name" binding:"required"`          // 姓名
	Email        string `json:"email"`                            // 邮箱
	DepartmentID uint   `json:"department_id" binding:"required"`  // 所属部门ID
	Role         string `json:"role"`                             // 角色
}

// EmployeeResponse 员工响应
type EmployeeResponse struct {
	ID           uint   `json:"id"`
	EmployeeID   string `json:"employee_id"`
	Name         string `json:"name"`
	Email        string `json:"email"`
	DepartmentID uint   `json:"department_id"`
	Department   string `json:"department,omitempty"` // 部门名称
	Role         string `json:"role"`
	IsApprover   bool   `json:"is_approver"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

// ListEmployeeResponse 员工列表响应
type ListEmployeeResponse struct {
	List  []*EmployeeResponse `json:"list"`
	Total int64               `json:"total"`
	Page  int                 `json:"page"`
}
