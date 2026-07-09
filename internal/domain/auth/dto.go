package auth

// LoginRequest 登录请求（支持工号或姓名）
type LoginRequest struct {
	Username string `json:"username" binding:"required"` // 工号或姓名
	Password string `json:"password" binding:"required"` // 密码
}

// RegisterRequest 注册请求（工号自动分配）
type RegisterRequest struct {
	Name         string `json:"name" binding:"required"`          // 姓名
	Password     string `json:"password" binding:"required,min=6"` // 密码（至少6位）
	DepartmentID uint   `json:"department_id" binding:"required"`   // 部门ID
	Email        string `json:"email"`                           // 邮箱（可选）
}

// LoginResponse 登录响应
type LoginResponse struct {
	Token      string `json:"token"`       // JWT Token
	EmployeeID string `json:"employee_id"` // 工号
	Name       string `json:"name"`        // 姓名
	Role       string `json:"role"`        // 角色
	ExpiresIn  int64  `json:"expires_in"`  // 过期秒数
}

// RegisterResponse 注册响应
type RegisterResponse struct {
	Message    string `json:"message"`     // 提示信息
	EmployeeID string `json:"employee_id"` // 分配的工号
	Name       string `json:"name"`        // 姓名
	Role       string `json:"role"`        // 角色
}
