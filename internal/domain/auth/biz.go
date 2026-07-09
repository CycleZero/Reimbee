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
// 负责处理用户登录认证、JWT 签发、员工注册等核心业务流程
type AuthBiz struct {
	logger    *log.Logger
	repo      *EmployeeRepo
	jwtSecret string        // JWT 签名密钥，从配置读取，生产环境必须修改
	jwtTTL    time.Duration // JWT 有效期，从配置读取，默认24小时
}

// NewAuthBiz 创建认证业务逻辑层
// 从 Viper 配置中读取 JWT 相关参数，完成业务层初始化
func NewAuthBiz(logger *log.Logger, repo *EmployeeRepo, vc *viper.Viper) *AuthBiz {
	logger.Debug("开始初始化认证业务层")

	// 从配置文件读取 JWT 签名密钥
	// 若未配置则使用默认值，生产环境务必通过配置覆盖
	secret := vc.GetString("jwt.secret")
	if secret == "" {
		// 兜底：使用硬编码默认值防止程序崩溃
		// 生产环境需在 config.yaml 中配置 jwt.secret 以覆盖此默认值
		logger.Debug("JWT密钥未在配置中设置，使用默认值", zap.Bool("生产环境需修改", true))
		secret = "reimbee-jwt-secret-change-in-production"
	}

	// 从配置文件读取 JWT 过期小时数，转换为 time.Duration
	// 使用 Duration 类型便于后续时间计算（如 now.Add(ttl)）
	ttl := time.Duration(vc.GetInt("jwt.expire_hours")) * time.Hour
	if ttl <= 0 {
		// 防止配置错误导致 JWT 永不过期或无意义的值
		logger.Debug("JWT有效期配置为0或负数，使用默认24小时")
		ttl = 24 * time.Hour
	}

	logger.Debug("认证业务层初始化完成", zap.Int64("JWT有效期(小时)", int64(ttl.Hours())))
	return &AuthBiz{
		logger:    logger,
		repo:      repo,
		jwtSecret: secret,
		jwtTTL:    ttl,
	}
}

// Login 验证用户名或工号+密码，返回 JWT
func (b *AuthBiz) Login(username, password string) (*LoginResponse, error) {
	b.logger.Debug("用户登录流程开始", zap.String("用户名", username))

	// 先按工号精确匹配，失败则按姓名匹配
	emp, err := b.repo.GetByEmployeeID(username)
	if err != nil {
		b.logger.Debug("工号未匹配，尝试姓名查询", zap.String("输入", username))
		emp, err = b.repo.GetByName(username)
		if err != nil {
			b.logger.Warn("登录失败：用户不存在", zap.String("输入", username))
			return nil, fmt.Errorf("用户名或密码错误")
		}
	}
	b.logger.Debug("员工信息查询成功", zap.String("用户", username), zap.String("角色", emp.Role))

	// ==========================================
	// 第二步：验证密码
	// ==========================================
	// bcrypt.CompareHashAndPassword 恒定时间比较，防止时序攻击
	// PasswordHash 字段使用 json:"-" 标签，不会序列化到 JSON 响应中
	b.logger.Debug("开始验证密码", zap.String("用户", username))
	if err := bcrypt.CompareHashAndPassword([]byte(emp.PasswordHash), []byte(password)); err != nil {
		// 密码不匹配同样返回统一错误信息，防止用户枚举
		b.logger.Warn("登录失败：密码错误", zap.String("用户", username))
		return nil, fmt.Errorf("用户名或密码错误")
	}
	b.logger.Debug("密码验证通过", zap.String("用户", username))

	// ==========================================
	// 第三步：构建 JWT Claims（载荷）
	// ==========================================
	now := time.Now()
	// 过期时间 = 当前时间 + 配置的有效期
	// 用于 JWT 的 exp 声明，客户端应在此时间前刷新 Token
	expiresAt := now.Add(b.jwtTTL)
	b.logger.Debug("正在构建JWT载荷",
		zap.String("用户", username),
		zap.Time("签发时间", now),
		zap.Time("过期时间", expiresAt),
	)

	claims := jwt.MapClaims{
		"user_id":     emp.ID,       // 数据库主键，用于关联其他业务表
		"employee_id": emp.EmployeeID, // 业务工号，如 EMP001，面向用户的标识
		"role":        emp.Role,     // 角色信息用于后续鉴权中间件判断权限
		"name":        emp.Name,     // 员工姓名，用于前端展示和日志记录
		"iat":         now.Unix(),   // JWT 签发时间（Issued At），用于判断 Token 新旧
		"exp":         expiresAt.Unix(), // JWT 过期时间（Expires At），超出后 Token 失效
	}

	// ==========================================
	// 第四步：使用 HS256 算法签发 JWT
	// ==========================================
	// HS256（HMAC-SHA256）是对称签名算法，服务端使用同一密钥签发和验证
	b.logger.Debug("开始签发JWT", zap.String("算法", "HS256"))
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	// SignedString 使用 jwtSecret 对 Token 进行签名，生成最终的 JWT 字符串
	tokenStr, err := token.SignedString([]byte(b.jwtSecret))
	if err != nil {
		// 签名失败通常意味着密钥格式问题或系统层面的异常
		b.logger.Error("JWT签发失败", zap.Error(err))
		return nil, fmt.Errorf("登录处理失败")
	}
	b.logger.Debug("JWT签发成功")

	// ==========================================
	// 第五步：组装登录响应，返回给调用方
	// ==========================================
	// ExpiresIn 以秒为单位返回有效期，方便前端计算 Token 刷新时机
	b.logger.Info("登录成功", zap.String("用户", username), zap.String("角色", emp.Role))
	return &LoginResponse{
		Token:      tokenStr,
		EmployeeID: emp.EmployeeID,
		Name:       emp.Name,
		Role:       emp.Role,
		ExpiresIn:  int64(b.jwtTTL.Seconds()),
	}, nil
}

// Register 注册新员工
// 流程：自动分配工号 → 密码 bcrypt 加密 → 写入数据库
// 注意：默认角色为 employee（普通员工），管理员需后续手动提升
func (b *AuthBiz) Register(req RegisterRequest) (*model.Employee, error) {
	b.logger.Debug("用户注册流程开始",
		zap.String("姓名", req.Name),
		zap.Uint("部门ID", req.DepartmentID),
	)

	// ==========================================
	// 第一步：自动分配工号
	// ==========================================
	// 工号格式为 EMP + 三位数字（如 EMP001, EMP002），由仓储层自动生成
	// 先查当前最大工号，在其基础上 +1 作为新工号
	b.logger.Debug("正在生成新工号")
	nextID, err := b.repo.NextEmployeeID()
	if err != nil {
		// 工号生成失败通常意味着数据库查询异常，中断注册流程
		b.logger.Error("生成工号失败", zap.Error(err))
		return nil, fmt.Errorf("注册处理失败")
	}
	b.logger.Debug("工号生成成功", zap.String("新工号", nextID))

	// ==========================================
	// 第二步：对明文密码进行 bcrypt 哈希加密
	// ==========================================
	// bcrypt.DefaultCost（10）是安全性与性能的平衡选择
	// 每次哈希结果不同（内置随机盐），即使相同密码也不会产生相同密文
	b.logger.Debug("正在对密码进行bcrypt哈希加密", zap.Int("cost", bcrypt.DefaultCost))
	hashed, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		// 密码哈希失败属于严重系统错误，通常由内存不足或 bcrypt 库异常引起
		b.logger.Error("密码哈希失败", zap.Error(err))
		return nil, fmt.Errorf("注册处理失败")
	}
	b.logger.Debug("密码哈希加密完成")

	// ==========================================
	// 第三步：构建员工数据模型
	// ==========================================
	// PasswordHash 存储哈希后的密码，不存储明文
	// 默认角色为 employee，只有管理员可在后台手动提升角色
	b.logger.Debug("正在构建员工数据模型", zap.String("工号", nextID), zap.String("姓名", req.Name))
	emp := &model.Employee{
		EmployeeID:   nextID,       // 自动生成的工号（EMP001 格式）
		Name:         req.Name,     // 员工姓名，来自注册请求
		PasswordHash: string(hashed), // bcrypt 哈希后的密码字符串，使用 json:"-" 避免序列化泄露
		DepartmentID: req.DepartmentID, // 所属部门 ID，外键关联 departments 表
		Email:        req.Email,    // 工作邮箱（可选字段）
		Role:         model.RoleEmployee, // 默认角色：普通员工，不具备审批和管理权限
	}

	// ==========================================
	// 第四步：将员工记录持久化到数据库
	// ==========================================
	// 仓储层的 Create 方法会调用 GORM 的 Create，自动处理 INSERT SQL
	// 成功后 emp.ID 会被 GORM 回填为数据库自增主键
	b.logger.Debug("正在将员工记录写入数据库", zap.String("工号", nextID))
	if err := b.repo.Create(emp); err != nil {
		// 写入失败可能原因：数据库连接中断、唯一索引冲突（工号重复）等
		b.logger.Error("创建员工失败", zap.Error(err), zap.String("工号", nextID))
		return nil, fmt.Errorf("注册处理失败")
	}
	b.logger.Debug("员工记录写入数据库成功", zap.String("工号", nextID))

	b.logger.Info("注册成功", zap.String("工号", nextID), zap.String("姓名", req.Name))
	return emp, nil
}
