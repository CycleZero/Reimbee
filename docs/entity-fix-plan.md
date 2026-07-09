# 实体关系修复实施方案

> 版本: v1.0 | 日期: 2026-07-09 | 状态: 待评审
>
> 关联文档: [entity-redesign-plan.md](./entity-redesign-plan.md)

---

## 修复总览

```
Phase A: 数据模型层（4 steps）  ← 不改业务逻辑，可独立合并
Phase B: Agent 状态层（3 steps） ← 改内存结构，不改数据库
Phase C: 持久化层    （3 steps） ← 新增 InvoiceItem repo/biz
Phase D: Agent 工具层（5 steps） ← 改工具 I/O 和流程
Phase E: 前端适配层（2 steps）  ← 改类型和卡片渲染
Phase F: 验证        （3 steps） ← 编译→测试→e2e
```

---

## Phase A: 数据模型层

> **目标**: 建立正确的三层实体关系，此阶段不改任何业务逻辑
> **验证**: `go build ./...` 通过

### Step A1 — 重命名 InvoiceItem → Receipt，新增外键

**文件**: `model/invoice_item.go` → `model/receipt.go`

```go
// 重命名 struct
type Receipt struct {  // was: InvoiceItem
    gorm.Model
    ItemID        uint    `gorm:"index;not null;comment:关联报销明细ID"` // was: ReimbursementID
    // ... 其余字段不变 ...
}
```

**操作**:
- 重命名文件
- `ReimbursementID uint` → `ItemID uint`
- 所有 json tag 和 comment 不变

### Step A2 — 新增 ReimbursementItem

**文件**: `model/reimbursement_item.go`（新建）

```go
type ReimbursementItem struct {
    gorm.Model
    ReimbursementID uint      `gorm:"index;not null;comment:关联报销单ID"`
    Category        string    `gorm:"type:varchar(50);not null;index;comment:费用类别"`
    Amount          int64     `gorm:"not null;default:0;comment:申请报销金额(分)"`
    Description     string    `gorm:"type:varchar(500);comment:事由说明"`
    Receipts        []Receipt `gorm:"foreignKey:ItemID;constraint:OnDelete:CASCADE"`
}
```

### Step A3 — 修改 Reimbursement 关联

**文件**: `model/reimbursement.go`

```diff
- Invoices        []InvoiceItem  `gorm:"foreignKey:ReimbursementID;constraint:OnDelete:CASCADE"`
+ Items           []ReimbursementItem `gorm:"foreignKey:ReimbursementID;constraint:OnDelete:CASCADE"`
```

### Step A4 — 更新所有引用 InvoiceItem 的代码

**需要改动的文件**（全局搜索替换）:

| 搜索 | 替换为 |
|------|--------|
| `InvoiceItem` (model 引用) | `Receipt` |
| `reimbursement_id` (Receipt 外键) | `item_id` |
| `.Invoices` (Reimbursement 字段) | `.Items` |
| `[]InvoiceItem` | `[]ReimbursementItem` |

**具体文件**:
- `internal/domain/reimbursement/repo.go` — `Preload("Invoices")` → `Preload("Items.Receipts")`
- `internal/domain/reimbursement/dto.go` — `InvoiceItemResponse` 重命名，增加 `Items` 字段
- `internal/domain/agent/tools/` 下所有引用 `model.InvoiceItem` 的文件
- `internal/testutil/db.go` — 测试 seed 函数适配新模型

### Phase A 验证

```bash
go build ./...
# 预期：通过（所有引用已更新）
```

---

## Phase B: Agent 状态层

> **目标**: 将 `ReimbursementState` 从扁平结构改为层级结构
> **验证**: `go test ./internal/domain/agent/...` 通过

### Step B1 — 重写 types/state.go

**文件**: `internal/domain/agent/types/state.go`

```go
type ReimbursementState struct {
    Items           []ItemState         `json:"items"`
    PendingReceipts []ReceiptState      `json:"pending_receipts"`
    TotalAmount     int64               `json:"total_amount"`
    CurrentPhase    string              `json:"current_phase"`
    BudgetResult    *BudgetCheckResult  `json:"budget_result,omitempty"`
    ComplianceResult json.RawMessage    `json:"compliance_result,omitempty"`
    ReimbursementID uint                `json:"reimbursement_id"`
    EmployeeID      string              `json:"employee_id"`
}

type ItemState struct {
    Category    string         `json:"category"`
    Amount      int64          `json:"amount"`
    Description string         `json:"description"`
    Receipts    []ReceiptState `json:"receipts"`
}

type ReceiptState struct {
    ImagePath   string `json:"image_path"`
    Amount      int64  `json:"amount"`      // OCR 金额或用户修正金额(分)
    Category    string `json:"category"`
    Date        string `json:"date"`
    InvoiceCode string `json:"invoice_code,omitempty"`
    InvoiceNo   string `json:"invoice_no,omitempty"`
    SellerName  string `json:"seller_name,omitempty"`
    OCRConfidence float64 `json:"ocr_confidence,omitempty"`
}
```

删除旧类型 `InvoiceState`。

### Step B2 — 更新 state 引用代码

**搜索**: `state.Invoices`、`InvoiceState`、`types.InvoiceState`

```bash
rg "state\.Invoices|InvoiceState" internal/domain/agent/
```

**需改动的文件**:

| 文件 | 改动 |
|------|------|
| `ocr_tool.go` | `state.Invoices = append(...)` → `state.PendingReceipts = append(...)` |
| `check_deadline_tool.go` | 遍历 `state.Items[].Receipts[]` 而非 `state.Invoices` |
| `list_invoices_tool.go` | 按 `state.Items` 分组展示 |
| `submit_reimb_tool.go` | 汇总 `Items` 的 Amount 而非读 `state.TotalAmount` |
| `create_reimb_tool.go` | 新增接收 `Items` 参数 |

### Step B3 — 更新 state_test.go

**文件**: `internal/domain/agent/types/state_test.go`

适配新结构体的测试用例。

### Phase B 验证

```bash
go test ./internal/domain/agent/types/...
# 预期：通过
```

---

## Phase C: 持久化层

> **目标**: OCR 结果和报销明细落库，不再仅存内存/Redis
> **验证**: `go test ./internal/domain/reimbursement/...` 通过

### Step C1 — 新增 ReceiptRepo + ReceiptBiz

**新建文件**: `internal/domain/reimbursement/receipt_repo.go`

```go
type ReceiptRepo struct {
    db *gorm.DB
}

func NewReceiptRepo(db *gorm.DB) *ReceiptRepo

// Create 创建单张票据记录
func (r *ReceiptRepo) Create(receipt *model.Receipt) error

// BatchCreate 批量创建票据记录
func (r *ReceiptRepo) BatchCreate(receipts []*model.Receipt) error

// UpdateByID 更新票据（OCR 确认、用户修正、审批裁决）
func (r *ReceiptRepo) UpdateByID(id uint, updates map[string]interface{}) error

// GetByItemID 按明细 ID 查询所有票据
func (r *ReceiptRepo) GetByItemID(itemID uint) ([]*model.Receipt, error)
```

**新建文件**: `internal/domain/reimbursement/receipt_biz.go`

```go
type ReceiptBiz struct {
    logger *log.Logger
    repo   *ReceiptRepo
}

func NewReceiptBiz(logger *log.Logger, repo *ReceiptRepo) *ReceiptBiz

// SaveFromOCR OCR 识别后创建票据（状态 = pending，待用户归类）
func (b *ReceiptBiz) SaveFromOCR(ctx context.Context, ocrResult *OCRSaveInput) (*model.Receipt, error)

// AssignToItem 将票据归入某条明细
func (b *ReceiptBiz) AssignToItem(receiptID, itemID uint) error

// ConfirmByUser 用户确认/修正票据信息
func (b *ReceiptBiz) ConfirmByUser(receiptID uint, amount int64, category string, note string) error

// SetComplianceResult 设置合规检查结果
func (b *ReceiptBiz) SetComplianceResult(receiptID uint, result, message string) error
```

### Step C2 — 新增 ReimbursementItemRepo + ReimbursementItemBiz

**新建文件**: `internal/domain/reimbursement/item_repo.go`

```go
type ItemRepo struct {
    db *gorm.DB
}

func NewItemRepo(db *gorm.DB) *ItemRepo

func (r *ItemRepo) Create(item *model.ReimbursementItem) error
func (r *ItemRepo) BatchCreate(items []*model.ReimbursementItem) error
func (r *ItemRepo) GetByReimbursementID(reimbID uint) ([]*model.ReimbursementItem, error)
```

**新建文件**: `internal/domain/reimbursement/item_biz.go`

```go
type ItemBiz struct {
    logger *log.Logger
    repo   *ItemRepo
}

func NewItemBiz(logger *log.Logger, repo *ItemRepo) *ItemBiz

func (b *ItemBiz) CreateFromState(items []types.ItemState) ([]*model.ReimbursementItem, error)
```

### Step C3 — 修改 ReimbursementBiz.Create/Submit

**文件**: `internal/domain/reimbursement/biz.go`

```go
// Create 改为同时创建明细和票据
func (b *ReimbursementBiz) Create(input *CreateReimbInput) (*model.Reimbursement, error) {
    // 1. 创建报销单（不变）
    // 2. 逐条创建 ReimbursementItem
    // 3. 逐张创建 Receipt 并关联到 Item
    // 4. 使用事务保证原子性
}

// Submit 改为汇总 Items 金额
func (b *ReimbursementBiz) Submit(id uint) (*model.Reimbursement, error) {
    // TotalAmount 从 Items 汇总计算，而非外部传入
}
```

**文件**: `internal/domain/reimbursement/dto.go`

```go
type CreateReimbInput struct {
    EmployeeID   string
    EmployeeName string
    DepartmentID uint
    SubmitNote   string
    Items        []ItemInput
}

type ItemInput struct {
    Category    string
    Amount      int64
    Description string
    Receipts    []ReceiptInput
}
```

### Phase C 验证

```bash
go test ./internal/domain/reimbursement/...
# 预期：通过
```

---

## Phase D: Agent 工具层

> **目标**: 工具 I/O 适配新模型，OCR 不再直接创建明细
> **验证**: `go test ./internal/domain/agent/tools/...` 通过

### Step D1 — 修改 OCR 工具

**文件**: `internal/domain/agent/tools/ocr_tool.go`

改动：
1. OCR 结果追加到 `state.PendingReceipts` 而非 `state.Items`
2. 同时调用 `ReceiptBiz.SaveFromOCR()` 落库
3. 输出中明确标注"票据已保存，请归类到报销明细"

### Step D2 — 修改 CreateReimbTool

**文件**: `internal/domain/agent/tools/create_reimb_tool.go`

```go
type CreateReimbInput struct {
    EmployeeID   string            `json:"employee_id"`
    EmployeeName string            `json:"employee_name"`
    DepartmentID uint              `json:"department_id"`
    SubmitNote   string            `json:"submit_note"`
    Items        []CreateItemInput `json:"items"`
}

type CreateItemInput struct {
    Category    string              `json:"category"`
    Amount      int64               `json:"amount"`
    Description string              `json:"description"`
    Receipts    []CreateReceiptInput `json:"receipts"`
}
```

执行时：创建 Reimbursement → 逐条创建 Item → 逐张创建 Receipt → 全部在一个事务中。

### Step D3 — 修改 SubmitReimbTool

**文件**: `internal/domain/agent/tools/submit_reimb_tool.go`

- `SubmitReimbInput` 不变（只需 `ReimbursementID`）
- 内部从 `state.Items` 汇总 `TotalAmount`
- 调用 `reimbursementBiz.Submit(id)`（Submit 内部从 DB 的 Items 汇总金额）

### Step D4 — 修改其他受影响工具

| 工具 | 文件 | 改动 |
|------|------|------|
| `list_invoices` | `list_invoices_tool.go` | 改为从 `state.Items` 读取，按明细分组展示 |
| `check_deadline` | `check_deadline_tool.go` | 遍历 `state.Items[].Receipts[]` |
| `check_compliance` | `compliance_agent_tool.go` | `ComplianceInput` 改为接收 `Items[]` 每项带 `Receipts[]` |
| `check_budget` | `budget_tool.go` | 汇总 Items 金额不变 |
| `query_progress` | `progress_tool.go` | 响应中包含 Items 层级 |
| `reimb_detail` | `reimb_detail_tool.go` | 响应中包含 Items 层级 |

### Step D5 — 更新 Agent Prompt

**文件**: `internal/domain/agent/prompt.go`

在员工 prompt 中新增引导语：

```
票据归类规则:
- OCR 识别后，票据暂存在"待归类"列表
- 你需要引导用户将票据归入报销明细:
  * 同一费用类别、同一事由的多张票据可归入一条明细
  * 每条明细需要填写: 费用类别、申请金额、事由说明
  * 票据金额可能与申请金额不同（如部分报销），需向用户确认
- 所有票据归类完毕后，调用 create_reimbursement 并传入 items 参数
```

### Phase D 验证

```bash
go test ./internal/domain/agent/tools/...
# 预期：通过
```

---

## Phase E: 前端适配层

> **目标**: 前端类型和卡片渲染适配新数据结构
> **验证**: `tsc --noEmit` 零错误

### Step E1 — 更新类型定义

**文件**: `web/src/chat/types.ts`

```typescript
// 新增
export interface ItemCardData {
  category: string;
  amount: number;       // 申请金额(元)
  description: string;
  receipts: ReceiptCardData[];
}

export interface ReceiptCardData {
  imagePath: string;
  amount: number;       // 票面金额(元)
  date: string;
  invoiceCode?: string;
  invoiceNo?: string;
  sellerName?: string;
}

// MessageCard 中 tool 卡片的 input/output 适配
```

**文件**: `web/src/types/sse.ts`

更新 `ToolCallData` / `ToolResultData` 中与 items/receipts 相关的字段。

### Step E2 — 更新卡片渲染

**文件**: `web/src/chat/renderers/AssistantBubble.tsx`

`ToolCard` 的展开内容改为层级展示：

```tsx
// 明细列表
{items.map(item => (
  <div key={item.category}>
    <div>{item.category} — ¥{item.amount}</div>
    <div>{item.description}</div>
    {/* 该明细下的票据 */}
    {item.receipts.map(receipt => (
      <div key={receipt.imagePath}>
        📎 {receipt.imagePath} (¥{receipt.amount})
      </div>
    ))}
  </div>
))}
```

### Phase E 验证

```bash
cd web && npx tsc --noEmit
# 预期：零错误
```

---

## Phase F: 验证

> **目标**: 全链路验证，确保不引入回归
> **验证**: 编译 + 全部测试 + e2e

### Step F1 — 编译验证

```bash
go build ./...
# 预期：通过
```

### Step F2 — 测试验证

```bash
# 全部领域测试
go test ./internal/domain/...

# 基础设施测试
go test ./infra/...

# 前端类型检查
cd web && npx tsc --noEmit
```

### Step F3 — E2E 测试验证

**文件**: `internal/domain/agent/interrupt_e2e_test.go`

适配新的 `ReimbursementState` 结构：

```go
// fakeModel 预设的 tool_call 需要适配新 item/receipt 结构
state := &types.ReimbursementState{
    Items: []types.ItemState{
        {
            Category: "差旅-交通",
            Amount:   150000, // ¥1500
            Receipts: []types.ReceiptState{
                {ImagePath: "test.png", Amount: 150000, Category: "差旅-交通", Date: "2026-06-20"},
            },
        },
    },
}
```

```bash
go test -v -run TestInterrupt ./internal/domain/agent/
# 预期：3/3 通过
```

---

## 变更文件清单（汇总）

### 新建文件（6 个）

| 文件 | 说明 |
|------|------|
| `model/reimbursement_item.go` | 报销明细模型 |
| `internal/domain/reimbursement/item_repo.go` | 明细数据访问层 |
| `internal/domain/reimbursement/item_biz.go` | 明细业务逻辑层 |
| `internal/domain/reimbursement/receipt_repo.go` | 票据数据访问层 |
| `internal/domain/reimbursement/receipt_biz.go` | 票据业务逻辑层 |

### 重命名文件（1 个）

| 旧 | 新 |
|----|----|
| `model/invoice_item.go` | `model/receipt.go` |

### 修改文件（约 18 个）

| 层 | 文件 | 改动类型 |
|----|------|---------|
| model | `reimbursement.go` | `Invoices` → `Items`，关联类型变更 |
| agent/types | `state.go` | 扁平→层级结构 |
| agent/types | `state_test.go` | 适配新结构 |
| agent | `prompt.go` | 新增票据归类引导 |
| agent/tools | `ocr_tool.go` | 输出到 `PendingReceipts` + 落库 |
| agent/tools | `create_reimb_tool.go` | 新增 `Items` 参数 |
| agent/tools | `submit_reimb_tool.go` | 从 Items 汇总金额 |
| agent/tools | `list_invoices_tool.go` | 按 Items 分组 |
| agent/tools | `check_deadline_tool.go` | 遍历 Items→Receipts |
| agent/tools | `compliance_agent_tool.go` | Input 改为 Items[] |
| agent/tools | `list_invoices_tool_test.go` | 适配新结构 |
| agent/tools | `check_deadline_tool_test.go` | 适配新结构 |
| agent | `interrupt_e2e_test.go` | 适配新结构 |
| reimbursement | `biz.go` | Create/Submit 改为接收 Items |
| reimbursement | `dto.go` | DTO 字段变更 |
| reimbursement | `repo.go` | Preload 路径变更 |
| reimbursement | `provider.go` | 新增 ReceiptRepo/Biz 到 ProviderSet |
| frontend | `types.ts` | 新增 ItemCardData / ReceiptCardData |
| frontend | `AssistantBubble.tsx` | 层级卡片渲染 |

### 不变文件（无需改动）

- `model/constants.go`、`employee.go`、`department.go`、`department_budget.go`、`approval_record.go`
- `internal/domain/auth/`、`employee/`、`department/`、`budget/`、`approval/`（全部）
- `internal/domain/compliance/biz.go`（规则引擎逻辑不变，仅 input 类型适配）
- `infra/` 全部
- `internal/router/` 全部
- `web/src/chat/useChatStream.ts`（SSE 事件类型不变）

---

## 风险与缓解

| 风险 | 概率 | 缓解 |
|------|:--:|------|
| e2e 测试失败（fakeModel 脚本与新 state 不匹配） | 中 | Phase F 中显式更新 fakeModel 预设数据 |
| 前端 ToolCard 渲染与后端 JSON 字段不匹配 | 中 | Phase E 中与 Phase D 同步更新字段名 |
| LLM prompt 改动后 Agent 行为变化 | 低 | prompt 仅增引导语，不改变核心流程 |
| 编译错误遗漏（某处引用未更新） | 低 | 每个 Phase 末尾执行 `go build ./...` |
| Phase C 事务边界不正确（创建报销单失败时残留 Item/Receipt） | 中 | Create 方法内使用 GORM Transaction |

---

## 预估工时

| Phase | 步骤数 | 预估时间 |
|-------|:--:|:--:|
| A: 数据模型 | 4 | 0.5h |
| B: Agent 状态 | 3 | 0.5h |
| C: 持久化层 | 3 | 1h |
| D: Agent 工具 | 5 | 1.5h |
| E: 前端适配 | 2 | 0.5h |
| F: 验证 | 3 | 0.5h |
| **合计** | **20** | **~4.5h** |
