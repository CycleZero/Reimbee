package auth

import (
	"net/http"

	"github.com/CycleZero/Reimbee/log"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// AuthService 认证 HTTP 服务层
type AuthService struct {
	biz    *AuthBiz
	logger *log.Logger
}

// NewAuthService 创建认证 HTTP 服务层
func NewAuthService(biz *AuthBiz, logger *log.Logger) *AuthService {
	logger.Debug("初始化认证HTTP服务")
	return &AuthService{biz: biz, logger: logger}
}

// Login 用户登录
func (s *AuthService) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误：工号和密码必填"})
		return
	}

	resp, err := s.biz.Login(req.EmployeeID, req.Password)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

// Register 用户注册
func (s *AuthService) Register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误：" + err.Error()})
		return
	}

	emp, err := s.biz.Register(req)
	if err != nil {
		s.logger.Warn("注册失败", zap.Error(err))
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}

	s.logger.Info("用户注册成功", zap.String("工号", emp.EmployeeID))
	c.JSON(http.StatusCreated, gin.H{
		"message":     "注册成功",
		"employee_id": emp.EmployeeID,
		"name":        emp.Name,
		"role":        emp.Role,
	})
}
