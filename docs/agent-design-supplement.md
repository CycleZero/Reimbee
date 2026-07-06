# Agent 层设计补全

> 版本: v1.0 | 日期: 2026-07-06 | 补充 `docs/agent-design.md` 的 8 项缺口

---

## 一、修改报销子流程

### 1.1 触发条件

报销单被驳回后，用户要求修改后重新提交。支持两类修改：

| 修改类型 | 用户表达 | 处理 |
|---------|---------|------|
| 修改金额 | "金额改成 1200" | 更新 InvoiceItem.Amount，记录修改原因 |
| 修改类别 | "这不是交通费，是办公用品" | 更新 InvoiceItem.Category |

### 1.2 流程设计

```
用户: "REIMB-2026-0003 被驳回了，帮我改金额重新提交"
    │
    ▼
┌────────────────────┐
│ 1. 查原报销单       │  Tool: query_progress(reimbursement_no)
│    获取驳回原因      │  返回: {status: rejected, reason: "金额不符", invoices: [...]}
└────────┬───────────┘
         │
         ▼
┌────────────────────┐
│ 2. 展示当前信息     │  Agent: "原报销单: 交通费 ¥1,500，开票日期 2026-07-01"
│    询问修改内容      │         "驳回原因: 金额与票面不一致。请告诉我如何修改"
└────────┬───────────┘
         │ 用户指定修改
         ▼
┌────────────────────┐
│ 3. 更新报销单       │  Tool: update_reimbursement (新增工具)
│                     │  更新字段 + 记录 IsUserModified + ModificationNote
└────────┬───────────┘
         │
         ▼
┌────────────────────┐
│ 4. 回到 Phase 1    │  报销单状态改为 draft，重新走三阶段流程
│    收集阶段         │  已上传的票据图片保留，不需要重新上传
└────────────────────┘
```

### 1.3 需要的额外工具

| 工具 | 输入 | 输出 |
|------|------|------|
| `update_reimbursement` | `reimbursement_id, field, value, reason` | 更新后的报销单 |

---

## 二、多票据并行处理逻辑

### 2.1 处理模式

用户在 Phase 1 一次上传多张票据时，Agent 有**三种可选模式**，由 Agent 自主决定：

| 模式 | 描述 | 适用场景 |
|------|------|---------|
| **串行确认** | OCR 一张 → 用户确认 → 下一张 | 票据数量 ≤ 2，用户时间充裕 |
| **批量预览** | 全部 OCR → 展示列表 → 逐张确认 | 票据数量 ≥ 3 |
| **快速模式** | 全部 OCR → 展示汇总 → 全部默认确认 | 用户催促"快点"，且 OCR 置信度均 > 0.9 |

### 2.2 批量预览交互

```
用户: [一次上传 3 张发票]
    │
    ▼
Agent: "正在识别 3 张票据..."
    │
    ├── 票据 1: OCR 完成 → ¥1,500 交通费 (置信度 0.98)
    ├── 票据 2: OCR 完成 → ¥450 住宿费 (置信度 0.95)
    └── 票据 3: OCR 完成 → ¥180 餐饮费 (置信度 0.65 ⚠️)
    │
    ▼
Agent: "识别完成，共 3 张票据:
       ① 交通费 ¥1,500 ✓
       ② 住宿费 ¥450 ✓
       ③ 餐饮费 ¥180 ⚠️ 置信度偏低，请核对
       合计: ¥2,130"
    │
    ├── "全对" → 全部确认，进入 Phase 2
    ├── "第 2 张改成 400" → 修改单张 → 重新汇总
    └── "第 3 张看不清，重传" → 删除第 3 张 → 回到收集
```

### 2.3 状态追踪

```go
// Phase 1 状态：追踪多票据处理进度
type Phase1State struct {
    Invoices []InvoiceState  // 每张票据的处理状态
}

type InvoiceState struct {
    Index        int               // 序号 1-based
    ImagePath    string            // 已上传的图片路径
    OCRResult    *InvoiceResult    // OCR 结果（可能为 nil）
    UserConfirmed bool             // 用户是否已确认
    Modified     bool              // 是否被用户修改
}
```

---

## 三、错误处理与降级策略

### 3.1 分级处理矩阵

| 错误场景 | 严重级别 | Agent 行为 | 用户感知 |
|---------|:--:|------|------|
| LLM API 超时（单次） | ⚠️ | 自动重试 1 次 | 无感知（延迟 2-3 秒） |
| LLM API 超时（2 次） | 🔴 | 返回"服务繁忙，请稍后再试" | 聊天框显示错误，保留上下文 |
| LLM API 429 限流 | ⚠️ | 指数退避重试（1s→2s→4s），最多 3 次 | 无感知 |
| 工具调用失败（OCR） | 🟡 | 返回 `{error:"...", retry:true}` | "票据不清，请重拍或手动输入" |
| 工具调用失败（预算查询） | 🔴 | 返回错误 + 终止当前流程 | "预算查询失败，无法继续" |
| 工具调用失败（PDF/邮件） | 🟡 | PDF 失败 → 终止；邮件失败 → 继续但提示 | "报销单已生成但邮件发送失败" |
| 用户中途关闭页面 | 🟢 | Checkpoint 保留 1 小时 | 重新打开后"您有未完成的报销" |
| Session 过期 | 🟢 | 返回"会话已过期" | "请重新开始" |
| Redis 不可用 | 🔴 | 降级内存 Map（单实例） | 日志告警 |
| MySQL 不可用 | 🔴 | 返回 503 | "服务暂不可用" |
| Phase 内无限循环 | 🟡 | 超过 10 轮次 → 强制退出 | "对话过长，请重新描述需求" |

### 3.2 LLM 重试策略

```go
type LLMRetryConfig struct {
    MaxRetries      int           // 最大重试次数，默认 3
    InitialBackoff  time.Duration // 初始退避，1s
    MaxBackoff      time.Duration // 最大退避，10s
    RetryableErrors []string      // 可重试的错误: timeout, rate_limit, server_error
}
```

### 3.3 孤儿 Checkpoint 清理

```
定时任务（每小时）:
  SELECT * FROM checkpoint_records
  WHERE updated_at < NOW() - INTERVAL 1 HOUR
  → DELETE（用户已放弃的会话）
```

---

## 四、SSE 事件生产映射

### 4.1 事件类型定义

| 事件类型 | 来源节点 | 携带数据 | 前端渲染 |
|---------|---------|------|------|
| `thinking` | 任意 LLM 节点 | `{message: "正在识别票据..."}` | 三点跳动动画 + 文字 |
| `tool_call` | ToolNode | `{tool: "recognize_invoice", input: {...}}` | 工具调用卡片 |
| `tool_result` | ToolNode | `{tool: "recognize_invoice", output: {...}}` | 卡片状态更新 |
| `message` | 任意 LLM 节点 | `{content: "识别到...", delta: true}` | 打字机追加文本 |
| `phase_change` | Graph Guard | `{from: "phase_1", to: "phase_2", summary: "..."}` | 阶段过渡提示 |
| `confirm_required` | ConfirmInvoice / FinalConfirm | `{prompt: "请确认票据信息"}` | 确认按钮组 |
| `error` | 任意节点 | `{message: "...", retry: true/false}` | 错误提示 + 重试 |
| `done` | Graph END | `{}` | 停止加载状态 |

### 4.2 三阶段事件流示例

```
Phase 1:
  event: thinking         → "正在识别票据..."      → 前端: 思考动画
  event: tool_call        → recognize_invoice      → 前端: 🔧 OCR 中
  event: tool_result      → {amount:1500}          → 前端: ✓ 完成
  event: message          → "识别到交通费 ¥1500"    → 前端: 追加文本
  event: confirm_required → "请确认票据信息"         → 前端: 显示确认按钮

Phase 2:
  event: phase_change     → phase1→phase2          → 前端: 阶段过渡
  event: tool_call        → check_compliance       → 前端: 🔧 合规检查
  event: tool_result      → {result:"pass"}        → 前端: ✓ 合规通过
  event: tool_call        → check_budget           → 前端: 🔧 预算检查
  event: tool_result      → {remaining:50000}      → 前端: ✓ 预算充足
  event: message          → "合规通过，预算充足"     → 前端: 追加文本
  event: confirm_required → "确认提交?"             → 前端: 最终确认

Phase 3:
  event: phase_change     → phase2→phase3          → 前端: 阶段过渡
  event: tool_call        → generate_pdf           → 前端: 🔧 生成PDF
  event: tool_result      → {path:"..."}           → 前端: ✓ PDF已生成
  event: tool_call        → send_email             → 前端: 🔧 发送邮件
  event: tool_result      → {success:true}         → 前端: ✓ 邮件已发送
  event: message          → "REIMB-2026-0001 已提交" → 前端: 完成消息
  event: done             → {}                     → 前端: 结束
```

---

## 五、完整 Prompt 设计

### 5.1 系统级 Prompt（Agent 总角色）

```
你是 Reimbee，一个专业的企业财务报销智能助手。你的职责是帮助员工高效、准确地完成报销全流程。

## 核心行为规范

1. **一次一步**: 每次只引导用户完成一个步骤，不一次性询问过多信息
2. **金额确认**: 涉及金额操作前必须让用户明确确认
3. **合规透明**: 发现问题时明确告知标准值、实际值和影响
4. **错误友好**: 工具失败时用通俗语言解释原因并给出建议
5. **专业简洁**: 使用中文，保持专业、友好、简洁的语气

## 当前流程阶段
你正在 {current_phase} 阶段。{phase_instruction}

## 可用工具
{tool_descriptions}

## 当前上下文
{context_summary}
```

### 5.2 各阶段 Prompt 片段

**Phase 1 指令**：
```
你的任务是收集报销所需的票据信息。
1. 引导用户上传发票图片（必需）
2. 上传后自动调用 OCR 识别
3. 将识别结果展示给用户确认
4. 如果识别失败，引导用户手动输入
5. 用户可以继续添加更多票据，或确认进入下一步
```

**Phase 2 指令**：
```
你的任务是校验报销的合规性和预算。
1. 调用合规检查工具
2. 如有 warning，展示超标项并询问用户是否继续
3. 如有 error，告知用户无法提交并说明原因
4. 调用预算检查工具
5. 如预算不足，告知并触发特殊审批
6. 所有检查通过后，汇总信息并请用户最终确认
```

**Phase 3 指令**：
```
你的任务是完成报销的最终提交。
1. 生成 PDF 报销单
2. 发送审批邮件
3. 如果邮件发送失败，告知用户但报销单已生成
4. 展示报销单号和后续步骤
```

### 5.3 意图分类 Prompt

```
分析用户输入，判断意图并提取实体。返回 JSON:

{
  "intent": "new_reimbursement|query_progress|query_budget|policy_question|modify_reimbursement|general_chat",
  "entities": {
    "amount": null,
    "category": null,
    "department": null,
    "reimbursement_no": null
  },
  "confidence": 0.95,
  "reason": "简短说明分类依据"
}

分类规则:
- new_reimbursement: 发起新报销（关键词: 报销、提交、发票）
- query_progress: 查询进度（关键词: 进度、到哪了、批了吗、状态）
- query_budget: 查询预算（关键词: 预算、还剩、余额）
- policy_question: 政策咨询（关键词: 标准、规定、多少、可以报吗）
- modify_reimbursement: 修改报销（关键词: 改、修改、重新提交、驳回）
- general_chat: 其他（问候、感谢、闲聊）

用户输入: {user_message}
```

---

## 六、工具参数 Schema 定义

### 6.1 recognize_invoice（OCR 识别）

```go
type OCRInput struct {
    ImagePath string `json:"image_path" jsonschema:"required" jsonschema_description:"票据图片的存储路径（由上传接口返回）"`
}

type OCROutput struct {
    Invoice *infra.InvoiceResult `json:"invoice"`
}
```

### 6.2 check_compliance（合规审查）

```go
type ComplianceInput struct {
    Amount      int64  `json:"amount" jsonschema:"required" jsonschema_description:"票据金额（分）"`
    Category    string `json:"category" jsonschema:"required" jsonschema_description:"费用类别: 差旅-交通/差旅-住宿/招待费/办公用品/印刷费/其他"`
    InvoiceDate string `json:"invoice_date" jsonschema:"required" jsonschema_description:"开票日期 YYYY-MM-DD"`
}

type ComplianceOutput struct {
    Result  string `json:"result"`  // pass / warning / error
    Level   string `json:"level"`   // pass / warning / error
    Message string `json:"message"` // 检测结果描述
    RuleID  string `json:"rule_id"` // 触发的规则 ID
}
```

### 6.3 check_budget（预算查询）

```go
type BudgetInput struct {
    DepartmentID uint  `json:"department_id" jsonschema:"required" jsonschema_description:"部门ID"`
    Amount       int64 `json:"amount" jsonschema:"required" jsonschema_description:"本次报销金额（分）"`
}

type BudgetOutput struct {
    Remaining           int64   `json:"remaining"`            // 可用余额（分）
    NeedSpecialApproval bool    `json:"need_special_approval"` // 是否需要特殊审批
    UsageRate           float64 `json:"usage_rate"`           // 部门预算使用率
}
```

### 6.4 generate_pdf（PDF 生成）

```go
type PDFInput struct {
    ReimbursementID uint `json:"reimbursement_id" jsonschema:"required" jsonschema_description:"报销单ID"`
}

type PDFOutput struct {
    PDFPath          string `json:"pdf_path"`          // PDF 文件路径
    ReimbursementNo  string `json:"reimbursement_no"`  // 报销单号
}
```

### 6.5 send_email（邮件发送）

```go
type EmailInput struct {
    ReimbursementID uint   `json:"reimbursement_id" jsonschema:"required" jsonschema_description:"报销单ID"`
    PDFPath         string `json:"pdf_path" jsonschema:"required" jsonschema_description:"PDF 文件路径"`
}

type EmailOutput struct {
    Success   bool   `json:"success"`
    MessageID string `json:"message_id,omitempty"`
    Error     string `json:"error,omitempty"`
}
```

### 6.6 query_progress（进度查询）

```go
type ProgressInput struct {
    ReimbursementNo string `json:"reimbursement_no" jsonschema_description:"报销单号（可选，为空查最近 5 条）"`
    EmployeeID      string `json:"employee_id" jsonschema_description:"员工工号（可选，从会话上下文自动填充）"`
}

type ProgressOutput struct {
    Reimbursements []ProgressItem `json:"reimbursements"`
}

type ProgressItem struct {
    No          string           `json:"no"`
    Status      string           `json:"status"`
    TotalAmount int64            `json:"total_amount"`
    SubmitNote  string           `json:"submit_note"`
    Approvals   []ApprovalStatus `json:"approvals"`
}

type ApprovalStatus struct {
    ApproverName string `json:"approver_name"`
    Action       string `json:"action"` // pending / approved / rejected
    Comment      string `json:"comment"`
}
```

### 6.7 query_reimbursements（报销记录查询）

```go
type QueryInput struct {
    EmployeeID string `json:"employee_id" jsonschema_description:"员工工号（可选）"`
    Page       int    `json:"page" jsonschema:"default=1" jsonschema_description:"页码"`
    PageSize   int    `json:"page_size" jsonschema:"default=5" jsonschema_description:"每页数量"`
}

type QueryOutput struct {
    List  []ReimbursementSummary `json:"list"`
    Total int64                  `json:"total"`
}

type ReimbursementSummary struct {
    No          string `json:"no"`
    Status      string `json:"status"`
    TotalAmount int64  `json:"total_amount"`
    CreatedAt   string `json:"created_at"`
}
```

---

## 七、Agent 测试策略

### 7.1 分层测试

```
┌─────────────────────────┐
│  集成测试 (E2E)           │  真实 LLM + 真实 Graph + Mock 工具
│  完整对话场景              │  数量: 5-10 个核心场景
├─────────────────────────┤
│  Graph 测试               │  Mock LLM + 真实 Graph + Mock 工具
│  流程正确性               │  数量: 每个子流程 3-5 个
├─────────────────────────┤
│  Tool 测试                │  真实工具（与 infra 测试共享）
│  工具输入输出正确性         │  数量: 每个工具 3-5 个
├─────────────────────────┤
│  Prompt 效果测试           │  真实 LLM + 评分模型
│  意图分类准确性             │  数量: 50-100 个标注样本
└─────────────────────────┘
```

### 7.2 Mock LLM 策略

```go
// MockLLM 可编程的 LLM 模拟器，用于测试 Graph 流程
type MockLLM struct {
    responses map[string]string  // prompt hash → response
}

func (m *MockLLM) Generate(ctx context.Context, messages []*schema.Message) (*schema.Message, error) {
    // 根据最后一个 user message 的内容返回预置回复
    key := messages[len(messages)-1].Content
    if resp, ok := m.responses[key]; ok {
        return schema.AssistantMessage(resp, nil), nil
    }
    return schema.AssistantMessage("收到", nil), nil // 默认回复
}
```

### 7.3 关键测试场景

| 场景 | 测试方式 | 验证点 |
|------|:--:|------|
| 完整报销流程 | Graph + Mock LLM | 状态流转正确，Guard 条件生效 |
| 合规 warning → 用户确认继续 | Graph + Mock LLM | Phase 2 正确展示 warning 并等待确认 |
| 合规 error → 拒绝提交 | Graph + Mock LLM | Guard 阻止进入 Phase 3 |
| 预算不足 → 特殊审批 | Graph + Mock LLM | `need_special_approval=true` 传递 |
| 用户中途取消 | Graph + Mock LLM | Session 正确清理 |
| OCR 失败 → 手动输入 | Graph + Mock LLM + Mock OCR | Agent 引导用户手动输入 |
| 多票据批量确认 | Graph + Mock LLM | 全部确认后才进入 Phase 2 |
| 意图分类准确性 | 真实 LLM + 标注样本 | 分类准确率 > 90% |

### 7.4 意图分类测试集示例

```go
var intentTestCases = []struct {
    input   string
    intent  string
}{
    {"我要报销一张发票", "new_reimbursement"},
    {"帮我查一下 REIMB-2026-0001 的状态", "query_progress"},
    {"上周的报销批了吗", "query_progress"},
    {"计算机学院还剩多少预算", "query_budget"},
    {"住宿标准是多少", "policy_question"},
    {"这张发票被驳回了，帮我改金额重新提交", "modify_reimbursement"},
    {"你好", "general_chat"},
    {"谢谢", "general_chat"},
    {"报销差旅费 1500", "new_reimbursement"},
    {"我提交的报销到哪一步了", "query_progress"},
}
```

---

## 八、Agent 配置落地

### 8.1 Viper 配置加载

```go
// agent/config.go — 从 Viper 加载 Agent 配置
type AgentConfig struct {
    SessionTTLMinutes        int     // 会话超时（分钟）
    MaxHistoryTurns          int     // 最大历史保留轮数
    MaxPhaseTurns            int     // 每个 Phase 最大轮次（防死循环）
    CheckpointCleanupHours   int     // 孤儿 Checkpoint 清理时间
    LLMMaxRetries            int     // LLM 最大重试次数
    LLMRetryBackoffSeconds   int     // 重试退避时间（秒）
    ToolTimeoutSeconds       int     // 工具调用超时（秒）
    IntentConfidenceThreshold float64 // 意图分类置信度阈值
}

func LoadAgentConfig(vc *viper.Viper) *AgentConfig {
    return &AgentConfig{
        SessionTTLMinutes:         vc.GetInt("agent.session_ttl_minutes"),
        MaxHistoryTurns:           vc.GetInt("agent.max_history_turns"),
        MaxPhaseTurns:             vc.GetInt("agent.max_phase_turns"),
        CheckpointCleanupHours:    vc.GetInt("agent.checkpoint_cleanup_hours"),
        LLMMaxRetries:             vc.GetInt("agent.llm_max_retries"),
        LLMRetryBackoffSeconds:    vc.GetInt("agent.llm_retry_backoff_seconds"),
        ToolTimeoutSeconds:        vc.GetInt("agent.tool_timeout_seconds"),
        IntentConfidenceThreshold: vc.GetFloat64("agent.intent_confidence_threshold"),
    }
}
```

### 8.2 完整 Agent 配置段

```yaml
agent:
  session_ttl_minutes: 30
  max_history_turns: 20
  max_phase_turns: 10             # 每个 Phase 最多 10 轮，超限强制退出
  checkpoint_cleanup_hours: 1
  llm_max_retries: 3
  llm_retry_backoff_seconds: 2    # 2s → 4s → 8s
  tool_timeout_seconds: 30
  intent_confidence_threshold: 0.7 # 置信度 < 0.7 时询问用户确认意图
```

### 8.3 配置热更新

| 配置项 | 是否可热更新 | 说明 |
|--------|:--:|------|
| `session_ttl_minutes` | ✅ | Viper Watch + 重启 |
| `max_history_turns` | ✅ | 同上 |
| `max_phase_turns` | ✅ | 同上 |
| `checkpoint_cleanup_hours` | ✅ | 定时任务读取 |
| `llm_max_retries` | ✅ | 同上 |
| `tool_timeout_seconds` | ✅ | 同上 |
| `compliance.*` | ✅ | 合规规则变更无需重启 |
| `openai.*` | ❌ | 需重启（Model 实例创建时读取） |
| `ocr.driver` | ❌ | 需重启（OCR 实例创建时读取） |

---

*文档结束*
