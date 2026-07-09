package auth

import (
	"testing"

	"github.com/CycleZero/Reimbee/infra"
	"github.com/CycleZero/Reimbee/log"
	"github.com/CycleZero/Reimbee/model"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// newTestAuthBiz 创建测试专用的 AuthBiz 实例
// 使用 SQLite 内存数据库，配置 JWT 测试参数，每次调用创建独立的数据源
func newTestAuthBiz(t *testing.T) *AuthBiz {
	t.Helper()

	// 打开 SQLite 内存数据库
	gormDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err, "打开SQLite内存数据库失败")

	// 自动迁移 Employee 表结构
	err = gormDB.AutoMigrate(&model.Employee{})
	assert.NoError(t, err, "自动迁移Employee表失败")

	// 构建 infra.Data，包含 DB 连接
	data := &infra.Data{DB: gormDB}

	// 创建仓储层
	repo := NewEmployeeRepo(data)

	// 创建日志器（nop，测试中不输出日志）
	logger := &log.Logger{Logger: zap.NewNop()}

	// 创建 Viper 配置，设置 JWT 参数
	vc := viper.New()
	vc.Set("jwt.secret", "test-secret")
	vc.Set("jwt.expire_hours", 24)

	return NewAuthBiz(logger, repo, vc)
}

// TestAuthBiz_Register_Success 测试注册成功场景
// 验证：工号以 EMP 开头、角色为 employee、密码已哈希（不等于明文）、DB 中有记录
func TestAuthBiz_Register_Success(t *testing.T) {
	biz := newTestAuthBiz(t)

	req := RegisterRequest{
		Name:         "张三",
		Password:     "password123",
		DepartmentID: 1,
		Email:        "zhangsan@example.com",
	}

	emp, err := biz.Register(req)
	assert.NoError(t, err, "注册应成功")
	assert.NotNil(t, emp, "返回的员工不应为 nil")

	// 验证工号以 EMP 开头
	assert.True(t, len(emp.EmployeeID) == 6 && emp.EmployeeID[:3] == "EMP",
		"工号应以EMP开头，实际: %s", emp.EmployeeID)

	// 验证角色默认为 employee
	assert.Equal(t, model.RoleEmployee, emp.Role, "默认角色应为employee")

	// 验证密码是 bcrypt 哈希，不等于明文
	assert.NotEqual(t, req.Password, emp.PasswordHash, "密码应被哈希，不能等于明文")
	err = bcrypt.CompareHashAndPassword([]byte(emp.PasswordHash), []byte(req.Password))
	assert.NoError(t, err, "密码哈希应能通过bcrypt验证")

	// 验证姓名和邮箱
	assert.Equal(t, req.Name, emp.Name)
	assert.Equal(t, req.Email, emp.Email)

	// 验证 DB 中确实有记录
	saved, err := biz.repo.GetByEmployeeID(emp.EmployeeID)
	assert.NoError(t, err, "应能从DB中查到已注册的员工")
	assert.Equal(t, emp.Name, saved.Name)
}

// TestAuthBiz_Login_Success 测试登录成功场景
// 验证：先注册再登录，Token 非空，响应字段正确
func TestAuthBiz_Login_Success(t *testing.T) {
	biz := newTestAuthBiz(t)

	// 先注册一个员工
	req := RegisterRequest{
		Name:         "李四",
		Password:     "mypassword",
		DepartmentID: 2,
		Email:        "lisi@example.com",
	}
	emp, err := biz.Register(req)
	assert.NoError(t, err)

	// 使用正确凭证登录
	resp, err := biz.Login(emp.EmployeeID, "mypassword")
	assert.NoError(t, err, "使用正确密码登录应成功")
	assert.NotNil(t, resp, "登录响应不应为 nil")

	// 验证 Token 非空
	assert.NotEmpty(t, resp.Token, "JWT Token不应为空")

	// 验证响应字段正确
	assert.Equal(t, emp.EmployeeID, resp.EmployeeID)
	assert.Equal(t, req.Name, resp.Name)
	assert.Equal(t, model.RoleEmployee, resp.Role)

	// 验证过期时间合理（应为24小时 = 86400秒）
	assert.Equal(t, int64(86400), resp.ExpiresIn, "过期时间应为24小时(86400秒)")
}

// TestAuthBiz_Login_WrongPassword 测试密码错误场景
// 验证：使用错误密码登录返回"用户名或密码错误"
func TestAuthBiz_Login_WrongPassword(t *testing.T) {
	biz := newTestAuthBiz(t)

	// 先注册
	req := RegisterRequest{
		Name:         "王五",
		Password:     "correctpassword",
		DepartmentID: 1,
	}
	emp, err := biz.Register(req)
	assert.NoError(t, err)

	// 使用错误密码登录
	resp, err := biz.Login(emp.EmployeeID, "wrongpassword")
	assert.Nil(t, resp, "错误密码登录不应返回响应")
	assert.Error(t, err, "错误密码登录应返回错误")
	assert.Equal(t, "用户名或密码错误", err.Error())
}

// TestAuthBiz_Login_UnknownEmployee 测试不存在的工号登录
// 验证：使用不存在的工号登录返回"用户名或密码错误"
func TestAuthBiz_Login_UnknownEmployee(t *testing.T) {
	biz := newTestAuthBiz(t)

	resp, err := biz.Login("EMP999", "anypassword")
	assert.Nil(t, resp, "不存在的工号不应返回响应")
	assert.Error(t, err)
	assert.Equal(t, "用户名或密码错误", err.Error())
}

// TestAuthBiz_Register_AutoID 测试工号自动递增
// 验证：注册两次，工号分别为 EMP001 和 EMP002
func TestAuthBiz_Register_AutoID(t *testing.T) {
	biz := newTestAuthBiz(t)

	// 注册第一个员工
	emp1, err := biz.Register(RegisterRequest{
		Name:         "员工一",
		Password:     "pass111111",
		DepartmentID: 1,
	})
	assert.NoError(t, err)
	assert.Equal(t, "EMP001", emp1.EmployeeID, "第一个员工工号应为EMP001")

	// 注册第二个员工
	emp2, err := biz.Register(RegisterRequest{
		Name:         "员工二",
		Password:     "pass222222",
		DepartmentID: 1,
	})
	assert.NoError(t, err)
	assert.Equal(t, "EMP002", emp2.EmployeeID, "第二个员工工号应为EMP002")
}
