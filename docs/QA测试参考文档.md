# Reimbee — QA 测试参考文档

> 版本: v1.0  
> 最后更新: 2026-07-06  
> 目标读者: 测试工程师 / QA  
> Base URL: `http://localhost:8080`  
> API 端点数: **29 REST + 1 SSE = 30 个端点**

---

## 目录

1. [项目概述](#一项目概述)
2. [命名与编码规约](#二命名与编码规约)
3. [认证与权限体系](#三认证与权限体系)
4. [通用响应格式与错误处理](#四通用响应格式与错误处理)
5. [API 端点完整参考](#五api-端点完整参考)
   - [5.1 健康检查](#51-健康检查)
   - [5.2 认证模块](#52-认证模块)
   - [5.3 部门管理](#53-部门管理)
   - [5.4 员工管理](#54-员工管理)
   - [5.5 预算管理](#55-预算管理)
   - [5.6 报销管理](#56-报销管理)
   - [5.7 审批管理](#57-审批管理)
   - [5.8 Agent SSE 流式对话](#58-agent-sse-流式对话)
6. [SSE 事件类型详解](#六sse-事件类型详解)
7. [数据模型与数据库表](#七数据模型与数据库表)
8. [变量取值与枚举常量](#八变量取值与枚举常量)
9. [测试场景矩阵](#九测试场景矩阵)

---

## 一、项目概述

**Reimbee** 是一个企业财务报销管理系统的后端服务，基于 Go 语言开发，采用 DDD-lite（领域驱动设计精简版）架构。系统提供 RESTful API 供前端调用，并内置基于 LLM 的智能报销助手（Agent），通过 SSE 协议实现流式对话交互。

### 1.1 技术栈

| 技术 | 用途 | 版本/说明 |
|------|------|-----------|
| Go | 编程语言 | 1.23+ |
| Gin | HTTP Web 框架 | v1.x |
| GORM | ORM（数据库操作） | v1.x |
| MySQL | 关系型数据库 | 8.0+ |
| Redis | 缓存/会话 | 6.0+ (可选) |
| Google Wire | 依赖注入代码生成 | - |
| Viper | 配置管理 | - |
| Zap (uber-go) | 结构化日志 | - |
| golang-jwt/jwt | JWT 认证 | v5 |
| Eino (CloudWeGo) | AI Agent 编排框架 | Graph + ReAct 模式 |

### 1.2 架构分层

```
HTTP 请求 → Router → Service（HTTP 层）→ Biz（业务逻辑层）→ Repo（数据访问层）→ DB
```

| 层 | 职责 | 文件命名 |
|----|------|---------|
| **model/** | 数据库表结构定义（GORM） | `*.go` |
| **repo** | 数据访问封装（CRUD + 查询） | `repo.go` |
| **biz** | 业务逻辑、校验、流程编排 | `biz.go` |
| **service** | HTTP 请求解析、参数校验、响应格式化 | `service.go` |
| **dto** | 请求/响应数据结构 | `dto.go` |
| **router** | 路由注册 + 中间件绑定 | `root.go` |

### 1.3 项目目录结构

```
Reimbee/
├── main.go              # 程序入口
├── app.go               # Gin Engine 初始化
├── wire.go / wire_gen.go # Wire DI 定义/生成代码
├── makefile             # 构建命令
├── config.yaml          # 配置文件
├── conf/                # 配置加载模块 (Viper)
├── log/                 # 日志模块 (Zap)
├── infra/               # 基础设施层 (MySQL/Redis 初始化)
├── model/               # 数据模型 (9 个模型文件)
├── internal/
│   ├── domain/          # 业务领域 (7 个领域模块)
│   │   ├── agent/       # AI Agent 对话
│   │   ├── auth/        # 认证 (登录/注册)
│   │   ├── department/  # 部门管理
│   │   ├── employee/    # 员工管理
│   │   ├── budget/      # 预算管理
│   │   ├── reimbursement/ # 报销单管理
│   │   ├── approval/    # 审批管理
│   │   └── compliance/  # 合规检查 + 知识库
│   ├── router/          # 路由层
│   │   └── middleware/  # 中间件 (JWT/CORS/RBAC/元数据)
│   └── common/          # 公共组件
└── data/                # 数据文件与文档
    └── API文档-前端对接.md
```

---

## 二、命名与编码规约

### 2.1 语言规范（硬性要求）

| 内容类型 | 使用语言 |
|----------|---------|
| 代码注释 | **中文** |
| 日志消息（zap） | **中文** |
| 错误消息（error 返回） | **中文** |
| 变量名 | 英文（Go 规范） |
| 函数名 | 英文（Go 规范，PascalCase / camelCase） |
| 类型名/结构体名 | 英文（Go 规范，PascalCase） |
| JSON 字段名 | 英文（snake_case，通过 `json:"xxx"` tag 定义） |

### 2.2 金额规范（关键！）

| 层级 | Go 类型 | JSON 类型 | 单位 | 示例 |
|------|---------|-----------|------|------|
| **API 传输**（请求/响应） | `int64`（DTO） | `number` | **元** | `2380`（¥23.80） |
| **数据库存储** | `int64` | - | **分** | `238000`（¥2,380.00） |

> ⚠️ **测试要点**: 请求参数中的金额字段值单位是**元**，不是分。例如报销总金额 2380 元应传 `2380`，对应数据库中存储 238000 分。

### 2.3 时间格式

所有 API 响应中的时间字段格式统一为：`YYYY-MM-DD HH:MM:SS`（Go 模板：`2006-01-02 15:04:05`）

### 2.4 分页规范

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `page` | query int | 1 | 页码，从 1 开始 |
| `page_size` | query int | 10 | 每页条数 |

分页接口的响应结构：
```json
{
  "list": [...],
  "total": 100,
  "page": 1
}
```

---

## 三、认证与权限体系

### 3.1 JWT 认证

- **认证方式**: Bearer Token
- **Header**: `Authorization: Bearer <token>`
- **JWT 签名算法**: HMAC-SHA256
- **默认密钥**: `reimbee-jwt-secret-change-in-production`（生产环境必须修改）
- **Token 有效期**: 24 小时（可在 `config.yaml` 的 `jwt.expire_hours` 配置）

**JWT Payload 包含字段**:

| 字段 | 类型 | 说明 |
|------|------|------|
| `user_id` | float64 | 用户 DB 主键 (uint) |
| `employee_id` | string | 员工工号 |
| `role` | string | 角色 |

### 3.2 角色权限矩阵

| 角色 | role 值 | 权限范围 |
|------|---------|---------|
| 普通员工 | `employee` | 查询部门/审批人/预算看板；创建/查询/提交自己的报销单 |
| 审批人 | `approver` | 员工权限 + 审批操作 + 待审批列表 + 员工列表 |
| 管理员 | `admin` | 全部权限（含部门/员工/预算的 CUD 操作） |

### 3.3 无需认证的端点

| 端点 | 说明 |
|------|------|
| `GET /health` | 健康检查 |
| `POST /api/auth/login` | 用户登录 |
| `POST /api/auth/register` | 用户注册 |

### 3.4 中间件链

所有 `/api/*` 路由按以下顺序应用中间件：
1. **CORS** — 允许跨域（`*` 源）
2. **AddMetaData** — 注入请求元数据
3. **AuthMiddleWire(false)** — JWT 强制认证
4. **RequireAdmin() / RequireApprover()** — 按路由组追加 RBAC 控制

### 3.5 CORS 配置

| 属性 | 值 |
|------|-----|
| 允许的源 | `*` |
| 允许的方法 | GET, POST, PUT, PATCH, DELETE, HEAD, OPTIONS |
| 允许的请求头 | Origin, X-Requested-With, Content-Type, Accept, Authorization |
| 凭证支持 | true |
| 预检缓存 | 86400 秒 (24h) |

---

## 四、通用响应格式与错误处理

### 4.1 成功响应

不同类型的接口返回不同格式：

| 场景 | HTTP 状态码 | 响应体格式 |
|------|------------|-----------|
| 查询单条 | 200 | 直接返回对象 JSON |
| 查询列表（分页） | 200 | `{"list": [...], "total": N, "page": N}` |
| 查询列表（无分页） | 200 | 直接返回数组 `[{...}, {...}]` |
| 创建资源 | 201 | 返回创建后的对象 JSON |
| 更新资源 | 200 | 返回更新后的对象 JSON |
| 删除资源 | 200 | `{"message": "删除成功描述"}` |
| 审批操作 | 200 | `{"message": "审批已通过/已驳回"}` |
| 提交报销单 | 200 | 返回提交后的报销单对象 |

### 4.2 错误响应格式

所有错误统一格式：
```json
{
  "error": "错误描述（中文）"
}
```

认证中间件级别的错误使用 `"message"` 字段：
```json
{
  "message": "未提供认证令牌"
}
```

### 4.3 HTTP 状态码使用规范

| 状态码 | 使用场景 |
|--------|---------|
| 200 | 查询成功、更新成功、删除成功、审批完成 |
| 201 | 资源创建成功 |
| 400 | 参数格式错误（ID 非数字、缺少必填字段、请求体 JSON 解析失败） |
| 401 | 未认证（Token 缺失或无效） |
| 403 | 已认证但无权限（角色不足） |
| 404 | 资源不存在 |
| 409 | 业务冲突（名称重复、状态不允许操作、预算不足） |
| 500 | 服务器内部错误（DB 异常等） |

---

## 五、API 端点完整参考

### 5.1 健康检查

#### `GET /health`

- **认证**: 无需
- **请求参数**: 无
- **成功响应**: `200` → `{"status": "ok"}`
- **测试要点**:
  - ✅ 服务启动后可访问
  - ✅ 不需要任何 Header
  - ✅ 返回固定 JSON 结构

---

### 5.2 认证模块

#### `POST /api/auth/register` — 用户注册

- **认证**: 无需
- **请求体**:
  ```json
  {
    "name": "张三",
    "password": "123456",
    "department_id": 1,
    "email": "zhangsan@example.com"
  }
  ```

| 字段 | 类型 | 必填 | 校验规则 |
|------|------|------|---------|
| `name` | string | ✅ | 姓名 |
| `password` | string | ✅ | 密码，至少 6 位 |
| `department_id` | uint | ✅ | 必须为已存在的部门 ID |
| `email` | string | 否 | 邮箱地址 |

- **成功响应**: `201 Created`
  ```json
  {
    "message": "注册成功",
    "employee_id": "EMP001",
    "name": "张三",
    "role": "employee"
  }
  ```

- **错误响应**:
  - `400` — 请求参数错误（缺少必填字段或密码不足 6 位）
  - `409` — 工号已存在（实际上工号自动分配，此错误极少出现）
  - `500` — 服务器内部错误

- **业务逻辑**: 工号自动分配（从 EMP001 开始按序递增），密码使用 bcrypt 加密存储，默认角色为 `employee`

- **测试场景**:
  1. ✅ 正常注册，验证工号自动分配
  2. ✅ 注册时密码少于 6 位 → 400
  3. ✅ 缺少 name → 400
  4. ✅ 缺少 department_id → 400
  5. ✅ department_id 为不存在的部门 → 数据库外键约束错误（500）
  6. ✅ 连续注册多个用户，验证工号递增

---

#### `POST /api/auth/login` — 用户登录

- **认证**: 无需
- **请求体**:
  ```json
  {
    "employee_id": "EMP001",
    "password": "123456"
  }
  ```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `employee_id` | string | ✅ | 工号 |
| `password` | string | ✅ | 密码 |

- **成功响应**: `200 OK`
  ```json
  {
    "token": "eyJhbGciOiJIUzI1NiIs...",
    "employee_id": "EMP001",
    "name": "张三",
    "role": "employee",
    "expires_in": 86400
  }
  ```

| 字段 | 类型 | 说明 |
|------|------|------|
| `token` | string | JWT Token，后续请求需放入 Authorization Header |
| `employee_id` | string | 员工工号 |
| `name` | string | 员工姓名 |
| `role` | string | 员工角色 |
| `expires_in` | int64 | Token 有效期（秒），默认 86400 = 24h |

- **错误响应**:
  - `400` — 请求参数错误（工号或密码为空）
  - `401` — 工号不存在或密码错误

- **测试场景**:
  1. ✅ 正常登录 → 200，验证 token 非空，expires_in = 86400
  2. ✅ 错误密码 → 401
  3. ✅ 不存在的工号 → 401
  4. ✅ 缺少 employee_id → 400
  5. ✅ 缺少 password → 400
  6. ✅ 用返回的 token 访问受保护接口 → 应可正常访问

---

### 5.3 部门管理

#### `GET /api/departments` — 获取部门列表（分页）

- **认证**: 所有已认证用户
- **请求参数**: 无
- **查询参数**:

| 参数 | 类型 | 默认 | 说明 |
|------|------|------|------|
| `page` | int | 1 | 页码 |
| `page_size` | int | 10 | 每页数量 |

- **成功响应**: `200 OK`
  ```json
  {
    "list": [
      {
        "id": 1,
        "name": "技术部",
        "manager_id": 3,
        "created_at": "2026-01-15 10:00:00",
        "updated_at": "2026-01-15 10:00:00"
      }
    ],
    "total": 5,
    "page": 1
  }
  ```

- **错误响应**: `500` — 服务器内部错误

- **测试场景**:
  1. ✅ 不带参数 → 默认 page=1, page_size=10
  2. ✅ page=1&page_size=2 → 每页 2 条
  3. ✅ 空列表时 total=0, list=[]
  4. ✅ page_size 传超大值

---

#### `GET /api/departments/:id` — 获取部门详情

- **认证**: 所有已认证用户
- **路径参数**: `id` (uint) — 部门主键 ID
- **成功响应**: `200 OK` — 同上单条 DepartmentResponse
- **错误响应**:
  - `400` — ID 格式错误（非数字）
  - `404` — 部门不存在

- **测试场景**:
  1. ✅ 正常查询 → 200
  2. ✅ id=99999（不存在）→ 404
  3. ✅ id=abc（非数字）→ 400

---

#### `POST /api/departments` — 创建部门 🔒管理员

- **认证**: 管理员
- **请求体**:
  ```json
  {
    "name": "市场部",
    "manager_id": 5
  }
  ```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `name` | string | ✅ | 部门名称，全局唯一 |
| `manager_id` | uint | 否 | 主管员工 ID，可以为 null |

- **成功响应**: `201 Created` — 同 GetByID 的 DepartmentResponse
- **错误响应**:
  - `400` — name 缺失
  - `409` — 部门名称已存在

- **测试场景**:
  1. ✅ 正常创建 → 201
  2. ✅ 重复名称创建 → 409
  3. ✅ 缺少 name → 400
  4. ✅ manager_id 不存在的员工 → 无校验（存储不存在的 manager_id）
  5. ✅ 非管理员角色访问 → 403
  6. ✅ 未登录访问 → 401

---

#### `PUT /api/departments/:id` — 更新部门 🔒管理员

- **认证**: 管理员
- **路径参数**: `id` (uint) — 部门主键 ID
- **请求体**:
  ```json
  {
    "name": "市场推广部",
    "manager_id": 6
  }
  ```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `name` | string | ✅ | 新部门名称 |
| `manager_id` | uint | 否 | 新主管 ID |

- **成功响应**: `200 OK` — DepartmentResponse
- **错误响应**:
  - `400` — ID 格式错误或 name 缺失
  - `404` — 部门不存在
  - `409` — 名称与其他部门冲突

- **测试场景**:
  1. ✅ 正常更新 → 200
  2. ✅ 更新不存在的部门 → 404
  3. ✅ 名称冲突 → 409
  4. ✅ 只更新 manager_id，不改 name

---

#### `DELETE /api/departments/:id` — 删除部门 🔒管理员

- **认证**: 管理员
- **路径参数**: `id` (uint) — 部门主键 ID
- **成功响应**: `200 OK` → `{"message": "部门删除成功"}`
- **错误响应**:
  - `400` — ID 格式错误
  - `404` — 部门不存在
  - `409` — 部门下存在关联员工，无法删除

- **测试场景**:
  1. ✅ 正常删除空部门 → 200
  2. ✅ 删除有员工的部门 → 409
  3. ✅ 删除不存在的部门 → 404

---

### 5.4 员工管理

#### `GET /api/employees` — 获取员工列表（分页）🔒审批人+

- **认证**: 审批人/管理员
- **查询参数**: page, page_size（与部门列表一致）
- **成功响应**: `200 OK`
  ```json
  {
    "list": [
      {
        "id": 1,
        "employee_id": "EMP001",
        "name": "张三",
        "department_id": 1,
        "department": "技术部",
        "email": "zhangsan@example.com",
        "role": "employee",
        "is_approver": false,
        "created_at": "2026-01-15 10:00:00",
        "updated_at": "2026-01-15 10:00:00"
      }
    ],
    "total": 50,
    "page": 1
  }
  ```

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | uint | 员工 DB 主键 |
| `employee_id` | string | 工号 |
| `name` | string | 姓名 |
| `department_id` | uint | 所属部门 ID |
| `department` | string | 所属部门名称 |
| `email` | string | 工作邮箱 |
| `role` | string | 角色 |
| `is_approver` | bool | 是否具有审批权限 |
| `created_at` | string | 创建时间 |
| `updated_at` | string | 更新时间 |

- **测试场景**:
  1. ✅ 审批人可访问 → 200
  2. ✅ 普通员工访问 → 403

---

#### `GET /api/employees/approvers` — 获取审批人列表（无分页）

- **认证**: 所有已认证用户
- **请求参数**: 无
- **成功响应**: `200 OK` — 数组 `[{...}, {...}]`（EmployeeResponse 格式）
- **测试场景**:
  1. ✅ 返回仅包含 is_approver=true 的员工
  2. ✅ 结果不包含分页结构（直接是数组）

---

#### `GET /api/employees/:id` — 获取员工详情 🔒审批人+

- 同 department GetByID，仅权限不同

---

#### `POST /api/employees` — 创建员工 🔒管理员

- **认证**: 管理员
- **请求体**:
  ```json
  {
    "employee_id": "EMP050",
    "name": "新员工",
    "department_id": 1,
    "email": "new@example.com",
    "role": "employee"
  }
  ```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `employee_id` | string | ✅ | 工号，全局唯一 |
| `name` | string | ✅ | 姓名 |
| `department_id` | uint | ✅ | 所属部门 ID |
| `email` | string | 否 | 邮箱 |
| `role` | string | 否 | 角色，默认 `employee` |

> ⚠️ **注意**: 此处 employee_id 需要手动传入，但 `/api/auth/register` 中工号是自动分配的。

- **成功响应**: `201 Created` — EmployeeResponse
- **错误响应**:
  - `400` — 缺少必填字段
  - `409` — 工号已存在

- **测试场景**:
  1. ✅ 正常创建 → 201
  2. ✅ 重复工号 → 409
  3. ✅ role 不填 → 默认 employee
  4. ✅ 使用 role=admin，is_approver 应自动为 true
  5. ✅ 缺少必填字段 → 400

---

#### `PUT /api/employees/:id` — 更新员工 🔒管理员

- **请求体**: 同 Create
- **成功响应**: `200 OK` — EmployeeResponse
- **错误响应**: `400` / `409`

- **测试场景**:
  1. ✅ 所有字段都可更新
  2. ✅ 工号冲突 → 409

---

#### `DELETE /api/employees/:id` — 删除员工 🔒管理员

- **成功响应**: `200 OK` → `{"message": "员工删除成功"}`
- **错误响应**:
  - `400` — ID 格式错误
  - `409` — 员工有关联数据（未完结报销单等），无法删除

---

### 5.5 预算管理

#### `GET /api/budgets/dashboard` — 预算看板

- **认证**: 所有已认证用户
- **查询参数**:

| 参数 | 类型 | 默认 | 说明 |
|------|------|------|------|
| `year` | int | 2026 | 财年 |

- **成功响应**: `200 OK`
  ```json
  {
    "departments": [
      {
        "id": 1,
        "department_id": 1,
        "department": "技术部",
        "fiscal_year": 2026,
        "annual_budget": 50000000,
        "spent_amount": 15000000,
        "frozen_amount": 5000000,
        "remaining": 30000000,
        "usage_rate": 30.0,
        "created_at": "2026-01-15 10:00:00",
        "updated_at": "2026-01-15 10:00:00"
      }
    ],
    "summary": {
      "total_budget": 50000000,
      "total_spent": 15000000,
      "total_remaining": 30000000,
      "overall_usage": 30.0
    }
  }
  ```

| 字段 | 类型 | 单位 | 计算公式 |
|------|------|------|---------|
| `annual_budget` | int64 | 元 | 年度预算 |
| `spent_amount` | int64 | 元 | 已结算金额 |
| `frozen_amount` | int64 | 元 | 冻结中（待审批） |
| `remaining` | int64 | 元 | `annual_budget - spent - frozen` |
| `usage_rate` | float64 | 百分比 | `spent / annual_budget * 100` |

- **测试场景**:
  1. ✅ 默认 year=2026 → 200
  2. ✅ 指定 year=2025 → 查询 2025 年数据
  3. ✅ 无预算记录时 departments 为空数组，summary 全为 0
  4. ✅ 验证 remaining = annual_budget - spent_amount - frozen_amount

---

#### `GET /api/budgets/:id` — 预算详情

- 同部门详情，按预算 ID 查询单条记录

---

#### `POST /api/budgets` — 创建预算 🔒管理员

- **请求体**:
  ```json
  {
    "department_id": 1,
    "fiscal_year": 2026,
    "annual_budget": 50000000
  }
  ```

| 字段 | 类型 | 必填 | 单位 | 说明 |
|------|------|------|------|------|
| `department_id` | uint | ✅ | - | 部门 ID |
| `fiscal_year` | int | ✅ | - | 财年 |
| `annual_budget` | int64 | ✅ | **元** | 年度预算 |

- **成功响应**: `201 Created`
- **错误响应**: `409` — 该部门同一年度已有预算记录

- **测试场景**:
  1. ✅ 正常创建 → 201
  2. ✅ 重复（department_id + fiscal_year 相同）→ 409
  3. ✅ 同一部门不同年份 → 应允许
  4. ✅ 验证数据库存储时 annual_budget 正确转换为分

---

#### `PUT /api/budgets/:id` — 更新预算 🔒管理员

- **请求体**:
  ```json
  {
    "annual_budget": 60000000
  }
  ```

- **成功响应**: `200 OK`
- **测试场景**: 更新预算金额后验证 remaining 重新计算

---

### 5.6 报销管理

#### 报销单状态流转图

```
draft ──submit──▶ pending ──approve──▶ approved
                    │
                    └──reject───▶ rejected
```

| 状态值 | 含义 | 可执行操作 |
|--------|------|-----------|
| `draft` | 草稿 | 编辑、提交(submit) |
| `pending` | 待审批 | 审批通过(approve)、驳回(reject) |
| `reviewing` | 审批中 | 审批通过、驳回 |
| `approved` | 已通过 | 只读，预算已扣减 |
| `rejected` | 已驳回 | 可重新编辑提交 |

#### `GET /api/reimbursements` — 报销单列表（分页）

- **认证**: 所有已认证用户
- **查询参数**:

| 参数 | 类型 | 默认 | 说明 |
|------|------|------|------|
| `page` | int | 1 | 页码 |
| `page_size` | int | 10 | 每页数量 |
| `employee_id` | string | 否 | 按工号筛选 |

- **成功响应**: `200 OK` — 带 list/total/page 的分页结构
  ```json
  {
    "list": [
      {
        "id": 1,
        "reimbursement_no": "REIMB-2026-0042",
        "employee_id": "EMP001",
        "employee_name": "张三",
        "department_id": 1,
        "department": "技术部",
        "total_amount": 238000,
        "status": "pending",
        "submit_note": "深圳出差",
        "need_special_approval": false,
        "invoices": [
          {
            "id": 1,
            "amount": 158000,
            "invoice_date": "2026-07-01",
            "category": "差旅-交通",
            "check_result": "pass"
          }
        ],
        "approvals": [
          {
            "id": 1,
            "approver_name": "李主管",
            "action": "pending",
            "comment": "",
            "action_at": null
          }
        ],
        "created_at": "2026-07-06 10:00:00",
        "updated_at": "2026-07-06 10:00:00"
      }
    ],
    "total": 15,
    "page": 1
  }
  ```

- **测试场景**:
  1. ✅ 不带筛选 → 返回所有报销单
  2. ✅ employee_id=EMP001 → 只返回该员工
  3. ✅ employee_id=不存在的工号 → list 为空

---

#### `GET /api/reimbursements/pending` — 待审批列表 🔒审批人+

- **认证**: 审批人/管理员
- **响应**: 数组（无分页），仅包含 status=pending 的报销单
- **测试场景**: 验证只返回 pending 状态的报销单

---

#### `GET /api/reimbursements/:id` — 报销单详情（按ID）

- **成功响应**: 200 — ReimbursementResponse（与列表中的单条格式相同）
- **错误**: `400`（ID 格式错误）/ `404`（不存在）

---

#### `GET /api/reimbursements/no/:no` — 报销单详情（按单号）

- **路径参数**: `no` = `REIMB-YYYY-NNNN` 格式的报销单号
- **成功响应**: 200 — 同上
- **错误**: `404` — 单号不存在

---

#### `POST /api/reimbursements` — 创建报销单（草稿）

- **认证**: 所有已认证用户
- **请求体**:
  ```json
  {
    "employee_id": "EMP001",
    "employee_name": "张三",
    "department_id": 1,
    "submit_note": "深圳出差往返机票 + 住宿"
  }
  ```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `employee_id` | string | ✅ | 员工工号 |
| `employee_name` | string | ✅ | 员工姓名 |
| `department_id` | uint | ✅ | 所属部门 ID |
| `submit_note` | string | 否 | 报销事由 |

- **成功响应**: `201 Created` — ReimbursementResponse (status=draft)
- **错误响应**: `400`（参数错误）/ `500`（生成单号失败等）

- **测试场景**:
  1. ✅ 正常创建 → 201，status=draft，total_amount=0
  2. ✅ 单号格式验证：REIMB-2026-0001 按序递增
  3. ✅ 缺少必填字段 → 400
  4. ✅ submit_note 为空 → 应允许

---

#### `POST /api/reimbursements/:id/submit` — 提交报销单

- **认证**: 所有已认证用户
- **请求体**:
  ```json
  {
    "total_amount": 238000
  }
  ```

| 字段 | 类型 | 必填 | 单位 |
|------|------|------|------|
| `total_amount` | int64 | ✅ | **元** |

- **业务流程**:
  1. 校验状态必须为 `draft`
  2. 检查部门预算是否充足（remaining >= total_amount）
  3. 冻结预算（frozen_amount 增加）
  4. 创建审批记录链
  5. 更新报销单状态为 `pending`

- **成功响应**: `200 OK` — ReimbursementResponse (status=pending)
- **错误响应**:
  - `400` — ID 格式错误或 total_amount 缺失
  - `409` — 状态不允许提交（非 draft）/ 预算不足 / 无审批人

- **测试场景**:
  1. ✅ 正常提交 → 200，状态变为 pending
  2. ✅ 再次提交同一单 → 409（非 draft）
  3. ✅ 提交已驳回的单 → 409
  4. ✅ total_amount 超过剩余预算 → 409
  5. ✅ 无审批人时提交 → 409

---

#### `POST /api/reimbursements/:id/approve` — 审批通过（批量）🔒审批人+

- **认证**: 审批人/管理员
- **请求体**: 无
- **业务流程**:
  1. 校验状态为 `pending` 或 `reviewing`
  2. 批量通过所有审批记录
  3. 预算扣减：frozen_amount → spent_amount（冻结→实际支出）
  4. 状态更新为 `approved`

- **成功响应**: `200 OK` — ReimbursementResponse (status=approved)
- **错误响应**: `409` — 状态不允许审批

- **测试场景**:
  1. ✅ 正常审批 → 200
  2. ✅ 审批 draft 状态的单 → 409
  3. ✅ 已 approved 的再次审批 → 409
  4. ✅ 验证预算：spent 增加，frozen 减少

---

#### `POST /api/reimbursements/:id/reject` — 驳回 🔒审批人+

- **认证**: 审批人/管理员
- **业务流程**:
  1. 校验状态为 `pending` 或 `reviewing`
  2. 解冻预算（frozen_amount 减少）
  3. 状态更新为 `rejected`

- **成功响应**: `200 OK` — ReimbursementResponse (status=rejected)
- **错误响应**: `409` — 状态不允许驳回

- **测试场景**:
  1. ✅ 正常驳回 → 200
  2. ✅ 驳回事由应显示在 submit_note 或审批记录的 comment 中
  3. ✅ 验证预算：frozen 恢复

---

### 5.7 审批管理

#### `GET /api/reimbursements/:id/approvals` — 审批进度查询

- **认证**: 所有已认证用户
- **成功响应**: `200 OK` — 数组
  ```json
  [
    {
      "id": 1,
      "reimbursement_id": 1,
      "approver_name": "李主管",
      "approver_email": "li@example.com",
      "action": "pending",
      "comment": "",
      "action_at": null,
      "created_at": "2026-07-06 10:00:00",
      "updated_at": "2026-07-06 10:00:00"
    }
  ]
  ```

| action 值 | 含义 |
|-----------|------|
| `pending` | 待审批 |
| `approved` | 已通过 |
| `rejected` | 已驳回 |

- **测试场景**:
  1. ✅ 新提交的报销单 → 有审批记录，action=pending
  2. ✅ 已审批过的 → action 变为 approved/rejected, action_at 有值

---

#### `POST /api/approvals/:id/approve` — 逐条审批通过 🔒审批人+

- **认证**: 审批人/管理员
- **路径参数**: `id` = 审批记录 ID
- **请求体** (可选):
  ```json
  {
    "comment": "已核实无误"
  }
  ```

- **成功响应**: `200 OK` → `{"message": "审批已通过"}`
- **错误响应**: `409` — 该审批记录已处理过

- **测试场景**:
  1. ✅ 正常通过 → 200
  2. ✅ 重复通过同一条 → 409
  3. ✅ 不传 comment → 应允许

---

#### `POST /api/approvals/:id/reject` — 逐条驳回审批 🔒审批人+

- **请求体**:
  ```json
  {
    "reason": "票据信息与申请不符"
  }
  ```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `reason` | string | ✅ | 驳回原因，不能为空 |

- **成功响应**: `200 OK` → `{"message": "审批已驳回"}`
- **错误响应**:
  - `400` — reason 为空
  - `409` — 已处理过

---

### 5.8 Agent SSE 流式对话

#### `GET /api/chat/stream`

- **认证**: 所有已认证用户
- **查询参数**:

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `session_id` | string | ✅ | 会话 ID，前端生成 UUID v7 |
| `message` | string | ✅ | 用户消息内容 |

- **Content-Type**: `text/event-stream`
- **HTTP 状态码**: `200`（建立 SSE 连接成功）
- **请求失败状态码**:
  - `400` — 缺少 session_id 或 message 参数
  - `401` — 未认证
  - `500` — 服务器不支持流式响应

- **SSE 连接生命周期**:
  1. 客户端发起 GET 请求，服务端设置 SSE 响应头
  2. 服务端推送一系列 SSE 事件
  3. 最后推送 `done` 事件，客户端关闭 EventSource

- **SSE 响应头**:
  ```
  Content-Type: text/event-stream
  Cache-Control: no-cache
  Connection: keep-alive
  X-Accel-Buffering: no
  ```

- **测试场景**:
  1. ✅ 缺少 session_id → 400
  2. ✅ 缺少 message → 400
  3. ✅ 正常对话 → 建立 SSE 连接，收事件流，收到 done 后关闭
  4. ✅ 同一 session_id 多次发送消息 → 上下文连续
  5. ✅ session_id 不同 → 独立会话

---

## 六、SSE 事件类型详解

SSE 数据推送格式：
```
event: <type>
data: <json>

```

### 6.1 事件类型速查表

| 事件类型 | 触发时机 | 前端 UI 组件 |
|----------|---------|-------------|
| `thinking` | LLM 开始推理 | 三点动画 + 文字提示 |
| `message` | LLM 输出文本 | 消息气泡（打字机效果） |
| `tool_call` | 工具调用开始 | 工具卡片（loading） |
| `tool_result` | 工具调用完成 | 工具卡片（结果展示） |
| `phase_change` | 报销阶段切换 | 进度条/阶段指示器 |
| `confirm_required` | 需要用户确认 | 确认弹窗 |
| `error` | 发生错误 | 错误提示（支持重试） |
| `done` | 流程结束 | 关闭 EventSource |

### 6.2 事件结构定义

#### `thinking` 事件
```json
{
  "type": "thinking",
  "data": {
    "message": "正在理解您的报销需求..."
  }
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `data.message` | string | 提示文字 |

#### `message` 事件
```json
{
  "type": "message",
  "data": {
    "content": "请上传您的",
    "delta": true
  }
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `data.content` | string | 文本内容 |
| `data.delta` | bool | true=流式增量追加，false=完整替换 |

> ⚠️ **测试要点**: delta=true 时前端应追加到已有文本末尾；delta=false 时前端应替换全部文本。两条 message 事件之间可能间隔很短，需确保 UI 不会掉字。

#### `tool_call` 事件
```json
{
  "type": "tool_call",
  "data": {
    "tool": "recognize_invoice",
    "input": {
      "image_path": "/uploads/invoice_001.jpg"
    }
  }
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `data.tool` | string | 工具名称 |
| `data.input` | any | 工具输入参数（结构因工具而异） |

#### `tool_result` 事件
```json
{
  "type": "tool_result",
  "data": {
    "tool": "recognize_invoice",
    "output": {
      "amount": 150000,
      "category": "差旅-交通",
      "date": "2026-07-01"
    }
  }
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `data.tool` | string | 工具名称（与 tool_call 一致） |
| `data.output` | any | 工具返回结果（结构因工具而异） |

#### `phase_change` 事件
```json
{
  "type": "phase_change",
  "data": {
    "from": "phase1_collect",
    "to": "phase2_validate",
    "summary": "票据收集完成，进入校验阶段"
  }
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `data.from` | string | 来源阶段 |
| `data.to` | string | 目标阶段 |
| `data.summary` | string | 阶段过渡摘要 |

**阶段枚举值**: `phase1_collect`（收集票据）, `phase2_validate`（合规+预算校验）, `phase3_execute`（PDF生成+邮件发送）

#### `confirm_required` 事件
```json
{
  "type": "confirm_required",
  "data": {
    "prompt": "请确认以下票据信息是否正确",
    "action": "confirm_invoice",
    "context": { ... }
  }
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `data.prompt` | string | 确认提示文字 |
| `data.action` | string | 动作标识 |
| `data.context` | any | 确认上下文数据 |

#### `error` 事件
```json
{
  "type": "error",
  "data": {
    "message": "票据识别失败，请检查图片清晰度",
    "retry": true,
    "code": "ocr_error"
  }
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `data.message` | string | 错误描述 |
| `data.retry` | bool | 是否允许用户重试 |
| `data.code` | string | 错误代码（前端可据此分类处理） |

#### `done` 事件
```json
{
  "type": "done",
  "data": null
}
```

> ⚠️ **测试要点**: done 事件的 data 为 null，不是空对象。前端收到此事件后应关闭 EventSource 连接。

### 6.3 SSE 测试场景

| 场景 | 预期行为 | 错误排查 |
|------|---------|---------|
| 正常对话 | thinking→message(delta)多次→…→done | 确认每步事件 type 正确 |
| 工具调用 | thinking→tool_call→tool_result→message | 验证 tool_call 和 tool_result 的 tool 字段一致 |
| 三阶段流程 | phase1→phase_change→phase2→phase_change→phase3→done | 验证 from/to 值正确 |
| 用户确认 | message→confirm_required→（用户响应后）→message | 确认弹窗交互 |
| LLM 错误 | error(retry=true) | 重试后端应重新处理 |
| 网络断开 | EventSource error 事件 | 前端需处理断线重连 |
| 并发对话 | 两个不同 session_id 同时对话 | 会话应隔离，互不影响 |

---

## 七、数据模型与数据库表

### 7.1 Employee（员工表）

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | uint | PK, AUTO_INCREMENT | 主键 |
| employee_id | varchar(20) | UNIQUE, NOT NULL | 工号 |
| name | varchar(50) | NOT NULL | 姓名 |
| password_hash | varchar(255) | NOT NULL, default '' | bcrypt 密码哈希 |
| department_id | uint | INDEX | 所属部门 ID |
| email | varchar(100) | | 工作邮箱 |
| role | varchar(20) | default 'employee' | employee/approver/admin |
| is_approver | bool | default false | 审批权限标记 |

### 7.2 Department（部门表）

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | uint | PK | 主键 |
| name | varchar(100) | UNIQUE, NOT NULL | 部门名称 |
| manager_id | uint | nullable | 主管员工 ID |

### 7.3 DepartmentBudget（预算表）

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | uint | PK | 主键 |
| department_id | uint | UNIQUE(department_id+fiscal_year) | 部门 ID |
| fiscal_year | int | UNIQUE(department_id+fiscal_year) | 财年 |
| annual_budget | int64 | NOT NULL, default 0 | 年度预算（**分**） |
| spent_amount | int64 | NOT NULL, default 0 | 已结算金额（**分**） |
| frozen_amount | int64 | NOT NULL, default 0 | 待审批冻结金额（**分**） |

### 7.4 Reimbursement（报销单表）

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | uint | PK | 主键 |
| reimbursement_no | varchar(20) | UNIQUE, NOT NULL | REIMB-YYYY-NNNN |
| employee_id | varchar(20) | INDEX, NOT NULL | 工号 |
| employee_name | varchar(50) | NOT NULL | 姓名 |
| department_id | uint | INDEX, NOT NULL | 部门 ID |
| total_amount | int64 | NOT NULL, default 0 | 报销总金额（**分**） |
| status | varchar(20) | INDEX, default 'draft' | 状态 |
| submit_note | text | | 报销事由 |
| need_special_approval | bool | default false | 特殊审批标记 |

### 7.5 InvoiceItem（票据明细表）

| 字段 | 类型 | 说明 |
|------|------|------|
| id | uint | 主键 |
| reimbursement_id | uint | 关联报销单 ID |
| invoice_code | varchar(50) | 发票代码 |
| invoice_number | varchar(50) | 发票号码 |
| amount | int64 | 用户确认后的金额（**分**） |
| invoice_date | varchar(20) | 开票日期 |
| seller_name | varchar(200) | 销售方名称 |
| category | varchar(50) | 费用类别 |
| image_path | varchar(500) | 票据图片路径 |
| ocr_raw_amount | int64 | OCR 原始识别金额（**分**） |
| ocr_raw_date | varchar(20) | OCR 原始日期 |
| ocr_raw_category | varchar(50) | OCR 原始类别 |
| ocr_confidence | float64 | OCR 置信度 0~1 |
| is_user_modified | bool | 是否被用户修改 |
| modification_note | text | 修改原因 |
| approver_choice | varchar(10) | 审批人裁决(ocr/user) |
| check_result | varchar(20) | 合规检查结果(pass/warning/error/pending) |
| check_message | text | 合规检查说明 |

### 7.6 ApprovalRecord（审批记录表）

| 字段 | 类型 | 说明 |
|------|------|------|
| id | uint | 主键 |
| reimbursement_id | uint | 关联报销单 ID |
| approver_name | varchar(50) | 审批人姓名 |
| approver_email | varchar(100) | 审批人邮箱 |
| action | varchar(20) | pending/approved/rejected |
| comment | text | 审批意见 |
| action_at | datetime | 操作时间（null=pending） |

### 7.7 PolicyDocument（政策文档表）

| 字段 | 类型 | 说明 |
|------|------|------|
| id | uint | 主键 |
| title | varchar(200) | 文档标题 |
| content | longtext | 原始文档内容 |
| version | varchar(20) | 版本号 |
| effective_date | varchar(20) | 生效日期 |
| status | varchar(20) | active/archived |

### 7.8 PolicyChunk（文档分块表）

| 字段 | 类型 | 说明 |
|------|------|------|
| id | uint | 主键 |
| document_id | uint | 关联文档 ID |
| chunk_index | int | 分块序号 |
| content | text | 分块文本 |
| embedding | mediumtext | 向量嵌入 JSON |

### 7.9 SessionMessage（会话消息表）

| 字段 | 类型 | 说明 |
|------|------|------|
| id | uint | 主键 |
| session_id | varchar(36) | 会话 ID (UUID v7) |
| role | varchar(20) | user/assistant/tool |
| content | text | 消息文本 |
| raw_json | mediumtext | 完整 Message JSON |

---

## 八、变量取值与枚举常量

### 8.1 角色常量

| 常量名 | 值 | 说明 |
|--------|-----|------|
| `RoleEmployee` | `"employee"` | 普通员工 |
| `RoleApprover` | `"approver"` | 审批人 |
| `RoleAdmin` | `"admin"` | 管理员 |

### 8.2 报销单状态

| 常量 | 值 | 说明 |
|------|-----|------|
| `ReimbStatusDraft` | `"draft"` | 草稿 |
| `ReimbStatusPending` | `"pending"` | 待审批 |
| `ReimbStatusReviewing` | `"reviewing"` | 审批中 |
| `ReimbStatusApproved` | `"approved"` | 已通过 |
| `ReimbStatusRejected` | `"rejected"` | 已驳回 |

### 8.3 审批动作

| 常量 | 值 | 说明 |
|------|-----|------|
| `ApprovalActionPending` | `"pending"` | 待审批 |
| `ApprovalActionApproved` | `"approved"` | 已通过 |
| `ApprovalActionRejected` | `"rejected"` | 已驳回 |

### 8.4 合规检查结果

| 常量 | 值 | 说明 |
|------|-----|------|
| `CheckResultPass` | `"pass"` | 通过 |
| `CheckResultWarning` | `"warning"` | 警告（可继续但需审批人确认） |
| `CheckResultError` | `"error"` | 严重违规（不可提交） |
| `CheckResultPending` | `"pending"` | 待检查 |

### 8.5 费用类别

| 常量 | 值 |
|------|-----|
| `CategoryTravel` | `"差旅-交通"` |
| `CategoryAccommodation` | `"差旅-住宿"` |
| `CategorySubsidy` | `"差旅-补助"` |
| `CategoryEntertainment` | `"招待费"` |
| `CategoryOffice` | `"办公用品"` |
| `CategoryPrinting` | `"印刷费"` |
| `CategoryOther` | `"其他"` |

### 8.6 SSE 事件类型

| 常量 | 值 |
|------|-----|
| `EventTypeThinking` | `"thinking"` |
| `EventTypeToolCall` | `"tool_call"` |
| `EventTypeToolResult` | `"tool_result"` |
| `EventTypeMessage` | `"message"` |
| `EventTypePhaseChange` | `"phase_change"` |
| `EventTypeConfirmRequired` | `"confirm_required"` |
| `EventTypeError` | `"error"` |
| `EventTypeDone` | `"done"` |

### 8.7 Agent 意图类别

| 常量 | 值 | 说明 |
|------|-----|------|
| `RouteNewReimbursement` | `"new_reimbursement"` | 新建报销 |
| `RouteQueryProgress` | `"query_progress"` | 进度查询 |
| `RouteQueryBudget` | `"query_budget"` | 预算查询 |
| `RoutePolicyQuestion` | `"policy_question"` | 政策咨询 |
| `RouteModifyReimbursement` | `"modify_reimbursement"` | 修改报销单 |
| `RouteGeneralChat` | `"general_chat"` | 通用对话 |

### 8.8 审批人裁决

| 常量 | 值 | 说明 |
|------|-----|------|
| `ApproverChoiceOCR` | `"ocr"` | 采纳 OCR 原始值 |
| `ApproverChoiceUser` | `"user"` | 采纳用户修正值 |

---

## 九、测试场景矩阵

### 9.1 认证与授权测试

| 编号 | 测试场景 | 预期结果 |
|------|---------|---------|
| AUTH-01 | 未登录访问任何 /api/* 接口 | 401 |
| AUTH-02 | 使用错误 Token 访问 | 401 |
| AUTH-03 | 使用过期 Token 访问 | 401 |
| AUTH-04 | 普通员工访问管理员接口（如 POST /api/departments） | 403 |
| AUTH-05 | 审批人访问管理员接口 | 403 |
| AUTH-06 | 普通员工访问审批人接口（如 GET /api/employees） | 403 |
| AUTH-07 | 登录后 Token 有效期内可正常使用 | 200/201 |
| AUTH-08 | 密码少于 6 位注册 | 400 |

### 9.2 CRUD 完整性测试

| 编号 | 测试场景 | 预期结果 |
|------|---------|---------|
| CRUD-01 | 创建→查询→更新→删除 完整生命周期（部门） | 每步正确 |
| CRUD-02 | 删除有子资源的记录（有员工的部门） | 409 |
| CRUD-03 | 查询不存在的记录 | 404 |
| CRUD-04 | ID 参数为非数字（如 `/api/departments/abc`） | 400 |
| CRUD-05 | 重复创建同名资源 | 409 |
| CRUD-06 | 分页边界：page=0, page=99999, page_size=0 | 默认值容错 |

### 9.3 金额精度测试（关键！）

| 编号 | 测试场景 | 预期结果 |
|------|---------|---------|
| AMT-01 | 创建预算 annual_budget=500000（元）→ DB 存 50000000（分） | 转换正确 |
| AMT-02 | 查询预算 → 返回 annual_budget=500000（元） | 返回元 |
| AMT-03 | 提交报销 total_amount=2380.50 元 → DB 存 238050 分 | 精度正确 |
| AMT-04 | 预算看板 remaining 计算：budget - spent - frozen | 计算正确 |
| AMT-05 | 预算使用率：spent/budget*100 | 浮点计算正确 |

### 9.4 报销状态机测试

| 编号 | 测试场景 | 预期结果 |
|------|---------|---------|
| SM-01 | draft → submit → 状态变为 pending | 200 |
| SM-02 | draft 状态直接 approve → 应拒绝 | 409 |
| SM-03 | pending → approve → 状态变为 approved | 200 |
| SM-04 | pending → reject → 状态变为 rejected | 200 |
| SM-05 | approved 再次 submit → 应拒绝 | 409 |
| SM-06 | rejected 再次 approve → 应拒绝 | 409 |
| SM-07 | 提交时预算不足 → 409 | 正确 |
| SM-08 | approve 后 spent+、frozen- | 预算流水正确 |
| SM-09 | reject 后 frozen- | 预算恢复正确 |

### 9.5 审批流程测试

| 编号 | 测试场景 | 预期结果 |
|------|---------|---------|
| APR-01 | 提交报销单后自动创建审批记录 | 审批记录存在 |
| APR-02 | 逐条审批通过 → action 变为 approved | 200 |
| APR-03 | 逐条驳回 → action 变为 rejected | 200 |
| APR-04 | 重复通过已审批记录 → 409 | 正确 |
| APR-05 | 驳回时 reason 为空 → 400 | 正确 |
| APR-06 | 批量 approve 一次性通过所有审批 | 全部 approved |
| APR-07 | 批量 approve 后 reimbursement status = approved | 状态同步 |

### 9.6 并发与边界测试

| 编号 | 测试场景 | 预期结果 |
|------|---------|---------|
| CONC-01 | 同时提交同一个报销单 | 第二个返回 409 |
| CONC-02 | 同时创建同一个部门名称 | 一个成功一个 409 |
| CONC-03 | 大量分页请求（page_size=10000） | 服务不崩溃 |
| CONC-04 | 长字符串注入（name 超 100 字符） | 数据库截断或拒绝 |
| CONC-05 | SQL 注入尝试（name 含 SQL 片段） | GORM 参数化查询防护 |
| CONC-06 | XSS 尝试（submit_note 含 HTML） | 正常存储，前端负责转义 |

### 9.7 SSE 流式对话测试

| 编号 | 测试场景 | 预期结果 |
|------|---------|---------|
| SSE-01 | 正确参数建立 SSE 连接 | 200，Content-Type=text/event-stream |
| SSE-02 | 缺少 session_id → 400 | 400 |
| SSE-03 | 缺少 message → 400 | 400 |
| SSE-04 | 对话开始收到 thinking 事件 | type=thinking |
| SSE-05 | 流式输出收到多条 message(delta=true) | 内容递增 |
| SSE-06 | 最后一条消息 delta=false | 前端应替换而非追加 |
| SSE-07 | 工具调用收到 tool_call+tool_result 对 | tool 字段一致 |
| SSE-08 | 阶段切换收到 phase_change | from/to 值有效 |
| SSE-09 | 需要确认收到 confirm_required | prompt+action+context |
| SSE-10 | 出错收到 error 事件 | message+retry+code |
| SSE-11 | 流程结束收到 done 事件 | data=null |
| SSE-12 | 连续发送多条消息（同 session_id） | 对话持续 |

### 9.8 数据一致性测试

| 编号 | 测试场景 | 预期结果 |
|------|---------|---------|
| DC-01 | 删除有预算的部门 → 409 | 不破坏引用完整性 |
| DC-02 | 删除有员工的部门 → 409 | 不破坏引用完整性 |
| DC-03 | 删除审批人的员工 → 409（有未完结审批） | 不破坏引用完整性 |
| DC-04 | 报销单审批后 budget.spent + budget.frozen = 提交前 | 总预算不变 |

---

## 附录 A：快速启动测试环境

```bash
# 1. 确保 MySQL 运行，创建数据库
mysql -u root -e "CREATE DATABASE IF NOT EXISTS reimbee CHARACTER SET utf8mb4;"

# 2. 配置数据库连接（如需要）
export DB_HOST=localhost DB_PORT=3306 DB_USER=root DB_PASSWORD=yourpass DB_NAME=reimbee

# 3. 编译运行
cd /path/to/Reimbee
go mod tidy
make rebuild
./bin/app

# 4. 验证服务
curl http://localhost:8080/health
# 预期: {"status":"ok"}
```

## 附录 B：测试用 curl 命令速查

```bash
# 注册
curl -X POST http://localhost:8080/api/auth/register \
  -H "Content-Type: application/json" \
  -d '{"name":"测试员","password":"123456","department_id":1}'

# 登录
curl -X POST http://localhost:8080/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"employee_id":"EMP001","password":"123456"}'

# 获取部门列表
curl http://localhost:8080/api/departments \
  -H "Authorization: Bearer <token>"

# 创建报销单
curl -X POST http://localhost:8080/api/reimbursements \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"employee_id":"EMP001","employee_name":"测试员","department_id":1,"submit_note":"差旅报销"}'

# 提交报销单
curl -X POST http://localhost:8080/api/reimbursements/1/submit \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"total_amount":238000}'

# 审批通过（审批人/admin）
curl -X POST http://localhost:8080/api/reimbursements/1/approve \
  -H "Authorization: Bearer <admin_token>"

# 预算看板
curl "http://localhost:8080/api/budgets/dashboard?year=2026" \
  -H "Authorization: Bearer <token>"

# SSE 对话（使用 curl 观察事件流）
curl -N "http://localhost:8080/api/chat/stream?session_id=test-001&message=我要报销" \
  -H "Authorization: Bearer <token>"
```

---

> **文档维护**: 本文档基于 Reimbee v1.0 后端的完整代码分析生成。如有 API 变更请同步更新此文档。
