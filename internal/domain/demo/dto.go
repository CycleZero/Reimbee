package demo

// CreateDemoRequest 创建请求
type CreateDemoRequest struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
}

// UpdateDemoRequest 更新请求
type UpdateDemoRequest struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
}

// ListDemoRequest 列表查询请求
type ListDemoRequest struct {
	Page     int `form:"page" binding:"min=1"`
	PageSize int `form:"page_size" binding:"min=1,max=100"`
}

// DemoResponse 响应
type DemoResponse struct {
	ID          uint   `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Status      int    `json:"status"`
	CreatedBy   uint   `json:"created_by"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// ListDemoResponse 列表响应
type ListDemoResponse struct {
	List  []*DemoResponse `json:"list"`
	Total int64           `json:"total"`
	Page  int             `json:"page"`
}
