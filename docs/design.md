# 企业财务报销助手 — 详细设计文档

> 版本: v1.0 | 日期: 2026-07-04 | 状态: 已评审

---

## 文档修订记录

| 版本 | 日期 | 作者 | 修订说明 |
|:--:|------|------|------|
| v1.0 | 2026-07-04 | — | 初始版本，覆盖全模块详细设计 |

---

## 一、引言

### 1.1 文档目的

本文档在《业务需求规格说明书》的基础上，对系统进行详细设计，包含模块划分、分层架构、接口协议、数据流、关键算法和异常处理策略。面向开发人员，作为编码实现的直接参考。

### 1.2 设计依据

| 文档 | 版本 | 说明 |
|------|:--:|------|
| 业务需求规格说明书 | v2.1 | 功能需求、业务规则、用例定义 |
| 技术选型报告 | v1.0 | 技术栈决策记录 |
| 项目规约 (AGENTS.md) | — | 语言规范、架构规范、金额规范 |

### 1.3 术语对照

| 业务术语 | 代码映射 |
|---------|---------|
| 报销单 | `model.Reimbursement` → `ReimbursementBiz` → `ReimbursementService` |
| 票据 | `model.InvoiceItem` |
| 审批记录 | `model.ApprovalRecord` → `ApprovalBiz` |
| 部门预算 | `model.DepartmentBudget` → `BudgetBiz` |
| 员工 | `model.Employee` |
| 部门 | `model.Department` |

---

## 二、系统架构

### 2.1 架构风格

系统采用 **DDD-lite（领域驱动设计轻量版）** 分层架构，基于 Google Wire 编译期依赖注入。

```
┌────────────────────────────────────────────────────────────────┐
│                        前端 (React)                             │
│                  REST JSON 请求 / SSE 流式响应                    │
└────────────────────────────┬───────────────────────────────────┘
                             │
┌────────────────────────────┴───────────────────────────────────┐
│                     Go 后端 (Gin HTTP)                           │
│                                                                  │
│  ┌───────────────────  router/  ──────────────────────────────┐ │
│  │  root.go: 路由注册    middleware/: CORS, JWT, Metadata      │ │
│  └────────────────────────────────────────────────────────────┘ │
│                              │                                   │
│  ┌───────────────────  internal/domain/  ─────────────────────┐ │
│  │  hub.go: ServiceHub 聚合所有 Service                        │ │
│  │                                                              │ │
│  │  ┌──── domain/{department,employee,budget,                 │ │
│  │  │       approval,reimbursement}/ ──────────────────────┐  │ │
│  │  │                                                       │  │ │
│  │  │  service.go  ← HTTP 入站适配器                         │  │ │
│  │  │      │  解析请求、参数校验、响应格式化                   │  │ │
│  │  │      ▼                                                 │  │ │
│  │  │  biz.go      ← 业务逻辑层                              │  │ │
│  │  │      │  业务校验、状态流转、事务协调                     │  │ │
│  │  │      ▼                                                 │  │ │
│  │  │  repo.go     ← 数据访问层                              │  │ │
│  │  │      GORM CRUD + AutoMigrate                           │  │ │
│  │  │                                                       │  │ │
│  │  │  dto.go      ← 请求/响应数据结构                        │  │ │
│  │  │  provider.go ← Wire ProviderSet                        │  │ │
│  │  └───────────────────────────────────────────────────────┘  │ │
│  └────────────────────────────────────────────────────────────┘ │
│                                                                  │
│  ┌───────────────────  model/  ───────────────────────────────┐ │
│  │  GORM 数据模型 (6 张表)                                      │ │
│  └────────────────────────────────────────────────────────────┘ │
│                                                                  │
│  ┌───────────────────  infra/  ───────────────────────────────┐ │
│  │  data.go: GORM DB + Redis                                   │ │
│  └────────────────────────────────────────────────────────────┘ │
│                                                                  │
│  ┌───────────────────  conf/  ─────────────────────────────────┐ │
│  │  viper.go: YAML + 环境变量配置                               │ │
│  └────────────────────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────────────────┘
```

### 2.2 依赖注入拓扑

```
wire.go (initApp)
  │
  ├── conf.GetConfig()              → *viper.Viper
  ├── log.GetLogger()               → *log.Logger
  │
  ├── infra.ProviderSet
  │     ├── NewData(vc, redis)      → *infra.Data (GORM + Redis)
  │     ├── NewRedisClient(vc)      → *redis.Client
  │     └── GetDB(data)             → *gorm.DB
  │
  ├── internal.ProviderSet           ← 聚合所有子域 ProviderSet
  │     │
  │     ├── department.ProviderSet
  │     │     └── Repo → Biz → Service
  │     ├── employee.ProviderSet
  │     │     └── Repo → Biz → Service
  │     ├── budget.ProviderSet
  │     │     └── Repo → Biz → Service
  │     ├── approval.ProviderSet
  │     │     └── Repo → Biz → Service
  │     ├── reimbursement.ProviderSet
  │     │     └── Repo → Biz → Service (依赖 BudgetBiz, ApprovalBiz)
  │     │
  │     ├── domain.NewServiceHub(   ← 聚合所有 Service
  │     │     *DepartmentService,
  │     │     *EmployeeService,
  │     │     *BudgetService,
  │     │     *ApprovalService,
  │     │     *ReimbursementService)
  │     │
  │     └── router.NewRegisterFunc()
  │
  └── NewMainApp(vc, hub, registerFunc, middlewire, data)
        └── Gin Engine + 路由注册 + 启动 HTTP Server
```

### 2.3 关键设计决策

| 决策 | 方案 | 理由 |
|------|------|------|
| DDD-lite | Service → Biz → Repo 三层 | 与 gin-template 模板一致，每层职责清晰 |
| 领域间通信 | 通过 Wire 注入 Biz 层 | 不跨层调用 Service，避免循环依赖 |
| 金额精度 | int64（分为单位） | 消除浮点误差，数据库用 BIGINT |
| 预算操作 | GORM Expr 原子更新 | 避免并发竞态：`spent_amount = spent_amount + ?` |
| 审批模型 | 多人并行审批 | 不设固定审批链，任何审批人驳回即终止 |
| 报销单号 | atomic 自增计数器 | 全局唯一，无锁竞争 |

---

## 三、数据模型设计

### 3.1 ER 图

```
┌──────────────┐       ┌──────────────────────┐
│  Department  │       │   DepartmentBudget    │
│──────────────│       │──────────────────────│
│PK id         │──1:N──│PK id                 │
│   name       │       │FK department_id      │
│FK manager_id │       │   fiscal_year        │
│              │       │   annual_budget      │
│              │       │   spent_amount       │
│              │       │   frozen_amount      │
│              │       └──────────────────────┘
│              │
│              │──1:N──┐
└──────────────┘       │
                       ▼
┌──────────────┐       ┌──────────────────────┐
│   Employee   │       │    Reimbursement      │
│──────────────│       │──────────────────────│
│PK id         │──1:N──│PK id                 │
│   employee_id│       │   reimbursement_no   │
│   name       │       │FK employee_id (string)│
│FK department_│       │FK department_id      │
│   email      │       │   total_amount       │
│   role       │       │   status             │
│   is_approver│       │   need_special_approv │
│              │       │                      │
└──────────────┘       │                      │
                       │──1:N──┐              │
                       │       ▼              │──1:N──┐
                       │┌──────────────┐      │       ▼
                       ││ InvoiceItem  │      │┌──────────────┐
                       ││──────────────│      ││ApprovalRecord│
                       ││PK id         │      ││──────────────│
                       ││FK reimburse..│      ││PK id         │
                       ││   amount     │      ││FK reimburse..│
                       ││   category   │      ││   approver.. │
                       ││   check_res..│      ││   action     │
                       │└──────────────┘      ││   comment    │
                       │                      │└──────────────┘
└──────────────────────┴──────────────────────┘
```

### 3.2 表结构

#### 3.2.1 departments（部门表）

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | INT UNSIGNED | PK AUTO_INCREMENT | 主键 |
| created_at | DATETIME | NOT NULL | 创建时间 |
| updated_at | DATETIME | NOT NULL | 更新时间 |
| deleted_at | DATETIME | INDEX | 软删除 |
| name | VARCHAR(100) | UNIQUE, NOT NULL | 部门名称 |
| manager_id | INT UNSIGNED | FK → employees.id | 部门主管 |

#### 3.2.2 employees（员工表）

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | INT UNSIGNED | PK AUTO_INCREMENT | 主键 |
| employee_id | VARCHAR(20) | UNIQUE, NOT NULL | 工号 |
| name | VARCHAR(50) | NOT NULL | 姓名 |
| department_id | INT UNSIGNED | FK → departments.id | 所属部门 |
| email | VARCHAR(100) | — | 工作邮箱 |
| role | VARCHAR(20) | DEFAULT 'employee' | employee/approver/admin |
| is_approver | TINYINT(1) | DEFAULT 0 | 是否为审批人 |

#### 3.2.3 department_budgets（部门预算表）

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | INT UNSIGNED | PK AUTO_INCREMENT | 主键 |
| department_id | INT UNSIGNED | FK, UNIQUE(dept+year) | 部门 |
| fiscal_year | INT | UNIQUE(dept+year), NOT NULL | 财年 |
| annual_budget | BIGINT | NOT NULL, DEFAULT 0 | 年度预算(分) |
| spent_amount | BIGINT | NOT NULL, DEFAULT 0 | 已结算(分) |
| frozen_amount | BIGINT | NOT NULL, DEFAULT 0 | 冻结中(分) |

**可用余额 = annual_budget - spent_amount - frozen_amount**（计算字段，不存储）

#### 3.2.4 reimbursements（报销单表）

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | INT UNSIGNED | PK AUTO_INCREMENT | 主键 |
| reimbursement_no | VARCHAR(20) | UNIQUE, NOT NULL | REIMB-YYYY-NNNN |
| employee_id | VARCHAR(20) | INDEX, NOT NULL | 申请人工号 |
| employee_name | VARCHAR(50) | NOT NULL | 申请人姓名(冗余) |
| department_id | INT UNSIGNED | FK, INDEX, NOT NULL | 部门 |
| total_amount | BIGINT | NOT NULL, DEFAULT 0 | 总金额(分) |
| status | VARCHAR(20) | INDEX, DEFAULT 'draft' | draft/pending/reviewing/approved/rejected |
| submit_note | TEXT | — | 报销事由 |
| need_special_approval | TINYINT(1) | DEFAULT 0 | 超预算标记 |

#### 3.2.5 invoice_items（票据明细表）

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | INT UNSIGNED | PK AUTO_INCREMENT | 主键 |
| reimbursement_id | INT UNSIGNED | FK, INDEX, NOT NULL | 关联报销单 |
| invoice_code | VARCHAR(50) | — | 发票代码 |
| invoice_number | VARCHAR(50) | — | 发票号码 |
| amount | BIGINT | NOT NULL, DEFAULT 0 | 金额(分) |
| invoice_date | VARCHAR(20) | — | 开票日期 |
| seller_name | VARCHAR(200) | — | 销售方 |
| category | VARCHAR(50) | INDEX, NOT NULL | 费用类别 |
| image_path | VARCHAR(500) | — | 票据图片路径 |
| ocr_raw_data | TEXT | — | OCR 原始 JSON |
| check_result | VARCHAR(20) | DEFAULT 'pending' | pass/warning/error |
| check_message | TEXT | — | 合规检查说明 |

#### 3.2.6 approval_records（审批记录表）

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | INT UNSIGNED | PK AUTO_INCREMENT | 主键 |
| reimbursement_id | INT UNSIGNED | FK, INDEX, NOT NULL | 关联报销单 |
| approver_name | VARCHAR(50) | NOT NULL | 审批人姓名 |
| approver_email | VARCHAR(100) | — | 审批人邮箱 |
| action | VARCHAR(20) | DEFAULT 'pending' | pending/approved/rejected |
| comment | TEXT | — | 审批意见 |
| action_at | DATETIME | — | 审批操作时间(NULL=未操作) |

---

## 四、模块详细设计

### 4.1 部门模块 (department)

**职责**：部门的增删改查，维护部门与主管的关联。

**类图**：

```
┌─────────────────────┐     ┌─────────────────────┐     ┌─────────────────────┐
│ DepartmentService   │────▶│   DepartmentBiz     │────▶│   DepartmentRepo    │
│─────────────────────│     │─────────────────────│     │─────────────────────│
│ biz *DepartmentBiz  │     │ repo *DepartmentRepo│     │ db *gorm.DB         │
│ logger *log.Logger  │     │ logger *log.Logger  │     │                     │
│─────────────────────│     │─────────────────────│     │─────────────────────│
│ List(c)             │     │ Create(name, mgrID) │     │ Create(*Department) │
│ GetByID(c)          │     │ GetByID(id)         │     │ GetByID(uint)       │
│ Create(c)           │     │ GetByName(name)     │     │ GetByName(string)   │
│ Update(c)           │     │ List(page,size)     │     │ List(page,size)     │
│ Delete(c)           │     │ Update(id,name,mgr) │     │ Update(*Department) │
│                     │     │ Delete(id)          │     │ Delete(uint)        │
└─────────────────────┘     └─────────────────────┘     └─────────────────────┘
```

**关键业务规则**：

- **BR-DEPT-01**: 部门名称全局唯一，创建和更新时校验
- **BR-DEPT-02**: 删除部门前检查是否有下属员工和预算记录，有则拒绝

**状态码约定**：

| 场景 | HTTP 状态码 |
|------|:--:|
| 创建成功 | 201 Created |
| 查询成功 | 200 OK |
| 更新成功 | 200 OK |
| 删除成功 | 200 OK |
| 名称重复 | 409 Conflict |
| 有下属资源无法删除 | 409 Conflict |
| 记录不存在 | 404 Not Found |
| 参数错误 | 400 Bad Request |

---

### 4.2 员工模块 (employee)

**职责**：员工信息的增删改查，审批人查询。

**关键业务规则**：

- **BR-EMP-01**: 工号全局唯一
- **BR-EMP-02**: `role` 为 `approver` 或 `admin` 时自动设置 `is_approver = true`
- **BR-EMP-03**: 查询审批人列表时只返回 `is_approver = true` 的员工

**API 设计**：

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/employees?page=1&page_size=10` | 分页列表 |
| GET | `/api/employees/approvers` | 审批人列表 |
| GET | `/api/employees/:id` | 详情 |
| POST | `/api/employees` | 创建 |
| PUT | `/api/employees/:id` | 更新 |
| DELETE | `/api/employees/:id` | 删除 |

---

### 4.3 预算模块 (budget)

**职责**：部门预算的增删改查、预算看板、预算冻结/扣减/解冻。

**关键业务规则**：

- **BR-BGT-01**: 同一部门同一财年只能有一条预算记录
- **BR-BGT-02**: 可用余额 = 年度预算 - 已结算 - 已冻结
- **BR-BGT-03**: 冻结/扣减/解冻操作使用数据库原子操作，防止并发
- **BR-BGT-04**: 预算状态：正常(使用率<60%) / 关注(60-80%) / 预警(80-95%) / 严重(>95%)

**预算状态流转**：

```
创建预算 ──▶ 提交报销(Freeze) ──▶ 审批通过(Deduct) ──▶ spent++
            │                      │
            │                      └── 冻结 → 实际支出，frozen--
            │
            └── 审批驳回(Unfreeze) ──▶ frozen--，恢复可用余额
```

**看板 API 响应结构**：

```json
{
  "departments": [{
    "id": 1,
    "department": "计算机科学与技术学院",
    "annual_budget": 50000000,     // 500,000.00元 = 50,000,000分
    "spent_amount": 32000000,
    "frozen_amount": 1500000,
    "remaining": 16500000,
    "usage_rate": 64.0
  }],
  "summary": {
    "total_budget": 110000000,
    "total_spent": 55000000,
    "total_remaining": 55000000,
    "overall_usage": 50.0
  }
}
```

---

### 4.4 审批模块 (approval)

**职责**：审批链创建、审批通过/驳回、审批进度查询。

**类图**：

```
┌─────────────────────┐     ┌─────────────────────┐     ┌─────────────────────┐
│ ApprovalService     │────▶│   ApprovalBiz       │────▶│   ApprovalRepo      │
│─────────────────────│     │─────────────────────│     │─────────────────────│
│ biz *ApprovalBiz    │     │ repo *ApprovalRepo  │     │ db *gorm.DB         │
│─────────────────────│     │─────────────────────│     │─────────────────────│
│ GetProgress(c)      │     │ CreateApprovalChain │     │ Create(*Approval..) │
│ Approve(c)          │     │ Approve(id,comment) │     │ GetByID(uint)       │
│ Reject(c)           │     │ Reject(id,reason)   │     │ ListByReimburse..   │
│                     │     │ IsAllApproved(id)   │     │ Update(*Approval..) │
│                     │     │ IsAnyRejected(id)   │     │                     │
│                     │     │ GetProgress(id)     │     │                     │
└─────────────────────┘     └─────────────────────┘     └─────────────────────┘
```

**关键业务规则**：

- **BR-APR-01**: 审批记录创建后状态为 `pending`
- **BR-APR-02**: 只有 `pending` 状态的审批可以操作（通过/驳回），防止重复操作
- **BR-APR-03**: 驳回时必须填写驳回原因
- **BR-APR-04**: 所有审批人都通过 → 报销单状态变 `approved`
- **BR-APR-05**: 任一审批人驳回 → 报销单状态变 `rejected`

**审批操作时序图**：

```
Employee          Service           Biz              Repo            DB
  │                  │                │                │               │
  │ POST /approvals/1/approve        │                │               │
  │─────────────────▶│                │                │               │
  │                  │ Approve(1,"同意")               │               │
  │                  │───────────────▶│                │               │
  │                  │                │ GetByID(1)     │               │
  │                  │                │───────────────▶│  SELECT       │
  │                  │                │◀───────────────│               │
  │                  │                │ 校验 action==pending            │
  │                  │                │ 更新 action=approved,action_at │
  │                  │                │──────────────────────────────▶│ UPDATE
  │  ◀── 200 OK ─────│◀───────────────│◀──────────────────────────────│
```

---

### 4.5 报销单模块 (reimbursement)

**职责**：报销单生命周期管理（创建 → 提交 → 审批 → 通过/驳回），是系统的核心编排模块。

**状态机**：

```
                    ┌─────────┐
                    │  draft   │  (创建)
                    └────┬────┘
                         │ Submit()
                         ▼
                    ┌─────────┐
              ┌─────│ pending  │ (待审批)
              │     └────┬────┘
              │          │
    Reject()  │     ┌────┴────┐
              │     ▼         ▼
         ┌────────┐  ┌──────────┐  ┌──────────┐
         │rejected│  │reviewing │  │ approved │
         └────────┘  └──────────┘  └──────────┘
              │                         ▲
              └─── Submit()重新提交 ────┘
```

**状态转换表**：

| 当前状态 | 操作 | 新状态 | 前置条件 | 副作用 |
|---------|------|--------|---------|--------|
| draft | Submit | pending | totalAmount > 0 | 冻结预算，创建审批链 |
| rejected | Submit | pending | totalAmount > 0 | 重新冻结预算，重建审批链 |
| pending | Approve | approved | 所有审批人通过 | 扣减预算(frozen→spent) |
| pending | Reject | rejected | 任一审批人驳回 | 解冻预算 |

**跨域编排**：ReimbursementBiz 是唯一跨域依赖的 Biz 层：

```
ReimbursementBiz
  ├── ReimbursementRepo    (本域)
  ├── BudgetBiz            (跨域 → 预算冻结/扣减/解冻)
  └── ApprovalBiz          (跨域 → 审批链状态查询)
```

**提交(Submit)的完整流程**：

```
Submit(id, totalAmount)
  │
  ├── 1. GetByID(id) → 校验状态(draft 或 rejected)
  │
  ├── 2. 校验 totalAmount > 0
  │
  ├── 3. BudgetBiz.CheckBudget(deptID, totalAmount)
  │        └── 计算 need_special_approval
  │
  ├── 4. BudgetBiz.Freeze(deptID, totalAmount)
  │        └── 数据库原子更新 frozen_amount
  │
  ├── 5. repo.Update(rm) → status=pending, total_amount=amount
  │        │
  │        └── 若失败 → BudgetBiz.Unfreeze() 回滚
  │
  └── 6. 返回报销单对象
```

**失败回滚策略**：

```
Freeze 成功 ──▶ Update 失败 ──▶ Unfreeze 回滚
Freeze 成功 ──▶ Update 成功 ──▶ 提交完成
Freeze 失败 ──▶ 直接返回错误，不更新状态
```

**报销单号生成**：

```go
var reimbursementSeq uint64  // atomic 自增计数器

func generateReimbursementNo() string {
    seq := atomic.AddUint64(&reimbursementSeq, 1)
    return fmt.Sprintf("REIMB-%d-%04d", currentYear, seq)
}
// 示例: REIMB-2026-0001, REIMB-2026-0002, ...
```

---

## 五、API 接口规范

### 5.1 通用规范

**请求格式**：`Content-Type: application/json`

**响应格式**：

```json
// 成功
{ "data": {...} }

// 列表
{ "list": [...], "total": 100, "page": 1 }

// 错误
{ "error": "错误描述" }
```

**HTTP 状态码使用**：

| 状态码 | 场景 |
|:--:|------|
| 200 | 查询成功、更新成功、删除成功 |
| 201 | 创建成功 |
| 400 | 参数错误、ID 格式错误 |
| 404 | 资源不存在 |
| 409 | 业务冲突（重复、状态不允许） |
| 500 | 服务器内部错误 |

### 5.2 接口列表

#### 部门

| 方法 | 路径 | 请求体 | 响应 |
|------|------|------|------|
| GET | `/api/departments?page=1&page_size=10` | — | `{list, total, page}` |
| GET | `/api/departments/:id` | — | `DepartmentResponse` |
| POST | `/api/departments` | `{name, manager_id?}` | `DepartmentResponse` |
| PUT | `/api/departments/:id` | `{name, manager_id?}` | `DepartmentResponse` |
| DELETE | `/api/departments/:id` | — | `{message}` |

#### 员工

| 方法 | 路径 | 请求体 | 响应 |
|------|------|------|------|
| GET | `/api/employees?page=1&page_size=10` | — | `{list, total, page}` |
| GET | `/api/employees/approvers` | — | `EmployeeResponse[]` |
| GET | `/api/employees/:id` | — | `EmployeeResponse` |
| POST | `/api/employees` | `{employee_id, name, email?, department_id, role?}` | `EmployeeResponse` |
| PUT | `/api/employees/:id` | `{name, email?, department_id, role?}` | `EmployeeResponse` |
| DELETE | `/api/employees/:id` | — | `{message}` |

#### 预算

| 方法 | 路径 | 请求体 | 响应 |
|------|------|------|------|
| GET | `/api/budgets/dashboard?year=2026` | — | `DashboardResponse` |
| GET | `/api/budgets/:id` | — | `BudgetResponse` |
| POST | `/api/budgets` | `{department_id, fiscal_year, annual_budget}` | `BudgetResponse` |
| PUT | `/api/budgets/:id` | `{annual_budget}` | `BudgetResponse` |

#### 报销单

| 方法 | 路径 | 请求体 | 响应 |
|------|------|------|------|
| GET | `/api/reimbursements?page=1&page_size=10&employee_id=E001` | — | `{list, total, page}` |
| GET | `/api/reimbursements/pending` | — | `ReimbursementResponse[]` |
| GET | `/api/reimbursements/no/:no` | — | `ReimbursementResponse` |
| GET | `/api/reimbursements/:id` | — | `ReimbursementResponse` |
| POST | `/api/reimbursements` | `{employee_id, employee_name, department_id, submit_note?}` | `ReimbursementResponse` |
| POST | `/api/reimbursements/:id/submit` | `{total_amount}` | `ReimbursementResponse` |
| POST | `/api/reimbursements/:id/approve` | — | `ReimbursementResponse` |
| POST | `/api/reimbursements/:id/reject` | — | `ReimbursementResponse` |

#### 审批

| 方法 | 路径 | 请求体 | 响应 |
|------|------|------|------|
| GET | `/api/reimbursements/:id/approvals` | — | `ApprovalRecordResponse[]` |
| POST | `/api/approvals/:id/approve` | `{comment?}` | `{message}` |
| POST | `/api/approvals/:id/reject` | `{reason}` | `{message}` |

---

## 六、日志规范

### 6.1 级别定义

| 级别 | 使用场景 | 示例 |
|:--:|------|------|
| **Debug** | 方法入口、中间状态、查询结果统计 | "开始创建员工"、"查询员工列表成功" |
| **Info** | 关键业务节点完成 | "员工创建成功"、"报销单已提交" |
| **Warn** | 业务校验不通过、降级触发 | "工号已存在，创建失败"、"预算不足，触发特殊审批" |
| **Error** | 异常、失败 | "创建员工失败"、"冻结预算失败" |

### 6.2 日志格式

使用 zap 结构化日志，所有消息使用中文：

```go
b.logger.Info("员工创建成功",
    zap.Uint("员工ID", emp.ID),
    zap.String("工号", emp.EmployeeID),
    zap.String("姓名", emp.Name),
)
```

---

## 七、异常处理策略

### 7.1 分层异常处理

| 层级 | 处理方式 | 示例 |
|------|---------|------|
| **Repo** | 返回原始 error | `return r.db.Create(d).Error` |
| **Biz** | 包装为业务语义错误，中文描述 | `return fmt.Errorf("工号'%s'已被使用", employeeID)` |
| **Service** | 映射到 HTTP 状态码 + JSON 响应 | `c.JSON(http.StatusConflict, gin.H{"error": err.Error()})` |

### 7.2 事务失败回滚

ReimbursementBiz.Submit() 采用先操作后校验 + 失败回滚模式：

```
操作 A (Freeze) 成功
操作 B (Update) 失败 → 回滚 A (Unfreeze)
操作 A (Freeze) 失败 → 直接返回错误
```

---

## 八、安全设计

| 措施 | 实现位置 | 状态 |
|------|---------|:--:|
| CORS 跨域控制 | `middleware/cors.go` | ✅ 已实现 |
| JWT 鉴权 | `middleware/auth.go` | ✅ 框架已就绪 |
| 请求元数据注入 | `middleware/metadata.go` | ✅ 已实现 |
| SQL 注入防护 | GORM 参数化查询 | ✅ 默认安全 |
| Panic 恢复 | Gin CustomRecovery | ✅ app.go 已注册 |
| 参数校验 | Gin binding tags | ✅ 每个 DTO 使用 `binding:"required"` |

---

## 九、部署架构

```
┌─────────────────────────────────────────────────────────────┐
│                    Docker Compose                             │
│                                                               │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌────────────┐  │
│  │ backend   │  │  mysql   │  │  redis   │  │  frontend  │  │
│  │ :8080     │  │  :3306   │  │  :6379   │  │  :3000     │  │
│  └──────────┘  └──────────┘  └──────────┘  └────────────┘  │
│                                                               │
│  ┌──────────┐                                                │
│  │ ocr       │  (可选，OCR 微服务)                             │
│  │ :50051    │                                                │
│  └──────────┘                                                │
└─────────────────────────────────────────────────────────────┘
```

---

## 十、目录结构总览

```
Reimbee/
├── main.go                          # 入口
├── app.go                           # MainApp: Gin Engine + HTTP Server
├── wire.go                          # Wire 注入入口
├── wire_gen.go                      # Wire 生成代码 (gitignored)
├── AGENTS.md                        # 项目规约
├── go.mod / go.sum
├── makefile
├── config.yaml.example
│
├── conf/
│   └── viper.go                     # Viper 配置加载
│
├── log/                             # zap 日志封装
│   ├── logger.go
│   ├── print.go
│   └── ...
│
├── model/                           # GORM 数据模型
│   ├── department.go                # 部门
│   ├── employee.go                  # 员工
│   ├── department_budget.go         # 部门预算
│   ├── reimbursement.go             # 报销单
│   ├── invoice_item.go              # 票据明细
│   └── approval_record.go           # 审批记录
│
├── infra/                           # 基础设施
│   ├── data.go                      # GORM + Redis 初始化
│   └── provider.go                  # Wire ProviderSet
│
├── internal/
│   ├── provider.go                  # 聚合所有子域 ProviderSet
│   ├── common/
│   │   └── request_meta.go
│   │
│   ├── domain/                      # 领域模块
│   │   ├── hub.go                   # ServiceHub
│   │   ├── provider.go
│   │   ├── department/              # 部门领域
│   │   │   ├── provider.go
│   │   │   ├── dto.go
│   │   │   ├── repo.go
│   │   │   ├── biz.go
│   │   │   └── service.go
│   │   ├── employee/                # 员工领域
│   │   │   └── (同上)
│   │   ├── budget/                  # 预算领域
│   │   │   └── (同上)
│   │   ├── approval/                # 审批领域
│   │   │   └── (同上)
│   │   └── reimbursement/           # 报销单领域（核心编排）
│   │       └── (同上)
│   │
│   └── router/                      # 路由 + 中间件
│       ├── provider.go
│       ├── root.go                  # 路由注册
│       └── middleware/
│           ├── auth.go              # JWT
│           ├── cors.go              # CORS
│           └── metadata.go          # 请求元数据注入
│
├── docs/                            # 文档
│   ├── requirements.md              # 业务需求规格说明书
│   ├── tech-selection.md            # 技术选型报告
│   └── design.md                    # 本文档
│
└── sql/                             # (待创建) 初始化 SQL
    ├── init.sql
    └── seed.sql
```

---

*文档结束*
