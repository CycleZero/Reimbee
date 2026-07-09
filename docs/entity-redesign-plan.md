# 实体关系重设计方案

> 版本: v1.0 | 日期: 2026-07-09 | 状态: 待评审
>
> 编制: 开发组 | 依据: 老师反馈 — 报销单→报销明细→票据三层关系

---

## 一、问题诊断

### 1.1 当前实体关系（错误）

```
Reimbursement (报销单)
  │
  │ 1:N (Invoices []InvoiceItem)
  ▼
InvoiceItem (票据明细)
  — 同时承担"报销明细"和"票据"双重角色
  — 无中间层
```

### 1.2 三个具体问题

| # | 问题 | 影响 |
|:--:|------|------|
| 1 | **缺少 ReimbursementItem 中间实体** | InvoiceItem 直接挂在 Reimbursement 下，无法表达"多张票据证明一条明细"的业务语义 |
| 2 | **InvoiceItem 数据库记录从未落库** | OCR 结果只存在会话内存/Redis 中，会话过期后发票数据全部丢失；`model.InvoiceItem` 的 17 个审计字段全部为空 |
| 3 | **Agent State 扁平化存储** | `ReimbursementState.Invoices` 是一维数组，OCR 每识别一张图就追加一条，无法区分"待归类票据"和"已确认明细" |

### 1.3 业务语义偏差

| 场景 | 当前行为 | 业务期望 |
|------|---------|---------|
| 3 张出租车发票 | 3 条独立 InvoiceItem | 1 条"市内交通 ¥145"明细 + 3 张票据作为证明 |
| 酒店发票 ¥350，只报 ¥300 | `Amount` 语义模糊（到底是票面还是申请？） | 明细申请金额 ¥300 + 票据票面金额 ¥350，差异可见 |
| OCR 识别后的数据 | 仅存 Redis session state | 持久化到数据库，形成审计链 |
| 提交报销单 | 只传 `TotalAmount` | 传完整 items + receipts 层级结构 |

---

## 二、目标实体关系

### 2.1 三层模型

```
Reimbursement (报销单)
  │
  │ 1:N
  ▼
ReimbursementItem (报销明细)
  │  — Category (费用类别)
  │  — Amount (申请报销金额)
  │  — Description (事由说明)
  │
  │ 1:N
  ▼
Receipt (票据/原始凭证)
   — InvoiceCode / InvoiceNumber (发票代码/号码)
   — Amount (票面金额)
   — InvoiceDate / SellerName
   — ImagePath (图片路径)
   — OCR 原始值 (OCRRawAmount / OCRRawDate / OCRRawCategory / OCRConfidence)
   — 用户修正记录 (IsUserModified / ModificationNote)
   — 审批人裁决 (ApproverChoice)
   — 合规检查结果 (CheckResult / CheckMessage)
```

### 2.2 实体职责分离

| 实体 | 职责 | 谁来创建 | 关键字段 |
|------|------|---------|---------|
| **Reimbursement** | 报销单主体，记录申请人和总金额 | `create_reimbursement` 工具 | ReimbursementNo, TotalAmount, Status, SubmitNote |
| **ReimbursementItem** | 一条报销明细，表达"我要报销什么" | LLM 引导用户确认后创建 | Category, Amount, Description |
| **Receipt** | 一张原始票据，作为明细的"证明材料" | OCR 识别后自动创建，用户可归入某条明细 | InvoiceCode, Amount(票面), ImagePath, OCRRaw* |

### 2.3 完整示例

```
报销单 REIMB-2026-0001  |  事由: 参加 IJCAI 2026 出差  |  总金额: ¥2,400.00
─────────────────────────────────────────────────────────────
明细 #1  差旅-交通  ¥1,500  北京→上海往返机票
  ├─ 票据: XX航空电子行程单  票面 ¥1,500  📎 ticket.png
  │    OCR: 金额¥1,500 置信度0.95  |  用户未修改
  │    合规: pass — 经济舱标准内
  │
明细 #2  差旅-住宿  ¥600  2晚酒店 (6/20-6/22)
  ├─ 票据: XX酒店发票  票面 ¥600  📎 hotel.pdf
  │    OCR: 金额¥600 置信度0.92  |  用户未修改
  │    合规: pass
  │
明细 #3  差旅-交通  ¥150  市内交通（3次出租车）
  ├─ 票据: 出租车发票A  票面 ¥45  📎 taxi1.png
  ├─ 票据: 出租车发票B  票面 ¥62  📎 taxi2.png
  ├─ 票据: 出租车发票C  票面 ¥38  📎 taxi3.png
  │    合规: pass
  │
明细 #4  招待费  ¥150  与张教授工作餐
  ├─ 票据: XX餐厅发票  票面 ¥200  📎 meal.png
  │    OCR: 金额¥200 置信度0.88  |  用户修改金额为¥150
  │    合规: warning — 人均¥150, 未超 ¥200/人 标准
```

关键体现：
- **明细 #3**：3 张票据汇总为 1 条明细（多票据→一明细）
- **明细 #4**：票据金额（¥200）≠ 申请金额（¥150）（部分报销）
- **每张票据有明确归属**：属于哪条明细

---

## 三、Agent 状态重设计

### 3.1 当前状态（扁平）

```go
type ReimbursementState struct {
    Invoices    []InvoiceState  // OCR 结果直接就是"明细"
    TotalAmount int64
}
type InvoiceState struct {
    ImagePath, Category string
    Amount              int64
    Date                string
}
```

### 3.2 新状态（层级）

```go
type ReimbursementState struct {
    Items           []ItemState    `json:"items"`            // 已确认的报销明细
    PendingReceipts []ReceiptState `json:"pending_receipts"` // OCR 识别后待归类的票据
    TotalAmount     int64          `json:"total_amount"`
    CurrentPhase    string         `json:"current_phase"`
    BudgetResult    *BudgetCheckResult `json:"budget_result,omitempty"`
    ComplianceResult json.RawMessage   `json:"compliance_result,omitempty"`
    ReimbursementID uint              `json:"reimbursement_id"`
    EmployeeID      string            `json:"employee_id"`
}

type ItemState struct {
    Category    string         `json:"category"`    // 费用类别
    Amount      int64          `json:"amount"`      // 申请金额(分)
    Description string         `json:"description"` // 事由说明
    Receipts    []ReceiptState `json:"receipts"`    // 归属该明细的票据
}

type ReceiptState struct {
    ImagePath   string `json:"image_path"`
    Amount      int64  `json:"amount"`     // OCR 识别金额或用户修正金额(分)
    Category    string `json:"category"`   // OCR 推断类别
    Date        string `json:"date"`       // 开票日期
    InvoiceCode string `json:"invoice_code,omitempty"`
    InvoiceNo   string `json:"invoice_no,omitempty"`
    SellerName  string `json:"seller_name,omitempty"`
}
```

### 3.3 状态流转

```
OCR 识别票据
  │
  ▼
PendingReceipts[] ← 新票据追加到这里（待归类）
  │
  │ 用户（通过 LLM）将票据归入明细
  ▼
Items[].Receipts[] ← 票据归属到明细
  │
  │ 合规检查：遍历 Items → 每项的 Receipts
  ▼
ComplianceResult ← 逐票据检查 + 按明细聚合
  │
  │ 预算检查：汇总所有 Items 的 Amount
  ▼
BudgetResult
  │
  │ 提交：序列化完整 Items + Receipts
  ▼
Reimbursement + Items + Receipts 落库
```

---

## 四、数据模型变更

### 4.1 删除/重命名

| 操作 | 文件 | 说明 |
|------|------|------|
| 重命名 | `model/invoice_item.go` → `model/receipt.go` | 语义更准确：Receipt = 票据/原始凭证 |
| 新增 | `model/reimbursement_item.go` | 报销明细实体 |
| 修改 | `model/reimbursement.go` | `Invoices []InvoiceItem` → `Items []ReimbursementItem` |

### 4.2 新模型定义

**ReimbursementItem (报销明细)**
```go
type ReimbursementItem struct {
    gorm.Model
    ReimbursementID uint      `gorm:"index;not null"`
    Category        string    `gorm:"type:varchar(50);not null;index;comment:费用类别"`
    Amount          int64     `gorm:"not null;default:0;comment:申请报销金额(分)"`
    Description     string    `gorm:"type:varchar(500);comment:事由说明"`
    Receipts        []Receipt `gorm:"foreignKey:ItemID;constraint:OnDelete:CASCADE"`
}
```

**Receipt (票据，由原 InvoiceItem 改名)**
```go
type Receipt struct {
    gorm.Model
    ItemID        uint    `gorm:"index;not null;comment:关联报销明细ID"`  // ← 改: 不再直接关联 Reimbursement
    InvoiceCode   string  `gorm:"type:varchar(50);comment:发票代码"`
    InvoiceNumber string  `gorm:"type:varchar(50);comment:发票号码"`
    Amount        int64   `gorm:"not null;default:0;comment:票面金额(分)"`  // ← 语义明确: 这是票面金额
    InvoiceDate   string  `gorm:"type:varchar(20);comment:开票日期"`
    SellerName    string  `gorm:"type:varchar(200);comment:销售方名称"`
    ImagePath     string  `gorm:"type:varchar(500);comment:票据图片路径"`
    OCRRawData    string  `gorm:"type:text;comment:OCR识别原始JSON"`
    OCRRawAmount  int64   `gorm:"not null;default:0;comment:OCR原始金额(分)"`
    OCRRawDate    string  `gorm:"type:varchar(20);comment:OCR原始日期"`
    OCRRawCategory string `gorm:"type:varchar(50);comment:OCR原始类别"`
    OCRConfidence float64 `gorm:"default:0;comment:OCR置信度0~1"`
    IsUserModified   bool   `gorm:"default:false;comment:用户是否修改了OCR结果"`
    ModificationNote string `gorm:"type:text;comment:用户修正说明"`
    ApproverChoice   string `gorm:"type:varchar(10);comment:审批人裁决 ocr/user"`
    CheckResult      string `gorm:"type:varchar(20);default:pending;comment:合规结果"`
    CheckMessage     string `gorm:"type:text;comment:合规说明"`
}
```

### 4.3 Reimbursement 变更

```go
type Reimbursement struct {
    gorm.Model
    // ... 不变字段 ...
    NeedSpecialApproval bool                `gorm:"default:false"`
    Items               []ReimbursementItem `gorm:"foreignKey:ReimbursementID;constraint:OnDelete:CASCADE"` // ← 改
    Approvals           []ApprovalRecord    `gorm:"foreignKey:ReimbursementID;constraint:OnDelete:CASCADE"`
}
```

---

## 五、Agent 工具变更

### 5.1 OCR 工具 (ocr_tool.go)

**当前**: 识别后直接追加到 `state.Invoices`

**改为**: 识别后追加到 `state.PendingReceipts`，附带 OCR 原始值。不自动创建明细。

```go
// 新流程
state.PendingReceipts = append(state.PendingReceipts, types.ReceiptState{
    ImagePath:   input.ImagePath,
    Amount:      amountInCents,
    Category:    result.Category,
    Date:        result.Date,
    InvoiceCode: result.InvoiceCode,
    InvoiceNo:   result.InvoiceNumber,
    SellerName:  result.SellerName,
})
```

### 5.2 新增：归类票据工具（或将此能力嵌入现有流程）

由 LLM 引导用户将 `PendingReceipts` 中的票据归入 `Items`：

```
AI: "识别到 3 张票据：
     1. XX航空 ¥1500 (差旅-交通)
     2. XX酒店 ¥600  (差旅-住宿)
     3. 出租车 ¥45   (差旅-交通)
     需要将哪些票据归入同一条明细？"

用户: "1 和 3 都是交通费，归到一起；2 单独"

AI: 创建 Items:
     明细1: 差旅-交通 ¥1545 — [票据1, 票据3]
     明细2: 差旅-住宿 ¥600  — [票据2]
```

**设计选择**：不新增独立工具。由 LLM 在对话中直接通过 `create_reimbursement` 工具传入 `Items` 参数完成归类。`PendingReceipts` 仅作为临时暂存区。

### 5.3 CreateReimbTool 变更

```go
// 旧
type CreateReimbInput struct {
    EmployeeID   string
    EmployeeName string
    DepartmentID uint
    SubmitNote   string
}

// 新 — 同时传入明细和票据
type CreateReimbInput struct {
    EmployeeID   string       `json:"employee_id"`
    EmployeeName string       `json:"employee_name"`
    DepartmentID uint         `json:"department_id"`
    SubmitNote   string       `json:"submit_note"`
    Items        []ItemInput  `json:"items"` // 报销明细列表
}

type ItemInput struct {
    Category    string        `json:"category"`
    Amount      int64         `json:"amount"`
    Description string        `json:"description"`
    Receipts    []ReceiptInput `json:"receipts"`
}

type ReceiptInput struct {
    ImagePath   string `json:"image_path"`
    Amount      int64  `json:"amount"`
    InvoiceDate string `json:"invoice_date"`
    // ... OCR 字段
}
```

### 5.4 SubmitReimbTool 变更

不再从 `state.TotalAmount` 取总金额，而是从 `state.Items` 汇总：

```go
// 汇总所有明细的申请金额
var totalAmount int64
for _, item := range state.Items {
    totalAmount += item.Amount
}
```

### 5.5 其他工具影响

| 工具 | 变更 |
|------|------|
| `list_invoices` | 改为按 Items→Receipts 层级展示 |
| `check_deadline` | 遍历 `state.Items[].Receipts[]` 而非 `state.Invoices[]` |
| `check_compliance` | `ComplianceInput.Invoices[]` → `ComplianceInput.Items[]`，每项带 Receipts |
| `check_budget` | 汇总 Items 总金额不变 |
| `query_progress` / `query_reimbursements` | 响应中返回 Items + Receipts 层级 |

---

## 六、数据库迁移策略

### 6.1 表变更

| 操作 | 表 | 说明 |
|------|----|------|
| 新增 | `reimbursement_items` | 报销明细表 |
| 重命名 | `invoice_items` → `receipts` | 票据表 |
| 改列 | `receipts.reimbursement_id` → `receipts.item_id` | 外键从指向报销单改为指向明细 |

### 6.2 迁移方式

项目当前无生产数据，直接走 **AutoMigrate + 删旧建新** 策略：

1. 删除旧 `invoice_items` 表
2. 新增 `reimbursement_items` 和 `receipts` 表
3. GORM AutoMigrate 自动创建

无需数据迁移脚本（当前无 InvoiceItem 行被实际创建）。

---

## 七、前端影响

### 7.1 卡片展示变更

`ToolCard` 展示从扁平票据列表改为层级明细：

```
🔧 报销单明细
  ├─ 明细 #1  差旅-交通  ¥1,545
  │   ├─ 📎 ticket.png (XX航空 ¥1,500)
  │   └─ 📎 taxi1.png (出租车 ¥45)
  ├─ 明细 #2  差旅-住宿  ¥600
  │   └─ 📎 hotel.pdf (XX酒店 ¥600)
```

### 7.2 类型变更

`MessageCard` 中 tool 卡片的 `input/output` 结构需适配新的 items/receipts 层级。

---

## 八、不变的部分

以下模块**不受本次变更影响**：

- 认证 (auth)、员工 (employee)、部门 (department) 域
- 预算 (budget) 域 — 只跟 TotalAmount 交互
- 审批 (approval) 域 — 只跟 Reimbursement ID 交互
- PDF 生成、邮件发送工具
- JWT 中间件、CORS、路由注册
- 基础设施层（DB、Redis、MinIO、Milvus）
- 前端聊天 SSE 流式框架、卡片渲染引擎
- 中断 (Interruptable) 机制

---

## 九、设计原则

1. **票据是"证明"**：Receipt 的金额是票面金额（不变），ReimbursementItem 的金额是申请金额（可能小于票面金额）。审批人看到两者差异。
2. **明细可含多张票据**：一条"市内交通"明细可以附 3 张出租车发票。
3. **OCR 不自动创建明细**：OCR 只是提取票据信息，由用户（通过 LLM 引导）决定如何归入明细。
4. **审计链完整**：每张 Receipt 保留 OCR 原始值 + 用户修改记录 + 审批人裁决，三者形成完整审计链。
5. **向下兼容**：单张票据场景下，自动创建一条明细包含一张票据，用户体验不受影响。
