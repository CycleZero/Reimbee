# 企业财务报销智能助手 — 需求规格说明书

> 版本: v1.0 | 日期: 2026-07-03 | 基于 CycleZero/gin-template + Eino ADK

---

## 文档修订记录

| 版本 | 日期 | 说明 |
|:--:|------|------|
| v1.0 | 2026-07-03 | 初稿，企业级需求分析 |

---

## 一、项目背景与定位

### 1.1 题目来源

中国石油大学（华东）2026 暑期小学期·软件工程实训·项目二（智能体开发），原题目"企业财务报销助手"。

### 1.2 原题目要求摘要

> - 使用 Gradio 做前端、LangChain 做智能体、Python 全栈
> - 票据 OCR 识别、合规审查、预算控制、PDF 生成、邮件推送、进度查询
> - 预置模拟数据 20-50 条
> - "如有精力可以做简单的查询页面"
> - "模拟系统审批页面等"

### 1.3 本方案差异化定位

| 维度 | 原方案（Python + LangChain + Gradio） | 本方案（Go + Eino + React） |
|------|:---:|:---:|
| 语言 | Python | Go（编译型、原生并发、单一二进制部署） |
| Agent 框架 | LangChain（过度抽象） | Eino ADK（字节跳动内部验证的 Go Agent 框架） |
| 前端 | Gradio（学术界工具） | React SPA + Ant Design（企业级 UI） |
| 架构 | 单进程 All-in-One | 微服务（Go Agent + Python OCR + 独立前端） |
| IOC | 无 | Google Wire 编译期依赖注入 |
| 部署 | pip 依赖地狱 | Docker 多阶段构建，镜像 < 20MB |
| 设计标准 | 课程作业级别 | 准生产环境标准 |

### 1.4 与原题目"模拟"要求的处理策略

原题目多次出现"模拟"字样（模拟数据、模拟审批），本方案的应对策略：

| 原题目"模拟"要求 | 本方案策略 |
|------------------|-----------|
| 预置模拟数据 | 设计完整 SQL 种子脚本，数据结构与真实 HR/财务系统对齐，接口设计不因"模拟"而退化 |
| 模拟系统审批页面 | 设计标准审批流转模型（多节点、多状态），当前用 mock handler 自动审批，接口签名与真实 OA 对接一致 |
| 如有精力做简单查询 | 作为核心功能设计（预算看板 + 进度查询），不降级为"可选" |

---

## 二、技术架构

### 2.1 整体架构

```
┌──────────────────────────────────────────────────────────────┐
│             React SPA (Vite + Ant Design 5 + ECharts)        │
│  ChatPage (对话报销) │ DashboardPage (预算看板) │             │
│  ProgressPage (进度查询)                                     │
└──────────────────────────┬───────────────────────────────────┘
                           │ REST (JSON) + SSE (text/event-stream)
┌──────────────────────────┴───────────────────────────────────┐
│               Go Backend (基于 gin-template 架构)             │
│                                                               │
│  main.go ──Wire──▶ MainApp { Engine, ServiceHub, Data }       │
│                                                               │
│  ┌─────────────────── conf/ (Viper) ───────────────────────┐ │
│  │  config.yaml → 环境变量覆盖 → 配置热加载（合规规则）       │ │
│  └─────────────────────────────────────────────────────────┘ │
│                                                               │
│  ┌─────────────────── infra/ ──────────────────────────────┐ │
│  │  Data (GORM+MySQL, Redis, OCRClient, SMTPClient)         │ │
│  └─────────────────────────────────────────────────────────┘ │
│                                                               │
│  ┌─────────────────── model/ (GORM) ───────────────────────┐ │
│  │  Reimbursement, InvoiceItem, ApprovalRecord,              │ │
│  │  DepartmentBudget, Employee                              │ │
│  └─────────────────────────────────────────────────────────┘ │
│                                                               │
│  ┌────── internal/domain/{reimbursement,budget,              │ │
│  │        approval,employee,agent}/ ────────────────────────┐│ │
│  │  service.go (HTTP Handler) → biz.go (Logic) → repo.go    ││ │
│  └─────────────────────────────────────────────────────────┘ │
│                                                               │
│  ┌─────────────────── router/ ─────────────────────────────┐ │
│  │  middleware/ (CORS, JWT, Metadata, RateLimit)            │ │
│  └─────────────────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────────────┘
                           │
          ┌────────────────┼────────────────┐
          ▼                ▼                ▼
   ┌─────────────┐ ┌─────────────┐ ┌─────────────┐
   │ OCR Service  │ │  MySQL 8.0  │ │ SMTP Server  │
   │ (Python)     │ │  + Redis    │ │              │
   └─────────────┘ └─────────────┘ └─────────────┘
```

### 2.2 gin-template 架构适配

gin-template 采用 DDD-lite + Google Wire 模式，核心分层：

```
Service (HTTP 入站适配器) → Biz (业务逻辑) → Repo (数据出口适配器)
```

**本项目各模块与模板的映射：**

| 模板原始结构 | 本项目映射 |
|-------------|----------|
| `model/demo.go` | 替换为 `model/reimbursement.go`, `model/invoice_item.go`, `model/approval_record.go`, `model/department_budget.go`, `model/employee.go` |
| `internal/domain/demo/` | 新增 `reimbursement/`, `budget/`, `approval/`, `employee/` 四个领域 + `agent/` Agent 编排层 |
| `internal/domain/hub.go` | 扩展 ServiceHub，注入 AgentRunner |
| `infra/data.go` | 新增 `OCRClient`, `SMTPClient`, `PDFGenerator`，均通过 Wire 注入 |
| `router/root.go` | 新增 SSE 路由 + reimbursement/budget/approval REST 路由组 |
| `internal/router/middleware/` | 新增 `ratelimit.go`, `request_id.go` |

### 2.3 Wire 依赖注入拓扑

```
wire.go (initApp)
  ├── conf.GetConfig()                    → *viper.Viper
  ├── log.GetLogger()                     → *log.Logger
  ├── infra.ProviderSet
  │     ├── NewData(vc, redis)            → *infra.Data
  │     ├── NewRedisClient(vc)             → *redis.Client
  │     ├── GetDB(data)                    → *gorm.DB
  │     ├── NewOCRClient(vc)               → *OCRClient
  │     ├── NewSMTPClient(vc)              → *SMTPClient
  │     └── NewPDFGenerator()              → *PDFGenerator
  ├── internal.ProviderSet
  │     ├── domain/reimbursement.ProviderSet
  │     ├── domain/budget.ProviderSet
  │     ├── domain/approval.ProviderSet
  │     ├── domain/employee.ProviderSet
  │     ├── domain/agent.ProviderSet
  │     └── router.NewRegisterFunc()
  └── NewMainApp(vc, hub, registerFunc, middlewire, data)
```

### 2.4 Eino 工具与 Wire DI 的集成问题（关键设计决策）

**问题**: Eino 的 `utils.InferTool()` 期望一个纯函数 `func(ctx, *Input) (*Output, error)`，但我们的工具需要依赖注入（访问 OCRClient、查询数据库、发送邮件）。

**方案对比**:

| 方案 | 描述 | 优点 | 缺点 |
|------|------|------|------|
| A: 闭包捕获 | 工厂函数返回闭包，捕获已注入的依赖 | 最简单，与 InferTool 兼容 | 无法通过 Wire 管理工具本身 |
| B: 实现 BaseTool 接口 | Tool 作为 struct，持有依赖引用 | 完全 Wire 兼容，类型安全 | 需手写 `Info()` 和 `InvokableRun()`，代码量增加 |
| C: Tool 工厂 + Provider | 每个工具一个工厂函数，接收依赖，返回 `tool.InvokableTool` | Wire 兼容 + 代码简洁 | 额外的抽象层 |

**选定方案**: **C（Tool 工厂 + Provider）**

```go
// internal/domain/agent/tools/ocr_tool.go
func NewOCRTool(ocrClient *infra.OCRClient) tool.InvokableTool {
    return utils.InferTool("recognize_invoice",
        "识别上传的票据图片/PDF，提取发票代码、金额、日期、购销方信息。调用时机：用户上传发票后。",
        func(ctx context.Context, input *OCRInput) (*OCRResult, error) {
            return ocrClient.Recognize(ctx, input.ImageData, input.MimeType)
        })
}

// 在 provider.go 中注册
var ToolProviderSet = wire.NewSet(
    NewOCRTool,
    NewComplianceTool,
    NewBudgetTool,
    NewPDFTool,
    NewEmailTool,
    NewProgressTool,
    wire.Struct(new(ToolSet), "*"),
)

type ToolSet struct {
    OCR        tool.InvokableTool
    Compliance tool.InvokableTool
    Budget     tool.InvokableTool
    PDF        tool.InvokableTool
    Email      tool.InvokableTool
    Progress   tool.InvokableTool
}

func (ts *ToolSet) All() []tool.BaseTool {
    return []tool.BaseTool{ts.OCR, ts.Compliance, ts.Budget, ts.PDF, ts.Email, ts.Progress}
}
```

---

## 三、领域模型设计

### 3.1 领域划分

```
reimbursement/   报销单生命周期管理（创建、查询、状态流转）
budget/          部门预算管理（查询、冻结、解冻）
approval/        审批流转引擎（多节点、多状态）
employee/        员工信息（基础数据，为后续对接 HR 系统预留）
agent/           LLM Agent 编排层（ChatModelAgent + Runner + Tools）
```

### 3.2 数据库 ER 模型

```
┌───────────────┐       ┌────────────────────┐
│   Employee    │       │  DepartmentBudget   │
│───────────────│       │────────────────────│
│ PK id         │       │ PK id              │
│    employee_id│       │    department      │
│    name       │       │    fiscal_year     │
│    department │───────│    annual_budget   │
│    email      │ 关联   │    spent_amount    │
│    role       │       │    frozen_amount   │
│    is_approver│       └────────────────────┘
└───────┬───────┘
        │ 1:N
        ▼
┌───────────────────┐
│  Reimbursement    │        ┌────────────────────┐
│───────────────────│        │   InvoiceItem       │
│ PK id             │ 1:N    │────────────────────│
│    reimbursement_no│───────│ PK id              │
│ FK employee_id    │        │ FK reimbursement_id│
│    employee_name  │        │    invoice_code    │
│    department     │        │    invoice_number  │
│    total_amount   │        │    amount          │
│    status         │        │    invoice_date    │
│    submit_note    │        │    seller_name     │
└────────┬──────────┘        │    category        │
         │                   │    image_path      │
         │ 1:N               │    ocr_raw_data    │
         ▼                   │    check_result    │
┌────────────────────┐       │    check_message   │
│  ApprovalRecord    │       └────────────────────┘
│────────────────────│
│ PK id              │
│ FK reimbursement_id│
│    approver_name   │
│    approver_email  │
│    node_name       │
│    node_order      │
│    action          │
│    comment         │
│    action_at       │
└────────────────────┘
```

### 3.3 状态机设计

#### 3.3.1 报销单状态流转

```
                    ┌─────────┐
                    │  draft   │ (初始创建，未提交)
                    └────┬────┘
                         │ submit
                         ▼
                    ┌─────────┐
                    │ pending  │ (已提交，等待审批)
                    └────┬────┘
                         │
              ┌──────────┼──────────┐
              ▼          ▼          ▼
         ┌────────┐ ┌────────┐ ┌──────────┐
         │approved│ │rejected│ │cancelled │
         └────────┘ └────┬───┘ └──────────┘
                         │
                         │ resubmit
                         ▼
                    ┌─────────┐
                    │ pending  │
                    └─────────┘
```

#### 3.3.2 审批节点状态

```
pending ──▶ approved
         ──▶ rejected
         ──▶ skipped (金额未触发该节点阈值)
```

### 3.4 GORM 模型定义

```go
// model/reimbursement.go
type Reimbursement struct {
    gorm.Model
    ReimbursementNo string         `gorm:"type:varchar(20);uniqueIndex;not null;comment:报销单号 REIMB-YYYY-NNNN"`
    EmployeeID      string         `gorm:"type:varchar(20);index;not null;comment:申请人工号"`
    EmployeeName    string         `gorm:"type:varchar(50);not null;comment:申请人姓名"`
    Department      string         `gorm:"type:varchar(100);index;not null;comment:部门"`
    TotalAmount     float64        `gorm:"type:decimal(12,2);not null;comment:报销汇总金额"`
    Status          string         `gorm:"type:varchar(20);default:draft;index;comment:draft/pending/approved/rejected/cancelled"`
    SubmitNote      string         `gorm:"type:text;comment:报销事由说明"`
    Invoices        []InvoiceItem  `gorm:"foreignKey:ReimbursementID;constraint:OnDelete:CASCADE"`
    Approvals       []ApprovalRecord `gorm:"foreignKey:ReimbursementID;constraint:OnDelete:CASCADE"`
}

// model/invoice_item.go
type InvoiceItem struct {
    gorm.Model
    ReimbursementID uint    `gorm:"index;not null;comment:关联报销单ID"`
    InvoiceCode     string  `gorm:"type:varchar(50);comment:发票代码"`
    InvoiceNumber   string  `gorm:"type:varchar(50);comment:发票号码"`
    Amount          float64 `gorm:"type:decimal(12,2);not null;comment:金额"`
    InvoiceDate     string  `gorm:"type:varchar(20);comment:开票日期"`
    SellerName      string  `gorm:"type:varchar(200);comment:销售方名称"`
    Category        string  `gorm:"type:varchar(50);not null;index;comment:费用类别(travel/accommodation/meal/office/other)"`
    ImagePath       string  `gorm:"type:varchar(500);comment:原始票据存储路径"`
    OCRRawData      string  `gorm:"type:text;comment:OCR识别原始JSON"`
    CheckResult     string  `gorm:"type:varchar(20);default:pending;comment:pass/warning/error"`
    CheckMessage    string  `gorm:"type:text;comment:合规检查结果说明"`
}

// model/approval_record.go
type ApprovalRecord struct {
    gorm.Model
    ReimbursementID uint       `gorm:"index;not null"`
    ApproverName    string     `gorm:"type:varchar(50);not null;comment:审批人姓名"`
    ApproverEmail   string     `gorm:"type:varchar(100);comment:审批人邮箱"`
    NodeName        string     `gorm:"type:varchar(50);not null;comment:审批节点名称(部门主管/财务复核/总监审批)"`
    NodeOrder       int        `gorm:"not null;comment:审批顺序 1=第一级 2=第二级..."`
    Action          string     `gorm:"type:varchar(20);default:pending;comment:pending/approved/rejected/skipped"`
    Comment         string     `gorm:"type:text;comment:审批意见"`
    ActionAt        *time.Time `gorm:"comment:审批操作时间"`
}

// model/department_budget.go
type DepartmentBudget struct {
    gorm.Model
    Department    string  `gorm:"type:varchar(100);uniqueIndex:idx_dept_year;not null;comment:部门名称"`
    FiscalYear    int     `gorm:"uniqueIndex:idx_dept_year;not null;comment:财年"`
    AnnualBudget  float64 `gorm:"type:decimal(14,2);not null;comment:年度预算总额"`
    SpentAmount   float64 `gorm:"type:decimal(14,2);default:0;comment:已结算金额"`
    FrozenAmount  float64 `gorm:"type:decimal(14,2);default:0;comment:冻结金额(已提交待审批)"`
}

// model/employee.go
type Employee struct {
    gorm.Model
    EmployeeID   string `gorm:"type:varchar(20);uniqueIndex;not null;comment:工号"`
    Name         string `gorm:"type:varchar(50);not null;comment:姓名"`
    Department   string `gorm:"type:varchar(100);index;comment:所属部门"`
    Email        string `gorm:"type:varchar(100);comment:邮箱"`
    Role         string `gorm:"type:varchar(20);default:employee;comment:employee/manager/finance/admin"`
    IsApprover   bool   `gorm:"default:false;comment:是否有审批权限"`
}
```

---

## 四、功能需求

### 4.1 功能全景图

```
┌─────────────────────────────────────────────────────┐
│                   前端 (React SPA)                    │
│  ┌──────────┐  ┌──────────────┐  ┌──────────────┐  │
│  │ F1 智能  │  │ F2 预算看板   │  │ F3 进度查询   │  │
│  │ 对话报销  │  │              │  │              │  │
│  └────┬─────┘  └──────┬───────┘  └──────┬───────┘  │
└───────┼───────────────┼──────────────────┼──────────┘
        │               │                  │
┌───────┴───────────────┴──────────────────┴──────────┐
│                   后端 (Go)                           │
│                                                      │
│  Agent Layer (Eino ADK)                              │
│  ┌──────────────────────────────────────────────┐   │
│  │  ChatModelAgent (ReAct Loop)                  │   │
│  │  ├── Tool: ocr_recognize                     │   │
│  │  ├── Tool: check_compliance                  │   │
│  │  ├── Tool: check_budget                      │   │
│  │  ├── Tool: generate_pdf                      │   │
│  │  ├── Tool: send_email                        │   │
│  │  └── Tool: query_progress                    │   │
│  └──────────────────────────────────────────────┘   │
│                                                      │
│  Business Layer                                      │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐            │
│  │Reimburse │ │ Budget   │ │ Approval │            │
│  │mentBiz   │ │ Biz      │ │ Biz      │            │
│  └──────────┘ └──────────┘ └──────────┘            │
│                                                      │
│  Infrastructure                                      │
│  ┌──────┐ ┌──────┐ ┌──────┐ ┌──────┐              │
│  │MySQL │ │Redis │ │OCR   │ │SMTP  │              │
│  │GORM  │ │      │ │Client│ │Client│              │
│  └──────┘ └──────┘ └──────┘ └──────┘              │
└──────────────────────────────────────────────────────┘
```

### 4.2 F1: 智能对话报销（核心流程, P0）

**概述**: 用户通过自然语言对话完成报销全流程，Agent 自动调度 OCR 识别、合规审查、预算控制、PDF 生成、邮件推送。

**交互协议（SSE 事件类型）**:

| 事件类型 | 方向 | 含义 | 前端渲染 |
|---------|:---:|------|---------|
| `thinking` | S→C | Agent 正在思考/分析 | 思考动画（三点跳动） |
| `tool_call` | S→C | 正在调用工具 | 工具调用卡片（tool name + input summary） |
| `tool_result` | S→C | 工具执行结果 | 更新工具卡片状态（✓/✗） |
| `message` | S→C | Agent 文本回复（流式） | 逐字符追加到对话气泡 |
| `confirm_required` | S→C | 需要用户确认 | 显示确认/取消按钮 |
| `error` | S→C | 发生错误 | 错误提示 + 重试按钮 |
| `done` | S→C | 本轮对话结束 | 停止加载状态 |

**支持的用户意图**:

| 意图 | 示例输入 | Agent 调度路径 |
|------|---------|--------------|
| 新建报销 | "我要报销一张 1500 元的差旅发票" | OCR → 合规 → 预算 → PDF → 邮件 |
| 查询进度 | "REIMB-2026-0001 审批到哪了" | Progress Tool |
| 查询预算 | "计算机学院还剩多少预算" | Budget Tool |
| 政策咨询 | "差旅住宿标准是多少" | RAG 检索（Phase 2） |
| 修改报销 | "把刚才那张发票金额改成 2000" | 更新 InvoiceItem |

**异常处理矩阵**:

| 异常场景 | Agent 行为 | 降级方案 | 用户感知 |
|---------|-----------|---------|---------|
| OCR 识别失败 | 告知失败原因，请求重新上传 | 提示"请确保票据清晰，或手动输入金额" | 可重试或手动输入 |
| 合规检查 warning | 告知具体违规项，询问是否继续提交 | 标记为 warning，允许用户确认后强制提交 | 展示黄色警告，需确认 |
| 合规检查 error | 告知违规，拒绝提交 | 引导用户修改后重新提交 | 展示红色错误，不可提交 |
| 预算不足 | 告知余额 + 触发特殊审批 | 标记 need_special_approval，走更高级别审批链 | 展示预算余额，告知需额外审批 |
| LLM API 超时 | 返回错误，保留对话上下文 | 前端显示重试按钮，自动携带最近 3 轮历史 | 可点击重试 |
| 邮件发送失败 | 告知发送失败 | 报销单仍保存，邮件状态标记为 failed | 报销单已保存但邮件未通知 |
| OCR 服务不可用 | 降级为手动输入模式 | Agent 跳过 OCR 工具，直接询问金额和类别 | 表单式手动输入 |

**涉及的数据变更（事务）**:

```
BEGIN
  1. INSERT Reimbursement (status=draft)
  2. INSERT InvoiceItem × N
  3. INSERT ApprovalRecord × M (根据审批链配置创建节点)
  4. UPDATE DepartmentBudget SET frozen_amount += total_amount
COMMIT
```

### 4.3 F2: 预算看板 (P1)

**API**: `GET /api/budget/dashboard`

**响应结构**:

```json
{
  "departments": [{
    "name": "计算机科学与技术学院",
    "annual_budget": 500000.00,
    "spent": 320000.00,
    "frozen": 15000.00,
    "remaining": 165000.00,
    "usage_rate": 67.00,
    "status": "normal",
    "pending_count": 3,
    "pending_amount": 15000.00
  }],
  "summary": {
    "total_budget": 1100000.00,
    "total_spent": 550000.00,
    "total_remaining": 550000.00,
    "overall_usage_rate": 50.00
  }
}
```

**可视化要求**:
- 环形图：各部门预算使用率对比
- 柱状图：各部门已审批 vs 待审批金额
- 数据表格：详细数字，支持排序
- 预警着色：usage_rate < 60% 绿色, 60-80% 黄色, 80-95% 橙色, > 95% 红色

### 4.4 F3: 审批进度查询 (P1)

**API**: `GET /api/progress/:reimbursement_no`

```json
{
  "reimbursement_no": "REIMB-2026-0001",
  "status": "pending",
  "total_amount": 3210.00,
  "department": "计算机科学与技术学院",
  "submitter": "王小明",
  "submitted_at": "2026-07-03 10:15:00",
  "nodes": [
    {
      "name": "部门主管审批",
      "approver": "张三",
      "status": "approved",
      "comment": "同意报销",
      "action_at": "2026-07-03 14:30:00",
      "order": 1
    },
    {
      "name": "财务复核",
      "approver": "李四",
      "status": "pending",
      "comment": null,
      "action_at": null,
      "order": 2
    }
  ],
  "invoices": [
    {"code": "3100221130", "amount": 2580.00, "category": "travel"},
    {"code": "3100221131", "amount": 450.00, "category": "accommodation"}
  ]
}
```

**审批链生成规则**:

| 审批节点 | 触发条件 | 审批人来源 |
|---------|---------|-----------|
| 部门主管审批 | 总是触发 | Employee 表 role=manager 且同部门 |
| 财务复核 | 总是触发 | Employee 表 role=finance |
| 总监审批 | total_amount > ¥10,000 或 need_special_approval=true | Employee 表 role=admin |

### 4.5 F4: OCR 微服务接口 (P1, 可降级)

**gRPC 协议**（首选）:

```protobuf
service OCRService {
  rpc RecognizeInvoice(InvoiceRequest) returns (InvoiceResponse);
}

message InvoiceRequest {
  bytes image_data = 1;
  string mime_type = 2;  // "image/jpeg", "image/png", "application/pdf"
}

message InvoiceResponse {
  string invoice_code = 1;
  string invoice_number = 2;
  double amount = 3;
  string date = 4;
  string seller_name = 5;
  string seller_tax_id = 6;
  string buyer_name = 7;
  double confidence = 8;
  string raw_text = 9;
  string error = 10;
  bool retry = 11;
}
```

**降级策略**: gRPC 不可用时降级为 HTTP REST `POST /recognize`，两者均不可用时降级为引导用户手动输入。

### 4.6 F5: 邮件通知 (P1)

**触发时机**: 报销单提交成功后

**邮件模板**:
- 主题: `[报销审批] {申请人} - ¥{金额} - {类别}`
- 正文: HTML 格式，包含报销摘要表格 + 审批链接
- 附件: PDF 报销单

**失败处理**:
- 发送失败不影响报销单创建
- ApprovalRecord 记录发送状态
- 支持管理后台手动重发

### 4.7 F6: 文件上传 (P1)

**约束**:
- 允许格式: JPG, PNG, BMP, PDF
- 单文件最大: 10MB
- 存储路径: `./uploads/{yyyy}/{mm}/{dd}/{uuid}.{ext}`
- 数据库仅存储相对路径

**安全措施**:
- MIME type 白名单校验（禁止仅依赖扩展名）
- 文件大小限制
- 文件名随机化（UUID）防止路径遍历

---

## 五、Agent 层详细设计

### 5.1 系统提示词策略

```
你是一个专业的企业财务报销助手，名称为"Reimbee"。你的职责是帮助员工高效、准确地完成报销流程。

## 核心原则
1. 每次只引导用户完成一个步骤，不要一次性询问过多信息
2. 涉及金额的操作必须明确告知用户并等待确认
3. 当发现合规问题时，明确告知具体问题和建议
4. 保持专业、友好的语气

## 可用工具
| 工具名 | 用途 | 调用时机 |
|--------|------|---------|
| recognize_invoice | OCR识别票据 | 用户上传发票图片/PDF后 |
| check_compliance | 合规审查 | OCR识别完成或用户手动输入金额后 |
| check_budget | 预算查询 | 提交报销前 |
| generate_pdf | 生成PDF报销单 | 所有检查通过，用户确认提交后 |
| send_email | 发送审批邮件 | PDF生成成功后 |
| query_progress | 查询审批进度 | 用户询问进度时 |
| query_reimbursements | 查询报销记录 | 用户询问历史报销时 |

## 合规规则参考
- 差旅住宿: 单日 ≤ ¥300
- 交通费: 单程 ≤ ¥1500
- 招待费: 人均 ≤ ¥200
- 办公用品: 单次 ≤ ¥5000
- 发票日期: 距当前 ≤ 90天

## 预算控制
- 提交前必须检查部门预算
- 超预算时告知用户并触发特殊审批流程
```

### 5.2 工具定义详情

| 工具名 | 输入参数 | 输出 | 必须依赖 |
|--------|---------|------|---------|
| `recognize_invoice` | `image_path: string` | `{invoice_code, amount, date, seller_name, category, confidence}` | OCRClient |
| `check_compliance` | `{amount: float64, category: string, invoice_date: string}` | `{result, message, warnings}` | ComplianceConfig |
| `check_budget` | `{department: string, amount: float64}` | `{remaining, need_special_approval, usage_rate}` | BudgetRepo |
| `generate_pdf` | `{reimbursement_id: uint}` | `{pdf_path, reimbursement_no}` | PDFGenerator, ReimbursementRepo |
| `send_email` | `{to: string, subject: string, body: string, attachment_path: string}` | `{success, message_id}` | SMTPClient |
| `query_progress` | `{reimbursement_no: string}` | `{status, nodes[], invoices[]}` | ApprovalRepo, ReimbursementRepo |
| `query_reimbursements` | `{employee_id: string, page: int, page_size: int}` | `{list[], total}` | ReimbursementRepo |

### 5.3 工具错误返回格式

所有工具必须返回 LLM 可理解的错误信息：

```go
// 标准错误返回格式
type ToolError struct {
    Error   string `json:"error"`    // 人类可读错误信息
    Retry   bool   `json:"retry"`    // 是否可重试
    Suggest string `json:"suggest"`  // 建议操作
}

// 示例: OCR 失败
{
    "error": "OCR 识别失败，无法解析票据内容",
    "retry": true,
    "suggest": "请确保票据图片清晰、完整、无遮挡，重新上传。或直接手动输入金额和类别。"
}
```

### 5.4 Session 管理

**存储**: Redis

**Key 设计**: `session:{session_id}` → JSON

**数据结构**:

```json
{
  "session_id": "uuid",
  "employee_id": "E001",
  "messages": [
    {"role": "user", "content": "...", "timestamp": "..."},
    {"role": "assistant", "content": "...", "timestamp": "..."}
  ],
  "context": {
    "current_reimbursement_id": null,
    "pending_confirmations": [],
    "last_tool_calls": []
  },
  "created_at": "2026-07-03T10:00:00Z",
  "updated_at": "2026-07-03T10:05:00Z"
}
```

**策略**:
- TTL: 30 分钟（对话超时自动过期）
- 最多保留 20 轮对话
- 上下文信息在报销提交成功后清理

---

## 六、关键设计决策与待解决问题

### 6.1 已解决

| 决策点 | 方案 | 理由 |
|--------|------|------|
| Eino 工具 + Wire DI | 工厂函数模式 | 与 Wire 编译期注入兼容，代码简洁 |
| 合规规则存储 | config.yaml + 环境变量覆盖 | 简单够用，reload 不需要重启（Viper Watch） |
| OCR 协议 | gRPC 优先，HTTP 降级 | gRPC 类型安全 + 二进制高效，HTTP 简单可调试 |
| 票据存储 | 本地文件系统，路径存 DB | 无需 OSS 依赖，Docker volume 挂载即可 |
| 审批模拟 | 真实审批链模型 + auto-approve mock | 接口设计不退化，mock 实现可演示 |

### 6.2 待解决（Phase 2 或需讨论）

| 问题 | 影响 | 暂定方案 | 风险 |
|------|------|---------|------|
| gpdf 稳定性 | PDF 生成可能有问题 | 先用 gpdf，备选方案用 Go template 生成 HTML 再转 PDF | gpdf 是 2026 年新库，社区成熟度待验证 |
| Eino v0.9 Streaming 兼容性 | SSE 流式是否稳定 | 先验证 Eino 的 `runner.Query()` iterator 在长对话中的表现 | 字节内部验证过，但公开文档案例少 |
| React SSE 自动重连 | 网络中断时对话丢失 | EventSource 内置重连，但需处理 session 恢复 | Last-Event-ID 机制需要前后端配合 |
| 多用户并发 Session 隔离 | 并发对话交叠 | Redis 天然隔离 + goroutine 安全 | 需测试高并发场景 |
| 前端工具调用进度展示 | UI 复杂度 | 使用 Ant Design Steps 组件展示步骤 | 前端状态管理复杂度较高 |

---

## 七、非功能需求

### 7.1 性能指标

| 指标 | 目标 | 测量方法 | 备注 |
|------|:---:|------|------|
| 对话首字延迟 P95 | ≤ 2s | SSE 首个 message 事件时间戳 | 不含 OCR |
| OCR 单张识别 P95 | ≤ 10s | OCRClient 调用耗时 | 含网络传输 |
| REST API P95 | ≤ 200ms | Gin middleware 计时 | 不含复杂报表查询 |
| 并发 Session | ≥ 20 | 同时 20 个 SSE 连接不阻塞 | goroutine per session |
| 内存占用 | ≤ 256MB | pprof heap profile | 不含 OCR 服务 |
| Docker 镜像大小 | ≤ 25MB | docker images | 多阶段构建 |

### 7.2 安全要求

| 领域 | 措施 | 优先级 |
|------|------|:--:|
| 认证 | JWT Bearer Token（gin-template 已内置） | P0 |
| 文件上传 | MIME 白名单 + 大小限制 + 随机文件名 | P0 |
| SQL 注入 | GORM 参数化查询（默认安全） | P0 |
| XSS | React 默认转义 + Content-Security-Policy header | P1 |
| 敏感配置 | API Key / SMTP Password 仅存于 config.yaml，不入库、不打印 | P0 |
| 速率限制 | 每 IP 每分钟 60 请求（Gin middleware） | P1 |

### 7.3 可观测性

| 维度 | 实现 | 状态 |
|------|------|:--:|
| 结构化日志 | zap（gin-template 已集成） | ✅ |
| 请求追踪 | X-Request-ID 中间件 | 待实现 |
| 性能分析 | pprof（gin-template 已集成） | ✅ |
| 健康检查 | GET /health | 待实现 |

### 7.4 部署

```yaml
# docker-compose.yml (目标)
services:
  backend:
    build:
      context: .
      dockerfile: Dockerfile
    ports: ["8080:8080"]
    volumes: ["./config.yaml:/app/config.yaml", "./uploads:/app/uploads"]
    depends_on: [mysql, redis]
    environment:
      - DB_HOST=mysql
      - REDIS_HOST=redis

  mysql:
    image: mysql:8.0
    environment:
      MYSQL_ROOT_PASSWORD: ${DB_PASSWORD}
      MYSQL_DATABASE: reimbursement
    volumes:
      - ./sql/init.sql:/docker-entrypoint-initdb.d/init.sql
      - mysql_data:/var/lib/mysql

  redis:
    image: redis:7-alpine
    volumes: [redis_data:/data]

  ocr:
    build: ./ocr-service
    ports: ["50051:50051"]

volumes:
  mysql_data:
  redis_data:
```

---

## 八、项目目录结构（最终形态）

```
Reimbee/
├── main.go                          # 入口
├── app.go                           # MainApp 封装
├── wire.go                          # Wire 依赖注入
├── go.mod / go.sum
├── makefile
├── config.yaml.example
├── Dockerfile
├── docker-compose.yml
│
├── conf/
│   └── viper.go                     # 配置加载
│
├── log/                             # 日志（gin-template 自带）
│
├── model/                           # GORM 数据模型
│   ├── reimbursement.go
│   ├── invoice_item.go
│   ├── approval_record.go
│   ├── department_budget.go
│   └── employee.go
│
├── infra/                           # 基础设施
│   ├── data.go                      # GORM + Redis（扩展）
│   ├── provider.go
│   ├── ocr_client.go                # OCR 微服务客户端 【新增】
│   ├── smtp_client.go               # SMTP 邮件客户端 【新增】
│   └── pdf_generator.go             # PDF 生成器 【新增】
│
├── internal/
│   ├── provider.go                  # 聚合 Wire Provider
│   ├── common/
│   │   └── request_meta.go
│   │
│   ├── domain/
│   │   ├── hub.go                   # ServiceHub
│   │   ├── provider.go
│   │   │
│   │   ├── reimbursement/           # 【新增】报销单领域
│   │   │   ├── provider.go
│   │   │   ├── dto.go
│   │   │   ├── service.go           # HTTP Handler
│   │   │   ├── biz.go               # 业务逻辑
│   │   │   └── repo.go              # 数据访问
│   │   │
│   │   ├── budget/                  # 【新增】预算领域
│   │   │   ├── provider.go
│   │   │   ├── dto.go
│   │   │   ├── service.go
│   │   │   ├── biz.go
│   │   │   └── repo.go
│   │   │
│   │   ├── approval/                # 【新增】审批领域
│   │   │   ├── provider.go
│   │   │   ├── dto.go
│   │   │   ├── service.go
│   │   │   ├── biz.go
│   │   │   └── repo.go
│   │   │
│   │   ├── employee/                # 【新增】员工领域
│   │   │   ├── provider.go
│   │   │   ├── dto.go
│   │   │   ├── service.go
│   │   │   ├── biz.go
│   │   │   └── repo.go
│   │   │
│   │   └── agent/                   # 【新增】Agent 编排层
│   │       ├── provider.go
│   │       ├── agent.go             # AgentRunner
│   │       ├── prompt.go            # 系统提示词
│   │       ├── session.go           # Redis Session 管理
│   │       └── tools/               # 7 个工具
│   │           ├── provider.go
│   │           ├── ocr_tool.go
│   │           ├── compliance_tool.go
│   │           ├── budget_tool.go
│   │           ├── pdf_tool.go
│   │           ├── email_tool.go
│   │           ├── progress_tool.go
│   │           └── query_tool.go
│   │
│   └── router/
│       ├── provider.go
│       ├── root.go                  # 路由注册（扩展）
│       └── middleware/
│           ├── auth.go
│           ├── cors.go
│           ├── metadata.go
│           ├── ratelimit.go         # 【新增】
│           └── request_id.go        # 【新增】
│
├── sql/
│   ├── init.sql                     # 建表
│   └── seed.sql                     # 种子数据
│
├── docs/
│   ├── requirements.md              # 本文件
│   └── architecture.md              # 架构设计文档
│
└── frontend/                        # React 前端
    ├── package.json
    ├── vite.config.ts
    └── src/
        ├── App.tsx
        ├── pages/
        │   ├── ChatPage.tsx
        │   ├── DashboardPage.tsx
        │   └── ProgressPage.tsx
        ├── components/
        │   ├── ChatPanel/
        │   ├── BudgetChart/
        │   ├── InvoiceUploader/
        │   └── ProgressTimeline/
        ├── hooks/
        │   └── useSSE.ts
        └── api/
            └── client.ts
```

---

## 九、开发阶段与风险

### 9.1 四天实施计划

| 阶段 | 天 | 模块 | 核心产出 | 验证标准 |
|------|:--:|------|---------|---------|
| **M1** | D1 | 骨架搭建 | ① go.mod 补充依赖 (eino, ark, gpdf, go-mail) ② model 5 张表 + AutoMigrate ③ infra OCRClient/SMTPClient/PDFGenerator ④ agent AgentRunner 创建（先配 1 个问候工具） ⑤ sql/init.sql + seed.sql | `go run main.go` 启动成功<br>GET /health 返回 200<br>Agent 能回复问候语 |
| **M2** | D2 | 核心功能 | ① 6 个工具全部实现 ② reimbursement/budget/approval/employee 完整 CRUD ③ SSE 流式对话接口 ④ OCR 微服务搭建 + Go 客户端联调 | 完整对话报销流程可走通<br>POST /api/reimbursements 可创建 |
| **M3** | D3 | 前端 + 联调 | ① React 项目搭建 ② ChatPage(SSE流式) ③ DashboardPage(ECharts) ④ ProgressPage ⑤ 前后端联调 | 前端 3 个页面可用<br>对话→看板→查进度全链路通 |
| **M4** | D4 | 收尾 | ① UI 打磨 + 异常处理完善 ② Dockerfile + docker-compose ③ 答辩 PPT + 实训报告 ④ 录制演示视频 | 系统可演示<br>文档齐全 |

### 9.2 风险矩阵

| # | 风险 | P | I | 缓解措施 | 触发条件 |
|---|------|:--:|:--:|---------|---------|
| R1 | Eino v0.9 API 不稳定 | M | H | 锁定版本，参考 aggo 项目已跑通的模式 | API 调用报错无法解决 |
| R2 | gpdf 中文支持有问题 | M | M | 备选 maroto v2 或 go-wkhtmltopdf | AddUTF8Font 渲染异常 |
| R3 | 火山方舟 API 限流/不可用 | L | H | OpenAI 兼容接口 fallback，Agent 层抽象 | 连续 3 次 429/503 |
| R4 | OCR 准确率不达标 | M | M | 提供手动修正入口；演示用预识别好的数据 | 演示时 OCR 错误明显 |
| R5 | 时间不足 | M | H | 按优先级降级：P0(M1+M2) > P1(M3) > P2(M4) | D2 结束时 M2 只完成 50% |
| R6 | Wire 注入循环依赖 | L | H | 遵循 gin-template pattern，不跨领域注入 Biz | wire gen 报循环依赖 |

---

## 十、附录

### A. 与原题目对比表

| 原题目要求 | 本方案 | 提升幅度 |
|-----------|--------|:--:|
| Gradio 界面 | React SPA + Ant Design 5 | UI 专业度 ↑↑↑ |
| LangChain | Eino ADK (字节跳动内部验证) | 性能 ↑ 工程化 ↑ |
| Python 单进程 | Go 微服务 + Wire DI + Docker | 架构现代化 ↑↑ |
| 预置模拟数据 20-50 条 | SQL 种子脚本 50+ 条 + 数据迁移脚本 | 数据量 ↑ 可维护性 ↑ |
| "如有精力做简单查询" | 预算看板 + 进度查询作为 P1 核心功能 | 定位提升 |
| "模拟系统审批页面" | 真实多节点审批链模型 | 设计不退化 |
| 无测试要求 | 单元测试 + 集成测试（Phase 2） | 质量保障 ↑ |

### B. 术语表

| 术语 | 说明 |
|------|------|
| Reimbee | 项目名称/Agent 名称 |
| SSE | Server-Sent Events，服务端推送技术 |
| Wire | Google Wire，Go 编译期依赖注入工具 |
| ADK | Agent Development Kit，Eino 的 Agent 开发套件 |
| ReAct | Reasoning + Acting，LLM 的思考-行动循环模式 |
| DTO | Data Transfer Object，请求/响应数据结构 |
| HITL | Human-in-the-Loop，人机交互中断/恢复 |
