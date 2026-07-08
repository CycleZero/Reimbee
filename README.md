# Reimbee — 企业智能报销助手

基于大语言模型（LLM）的企业报销全流程智能助手，支持票据 OCR 识别、合规审核、预算检查、审批流程自动化。

## 技术栈

| 技术 | 说明 |
|------|------|
| [Gin](https://github.com/gin-gonic/gin) | HTTP Web 框架 |
| [Wire](https://github.com/google/wire) | 依赖注入代码生成 |
| [Viper](https://github.com/spf13/viper) | 配置管理 |
| [Zap](https://github.com/uber-go/zap) | 高性能日志 |
| [GORM](https://gorm.io) | ORM 框架 (MySQL) |
| [Redis](https://github.com/redis/go-redis) | 会话缓存 |
| [JWT](https://github.com/golang-jwt/jwt) | 认证授权 |
| [Blades](https://github.com/CycleZero/blades) | LLM Agent 框架（魔改自 go-kratos/blades） |
| [OpenAI-Go](https://github.com/openai/openai-go) | LLM 模型接入（OpenAI 兼容 API） |
| [Milvus](https://milvus.io) | 向量数据库（合规政策 RAG） |
| [MinIO](https://min.io) | 对象存储（票据图片） |

## 项目结构

```
Reimbee/
├── main.go                          # 程序入口
├── app.go                           # 应用封装（Gin Engine）
├── wire.go / wire_gen.go            # Wire 依赖注入
├── makefile                         # 构建命令
├── config.yaml / config.yaml.example # 配置文件
│
├── conf/                            # 配置模块
│   └── viper.go                     # Viper 配置加载
│
├── log/                             # 日志模块
│   └── logger.go                    # Zap 彩色日志
│
├── model/                           # 数据模型（GORM）
│   ├── constants.go                 # 角色、状态、费用类别常量
│   ├── employee.go                  # 员工模型
│   ├── department.go                # 部门模型
│   ├── department_budget.go         # 部门预算模型
│   ├── reimbursement.go             # 报销单模型
│   ├── invoice_item.go              # 票据行项目
│   ├── approval_record.go           # 审批记录
│   ├── policy_document.go           # 政策文档
│   ├── session_meta.go              # 会话元数据
│   └── session_message.go           # 会话消息（Blades 持久化）
│
├── infra/                           # 基础设施层
│   ├── provider.go                  # Wire ProviderSet
│   ├── data.go                      # MySQL + Redis 初始化
│   ├── session_repo.go              # 会话持久化 CRUD
│   ├── session_redis.go             # Redis 会话缓存
│   ├── session_state.go             # 会话业务状态
│   ├── state_store.go               # StateStore 接口
│   ├── storage.go / *_minio.go / *_local.go  # 文件存储（MinIO/本地）
│   ├── ocr.go / *_paddle.go / *_multimodal.go # OCR 识别
│   ├── pdf.go / *_maroto.go         # PDF 生成（Maroto）
│   ├── smtp.go / *_sender.go        # 邮件发送
│   ├── embedding/                   # 文本向量化（多种模型）
│   └── vectorstore/                 # 向量存储（Milvus/PGVector/Chroma）
│
├── internal/                        # 内部模块
│   ├── provider.go                  # 内部 Wire 聚合
│   ├── common/
│   │   └── request_meta.go          # 请求元数据（用户身份、角色）
│   ├── domain/                      # 业务领域（DDD-lite 分层）
│   │   ├── agent/                   # 🧠 LLM Agent 核心
│   │   │   ├── biz.go               #   ReimburseAgent（Run / HandleApprove）
│   │   │   ├── session.go           #   Session（blades.Session 实现）
│   │   │   ├── role.go              #   角色动态工具解析器
│   │   │   ├── prompt.go            #   角色定制系统提示词
│   │   │   ├── sse.go               #   SSE 流式事件输出
│   │   │   ├── middleware.go         #   日志中间件
│   │   │   ├── tools/               #   🔧 19 个 Agent 工具
│   │   │   │   ├── ocr_tool.go           # 票据识别
│   │   │   │   ├── budget_tool.go        # 预算检查
│   │   │   │   ├── compliance_agent_tool.go # 合规审核（子 Agent + RAG）
│   │   │   │   ├── search_policy_tool.go # 政策检索（向量搜索）
│   │   │   │   ├── create_reimb_tool.go  # 创建报销单
│   │   │   │   ├── submit_reimb_tool.go  # 提交报销（需确认）
│   │   │   │   ├── approve_tool.go       # 审批通过（需确认）
│   │   │   │   ├── reject_tool.go        # 驳回
│   │   │   │   ├── cancel_reimb_tool.go  # 取消草稿（需确认）
│   │   │   │   ├── pending_tool.go       # 待审批列表
│   │   │   │   ├── progress_tool.go      # 审批进度
│   │   │   │   ├── query_tool.go         # 报销记录查询
│   │   │   │   ├── reimb_detail_tool.go  # 报销单详情
│   │   │   │   ├── department_tool.go    # 部门查询
│   │   │   │   ├── pdf_tool.go           # 生成报销 PDF
│   │   │   │   ├── email_tool.go         # 邮件发送
│   │   │   │   ├── list_invoices_tool.go # 票据汇总
│   │   │   │   ├── check_deadline_tool.go   # 有效期校验
│   │   │   │   ├── interruptable.go      # 可中断工具包装器
│   │   │   │   └── role_guard.go         # 角色鉴权包装器
│   │   │   └── types/
│   │   │       └── state.go          # ReimbursementState 流程状态
│   │   ├── auth/                     # 认证模块
│   │   ├── employee/                 # 员工管理
│   │   ├── department/               # 部门管理
│   │   ├── budget/                   # 预算管理
│   │   ├── reimbursement/            # 报销单 CRUD
│   │   ├── approval/                 # 审批流程
│   │   └── compliance/               # 合规知识库
│   └── router/                       # 路由层
│       ├── root.go                   # 路由注册
│       └── middleware/               # 中间件（CORS、JWT、元数据）
└── web/                              # 前端（React + TypeScript）
    └── src/
        └── chat/                     # 聊天对话界面
```

## 分层架构

遵循 DDD-lite 架构，每个业务模块五层：

```
HTTP 请求 → service.go (HTTP) → biz.go (业务逻辑) → repo.go (数据访问) → DB
                            ↕
                         model (数据模型)
```

- **service.go**：HTTP 请求解析、参数校验、响应格式化，不持有 Repo
- **biz.go**：业务逻辑、流程编排、数据校验
- **repo.go**：数据库操作封装（GORM）
- **dto.go**：请求/响应数据传输对象
- **model/**：GORM 数据模型定义

## Agent 工作流程

```
用户上传票据图片
    ↓
🎯 recognize_invoice（OCR 识别票据信息）
    ↓
🔍 search_policy + check_compliance（RAG 合规审核）
    ↓
💰 check_budget（预算检查）
    ↓
📝 create_reimbursement → submit_reimbursement（创建并提交报销单）
    ↓  ← 用户确认（中断机制）
📄 generate_pdf → send_email（生成 PDF 并邮件通知）
    ↓
👔 approve_reimbursement / reject_reimbursement（审批人操作）
```

**三种角色**：
- **员工**（employee）：票据上传、创建报销、提交审批
- **审批人**（approver）：审批通过/驳回、查看待审批
- **管理员**（admin）：审批权限 + 全部查看权限

## 快速开始

### 环境要求

- Go 1.26+
- MySQL 8.0+
- Redis 6.0+
- MinIO（票据图片存储）
- Milvus（合规政策向量检索，可选）

### 安装步骤

```bash
# 1. 克隆项目
git clone https://github.com/CycleZero/Reimbee.git
cd Reimbee

# 2. 复制配置文件
cp config.yaml.example config.yaml
# 编辑 config.yaml，修改数据库、Redis、LLM、MinIO、Milvus 等连接信息

# 3. 安装依赖
go mod tidy

# 4. 安装 Wire 工具
go install github.com/google/wire/cmd/wire@latest

# 5. 生成依赖注入代码
make wire

# 6. 启动服务
make run
```

服务启动后：
- API: `http://localhost:8080`
- Swagger 文档: `http://localhost:8080/swagger/index.html`
- pprof: `http://localhost:6060/debug/pprof/`

## 配置说明

```yaml
# MySQL
data.db:
  host: localhost
  port: 3306
  user: root
  password: ""
  db_name: reimbee

# Redis
data.redis:
  host: localhost
  port: 6379

# LLM（OpenAI 兼容 API）
openai:
  base_url: "https://api.openai.com/v1"
  api_key: "sk-xxx"
  model: "gpt-4o"          # 也支持 deepseek-chat / doubao-pro-32k
  temperature: 0.3
  max_tokens: 4096

# 日志
log:
  mode: dev                  # dev | prod
  level: debug               # debug | info | warn | error
  dir: ./data/log
```

## 主要 API

| 方法 | 路径 | 描述 |
|------|------|------|
| POST | `/api/auth/login` | 用户登录 |
| POST | `/api/employees` | 创建员工 |
| GET | `/api/departments` | 部门列表 |
| GET | `/api/budgets/dashboard` | 预算看板 |
| POST | `/api/reimbursements` | 创建报销单（REST） |
| GET | `/api/approvals/pending` | 待审批列表 |
| POST | `/api/approvals/:id/approve` | 审批通过 |
| **GET** | **`/api/chat/stream`** | **Agent SSE 流式对话** |
| **POST** | **`/api/chat/approve`** | **Agent 中断审批恢复** |
| GET | `/api/chat/sessions` | 会话列表（游标分页） |
| GET | `/api/chat/sessions/:id/messages` | 会话消息历史 |

## Agent 工具列表

| 工具名称 | 角色 | 需要确认 | 说明 |
|---------|------|----------|------|
| `recognize_invoice` | 员工 | 否 | OCR 识别票据图片 |
| `check_budget` | 员工 | 否 | 检查部门预算余额 |
| `search_policy` | 共享 | 否 | RAG 检索报销政策 |
| `check_compliance` | 共享 | 否 | 合规审核（子 Agent） |
| `list_invoices` | 共享 | 否 | 汇总已收集票据 |
| `check_deadline` | 共享 | 否 | 校验票据 90 天有效期 |
| `get_department_id` | 员工 | 否 | 查询部门 ID |
| `create_reimbursement` | 员工 | 否 | 创建报销单草稿 |
| `submit_reimbursement` | 员工 | ✅ | 提交报销单 |
| `cancel_reimbursement` | 员工 | ✅ | 取消报销单草稿 |
| `generate_pdf` | 员工 | 否 | 生成报销单 PDF |
| `send_email` | 员工 | 否 | 发送邮件通知 |
| `query_reimbursements` | 共享 | 否 | 分页查询报销记录 |
| `get_reimbursement_detail` | 共享 | 否 | 查看报销单详情 |
| `get_reimbursement_progress` | 共享 | 否 | 查看审批进度 |
| `list_pending` | 审批人 | 否 | 查看待审批列表 |
| `approve_reimbursement` | 审批人 | ✅ | 审批通过 |
| `reject_reimbursement` | 审批人 | 否 | 驳回报销单 |
| `test_interrupt` | 共享 | ✅ | 中断流程测试 |

> 需要确认的工具（✅）会触发**中断机制**：前端弹出确认框，用户确认后 Agent 继续执行。

## 构建命令

```bash
make wire         # 生成 Wire 依赖注入代码
make build        # 编译
make rebuild      # wire + build
make run          # 直接运行
make tidy         # 整理依赖
make build-linux  # 交叉编译 Linux
go test ./...     # 运行全部测试
```

## 代码规范

- **注释**：所有代码注释使用中文
- **日志**：所有 zap 日志消息使用中文
- **错误**：所有 error 返回的消息使用中文
- **标识符**：变量、函数、类型名使用英文（Go 语言规范）
- **金额**：数据库存储 `int64`（分为单位），API 传输 `float64`（元为单位）
- **分层**：model → repo → biz → service → router，Service 层不持有 Repo

## 许可证

MIT
