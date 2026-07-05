# Agent 层详细设计 v2.0 — 流程编排架构

> 版本: v2.1 | 日期: 2026-07-04 | 状态: 设计评审
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

### 5.1 存储架构

```
┌──────────────────────────────────────────────────────────────┐
│                       API 层                                  │
│  Session Store 接口（统一抽象，底层可切换）                      │
└──────────────────────────┬───────────────────────────────────┘
                           │
          ┌────────────────┼────────────────┐
          ▼                ▼                ▼
    ┌──────────┐    ┌──────────┐    ┌──────────────┐
    │  MySQL   │    │  Redis   │    │  Memory       │
    │ (默认)   │    │ (可选)    │    │ (降级)        │
    │──────────│    │──────────│    │──────────────│
    │ 持久化    │    │ 热缓存    │    │ 单实例内存     │
    │ 可审计    │    │ 加速读取  │    │ 重启丢失      │
    │ 不丢失    │    │          │    │ 仅开发/测试    │
    └──────────┘    └──────────┘    └──────────────┘
```

**默认选型**：**MySQL 持久化 + Redis 热缓存**（双层）。Redis 不可用时降级为纯 MySQL。

### 5.2 为什么 MySQL 是主存储

| 维度 | Redis | MySQL |
|------|:---:|:---:|
| 服务重启 | ❌ 全部丢失 | ✅ 持久化 |
| 对话可审计 | ❌ | ✅ SQL 查询历史 |
| 数据量 | 受内存限制 | 磁盘，近乎无限 |
| 关联查询 | ❌ | ✅ JOIN 会话→用户 |
| 部署依赖 | 需额外实例 | ✅ 已有（gin-template 自带） |

**Redis 的角色**：不作为数据源，仅作为**热缓存**——最近 5 分钟的活跃 Session 缓存在 Redis 中加速读取。缓存未命中时回源 MySQL。

### 5.3 数据模型

**新增 MySQL 表**：

```sql
-- 会话消息表（持久化对话历史）
CREATE TABLE session_messages (
    id         BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    session_id VARCHAR(36)  NOT NULL COMMENT '会话ID（UUID v7）',
    role       VARCHAR(10)  NOT NULL COMMENT 'user / assistant',
    content    TEXT         NOT NULL COMMENT '消息内容',
    created_at DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    
    INDEX idx_session_time (session_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='会话消息表';
```

**数据量评估**：

| 指标 | 值 |
|------|:--:|
| 单条消息 | ~500 bytes（含索引 ~1KB） |
| 单 Session（20 轮 40 条） | ~40 KB |
| 每日 1000 个活跃 Session | ~40 MB |
| 保留 30 天 | ~1.2 GB |

**清理策略**：定时任务（每天凌晨）删除 30 天前的消息。

### 5.4 Session Store 接口设计

```go
// SessionStore 会话持久化接口（屏蔽底层存储差异）
type SessionStore interface {
    // AppendMessage 追加一条消息，持久化到 MySQL
    AppendMessage(ctx context.Context, sessionID string, msg Message) error
    
    // GetHistory 获取最近 N 轮对话（先查 Redis 缓存，未命中回源 MySQL）
    GetHistory(ctx context.Context, sessionID string, maxTurns int) ([]Message, error)
    
    // EndSession 结束会话——标记会话完成，清理 Redis 缓存（MySQL 数据保留）
    EndSession(ctx context.Context, sessionID string) error
    
    // DeleteSession 硬删除会话（用户主动清除历史）
    DeleteSession(ctx context.Context, sessionID string) error
}

type Message struct {
    ID        int64     `json:"id"`
    SessionID string    `json:"session_id"`
    Role      string    `json:"role"`    // "user" | "assistant"
    Content   string    `json:"content"`
    Timestamp time.Time `json:"timestamp"`
}
```

### 5.5 MySQL 实现（默认）

```go
type MySQLSessionStore struct {
    db    *gorm.DB
    cache *RedisSessionCache  // 可选，nil 时降级纯 MySQL
}

func (s *MySQLSessionStore) AppendMessage(ctx context.Context, sessionID string, msg Message) error {
    // 1. 持久化到 MySQL
    record := &SessionMessage{
        SessionID: sessionID,
        Role:      msg.Role,
        Content:   msg.Content,
    }
    if err := s.db.Create(record).Error; err != nil {
        return fmt.Errorf("保存消息失败: %w", err)
    }

    // 2. 更新 Redis 缓存（非阻塞，失败不影响主流程）
    if s.cache != nil {
        _ = s.cache.PrependMessage(ctx, sessionID, msg)
    }
    return nil
}

func (s *MySQLSessionStore) GetHistory(ctx context.Context, sessionID string, maxTurns int) ([]Message, error) {
    // 1. 尝试 Redis 缓存
    if s.cache != nil {
        if msgs, ok := s.cache.GetHistory(ctx, sessionID, maxTurns); ok {
            return msgs, nil
        }
    }

    // 2. 回源 MySQL
    var records []SessionMessage
    limit := maxTurns * 2
    err := s.db.Where("session_id = ?", sessionID).
        Order("id DESC").Limit(limit).Find(&records).Error
    if err != nil {
        return nil, err
    }

    // 反转顺序（MySQL 返回 DESC，前端需要时间正序）
    messages := make([]Message, len(records))
    for i, r := range records {
        messages[len(records)-1-i] = Message{
            ID:        r.ID,
            SessionID: r.SessionID,
            Role:      r.Role,
            Content:   r.Content,
            Timestamp: r.CreatedAt,
        }
    }

    // 3. 回填缓存
    if s.cache != nil {
        _ = s.cache.WarmUp(ctx, sessionID, messages)
    }
    return messages, nil
}
```

### 5.6 Redis 缓存层（可选加速）

```go
type RedisSessionCache struct {
    client *redis.Client
    ttl    time.Duration // 5 分钟（热缓存，短 TTL）
}

// PrependMessage 追加单条消息到缓存列表头部
func (c *RedisSessionCache) PrependMessage(ctx context.Context, sessionID string, msg Message) error {
    key := fmt.Sprintf("reimbee:cache:session:%s:messages", sessionID)
    data, _ := json.Marshal(msg)
    pipe := c.client.Pipeline()
    pipe.LPush(ctx, key, data)
    pipe.LTrim(ctx, key, 0, 39)        // 保留最近 40 条
    pipe.Expire(ctx, key, c.ttl)        // 5 分钟 TTL
    _, err := pipe.Exec(ctx)
    return err
}

// GetHistory 从缓存读取——返回 (nil, false) 表示缓存未命中
func (c *RedisSessionCache) GetHistory(ctx context.Context, sessionID string, maxTurns int) ([]Message, bool) {
    key := fmt.Sprintf("reimbee:cache:session:%s:messages", sessionID)
    limit := int64(maxTurns * 2)
    results, err := c.client.LRange(ctx, key, 0, limit-1).Result()
    if err != nil || len(results) == 0 {
        return nil, false // 缓存未命中
    }
    messages := make([]Message, len(results))
    for i, raw := range results {
        json.Unmarshal([]byte(raw), &messages[len(results)-1-i])
    }
    return messages, true
}
```

**缓存策略**：
- **读**：先 Redis → 未命中 → MySQL → 回填 Redis
- **写**：先 MySQL → 同步更新 Redis（非阻塞）
- **TTL**：Redis 5 分钟短期缓存（MySQL 是真实数据源，Redis 只是加速层）
- **失效**：`EndSession` 时主动删除 Redis 缓存 Key

### 5.7 Eino Graph Checkpoint 持久化

Graph 的 Checkpoint 也需要持久化，但生命周期短（仅在流程执行中），MySQL 更合适：

```go
// MySQLCheckpointStore Eino Checkpoint 的 MySQL 实现
type MySQLCheckpointStore struct {
    db *gorm.DB
}

// 对应的 GORM 模型
type CheckpointRecord struct {
    ID        string    `gorm:"primaryKey;type:varchar(128)"`
    Data      string    `gorm:"type:mediumtext;not null"` // Checkpoint JSON
    CreatedAt time.Time
    UpdatedAt time.Time
}

func (s *MySQLCheckpointStore) Get(ctx context.Context, id string) (*Checkpoint, error) {
    var record CheckpointRecord
    err := s.db.WithContext(ctx).First(&record, "id = ?", id).Error
    if errors.Is(err, gorm.ErrRecordNotFound) {
        return nil, ErrCheckpointNotFound
    }
    var cp Checkpoint
    json.Unmarshal([]byte(record.Data), &cp)
    return &cp, nil
}

func (s *MySQLCheckpointStore) Put(ctx context.Context, id string, cp *Checkpoint) error {
    data, _ := json.Marshal(cp)
    return s.db.WithContext(ctx).Save(&CheckpointRecord{
        ID: id, Data: string(data),
    }).Error
}

func (s *MySQLCheckpointStore) Delete(ctx context.Context, id string) error {
    return s.db.WithContext(ctx).Delete(&CheckpointRecord{}, "id = ?", id).Error
}
```

**Checkpoint 清理**：
- 报销流程结束时立即清理
- 定时任务清理超过 1 小时的孤儿 Checkpoint（用户中途关闭页面未结束会话）

### 5.8 Checkpoint ID 与 Session ID

```
CheckpointID = GraphName + ":" + SessionID

示例:
  reimbursement_workflow:550e8400-e29b-41d4-a716-446655440000
  query_progress_workflow:550e8400-e29b-41d4-a716-446655440000
```

同一个 Session 可跨多个子流程，CheckpointID 不同，互不干扰。

### 5.9 Session 生命周期

```
用户首次对话
    │
    ▼
┌─────────────────┐
│ 生成 SessionID   │  UUID v7（时间有序）
└────────┬────────┘
         │
         ▼
   每次发消息
         │
         ├── MySQL: INSERT session_messages（持久）
         ├── Redis: LPUSH 缓存 + 续期 5min（可选）
         └── MySQL: Save Checkpoint（Graph 状态）
         │
         ▼
   报销完成 / 流程结束
         │
         ├── MySQL: 消息数据保留（审计）
         ├── Redis: DEL 缓存 Key
         └── MySQL: DELETE Checkpoint
         │
   30 天无活跃
         │
         └── 定时任务清理 MySQL 历史消息
```

| 条件 | MySQL | Redis |
|------|-------|-------|
| 用户发消息 | INSERT 消息（永久） | LPUSH 缓存（5min TTL） |
| 报销完成 | 消息保留 | DEL 缓存 Key |
| 用户清除历史 | DELETE 消息 | DEL 缓存 Key |
| 定时任务 | DELETE 30 天前消息 | 无需（TTL 自动过期） |

### 5.10 AgentRunner 中的集成

```go
type AgentRunner struct {
    graph      *compose.Runnable       // 编译后的顶层 Graph
    session    SessionStore             // 对话历史（MySQL + Redis 缓存）
    checkpoint CheckPointStore          // Graph 状态（MySQL）
}

func (a *AgentRunner) StreamChat(ctx context.Context, sessionID, userMsg string, w io.Writer) error {
    // 1. 持久化用户消息
    a.session.AppendMessage(ctx, sessionID, Message{Role: "user", Content: userMsg})

    // 2. 获取历史（缓存优先，未命中回源 MySQL）
    history, _ := a.session.GetHistory(ctx, sessionID, 10)

    // 3. 执行 Graph（Eino 自动通过 Checkpoint 恢复或新建）
    iter, err := a.graph.Stream(ctx, GraphInput{
        SessionID: sessionID,
        Message:   userMsg,
        History:   history,
    })
    if err != nil {
        return fmt.Errorf("Graph 执行失败: %w", err)
    }

    // 4. 收集回复并持久化
    var fullResponse string
    for event := range iter {
        writeSSE(w, event)
        fullResponse += event.Text
    }
    a.session.AppendMessage(ctx, sessionID, Message{Role: "assistant", Content: fullResponse})

    // 5. 如果报销已完成，清理 Checkpoint
    if eventIndicatesCompletion(iter.LastEvent()) {
        a.session.EndSession(ctx, sessionID)
    }

    return nil
}
```

### 5.11 并发安全

| 场景 | 策略 |
|------|------|
| 同 Session 并发消息 | MySQL INSERT 天然并发安全（auto_increment）；前端禁用发送按钮 |
| 多个 Session | MySQL 按 session_id 分区，互不影响 |
| Redis 缓存未命中 | 回源 MySQL + 回填缓存（可能短暂不一致，可接受） |
| MySQL 不可用 | 降级为内存 Map（单实例），日志严重告警 |

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
