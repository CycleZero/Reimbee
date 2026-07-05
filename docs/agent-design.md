# Agent 层详细设计

> 版本: v1.0 | 日期: 2026-07-04 | 状态: 设计评审

---

## 一、设计目标

在现有 DDD-lite 架构上新增 Agent 领域模块，将 LLM 驱动的对话能力无缝集成到报销业务流程中。Agent 层是对外暴露的**唯一对话入口**，内部编排 7 个工具完成多步骤报销流程。

---

## 二、架构定位

### 2.1 在现有分层中的位置

```
router/root.go          ← SSE 端点注册
    │
domain/hub.go           ← ServiceHub 注入 AgentRunner
    │
domain/agent/           ← 【本次设计】
  ├── agent.go           AgentRunner 核心
  ├── prompt.go          系统提示词
  ├── session.go         Redis Session 管理
  ├── dto.go             对话消息 DTO
  └── tools/             7 个工具
      ├── ocr_tool.go
      ├── compliance_tool.go
      ├── budget_tool.go
      ├── pdf_tool.go
      ├── email_tool.go
      ├── progress_tool.go
      └── query_tool.go
    │
    ├── 依赖 infra 层
    │   ├── OCRRecognizer (接口)
    │   ├── PDFGenerator   (接口)
    │   └── EmailSender     (接口)
    │
    └── 依赖其他 domain
        ├── budget.BudgetBiz
        ├── approval.ApprovalBiz
        └── reimbursement.ReimbursementBiz
```

### 2.2 Agent 层不做什么

- **不处理 HTTP 请求/响应**——那是 Service 层的职责。Agent 层只产出 `AgentEvent` 流，由 Service 层转 SSE
- **不直接访问数据库**——通过 Biz 层调用
- **不管理 Gin Context**——Agent 层与 HTTP 框架完全解耦

---

## 三、核心组件设计

### 3.1 AgentRunner

```go
// AgentRunner Agent 运行时——封装 Eino ChatModelAgent + Runner
// 整个应用生命周期内为单例（通过 Wire 注入到 ServiceHub）
type AgentRunner struct {
    runner       *adk.Runner          // Eino Runner
    sessionStore SessionStore          // Redis Session 管理
    tools        []tool.BaseTool       // 已注册的工具列表
}
```

**构造流程**：

```
NewAgentRunner(vc, ocrClient, pdfGen, mailSender, budgetBiz, approvalBiz, reimbBiz)
  │
  ├── 1. 创建 Ark ChatModel（火山方舟）
  ├── 2. 逐一创建 7 个 Tool（工厂函数 + 闭包注入依赖）
  ├── 3. 构建系统提示词
  ├── 4. NewChatModelAgent(name, instruction, model, tools)
  ├── 5. NewRunner(agent, EnableStreaming=true)
  └── 返回 AgentRunner
```

**核心方法**：

| 方法 | 签名 | 说明 |
|------|------|------|
| `StreamChat` | `(ctx, sessionID, userMessage, w io.Writer) error` | 流式对话，将 `runner.Query()` 迭代器转为 SSE 写入 `w` |
| `Chat` | `(ctx, sessionID, userMessage) (string, error)` | 非流式对话，收集完整回复（用于调试/简单场景） |
| `GetHistory` | `(sessionID string) []Message` | 获取会话历史 |
| `ClearSession` | `(sessionID string) error` | 清除会话 |

### 3.2 工具注册设计

**核心问题**：Eino 的 `utils.InferTool()` 期望纯函数，但工具需要注入依赖。

**方案**：**工厂函数 + 闭包**。每个工具对应一个工厂函数，接收注入的依赖，返回闭包形式的工具。

```go
// tools/ocr_tool.go

// NewOCRTool 创建 OCR 识别工具
// 依赖: OCRRecognizer
func NewOCRTool(recognizer infra.OCRRecognizer) tool.InvokableTool {
    return utils.InferTool("recognize_invoice",
        "识别用户上传的票据图片（支持 JPG/PNG/PDF），提取发票代码、号码、金额、开票日期、购销方信息。"+
            "调用时机：用户上传发票图片后。"+
            "返回结果包含 confidence 置信度，低于 0.6 需提示用户核对。",
        func(ctx context.Context, input *OCRInput) (*infra.InvoiceResult, error) {
            return recognizer.Recognize(ctx, input.ImageData, input.MimeType)
        })
}

// OCRInput 工具输入参数——通过 struct tags 定义 JSON Schema
type OCRInput struct {
    ImageData []byte `json:"image_data" jsonschema:"required" jsonschema_description:"票据图片的二进制数据"`
    MimeType  string `json:"mime_type" jsonschema:"required" jsonschema_description:"图片 MIME 类型，如 image/jpeg"`
}
```

**所有工具的依赖关系**：

```
NewOCRTool(OCRRecognizer)
NewComplianceTool(ComplianceConfig)        ← 读取合规规则
NewBudgetTool(BudgetBiz)
NewPDFTool(PDFGenerator, ReimbursementBiz) ← 需要查询报销单信息
NewEmailTool(EmailSender)                  ← 发送邮件
NewProgressTool(ApprovalBiz, ReimbursementBiz)
NewQueryTool(ReimbursementBiz)
```

### 3.3 ToolSet 聚合

```go
// tools/provider.go

// ToolSet 聚合所有 Agent 工具
type ToolSet struct {
    OCR        tool.InvokableTool
    Compliance tool.InvokableTool
    Budget     tool.InvokableTool
    PDF        tool.InvokableTool
    Email      tool.InvokableTool
    Progress   tool.InvokableTool
    Query      tool.InvokableTool
}

// All 返回工具列表为 Eino 兼容的 []tool.BaseTool
func (ts *ToolSet) All() []tool.BaseTool {
    return []tool.BaseTool{
        ts.OCR, ts.Compliance, ts.Budget,
        ts.PDF, ts.Email, ts.Progress, ts.Query,
    }
}

var ToolProviderSet = wire.NewSet(
    NewOCRTool,
    NewComplianceTool,
    NewBudgetTool,
    NewPDFTool,
    NewEmailTool,
    NewProgressTool,
    NewQueryTool,
    wire.Struct(new(ToolSet), "*"),
)
```

### 3.4 Session 管理

**存储**: Redis，TTL = 30 分钟

**数据模型**：

```go
type SessionStore interface {
    // AppendMessage 追加一条消息到会话历史
    AppendMessage(sessionID string, msg Message) error
    // GetHistory 获取会话最近 N 轮对话
    GetHistory(sessionID string, maxTurns int) ([]Message, error)
    // ClearSession 清除会话
    ClearSession(sessionID string) error
    // SetContext 存储会话上下文（当前报销单 ID 等）
    SetContext(sessionID string, key string, value any) error
    // GetContext 获取会话上下文
    GetContext(sessionID string, key string) (any, error)
}

type Message struct {
    Role      string    `json:"role"`      // user / assistant
    Content   string    `json:"content"`
    Timestamp time.Time `json:"timestamp"`
}
```

**Redis Key 设计**：

```
reimbee:session:{sessionID}:messages  → JSON 数组（最近 20 条消息）
reimbee:session:{sessionID}:context   → Hash（键值对上下文）
TTL: 1800s (30 分钟)
```

---

## 四、工具详细设计

### 4.1 recognize_invoice（OCR 识别）

| 属性 | 值 |
|------|-----|
| **工具名** | `recognize_invoice` |
| **输入** | `image_data: []byte` + `mime_type: string` |
| **输出** | `InvoiceResult`（含 confidence + error + retry 标志） |
| **依赖** | `infra.OCRRecognizer` |
| **错误处理** | OCR 失败返回 `InvoiceResult{Error: "...", Retry: true}`，不抛异常 |

**Agent 使用场景**：用户上传发票后，Agent 自动调用此工具。失败时 Agent 提示用户重新上传或手动输入。

### 4.2 check_compliance（合规审查）

| 属性 | 值 |
|------|-----|
| **工具名** | `check_compliance` |
| **输入** | `amount: int64（分）` + `category: string` + `invoice_date: string` |
| **输出** | `{result: "pass"/"warning"/"error", message: string, rule_id: string}` |
| **依赖** | 合规规则配置（config.yaml） |

**合规规则表**（在配置中定义，可热更新）：

```yaml
compliance:
  rules:
    - id: hotel_daily
      category: 差旅-住宿
      max_amount: 30000  # 300元(分)
      level: warning
    - id: transport
      category: 差旅-交通
      max_amount: 150000 # 1500元(分)
      level: warning
    - id: entertainment
      category: 招待费
      max_amount_per_person: 20000 # 200元(分)
      level: error
    - id: office
      category: 办公用品
      max_amount: 500000 # 5000元(分)
      level: warning
    - id: invoice_age
      max_age_days: 90
      level: error
```

### 4.3 check_budget（预算查询）

| 属性 | 值 |
|------|-----|
| **工具名** | `check_budget` |
| **输入** | `department_id: uint` + `amount: int64（分）` |
| **输出** | `{remaining: int64, need_special_approval: bool, usage_rate: float64}` |
| **依赖** | `budget.BudgetBiz.CheckBudget()` |

### 4.4 generate_pdf（PDF 生成）

| 属性 | 值 |
|------|-----|
| **工具名** | `generate_pdf` |
| **输入** | `reimbursement_id: uint` |
| **输出** | `{file_path: string, reimbursement_no: string}` |
| **依赖** | `infra.PDFGenerator` + `reimbursement.ReimbursementBiz.GetByID()` |

**流程**：查询报销单及关联票据 → 传入 PDF 生成器 → 保存到 `uploads/` → 返回路径。

### 4.5 send_email（邮件推送）

| 属性 | 值 |
|------|-----|
| **工具名** | `send_email` |
| **输入** | `reimbursement_id: uint` + `pdf_path: string` |
| **输出** | `{success: bool, message_id: string}` |
| **依赖** | `infra.EmailSender` + 查询审批人信息 |

**流程**：根据报销单的审批记录查询审批人邮箱 → 组装 HTML 邮件 → 附带 PDF 附件 → 发送。

### 4.6 query_progress（进度查询）

| 属性 | 值 |
|------|-----|
| **工具名** | `query_progress` |
| **输入** | `reimbursement_no: string（可选）` + `employee_id: string（可选）` |
| **输出** | `{reimbursements: [{no, status, nodes[]}]}` |
| **依赖** | `approval.ApprovalBiz` + `reimbursement.ReimbursementBiz` |

### 4.7 query_reimbursements（报销记录查询）

| 属性 | 值 |
|------|-----|
| **工具名** | `query_reimbursements` |
| **输入** | `employee_id: string` + `page: int` + `page_size: int` |
| **输出** | `{list: [...], total: int}` |
| **依赖** | `reimbursement.ReimbursementBiz.List()` |

---

## 五、系统提示词设计

### 5.1 三层提示词结构

```
系统提示词 = 角色定位 + 行为规范 + 工具清单
```

### 5.2 完整提示词

```
你是 Reimbee，一个专业的企业财务报销智能助手。你的职责是帮助员工高效、准确地完成报销流程。

## 核心行为规范

1. **一次一步**：每次只引导用户完成一个步骤。不要一次性询问过多信息。
   - 反例："请上传发票，并告诉我金额、类别、日期、部门"
   - 正例："请上传您的票据图片。"
   （用户上传后）→ "识别到差旅费 ¥1,500，开票日期 2026-07-01。正确吗？"

2. **涉及金额必须确认**：在提交报销前，必须明确告知总金额并等待用户确认。

3. **合规问题透明化**：发现超标时，明确告知违规项、标准值和实际值。
   格式："⚠️ 住宿费 ¥350 超出标准（¥300/天），超出 ¥50 需审批人确认。是否继续？"

4. **错误友好**：工具调用失败时，用通俗语言解释原因并给出建议。
   - 反例："OCR 失败，错误码 E500"
   - 正例："票据图片不够清晰，识别失败。请确保票据平整、光线充足，重新拍照上传。也可以直接输入金额。"

5. **保持专业、简洁、友好的语气**。使用中文回复。

## 可用工具

| 工具名 | 用途 | 调用时机 |
|--------|------|---------|
| recognize_invoice | OCR 识别票据 | 用户上传发票图片后 |
| check_compliance | 合规审查 | OCR 识别完成或用户手动输入金额后 |
| check_budget | 预算查询 | 提交报销前，需确认预算充足 |
| generate_pdf | 生成 PDF 报销单 | 所有检查通过，用户确认提交后 |
| send_email | 发送审批邮件 | PDF 生成成功后 |
| query_progress | 查询审批进度 | 用户询问进度时 |
| query_reimbursements | 查询报销记录 | 用户询问历史报销时 |

## 典型对话流程

### 新建报销
用户："我要报销一张差旅发票"
→ 引导上传票据 → OCR 识别 → 确认金额 → 合规检查 → 预算检查 → 确认提交 → 生成 PDF → 发送邮件

### 查询进度
用户："REIMB-2026-0001 审批到哪了"
→ 调用 query_progress → 展示审批链状态

### 查询预算
用户："我们部门还剩多少预算"
→ 调用 check_budget → 展示预算信息

### 政策咨询
用户："住宿标准是多少"
→ 调用 check_compliance 查询规则 → 回复标准
```

---

## 六、SSE 事件协议

### 6.1 事件类型

| 事件类型 | 含义 | 前端渲染 |
|---------|------|---------|
| `thinking` | Agent 正在推理 | 思考动画 |
| `tool_call` | 正在调用工具 | 工具调用卡片（名称 + 输入） |
| `tool_result` | 工具返回结果 | 更新卡片状态（成功/失败） |
| `message` | Agent 文本回复（流式片段） | 逐字追加到对话气泡 |
| `error` | 发生错误 | 错误提示 + 可能的重试建议 |
| `done` | 本轮对话结束 | 停止加载动画 |

### 6.2 事件格式

```
event: thinking
data: {"message": "正在识别票据..."}

event: tool_call
data: {"tool": "recognize_invoice", "input": {"image": "base64..."}}

event: tool_result
data: {"tool": "recognize_invoice", "output": {"amount": 1500.00, "confidence": 0.98}}

event: message
data: {"content": "识别到"}

event: message
data: {"content": "一张差旅发票，金额 ¥1,500.00"}

event: done
data: {}
```

### 6.3 流式转 SSE 核心逻辑

```go
func (a *AgentRunner) StreamChat(ctx context.Context, sessionID, userMessage string, w io.Writer) error {
    // 1. 加载会话历史
    history := a.sessionStore.GetHistory(sessionID, 10)
    a.sessionStore.AppendMessage(sessionID, Message{Role: "user", Content: userMessage})

    // 2. 启动 Agent 查询
    iter := a.runner.Query(ctx, userMessage, adk.WithMessages(convertToEinoMessages(history)))

    // 3. 消费事件流，输出 SSE
    for {
        event, ok := iter.Next()
        if !ok { break }

        switch {
        case event.IsToolCall():
            writeSSE(w, "tool_call", event.ToolCallInfo)
        case event.IsToolResult():
            writeSSE(w, "tool_result", event.ToolOutput)
        case event.IsThinking():
            writeSSE(w, "thinking", event.ThinkingContent)
        case event.IsMessage():
            writeSSE(w, "message", event.MessageChunk)
        }
    }

    // 4. 保存助手回复到会话历史
    a.sessionStore.AppendMessage(sessionID, Message{Role: "assistant", Content: fullResponse})

    writeSSE(w, "done", nil)
    return nil
}
```

---

## 七、ServiceHub 集成

### 7.1 ServiceHub 扩展

```go
type ServiceHub struct {
    // ... 原有字段 ...
    AgentRunner *agent.AgentRunner  // 【新增】
}
```

### 7.2 SSE 端点注册

```go
// router/root.go
api.POST("/chat/stream", func(c *gin.Context) {
    var req struct {
        SessionID string `json:"session_id"`
        Message   string `json:"message"`
    }
    c.ShouldBindJSON(&req)

    c.Header("Content-Type", "text/event-stream")
    c.Header("Cache-Control", "no-cache")
    c.Header("Connection", "keep-alive")

    flusher := c.Writer.(http.Flusher)
    hub.AgentRunner.StreamChat(c.Request.Context(), req.SessionID, req.Message, c.Writer)
    flusher.Flush()
})
```

---

## 八、Wire 依赖注入拓扑（Agent 部分）

```
agent.ProviderSet
  │
  ├── tools.ToolProviderSet
  │     ├── NewOCRTool(infra.OCRRecognizer)     → 注入 OCRRecognizer
  │     ├── NewComplianceTool(vc)                → 注入 Viper 配置
  │     ├── NewBudgetTool(budget.BudgetBiz)      → 注入 BudgetBiz
  │     ├── NewPDFTool(pdf, reimbBiz)            → 注入 PDFGenerator + ReimbursementBiz
  │     ├── NewEmailTool(mail)                   → 注入 EmailSender
  │     ├── NewProgressTool(appBiz, reimbBiz)    → 注入 ApprovalBiz + ReimbursementBiz
  │     └── NewQueryTool(reimbBiz)               → 注入 ReimbursementBiz
  │     └── wire.Struct(new(ToolSet), "*")
  │
  ├── NewAgentRunner(vc, model, toolSet, sessionStore)
  │     └── → AgentRunner（注入到 ServiceHub）
  │
  └── NewRedisSessionStore(redis) → SessionStore
```

---

## 九、配置项（config.yaml 新增）

```yaml
# Agent 层配置
ark:
  api_key: "${ARK_API_KEY}"
  model: "doubao-pro-32k"
  temperature: 0.3
  max_tokens: 4096

# OCR 配置
ocr:
  driver: "mock"            # paddle / mock（演示用 mock）
  paddle:
    endpoint: "http://localhost:5001"
    timeout: 30s

# SMTP 配置
smtp:
  host: "smtp.qq.com"
  port: 587
  user: "${SMTP_USER}"
  password: "${SMTP_PASSWORD}"
  from: "noreply@reimbee.com"

# 合规规则
compliance:
  hotel_daily_max: 30000       # 300元(分)
  transport_max: 150000        # 1500元(分)
  entertainment_per_person: 20000 # 200元(分)
  office_max: 500000           # 5000元(分)
  invoice_max_age_days: 90
```

---

## 十、文件清单

```
internal/domain/agent/
├── provider.go          # Wire ProviderSet（聚合 tools + agent）
├── agent.go             # AgentRunner（StreamChat / Chat / Session）
├── prompt.go            # buildSystemPrompt() 提示词构建函数
├── session.go           # SessionStore 接口 + RedisSessionStore 实现
├── dto.go               # Message / ToolCallInfo 等 DTO
├── config.go            # 合规规则加载（从 Viper 读取）
│
└── tools/
    ├── provider.go      # ToolProviderSet + ToolSet 聚合
    ├── ocr_tool.go      # recognize_invoice
    ├── compliance_tool.go # check_compliance
    ├── budget_tool.go   # check_budget
    ├── pdf_tool.go      # generate_pdf
    ├── email_tool.go    # send_email
    ├── progress_tool.go # query_progress
    └── query_tool.go    # query_reimbursements

router/root.go           # 新增 POST /api/chat/stream

infra/
├── ocr.go               # ✅ 已完成
├── ocr_mock.go          # ✅
├── pdf.go               # ✅
├── pdf_mock.go          # ✅
├── smtp.go              # ✅
└── smtp_mock.go         # ✅
```

---

## 十一、设计评审检查清单

| 检查项 | 状态 |
|--------|:--:|
| Agent 层与 HTTP 层解耦（不持有 Gin Context） | ✅ |
| 工具通过接口依赖注入（不直接实例化依赖） | ✅ |
| OCR 失败不阻塞流程（返回 Error + Retry 标志而非抛异常） | ✅ |
| Session 通过 Redis 持久化（TTL 自动过期） | ✅ |
| SSE 事件格式与前端协议对齐 | ✅ |
| 系统提示词覆盖所有工具的使用说明 | ✅ |
| 每个工具的描述包含"调用时机"让 LLM 自行决策 | ✅ |
| 合规规则配置化（YAML，可热更新） | ✅ |
| Wire 注入链无循环依赖 | ✅ |
| Mock 实现可用于端到端测试 | ✅ |

---

*文档结束*
