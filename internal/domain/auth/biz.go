package auth

import (
	"fmt"
	"time"

	"github.com/CycleZero/Reimbee/log"
	"github.com/CycleZero/Reimbee/model"
	"github.com/golang-jwt/jwt/v5"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

// AuthBiz 认证业务逻辑层
type AuthBiz struct {
	logger    *log.Logger
	repo      *EmployeeRepo
	jwtSecret string
	jwtTTL    time.Duration
}

// NewAuthBiz 创建认证业务逻辑层
func NewAuthBiz(logger *log.Logger, repo *EmployeeRepo, vc *viper.Viper) *AuthBiz {
	secret := vc.GetString("jwt.secret")
	if secret == "" {
		secret = "reimbee-jwt-secret-change-in-production"
	}
	ttl := time.Duration(vc.GetInt("jwt.expire_hours")) * time.Hour
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	logger.Debug("初始化认证业务层", zap.Int64("JWT有效期(小时)", int64(ttl.Hours())))
	return &AuthBiz{
		logger:    logger,
		repo:      repo,
		jwtSecret: secret,
		jwtTTL:    ttl,
	}
}

// Login 验证工号密码，返回 JWT
func (b *AuthBiz) Login(employeeID, password string) (*LoginResponse, error) {
	b.logger.Debug("用户登录", zap.String("工号", employeeID))

	emp, err := b.repo.GetByEmployeeID(employeeID)
	if err != nil {
		b.logger.Warn("登录失败：工号不存在", zap.String("工号", employeeID))
		return nil, fmt.Errorf("工号或密码错误")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(emp.PasswordHash), []byte(password)); err != nil {
		b.logger.Warn("登录失败：密码错误", zap.String("工号", employeeID))
		return nil, fmt.Errorf("工号或密码错误")
	}

	now := time.Now()
	expiresAt := now.Add(b.jwtTTL)
	claims := jwt.MapClaims{
		"user_id":     emp.ID,
		"employee_id": emp.EmployeeID,
		"role":        emp.Role,
		"name":        emp.Name,
		"iat":         now.Unix(),
		"exp":         expiresAt.Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err := token.SignedString([]byte(b.jwtSecret))
	if err != nil {
		b.logger.Error("JWT签发失败", zap.Error(err))
		return nil, fmt.Errorf("登录处理失败")
	}

	b.logger.Info("登录成功", zap.String("工号", employeeID), zap.String("角色", emp.Role))
	return &LoginResponse{
		Token:      tokenStr,
		EmployeeID: emp.EmployeeID,
		Name:       emp.Name,
		Role:       emp.Role,
		ExpiresIn:  int64(b.jwtTTL.Seconds()),
	}, nil
}

// Register 注册新员工（工号自动分配，密码加密）
func (b *AuthBiz) Register(req RegisterRequest) (*model.Employee, error) {
	b.logger.Debug("用户注册", zap.String("姓名", req.Name))

	// 自动分配工号
	nextID, err := b.repo.NextEmployeeID()
	if err != nil {
		b.logger.Error("生成工号失败", zap.Error(err))
		return nil, fmt.Errorf("注册处理失败")
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		b.logger.Error("密码哈希失败", zap.Error(err))
		return nil, fmt.Errorf("注册处理失败")
	}

	emp := &model.Employee{
		EmployeeID:   nextID,
		Name:         req.Name,
		PasswordHash: string(hashed),
		DepartmentID: req.DepartmentID,
		Email:        req.Email,
		Role:         model.RoleEmployee,
	}

	if err := b.repo.Create(emp); err != nil {
		b.logger.Error("创建员工失败", zap.Error(err))
		return nil, fmt.Errorf("注册处理失败")
	}

	b.logger.Info("注册成功", zap.String("工号", nextID), zap.String("姓名", req.Name))
	return emp, nil
}
