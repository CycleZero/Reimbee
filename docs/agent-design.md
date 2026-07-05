# Agent 层详细设计 v2.0 — 流程编排架构

> 版本: v2.0 | 日期: 2026-07-04 | 状态: 设计评审
>
> v1.0（ChatModelAgent + ReAct）已废弃。v2.0 采用 **compose.Graph 流程编排**。

---

## 一、设计理念

### 1.1 为什么不能只用 ReAct

报销是**强流程约束**的业务场景。每一步有明确的先后依赖关系：

```
OCR 识别 → 用户确认 → 合规检查 → 预算检查 → 最终确认 → 生成 PDF → 发送邮件
   │          │          │          │          │
   不能跳过   不能跳过    不能跳过    不能跳过    不能跳过
```

纯 ReAct Agent（"给 LLM 一堆工具，让它自己决定调用顺序"）的问题：

| 问题 | ReAct 表现 | 后果 |
|------|-----------|------|
| 跳过合规检查 | LLM 可能"忘记"调用 check_compliance | 超标报销直接提交 |
| 不确认就提交 | LLM 直接调 generate_pdf | 金额/类别可能错误 |
| 工具顺序错 | 先发邮件再生成 PDF | 附件缺失 |
| 用户意图纠缠 | 报销中途用户问"预算还剩多少"，ReAct 可能丢失上下文 | 报销中断、状态混乱 |
| 不可审计 | 每次 LLM 决策路径不同 | 无法追踪为什么某步被跳过 |

### 1.2 正确方案：Graph 编排 + LLM 作为节点

```
┌─────────────────────────────────────────────────┐
│              业务流程（确定性）                     │
│                                                  │
│  意图分类 ──▶ 工作流路由 ──▶ 子流程执行             │
│                │                                 │
│     ┌──────────┼──────────┐                      │
│     ▼          ▼          ▼                      │
│  报销流程   进度查询   预算查询                     │
│  (9 步确定性  (取数据    (取数据                    │
│   顺序)      + 格式化)   + 格式化)                  │
│                                                  │
│  每个节点内部：LLM 负责自然语言理解/生成             │
│  节点之间：Graph 保证执行顺序                       │
└─────────────────────────────────────────────────┘
```

**核心原则**：LLM 是节点内的决策引擎，Graph 是节点间的流程路由器。

---

## 二、顶层架构

### 2.1 Graph 拓扑

```
                    ┌──────────────┐
                    │   入口节点     │
                    │ IntentClassify│  ← LLM：分类用户意图
                    └──────┬───────┘
                           │
                    ┌──────▼───────┐
                    │  WorkflowRoute│  ← Lambda：根据意图路由
                    └──┬──┬──┬──┬──┘
           ┌───────────┘  │  │  └───────────┐
           ▼              ▼  ▼              ▼
    ┌────────────┐  ┌───────────┐  ┌──────────────┐
    │ 报销子流程   │  │ 进度查询   │  │ 预算查询      │
    │ (9 nodes)  │  │ (2 nodes) │  │ (2 nodes)    │
    └────────────┘  └───────────┘  └──────────────┘
```

### 2.2 子流程与意图映射

| 用户意图 | 子流程 | 确定性步骤 |
|---------|--------|:--:|
| 新建报销 / 我要报销 | `NewReimbursementWorkflow` | 7 步（见 §3） |
| 查询进度 / 我的报销到哪了 | `QueryProgressWorkflow` | 2 步（查 + 格式化） |
| 查询预算 / 还剩多少钱 | `QueryBudgetWorkflow` | 2 步（查 + 格式化） |
| 政策咨询 / 标准是多少 | `PolicyQuestionHandler` | 1 步（LLM 直接回复） |
| 修改报销 | `ModifyReimbursementWorkflow` | 3 步（查 + LLM 对话 + 更新） |

---

## 三、报销子流程（核心工作流）

### 3.1 状态机

```
START
  │
  ▼
┌───────────────┐
│ CollectReceipt │  LLM 节点：引导用户上传票据，等待用户操作
│  收集票据      │  用户："这是发票" / 上传图片
└───────┬───────┘
        │ 用户已上传票据
        ▼
┌───────────────┐
│   OCRNode     │  Tool 节点：调用 recognize_invoice
│  OCR 识别     │  输入：图片路径  输出：结构化 InvoiceResult
└───────┬───────┘
        │
        ▼
┌───────────────┐
│ ConfirmInvoice │  LLM 节点：展示 OCR 结果，询问"正确吗？"
│  确认票据      │  "对" → 继续  /  "不对，金额改为2000" → 修改
└───────┬───────┘
        │ 用户确认
        ▼
┌───────────────┐
│ ComplianceNode │  Tool 节点：调用 check_compliance
│  合规检查      │  返回 pass / warning / error
└───────┬───────┘
        │
        ▼
┌───────────────┐
│ ComplianceReview│ LLM 节点：展示合规结果
│  合规审查      │  pass → 继续  /  warning → 告知风险，确认是否继续
└───────┬───────┘  error → 拒绝，引导修改
        │ 合规通过或用户确认警告
        ▼
┌───────────────┐
│  BudgetNode   │  Tool 节点：调用 check_budget
│  预算检查      │  返回 余额 + need_special_approval
└───────┬───────┘
        │
        ▼
┌───────────────┐
│ BudgetReview  │  LLM 节点：展示预算状态
│  预算确认      │  预算充足 → 继续  /  预算不足 → 告知并确认
└───────┬───────┘
        │ 用户确认提交
        ▼
┌───────────────┐
│ FinalConfirm  │  LLM 节点：最终确认——汇总票据金额、合规结果、预算状态
│  最终确认      │  "确认提交？"  →  "确认"  →  不可逆操作开始
└───────┬───────┘
        │ 用户最终确认
        ▼
┌───────────────┐
│ GeneratePDF   │  Tool 节点：生成 PDF 报销单
│  生成 PDF     │  调用 generate_pdf
└───────┬───────┘
        │
        ▼
┌───────────────┐
│  SendEmail    │  Tool 节点：发送审批邮件
│  发送邮件      │  调用 send_email
└───────┬───────┘
        │
        ▼
┌───────────────┐
│ SubmitComplete│  LLM 节点：成功消息
│  提交完成      │  "报销单 REIMB-2026-0001 已生成并发送给审批人"
└───────────────┘
        │
        ▼
       END
```

### 3.2 节点类型分类

| 节点类型 | 数量 | 引擎 | 职责 |
|---------|:--:|------|------|
| **LLM 节点**（ChatModelNode） | 6 | LLM | 自然语言理解/生成、用户交互 |
| **Tool 节点**（ToolsNode） | 4 | 确定性代码 | OCR、合规、预算、PDF、邮件 |
| **结束节点** | 1 | — | 流程终止 |

**7 步确定性流程，每步之间的走向由 Graph 保证，LLM 只负责每步内部的"说什么"。**

### 3.3 为什么用 Graph 而不是 ChatModelAgent

| 维度 | ChatModelAgent (ReAct) | compose.Graph |
|------|:---:|:---:|
| 流程确定性 | ❌ LLM 决定下一步 | ✅ Graph 边定义 |
| 合规检查 | ❌ 可能被跳过 | ✅ 必经过 ComplianceNode |
| 用户确认 | ❌ 可能忘记确认 | ✅ ConfirmInvoice → FinalConfirm 两次确认 |
| 可审计性 | ❌ 每次路径不同 | ✅ 固定路径，可日志 |
| 异常处理 | ❌ LLM 自行判断 | ✅ 每个节点有明确错误分支 |
| 多意图并发 | ❌ 报销中途用户问进度，上下文丢失 | ✅ 子流程隔离，不互相干扰 |

---

## 四、意图分类设计

### 4.1 意图分类节点（IntentClassify）

**节点类型**: ChatModelNode（LLM）

**输入**: 用户的自然语言消息 + 当前 Graph 状态

**输出**: 结构化 JSON（通过 Eino 的 formatted output 或 prompt 约束）

```json
{
  "intent": "new_reimbursement",
  "entities": {
    "amount": null,
    "category": null,
    "department": "计算机学院"
  },
  "confidence": 0.95
}
```

**意图分类 Prompt**：

```
分析用户意图，从以下类别中选择最匹配的一个：

- new_reimbursement: 用户想发起新的报销
- query_progress: 用户想查询报销进度
- query_budget: 用户想查询部门预算
- policy_question: 用户想了解报销政策/标准
- modify_reimbursement: 用户想修改已有报销单
- general_chat: 问候、感谢等非业务对话

返回 JSON：
{"intent": "<类别>", "entities": {"amount": null, "category": null, "department": null}, "confidence": 0.95}

用户输入：{user_message}
```

### 4.2 工作流路由节点（WorkflowRoute）

**节点类型**: LambdaNode（确定性 Go 函数）

**输入**: 意图分类结果

**输出**: 路由到的子流程标识

```go
func workflowRouter(ctx context.Context, intent *IntentResult) (string, error) {
    switch intent.Intent {
    case "new_reimbursement":
        return "workflow_new_reimbursement", nil
    case "query_progress":
        return "workflow_query_progress", nil
    case "query_budget":
        return "workflow_query_budget", nil
    case "policy_question":
        return "workflow_policy_question", nil
    case "modify_reimbursement":
        return "workflow_modify", nil
    default:
        return "workflow_general_chat", nil
    }
}
```

---

## 五、会话持久化设计

### 5.1 两层持久化模型

报销流程的持久化分为**两层**，职责不同：

```
┌─────────────────────────────────────────────┐
│              第一层：对话历史                   │
│         SessionStore（自管理）                  │
│                                              │
│  Redis Key: reimbee:session:{id}:messages    │
│  内容：用户可见的消息列表（最近 20 轮）           │
│  用途：前端展示对话历史、LLM 上下文窗口            │
│  TTL：30 分钟                                 │
└─────────────────────────────────────────────┘
                      │
                      │ 关联（同一个 sessionID）
                      ▼
┌─────────────────────────────────────────────┐
│           第二层：Graph 执行状态                │
│      Eino CheckPointStore（Eino 管理）         │
│                                              │
│  Redis Key: reimbee:ckpt:{graphID}:{threadID}│
│  内容：当前节点、状态变量、中间结果               │
│  用途：Graph Interrupt 后恢复执行               │
│  TTL：随 Session TTL 联动                      │
└─────────────────────────────────────────────┘
```

**为什么分两层**：
- **对话历史**需要暴露给前端展示，且可能被多个子流程共享
- **Graph 状态**是 Eino 内部机制，对外不可见，包含当前节点位置、中间变量等运行时信息
- 两层独立可分别管理，一个过期不影响另一个

### 5.2 第一层：对话历史 SessionStore

**接口定义**：

```go
// SessionStore 对话历史持久化接口
type SessionStore interface {
    // AppendMessage 追加一条消息到会话历史（同时续期 TTL）
    AppendMessage(ctx context.Context, sessionID string, msg Message) error
    
    // GetHistory 获取会话最近 N 轮对话（每轮 = user + assistant）
    GetHistory(ctx context.Context, sessionID string, maxTurns int) ([]Message, error)
    
    // ClearSession 清除整个会话（报销完成或用户主动结束）
    ClearSession(ctx context.Context, sessionID string) error
    
    // Touch 续期 TTL（每次用户交互时调用）
    Touch(ctx context.Context, sessionID string) error
}

type Message struct {
    Role      string    `json:"role"`      // "user" | "assistant"
    Content   string    `json:"content"`
    Timestamp time.Time `json:"timestamp"`
}
```

**Redis 存储格式**：

```
Key:   reimbee:session:{sessionID}:messages
Type:  List（LPUSH 追加，LTRIM 控制长度）
TTL:   1800s（30 分钟，每次交互续期）
```

**实现要点**：

```go
type RedisSessionStore struct {
    client *redis.Client
    ttl    time.Duration // 30 分钟
}

func (s *RedisSessionStore) AppendMessage(ctx context.Context, sessionID string, msg Message) error {
    key := fmt.Sprintf("reimbee:session:%s:messages", sessionID)
    data, _ := json.Marshal(msg)
    
    pipe := s.client.Pipeline()
    pipe.LPush(ctx, key, data)             // 追加到列表头部
    pipe.LTrim(ctx, key, 0, 39)            // 保留最近 40 条（20 轮 × 2）
    pipe.Expire(ctx, key, s.ttl)           // 续期
    _, err := pipe.Exec(ctx)
    return err
}

func (s *RedisSessionStore) GetHistory(ctx context.Context, sessionID string, maxTurns int) ([]Message, error) {
    key := fmt.Sprintf("reimbee:session:%s:messages", sessionID)
    limit := maxTurns * 2 // 每轮 user + assistant
    results, err := s.client.LRange(ctx, key, 0, int64(limit-1)).Result()
    if err != nil {
        return nil, err
    }
    // 列表头部是最新的，需反转按时间正序返回
    messages := make([]Message, len(results))
    for i, raw := range results {
        json.Unmarshal([]byte(raw), &messages[len(results)-1-i])
    }
    return messages, nil
}
```

### 5.3 第二层：Eino Graph Checkpoint

**原理**：Eino 的 `compose.Graph` 支持 `Interrupt`——当 LLM 节点需要等待用户输入时，Graph 暂停执行，将当前状态保存为 Checkpoint，等待外部调用 `Resume` 恢复。

```
用户: "我要报销"                     用户: "是的，确认"
       │                                  │
       ▼                                  ▼
┌─────────────┐    Interrupt     ┌─────────────┐    Resume
│ IntentClassify│ ──────────────▶ │ 等待用户输入  │ ──────────▶ │ ConfirmInvoice│ ...
└─────────────┘   保存 Checkpoint └─────────────┘   恢复 Checkpoint
                        │
                  ┌─────▼─────┐
                  │   Redis    │
                  │  Checkpoint│
                  └───────────┘
```

**Eino CheckPointStore 接口**：

```go
// Eino 内部接口（已由框架定义，我们只需实现 Store 适配器）
type CheckPointStore interface {
    Get(ctx context.Context, id string) (*Checkpoint, error)
    Put(ctx context.Context, id string, cp *Checkpoint) error
    Delete(ctx context.Context, id string) error
}
```

**Redis 适配器实现**：

```go
// RedisCheckpointStore Eino Checkpoint 的 Redis 实现
type RedisCheckpointStore struct {
    client *redis.Client
    ttl    time.Duration
}

func NewRedisCheckpointStore(client *redis.Client, ttl time.Duration) *RedisCheckpointStore {
    return &RedisCheckpointStore{client: client, ttl: ttl}
}

func (s *RedisCheckpointStore) Get(ctx context.Context, id string) (*Checkpoint, error) {
    key := fmt.Sprintf("reimbee:ckpt:%s", id)
    raw, err := s.client.Get(ctx, key).Bytes()
    if err == redis.Nil {
        return nil, ErrCheckpointNotFound
    }
    if err != nil {
        return nil, err
    }
    var cp Checkpoint
    json.Unmarshal(raw, &cp)
    return &cp, nil
}

func (s *RedisCheckpointStore) Put(ctx context.Context, id string, cp *Checkpoint) error {
    key := fmt.Sprintf("reimbee:ckpt:%s", id)
    data, _ := json.Marshal(cp)
    return s.client.Set(ctx, key, data, s.ttl).Err()
}

func (s *RedisCheckpointStore) Delete(ctx context.Context, id string) error {
    key := fmt.Sprintf("reimbee:ckpt:%s", id)
    return s.client.Del(ctx, key).Err()
}
```

### 5.4 Checkpoint ID 与 Session ID 的关系

```
CheckpointID = GraphName + ":" + SessionID

示例:
  GraphName = "reimbursement_workflow"
  SessionID = "abc123"
  CheckpointID = "reimbursement_workflow:abc123"
  Redis Key   = "reimbee:ckpt:reimbursement_workflow:abc123"
```

**一个 Session 可以跨越多个子流程**：
- 用户先问"我部门预算还剩多少"→ 预算子流程 → Checkpoint A
- 再问"帮我把上周那张发票提交了"→ 报销子流程 → Checkpoint B
- 两次用的是同一个 SessionID（对话历史共享），但是不同的 CheckpointID（不同子流程）

### 5.5 Session 生命周期

```
                  用户首次对话
                       │
                       ▼
              ┌─────────────────┐
              │ 生成 SessionID   │  UUID v7（时间有序）
              │ 存入 Redis       │  TTL = 30 min
              └────────┬────────┘
                       │
          ┌────────────┼────────────┐
          ▼            ▼            ▼
     用户发消息    用户发消息    用户发消息
          │            │            │
          ▼            ▼            ▼
    AppendMessage  AppendMessage  AppendMessage
    Touch TTL      Touch TTL      Touch TTL
          │            │            │
          ▼            ▼            ▼
    报销完成?      进度已查?      预算已查?
          │            │            │
          ▼            ▼            ▼
    ClearSession   ClearSession   ClearSession
    (清理消息+ckpt) (清理消息+ckpt) (清理消息+ckpt)
                       │
          ┌────────────┼────────────┐
          ▼                         ▼
     30 分钟无交互              用户主动结束
          │                         │
          ▼                         ▼
    Redis TTL 自动过期        ClearSession
```

**TTL 策略**：

| 条件 | 动作 |
|------|------|
| 每次用户发送消息 | 续期 `reimbee:session:{id}:messages` TTL 为 30min |
| 每次 Eino Put Checkpoint | 续期 `reimbee:ckpt:{id}` TTL 为 30min |
| 报销提交成功 | 清除消息历史 + Checkpoint（流程已结束） |
| 30 分钟无交互 | Redis 自动过期（兜底清理） |
| 用户说"取消"/"算了" | 主动调用 ClearSession |

### 5.6 AgentRunner 中的集成

```go
type AgentRunner struct {
    graph      *compose.Runnable  // 编译后的顶层 Graph
    session    SessionStore        // 第一层：对话历史
    checkpoint CheckPointStore     // 第二层：Graph 执行状态
}

func (a *AgentRunner) StreamChat(ctx context.Context, sessionID, userMsg string, w io.Writer) error {
    // 1. 保存用户消息到对话历史
    a.session.AppendMessage(ctx, sessionID, Message{Role: "user", Content: userMsg})
    a.session.Touch(ctx, sessionID)

    // 2. 获取历史（注入 Graph 的 Invoke 参数中）
    history := a.session.GetHistory(ctx, sessionID, 10)

    // 3. 执行 Graph（Eino 自动管理 Checkpoint）
    //    - 如果是新会话：Graph 从 START 开始
    //    - 如果是恢复：Graph 从上次 Interrupt 的节点继续
    iter, err := a.graph.Stream(ctx, GraphInput{
        SessionID: sessionID,
        Message:   userMsg,
        History:   history,
    })

    // 4. 收集完整回复，存入对话历史
    var fullResponse string
    for event := range iter {
        writeSSE(w, event)
        fullResponse += event.Text
    }
    a.session.AppendMessage(ctx, sessionID, Message{Role: "assistant", Content: fullResponse})

    return nil
}
```

### 5.7 并发安全

| 场景 | 策略 |
|------|------|
| 同一 Session 并发请求 | Redis LPUSH 天然原子，客户端侧通过前端禁用发送按钮防并发 |
| 多个 Session 之间 | Redis 天然隔离，不同 Key 互不影响 |
| Graph 执行中 Session 过期 | Eino Checkpoint Get 失败 → 返回"会话已过期，请重新开始" |
| Redis 不可用 | 降级为内存存储（单实例），日志告警 |

### 5.8 数据量估算

| 指标 | 估算值 |
|------|:--:|
| 单条消息大小 | ~500 bytes（JSON） |
| 20 轮对话（40 条消息） | ~20 KB |
| 单个 Checkpoint | ~2 KB（状态变量 + 节点位置） |
| 100 并发 Session | ~2.2 MB Redis 内存 |
| TTL 30 分钟后 | 自动回收 |

---

## 六、LLM 节点内的 Prompt 设计

## 六、LLM 节点内的 Prompt 设计

LLM 节点不再需要"工具调用决策"。它只需做一件事：**基于当前节点上下文，生成合适的自然语言回复**。

### 6.1 各类 LLM 节点的 Prompt 模板

**CollectReceipt（收集票据）**：
```
你是 Reimbee，帮助用户完成报销。当前步骤：收集票据。

用户意图是发起报销。请友好地引导用户上传票据图片。
告诉用户可以拍照或选择文件，支持 JPG/PNG/PDF 格式。

历史对话：{history}
用户最新消息：{user_message}
```

**ConfirmInvoice（确认票据）**：
```
你是 Reimbee。OCR 识别完成，请向用户确认以下信息：

识别结果：{invoice_result}
- 金额：¥{amount}
- 类别：{category}
- 日期：{date}
- 销售方：{seller}

询问用户信息是否正确。如果用户要求修改，更新相应字段。
```

**ComplianceReview（合规审查）**：
```
你是 Reimbee。合规检查结果：

{compliance_result}

- 如果 pass → 告知用户"合规检查通过"，自然过渡到预算检查
- 如果 warning → 展示超标项和标准，询问是否继续
- 如果 error → 告知用户无法提交，说明原因和建议
```

**FinalConfirm（最终确认）**：
```
你是 Reimbee。这是最终确认步骤，之后将不可逆地提交报销。

汇总信息：
- 票据：{invoice_summary}
- 合规：{compliance_summary}
- 预算：{budget_summary}
- 总金额：¥{total}

请明确要求用户确认："确认提交报销吗？提交后将无法修改。"
```

---

## 七、文件结构

```
internal/domain/agent/
├── provider.go              # Wire ProviderSet
├── runner.go                # AgentRunner（顶层 Graph 管理）
├── prompt.go                # 所有 Prompt 模板
├── session.go               # SessionState + Redis 持久化
├── dto.go                   # IntentResult, Message 等 DTO
│
├── graph/                   # Graph 定义
│   ├── root.go              # 顶层 Graph（意图分类 → 路由 → 子流程）
│   ├── reimbursement.go     # NewReimbursementWorkflow（7 步）
│   ├── progress.go          # QueryProgressWorkflow
│   ├── budget.go            # QueryBudgetWorkflow
│   ├── policy.go            # PolicyQuestionHandler
│   └── modify.go            # ModifyReimbursementWorkflow
│
├── nodes/                   # 自定义节点（LambdaNode / ToolsNode 封装）
│   ├── intent.go            # IntentClassify：LLM 节点（意图分类 Prompt）
│   ├── router.go            # WorkflowRoute：Lambda 节点（路由函数）
│   ├── ocr.go               # OCRNode：ToolsNode（调用 OCR 工具）
│   ├── compliance.go        # ComplianceNode + ComplianceReview
│   ├── budget.go            # BudgetNode + BudgetReview
│   ├── confirm.go           # ConfirmInvoice + FinalConfirm
│   └── finish.go            # GeneratePDF + SendEmail + SubmitComplete
│
└── tools/                   # 7 个 Tool 定义（不变）
    ├── provider.go
    ├── ocr_tool.go
    ├── compliance_tool.go
    ├── budget_tool.go
    ├── pdf_tool.go
    ├── email_tool.go
    ├── progress_tool.go
    └── query_tool.go
```

---

## 八、与 v1.0（ChatModelAgent）的关键差异

| 维度 | v1.0 ChatModelAgent + ReAct | v2.0 compose.Graph 编排 |
|------|:---:|:---:|
| **架构模式** | Agent 自主决策 | Graph 确定性编排 |
| **流程保证** | LLM 决定顺序，可能出错 | Graph 边保证顺序 |
| **工具调用** | LLM 决定何时调哪个 | Graph 在指定节点调用 |
| **用户确认** | 依赖 Prompt 约束（不可靠） | Graph 节点强制执行 |
| **合规检查** | LLM 可能跳过 | 必经 ComplianceNode |
| **状态管理** | 手动管理 Session | Eino Checkpoint 自动 |
| **代码量** | 少（一个 Agent + 工具） | 多（多个 Graph 子流程） |
| **可测试性** | 难（LLM 行为不确定） | 易（子流程可独立测试） |
| **生产可靠性** | 低（黑盒决策） | 高（白盒流程） |

---

## 九、实施计划

| 阶段 | 内容 | 产出 |
|:--:|------|------|
| **P1** | 顶层 Graph（意图分类 + 路由） | 能识别意图并路由到子流程 |
| **P2** | 报销子流程（7 步） | 核心功能可用 |
| **P3** | 进度查询 + 预算查询子流程 | 辅助功能 |
| **P4** | SSE 流式端点 + 前端联调 | 完整端到端 |

---

*文档结束*
