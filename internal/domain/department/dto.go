package department

// CreateDepartmentRequest 创建部门请求
type CreateDepartmentRequest struct {
	Name      string `json:"name" binding:"required"`                                    // 部门名称
	ManagerID *uint  `json:"manager_id"`                                                 // 部门主管员工ID，可选
}

// UpdateDepartmentRequest 更新部门请求
type UpdateDepartmentRequest struct {
	Name      string `json:"name" binding:"required"`                                     // 新部门名称
	ManagerID *uint  `json:"manager_id"`                                                  // 部门主管员工ID，可选
}

// DepartmentResponse 部门响应
type DepartmentResponse struct {
	ID        uint   `json:"id"`
	Name      string `json:"name"`
	ManagerID *uint  `json:"manager_id,omitempty"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// ListDepartmentResponse 部门列表响应
type ListDepartmentResponse struct {
	List  []*DepartmentResponse `json:"list"`
	Total int64                 `json:"total"`
	Page  int                   `json:"page"`
}
