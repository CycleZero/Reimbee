package auth

import (
	"net/http"

	"github.com/CycleZero/Reimbee/log"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type AuthService struct {
	biz    *AuthBiz
	logger *log.Logger
}

func NewAuthService(biz *AuthBiz, logger *log.Logger) *AuthService {
	logger.Debug("初始化认证HTTP服务")
	return &AuthService{biz: biz, logger: logger}
}

// Login 用户登录
// @Summary 用户登录
// @Description 验证工号和密码，返回 JWT Token
// @Tags 认证
// @Accept json
// @Produce json
// @Param request body LoginRequest true "登录请求"
// @Success 200 {object} LoginResponse "登录成功，返回 JWT"
// @Failure 400 {object} map[string]interface{} "请求参数错误"
// @Failure 401 {object} map[string]interface{} "工号或密码错误"
// @Router /api/auth/login [post]
func (s *AuthService) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误：用户名和密码必填"})
		return
	}

	resp, err := s.biz.Login(req.Username, req.Password)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

// Register 用户注册
// @Summary 用户注册
// @Description 注册新用户，工号自动分配（EMP001起），密码 bcrypt 加密
// @Tags 认证
// @Accept json
// @Produce json
// @Param request body RegisterRequest true "注册请求"
// @Success 201 {object} RegisterResponse "注册成功，返回分配的工号"
// @Failure 400 {object} map[string]interface{} "请求参数错误"
// @Failure 409 {object} map[string]interface{} "工号已存在"
// @Failure 500 {object} map[string]interface{} "服务器内部错误"
// @Router /api/auth/register [post]
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

	c.JSON(http.StatusCreated, RegisterResponse{
		Message:    "注册成功",
		EmployeeID: emp.EmployeeID,
		Name:       emp.Name,
		Role:       emp.Role,
	})
}
