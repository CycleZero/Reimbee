# Gin Template

基于 Gin 框架的 Go 后端项目通用起步模板，提取自生产级项目的最佳实践。

## 技术栈

| 技术 | 说明 |
|------|------|
| [Gin](https://github.com/gin-gonic/gin) | HTTP Web 框架 |
| [Wire](https://github.com/google/wire) | 依赖注入代码生成 |
| [Viper](https://github.com/spf13/viper) | 配置管理 |
| [Zap](https://github.com/uber-go/zap) | 高性能日志 |
| [GORM](https://gorm.io) | ORM 框架 (MySQL) |
| [Redis](https://github.com/redis/go-redis) | 缓存/会话 |
| [JWT](https://github.com/golang-jwt/jwt) | 认证授权 |

## 项目结构

```
gin-template/
├── main.go                     # 程序入口
├── app.go                      # 应用封装（Gin Engine）
├── wire.go / wire_gen.go       # Wire 依赖注入
├── makefile                    # 构建命令
├── config.yaml.example         # 配置文件示例
│
├── conf/                       # 配置模块
│   └── viper.go                # Viper 配置加载
│
├── log/                        # 日志模块
│   └── logger.go               # Zap 彩色日志
│
├── infra/                      # 基础设施层
│   ├── provider.go             # Wire ProviderSet
│   └── data.go                 # MySQL + Redis 初始化
│
├── model/                      # 数据模型
│   └── demo.go                 # 示例模型
│
├── internal/                   # 内部模块
│   ├── provider.go             # 内部 Wire 聚合
│   ├── common/                 # 公共组件
│   │   └── request_meta.go     # 请求元数据
│   ├── domain/                 # 业务领域（DDD 分层）
│   │   ├── hub.go              # ServiceHub 服务聚合
│   │   ├── provider.go         # Domain Wire 聚合
│   │   └── demo/               # 示例业务模块
│   │       ├── provider.go     # 模块 Wire Set
│   │       ├── service.go      # HTTP 处理层
│   │       ├── biz.go          # 业务逻辑层
│   │       ├── repo.go         # 数据访问层
│   │       └── dto.go          # 数据传输对象
│   └── router/                 # 路由层
│       ├── provider.go         # 中间件注册
│       ├── root.go             # 路由注册
│       └── middleware/         # 中间件
│           ├── cors.go         # 跨域处理
│           ├── auth.go         # JWT 认证
│           └── metadata.go     # 请求元数据
```

## 分层架构

每个业务模块遵循三层架构：

```
HTTP 请求 → service.go (HTTP 层) → biz.go (业务逻辑层) → repo.go (数据访问层) → DB
```

- **service.go**: 处理 HTTP 请求解析、参数校验、响应格式化
- **biz.go**: 业务逻辑、数据校验、流程编排
- **repo.go**: 数据库操作封装（GORM）
- **dto.go**: 请求/响应数据结构定义

## 快速开始

### 环境要求

- Go 1.23+
- MySQL 8.0+
- Redis 6.0+（可选）

### 安装步骤

```bash
# 1. 克隆模板
git clone <your-repo-url> myproject
cd myproject

# 2. 修改模块名（全局替换 gin-template → your-module-name）
# 修改 go.mod 第一行

# 3. 复制配置文件
cp config.yaml.example config.yaml
# 编辑 config.yaml，修改数据库连接信息

# 4. 安装依赖
go mod tidy

# 5. 安装 Wire 工具
go install github.com/google/wire/cmd/wire@latest

# 6. 生成依赖注入代码
make wire

# 7. 启动服务
make run
```

服务启动后访问：
- API: `http://localhost:8000/api/demo`
- pprof: `http://localhost:6060/debug/pprof/`

## 配置说明

```yaml
data:
  db:                  # MySQL 配置
    host: localhost
    port: 3306
    user: root
    password: your_password
    db_name: gin_template
  redis:               # Redis 配置
    host: localhost
    port: 6379
    password: ""

server:
  http:
    host: 0.0.0.0
    port: 8000
    pprof:             # 性能分析
      enable: true
      port: 6060

log:
  mode: dev            # dev | prod
  level: debug         # debug | info | warn | error
  dir: ./data/log      # 日志目录

app:
  dev_mode: true       # 开发模式
  enable_db_debug: true # 数据库调试日志
```

## API 文档

内置 Demo 模块提供 CRUD 示例：

| 方法 | 路径 | 描述 |
|------|------|------|
| POST | /api/demo | 创建 |
| GET | /api/demo | 列表 |
| GET | /api/demo/:id | 详情 |
| PUT | /api/demo/:id | 更新 |
| DELETE | /api/demo/:id | 删除 |

## 构建命令

```bash
make wire        # 生成 Wire 依赖注入代码
make build       # 编译
make rebuild     # wire + build
make run         # 直接运行
make tidy        # 整理依赖
make build-linux # 交叉编译 Linux
```

## 添加新业务模块

1. 在 `internal/domain/` 下创建新目录，例如 `user/`
2. 创建 `provider.go`、`service.go`、`biz.go`、`repo.go`、`dto.go`
3. 在 `internal/domain/hub.go` 的 `ServiceHub` 中添加新 Service
4. 在 `internal/domain/provider.go` 中引入新模块的 ProviderSet
5. 在 `internal/router/root.go` 中注册新路由
6. 运行 `make wire` 重新生成依赖注入代码

## 许可证

MIT
