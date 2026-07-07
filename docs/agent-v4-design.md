# Agent 层 v4.0 — 简化流程 + Interrupt 确认 + 持久化重构

> **日期**: 2026-07-07  
> **状态**: 设计阶段，待评审  
> **前置**: agent-v3-spec.md, agent-v3-execution-plan.md  
> **核心依赖**: Eino v0.9.12 — TurnLoop / ChatModelAgent / Interrupt / Checkpoint

---

## 目录

1. [动机：v3 Phase 系统的根本问题](#1-动机)
2. [架构总览](#2-架构总览)
3. [Agent 设计：单 Agent + ReAct + Interrupt](#3-agent-设计)
4. [Session 持久化重构](#4-session-持久化重构)
5. [Interrupt 确认机制](#5-interrupt-确认机制)
6. [工具集重新设计](#6-工具集重新设计)
7. [合规审核 LLM 工具](#7-合规审核-llm-工具)
8. [数据流](#8-数据流)
9. [API 变更](#9-api-变更)
10. [文件变更清单](#10-文件变更清单)
11. [实施计划](#11-实施计划)

---

## 1. 动机

### v3 Phase 系统的根本问题

| # | 问题 | 根因 | 影响 |
|---|------|------|------|
| P1 | **8 个 Agent 过于复杂** | 手工 Phase 状态机 + 每阶段独立 Agent | 维护成本高，新增工具需同步多个 Agent |
| P2 | **Phase 切换依赖 LLM 调用 confirm 工具** | `selectPhaseAgent()` 根据 `UserConfirmed`/`FinalConfirmed` 路由 | LLM 偶尔不调用 confirm 工具导致卡在某个 Phase |
| P3 | **无正式确认机制** | 确认只是普通工具调用，没有 Eino Interrupt | 用户无法在 UI 显式点击确认按钮，无法满足审计合规要求 |
| P4 | **状态机复杂** | `CurrentPhase` + `UserConfirmed` + `FinalConfirmed` 三个字段联合判断 | 逻辑分散在 selectPhaseAgent 和 confirm 工具中 |
| P5 | **Session 持久化耦合** | `SessionStore` 同时存储 Eino `*schema.Message` + 自定义 `ReimbursementState` | 格式不统一，序列化开销大，查询不灵活 |

### v4 核心理念

> **让 LLM 通过 ReAct 循环自主决策流程，只在最终提交环节使用 Interrupt 做人机确认。**

- **LLM 负责**：理解用户意图 → 引导上传 → 调用 OCR → 展示结果 → 调用合规 → 调用预算 → 汇总 → 请求确认
- **框架负责**：在提交前暂停执行（Interrupt），用户 UI 点击确认后恢复（Checkpoint Resume）
- **不再需要**：Phase 状态机、confirm 工具、多 Agent 切换

---

## 2. 架构总览

### v3 → v4 对比

| 方面 | v3 | v4 |
|------|:--:|:--:|
| Agent 数量 | 8 个（3 Phase + 5 子流程） | 1 个 ChatModelAgent |
| 流控方式 | Phase 状态机 + selectPhaseAgent | LLM ReAct 自主决策 |
| 确认方式 | LLM 调用 confirm 工具 | Eino Interrupt + UI 显式确认 |
| Checkpoint | 预留未启用 | 启用（中断恢复必需） |
| Session 表 | 1 张 `session_messages` + 1 张 `session_states` | `session_meta` + `session_messages` |
| 意图分类 | LLM + 关键词降级 | LLM 内置（系统 Prompt 引导） |
| 合规检查 | RAG 知识库 + 阈值比对 | LLM 工具封装 + 阈值比对 |

### 核心组件

```
┌─────────────────────────────────────────────────────────────┐
│                     Reimbee Agent v4                        │
│                                                             │
│  ┌───────────────────────────────────────────────────────┐ │
│  │              LoopManager (会话生命周期)                 │ │
│  │  loops map[sessionID]*SessionLoop                     │ │
│  └───────────────────────┬───────────────────────────────┘ │
│                          │                                  │
│  ┌───────────────────────▼───────────────────────────────┐ │
│  │           SessionLoop (TurnLoop 封装)                  │ │
│  │  GenInput ──▶ PrepareAgent ──▶ OnAgentEvents          │ │
│  │  (加载消息)    (返回唯一Agent)   (SSE + Checkpoint)    │ │
│  └───────────────────────┬───────────────────────────────┘ │
│                          │                                  │
│  ┌───────────────────────▼───────────────────────────────┐ │
│  │         ReimburseAgent (唯一 ChatModelAgent)           │ │
│  │  Instruction: 报销全流程引导                          │ │
│  │  Tools: OCR | ComplianceLLM | Budget |                 │ │
│  │         CreateReimb | SubmitReimb(Interrupt) |         │ │
│  │         PDF | Email | Progress | QueryRecords         │ │
│  │  MaxIterations: 15                                     │ │
│  └───────────────────────────────────────────────────────┘ │
│                                                             │
│  ┌───────────────────────────────────────────────────────┐ │
│  │                  Session 持久化                        │ │
│  │  session_meta (元数据) + session_messages (消息)       │ │
│  └───────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────┘
```

---

## 3. Agent 设计

### 3.1 唯一 Agent：ReimburseAgent

不再区分 Phase Agent 和子流程 Agent。一个 ChatModelAgent 通过**不同的工具集**和**Prompt 引导**完成所有任务。

```go
// phase_agents.go → 改为单例创建
func (m *LoopManager) initAgent(ctx context.Context, deps LoopManagerDeps) {
    m.reimburseAgent = mustNewAgent(ctx, deps,
        "reimburse_agent",
        "企业报销全流程智能助手",
        BuildSystemPromptV4(),
        []tool.BaseTool{
            deps.ToolSet.OCR,
            deps.ToolSet.ComplianceLLM,  // ★ 改为 LLM 工具
            deps.ToolSet.Budget,
            deps.ToolSet.CreateReimb,
            deps.ToolSet.SubmitReimb,    // ★ 内含 Interrupt
            deps.ToolSet.PDF,
            deps.ToolSet.Email,
            deps.ToolSet.Progress,
            deps.ToolSet.QueryRecords,
        },
    )
}
```

### 3.2 系统 Prompt 设计

```
你是 Reimbee，一个专业的企业财务报销智能助手。你的职责是帮助员工高效、准确地完成报销全流程。

## 报销流程

1. **信息收集**：
   - 引导用户上传票据图片
   - 用户上传后，自动调用 recognize_invoice 进行 OCR 识别
   - 展示识别结果（金额、类别、日期、销售方），请用户逐项核对
   - 支持用户修正 OCR 结果（修改金额、类别等）
   - 用户可继续添加更多票据，或告知"完成了"

2. **合规与预算检查**：
   - 用户确认票据后，调用 check_compliance 逐项检查合规性
   - 展示检查结果：通过/超标/违规
   - 调用 check_budget 检查部门预算

3. **提交确认**：
   - 合规和预算检查通过后，汇总全部信息
   - 明确告知用户："请确认以上信息无误，我将为您提交报销单"
   - 调用 submit_reimbursement 工具（用户需在UI点击确认按钮）

## 行为规范
- 一次只问一个问题，逐步引导
- 涉及金额时必须让用户确认
- 合规问题明确告知标准值和实际值
- 保持专业、友好、简洁的语气

## 异常处理
- OCR 识别失败时引导用户手动输入
- 合规超标时告知具体超标项和标准
- 预算不足时说明将触发特殊审批
```

### 3.3 PrepareAgent 简化

```go
// session_loop.go — v4 始终返回同一个 Agent
func (m *LoopManager) makePrepareAgent(sessionID string) func(...) (adk.Agent, error) {
    return func(ctx context.Context, loop *adk.TurnLoop[string, *schema.Message],
        consumed []string) (adk.Agent, error) {
        // v4: 无意图分类，无 Phase 选择，始终返回唯一 Agent
        return m.reimburseAgent, nil
    }
}
```

### 3.4 移除的组件

| v3 组件 | v4 处理 |
|---------|--------|
| `phase1Agent` / `phase2Agent` / `phase3Agent` | 合并为 `reimburseAgent` |
| `chatAgent` / `progressAgent` / `budgetAgent` / `policyAgent` / `modifyAgent` | 合并到 `reimburseAgent` |
| `selectPhaseAgent()` | 删除 |
| `classifyIntentByLLM()` / `classifyByKeywords()` | 删除（LLM 自主理解意图） |
| `confirm_invoice` 工具 | 删除（LLM 在对话中确认） |
| `confirm_submit` 工具 | 替换为 `submit_reimbursement` 内建 Interrupt |
| `ReimbursementState.CurrentPhase` / `UserConfirmed` / `FinalConfirmed` | 删除 |
| `ReimbursementState.Phase1Turns` / `Phase2Turns` / `Phase3Turns` | 删除 |

---

## 4. Session 持久化重构

### 4.1 当前问题

```
v3 表结构:
  session_messages (id, session_id, role, content, raw_json, created_at)
  session_states   (id, session_id, state_key, data, created_at, updated_at)
  
问题:
  1. raw_json 存储完整 Eino *schema.Message，包含 ToolCalls 等嵌套结构
  2. 查询消息历史需反序列化全部 JSON
  3. 无会话元数据（状态、过期时间等）
  4. session_states 的 data 也是 JSON 序列化，不便于查询
```

### 4.2 v4 表设计

#### 表 1: `session_meta` — 会话元数据

```sql
CREATE TABLE session_meta (
    id            BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    session_id    VARCHAR(36)  NOT NULL COMMENT '会话UUID v7',
    user_id       BIGINT UNSIGNED NOT NULL COMMENT '用户ID',
    employee_id   VARCHAR(32)  NOT NULL DEFAULT '' COMMENT '员工工号',
    role          VARCHAR(20)  NOT NULL DEFAULT 'employee' COMMENT '角色',
    status        VARCHAR(20)  NOT NULL DEFAULT 'active' COMMENT 'active/completed/expired',
    
    -- 业务快照（冗余，用于列表展示，不参与流程逻辑）
    summary       VARCHAR(512) NOT NULL DEFAULT '' COMMENT '会话摘要（最后一条用户消息截断）',
    
    -- Checkpoint 关联
    checkpoint_id VARCHAR(128) NOT NULL DEFAULT '' COMMENT 'Eino CheckpointID（用于中断恢复）',
    
    message_count INT UNSIGNED NOT NULL DEFAULT 0 COMMENT '消息总数（冗余计数器）',
    
    created_at    DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    expires_at    DATETIME     NULL COMMENT '过期时间（NULL=永不过期）',
    
    UNIQUE INDEX idx_session_id (session_id),
    INDEX idx_user_id (user_id),
    INDEX idx_status (status),
    INDEX idx_expires_at (expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='会话元数据表';
```

```go
// model/session_meta.go
type SessionMeta struct {
    ID           uint       `gorm:"primaryKey;autoIncrement"`
    SessionID    string     `gorm:"type:varchar(36);uniqueIndex;not null;comment:会话UUID v7"`
    UserID       uint       `gorm:"not null;index;comment:用户ID"`
    EmployeeID   string     `gorm:"type:varchar(32);not null;default:'';comment:员工工号"`
    Role         string     `gorm:"type:varchar(20);not null;default:'employee';comment:角色"`
    Status       string     `gorm:"type:varchar(20);not null;default:'active';comment:active/completed/expired"`
    Summary      string     `gorm:"type:varchar(512);not null;default:'';comment:会话摘要"`
    CheckpointID string     `gorm:"type:varchar(128);not null;default:'';comment:Eino CheckpointID"`
    MessageCount uint       `gorm:"not null;default:0;comment:消息总数"`
    CreatedAt    time.Time  `gorm:"autoCreateTime"`
    UpdatedAt    time.Time  `gorm:"autoUpdateTime"`
    ExpiresAt    *time.Time `gorm:"index;comment:过期时间"`
}
```

#### 表 2: `session_messages` — 消息明细（重构）

```sql
CREATE TABLE session_messages (
    id            BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    session_id    VARCHAR(36)  NOT NULL COMMENT '会话ID',
    seq           INT UNSIGNED NOT NULL COMMENT '消息序号（会话内单调递增）',
    role          VARCHAR(20)  NOT NULL COMMENT 'user/assistant/tool',
    
    -- 文本内容（人类可读）
    content       TEXT         NULL COMMENT '消息文本（user消息为用户输入，assistant消息为LLM回复，tool消息为工具返回摘要）',
    
    -- 工具调用详情（仅 tool 角色填充）
    tool_name     VARCHAR(64)  NOT NULL DEFAULT '' COMMENT '工具名称',
    tool_input    TEXT         NULL COMMENT '工具输入参数（JSON）',
    tool_output   TEXT         NULL COMMENT '工具输出结果（JSON）',
    
    -- 完整元数据（仅 Eino 框架消费，业务不直接读取）
    message_meta  JSON         NULL COMMENT 'Eino Message 元数据（ToolCalls, ResponseMeta等）',
    
    created_at    DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    
    INDEX idx_session_seq (session_id, seq),
    INDEX idx_session_role (session_id, role)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='会话消息明细表';
```

```go
// model/session_message.go — v4 重构
type SessionMessage struct {
    ID          uint      `gorm:"primaryKey;autoIncrement"`
    SessionID   string    `gorm:"type:varchar(36);index:idx_session_seq,priority:1;not null;comment:会话ID"`
    Seq         uint      `gorm:"index:idx_session_seq,priority:2;not null;comment:消息序号"`
    Role        string    `gorm:"type:varchar(20);index:idx_session_role,priority:2;not null;comment:user/assistant/tool"`
    Content     *string   `gorm:"type:text;comment:消息文本内容"`
    ToolName    string    `gorm:"type:varchar(64);not null;default:'';comment:工具名称"`
    ToolInput   *string   `gorm:"type:text;comment:工具输入参数JSON"`
    ToolOutput  *string   `gorm:"type:text;comment:工具输出结果JSON"`
    MessageMeta *string   `gorm:"type:json;comment:Eino Message元数据"`
    CreatedAt   time.Time `gorm:"autoCreateTime"`
}
```

### 4.3 读写模式

```
读路径（GenInput 加载历史）:
  1. 查 session_meta → 获取 message_count
  2. 查 session_messages WHERE session_id=? ORDER BY seq ASC LIMIT N
  3. 将结构化字段还原为 []*schema.Message（供 Eino Agent 消费）

写路径（OnAgentEvents 持久化）:
  1. 将 schema.Message 拆分：content → content, tool_name/tool_input/tool_output → 对应字段
  2. 分配 seq（从 meta.message_count 递增）
  3. 批量 INSERT session_messages
  4. UPDATE session_meta SET message_count=?, summary=?, updated_at=NOW()
```

### 4.4 与旧 SessionStore 的兼容

`infra.SessionStore` 接口保留但实现简化：

```go
// infra/session.go — v4 接口（保持兼容）
type SessionStore interface {
    // 消息
    SaveMessages(ctx, sessionID string, msgs []*schema.Message) error
    GetHistory(ctx, sessionID string, limit int) ([]*schema.Message, error)
    
    // 会话元数据
    CreateSession(ctx, meta *SessionMeta) error
    GetSession(ctx, sessionID string) (*SessionMeta, error)
    UpdateSession(ctx, sessionID string, updates map[string]any) error
    DeleteSession(ctx, sessionID string) error
    ListSessions(userID uint, status string) ([]*SessionMeta, error)
    
    // 已废弃（v4 不再使用 StateKeyReimbursement）
    // SaveState / GetState / DeleteState → 删除
}
```

---

## 5. Interrupt 确认机制

### 5.1 设计目标

用户说"确认提交" → LLM 调用 `submit_reimbursement` 工具 → 工具内触发 `adk.Interrupt()` → 框架暂停，保存 Checkpoint → 前端弹出确认按钮 → 用户点击 → 恢复执行 → 提交完成

```
LLM: "请确认报销信息，我将为您提交"
User: "确认提交"
    ↓
LLM 调用 submit_reimbursement 工具
    ↓
submit_reimbursement 工具内部:
    1. 汇总信息 → InterruptInfo
    2. adk.Interrupt(ctx, InterruptInfo) → 暂停
    ↓
OnAgentEvents 检测到 Interrupt → 发送 confirm_required SSE 事件
    ↓
前端 UI 弹出确认卡片: "报销单汇总 | 3张票据 ¥1,250.00 | [确认提交] [取消]"
    ↓
用户点击 [确认提交] → POST /api/chat/approve?session_id=X&approved=true
    ↓
后端: 创建新 TurnLoop (相同 CheckpointID) → Push 审批结果 → GenResume → 继续执行
    ↓
submit_reimbursement 工具继续: 收到 approved=true → 调用 ReimbursementBiz.Submit()
    ↓
返回提交结果 → LLM 展示报销单号 → 生成PDF → 发送邮件 → 完成
```

### 5.2 submit_reimbursement 工具实现

```go
// tools/submit_reimb_tool.go — v4 重写，内置 Interrupt

type SubmitReimbInput struct {
    // 提交信息由工具从 ReimbursementState 中自动获取（无需 LLM 手动传入）
    // 此结构体仅用于 Eino jsonschema 生成
    Confirmed bool `json:"confirmed" jsonschema:"required" jsonschema_description:"用户是否确认提交"`
}

type SubmitReimbOutput struct {
    ReimbursementNo     string `json:"reimbursement_no"`
    Status              string `json:"status"`
    NeedSpecialApproval bool   `json:"need_special_approval"`
}

func NewSubmitReimbTool(
    reimbursementBiz *reimbursement.ReimbursementBiz,
    logger *log.Logger,
) *SubmitReimbTool {
    
    t, err := utils.InferTool[SubmitReimbInput, SubmitReimbOutput](
        "submit_reimbursement",
        "提交报销单进入审批流程。调用此工具将触发用户UI确认（中断），用户确认后执行提交。提交后不可撤销。",
        func(ctx context.Context, input SubmitReimbInput) (SubmitReimbOutput, error) {

            // ── 1. 从 context 加载当前报销状态 ──
            var state ReimbursementState
            if raw, ok := ctx.Value(StateContextKey{}).(*ReimbursementState); ok {
                state = *raw
            }

            // ── 2. 构建中断信息 ──
            interruptInfo := SubmitConfirmInfo{
                Invoices:    state.Invoices,
                TotalAmount: state.TotalAmount,
                Budget:      state.BudgetResult,
                Compliance:  state.ComplianceResult,
                Message:     "请确认报销单信息无误后提交",
            }

            // ── 3. ★ Eino Interrupt: 暂停执行，等待用户UI确认 ──
            logger.Info("触发提交确认中断", 
                zap.Int("票据数", len(state.Invoices)),
                zap.Int64("总金额(分)", state.TotalAmount))
            
            // Interrupt 会保存当前执行状态到 CheckpointStore
            event := adk.Interrupt(ctx, interruptInfo)
            
            // Interrupt 返回一个 AgentEvent，我们将其作为错误返回给框架
            // 框架检测到 Interrupted action → 保存 Checkpoint → 暂停 Agent
            return SubmitReimbOutput{}, event.Action.Interrupted

            // ── 注意 ──
            // 上面的 return 会在 Interrupt 触发后执行。
            // 实际上是 Interrupt() 函数内部通过 panic/signal 机制暂停，
            // 恢复后通过 ResumeInfo 传递用户确认结果。
        },
    )
    // ...
}
```

**更准确地实现（基于 Eino Interrupt API）**：

```go
func NewSubmitReimbTool(...) *SubmitReimbTool {
    t, err := utils.InferTool[SubmitReimbInput, SubmitReimbOutput](
        "submit_reimbursement",
        "提交报销单进入审批流程。此操作将触发用户UI确认，确认后不可撤销。",
        func(ctx context.Context, input SubmitReimbInput) (SubmitReimbOutput, error) {

            // ── 检查是否为恢复执行（用户已确认）──
            resumeInfo, isResume := adk.GetResumeInfo(ctx)
            if !isResume {
                // 首次调用：加载状态，触发中断
                var state ReimbursementState
                if raw, ok := ctx.Value(StateContextKey{}).(*ReimbursementState); ok {
                    state = *raw
                }
                
                // 构建确认信息（传递给前端）
                confirmData := SubmitConfirmInfo{
                    Invoices:    state.Invoices,
                    TotalAmount: state.TotalAmount,
                    Budget:      state.BudgetResult,
                    Compliance:  state.ComplianceResult,
                }
                
                logger.Info("触发提交确认中断",
                    zap.Int("票据数", len(state.Invoices)),
                    zap.Int64("总金额(分)", state.TotalAmount))
                
                // ★ Interrupt: 框架暂停执行，保存 Checkpoint
                // 恢复时此函数会被重新调用，isResume=true
                return SubmitReimbOutput{}, adk.NewInterruptError(confirmData)
            }

            // ── 恢复执行：用户已确认 ──
            logger.Info("收到用户确认，执行提交",
                zap.Any("确认结果", resumeInfo.ResumeData))
            
            // 从 resumeInfo 获取批准结果
            approval, ok := resumeInfo.ResumeData.(*ApprovalResult)
            if !ok || !approval.Approved {
                return SubmitReimbOutput{}, fmt.Errorf("用户取消提交")
            }

            // 加载状态并执行提交
            var state ReimbursementState
            // ... 从 context 加载状态 ...
            
            rm, err := reimbursementBiz.Submit(state.ReimbursementID, state.TotalAmount)
            if err != nil {
                return SubmitReimbOutput{}, fmt.Errorf("提交失败: %w", err)
            }

            return SubmitReimbOutput{
                ReimbursementNo:     rm.ReimbursementNo,
                Status:              rm.Status,
                NeedSpecialApproval: rm.NeedSpecialApproval,
            }, nil
        },
    )
}
```

### 5.3 HTTP 层：审批确认端点

```go
// POST /api/chat/approve?session_id=X
// Body: {"approved": true, "reason": ""}

func (s *AgentService) HandleApprove(c *gin.Context) {
    sessionID := c.Query("session_id")
    var req struct {
        Approved bool   `json:"approved"`
        Reason   string `json:"reason,omitempty"`
    }
    c.ShouldBindJSON(&req)

    // 创建新 TurnLoop（相同 CheckpointID），Push 审批结果
    // Eino 自动通过 GenResume 恢复执行
    item := &ApproveItem{
        Approved:     req.Approved,
        Reason:       req.Reason,
        InterruptID:  sessionID, // 与 CheckpointID 相同
    }

    // 使用相同的 CheckpointID 创建新 loop → 自动触发 GenResume
    s.loopManager.PushApprove(sessionID, item, sseWriter, doneCh)
    
    // 阻塞等待恢复执行完成
    <-doneCh
}
```

### 5.4 GenResume 回调

```go
func (m *LoopManager) makeGenResume(sessionID string) func(
    ctx context.Context,
    loop *adk.TurnLoop[string, *schema.Message],
    interruptedItems, unhandledItems, newItems []string,
) (*adk.GenResumeResult[string, *schema.Message], error) {

    return func(...) (*adk.GenResumeResult[string, *schema.Message], error) {
        // 从 newItems 中提取审批结果
        var approval *ApprovalResult
        for _, item := range newItems {
            var ar ApprovalResult
            if err := json.Unmarshal([]byte(item), &ar); err == nil {
                approval = &ar
                break
            }
        }

        return &adk.GenResumeResult[string, *schema.Message]{
            ResumeParams: &adk.ResumeParams{
                Targets: map[string]any{
                    sessionID: approval, // key = InterruptID = sessionID
                },
            },
            Consumed:  newItems,
            Remaining: unhandledItems,
        }, nil
    }
}
```

### 5.5 Checkpoint 启用

```go
// session_loop.go — createSessionLoop v4
func (m *LoopManager) createSessionLoop(sessionID string) *SessionLoop {
    ctx, cancel := context.WithCancel(context.Background())

    sl := &SessionLoop{sessionID: sessionID, cancel: cancel, lastActive: time.Now()}

    cfg := adk.TurnLoopConfig[string, *schema.Message]{
        GenInput:      m.makeGenInput(sessionID),
        PrepareAgent:  m.makePrepareAgent(sessionID),
        OnAgentEvents: m.makeOnAgentEvents(sessionID),
        
        // ★ v4 启用 Checkpoint（中断恢复必需）
        GenResume:    m.makeGenResume(sessionID),
        Store:        m.checkpointStore,  // MySQLCheckpointStore
        CheckpointID: sessionID,
    }

    sl.turnLoop = adk.NewTurnLoop(cfg)
    sl.turnLoop.Run(ctx)
    return sl
}
```

### 5.6 SSE 事件：confirm_required → done

```
# 正常流程 SSE 事件序列
event: thinking     →  {"message":"正在汇总报销信息..."}
event: message      →  {"content":"以下是您的报销单汇总：\n\n...","delta":false}
event: tool_call    →  {"tool":"submit_reimbursement","input":{...}}

# ★ Interrupt 触发
event: interrupted  →  {
    "type": "confirm_required",
    "data": {
        "interrupt_id": "session-xxx",
        "action": "confirm_submit",
        "invoices": [...],
        "total_amount": 125000,  // 分
        "budget_remaining": 5000000
    }
}

# 用户点击确认后，新 SSE 连接继续
event: thinking     →  {"message":"正在提交报销单..."}
event: message      →  {"content":"报销单已提交成功！\n\n报销单号: REIMB-2026-0001\n...","delta":false}
event: done         →  {}
```

### 5.7 前端确认 UI 交互

```typescript
// Chat.tsx 中断处理
function handleSSEEvent(event: SSEEvent) {
    if (event.type === 'confirm_required') {
        // 显示确认弹窗
        setConfirmDialog({
            visible: true,
            title: '确认提交报销单',
            data: event.data,
            onConfirm: async () => {
                await api.approveChat(sessionId, { approved: true });
                setConfirmDialog({ visible: false });
            },
            onCancel: async () => {
                await api.approveChat(sessionId, { approved: false });
                setConfirmDialog({ visible: false });
            },
        });
    }
}
```

---

## 6. 工具集重新设计

### 6.1 工具清单（v4）

| 工具名 | 阶段 | 功能 | 变更 |
|--------|------|------|------|
| `recognize_invoice` | 信息收集 | OCR 识别票据 | **保留，简化**（移除 SessionStore 依赖，仅返回结果） |
| `check_compliance` | 合规检查 | ★ LLM+RAG 合规审核 | **重写**（LLM + KB.Search()，见 §7） |
| `check_budget` | 预算检查 | 查询部门预算 | **保留**（封装不变） |
| `create_reimbursement` | 执行 | 创建报销单草稿 | **保留** |
| `submit_reimbursement` | 执行 | ★ 提交报销单（含 Interrupt） | **重写**（见 §5.2） |
| `generate_pdf` | 执行 | 生成报销单 PDF | **保留** |
| `send_email` | 执行 | 发送审批通知邮件 | **保留** |
| `query_progress` | 查询 | 查询审批进度 | **保留** |
| `query_reimbursements` | 查询 | 查询报销记录列表 | **保留** |

### 6.2 移除的工具

| 工具 | 移除原因 |
|------|---------|
| `confirm_invoice` | Phase 确认由 LLM 在对话中自然完成 |
| `confirm_submit` | 替换为 `submit_reimbursement` 内建 Interrupt |
| `check_compliance` (v3 RAG 版) | 替换为 LLM+RAG 版（保留 KB.Search()，审查逻辑从代码阈值比对改为 LLM 语义理解） |

### 6.3 ToolSet 简化

```go
// tools/provider.go — v4
type ToolSet struct {
    OCR            tool.InvokableTool
    ComplianceLLM  tool.InvokableTool  // ★ 改名
    Budget         tool.InvokableTool
    CreateReimb    tool.InvokableTool
    SubmitReimb    tool.InvokableTool  // ★ 内建 Interrupt
    PDF            tool.InvokableTool
    Email          tool.InvokableTool
    Progress       tool.InvokableTool
    QueryRecords   tool.InvokableTool
}

// v4: 不再需要 Phase 分组方法
// 删除: GetPhase1Tools(), GetPhase2Tools(), GetPhase3Tools()
// 删除: GetPhase1BaseTools(), GetPhase2BaseTools(), GetPhase3BaseTools()
```

---

## 7. 合规审核工具：AgentTool（LLM + RAG Tool）

### 7.1 设计思路

**不**在工具函数内部手工调用 `KB.Search()` 再拼 Prompt，而是使用 Eino 原生模式：

```
check_compliance = AgentTool（包装一个 ChatModelAgent）
    └── ComplianceMiniAgent (ChatModelAgent)
        ├── Instruction: "你是企业合规审核专家..."
        ├── Tools: [search_policy]  ← RAG 检索封装为 Tool
        └── ReAct: search_policy → LLM 审核 → 返回结构化结果
```

主 Agent 将 `check_compliance` 看作一个普通工具调用——传入票据信息，得到合规结论。
但这个工具内部是一个**完整的 ChatModelAgent**，它有自己的 ReAct 循环：先用 `search_policy` 检索政策，再结合票据信息判定合规性。

**与 v3 RAG 版的关键区别**：

| | v3 RAG 版 | v4 AgentTool 版 |
|---|----------|----------------|
| RAG 调用方式 | 代码直接调用 `kb.Search()` | LLM 通过 Tool 调用 `search_policy` |
| 检索策略 | 固定：按类别+金额合成 query | LLM 自主决定检索词和检索时机 |
| 审核逻辑 | 程序化阈值比对 | LLM + ReAct 循环推理 |
| 模糊规则 | 无法处理 | LLM 自主理解后决策 |
| 多次检索 | 不支持 | LLM 可多次调用 search_policy（如先查类别标准再查城市级别） |

### 7.2 实现：三层结构

#### 层 1: `search_policy` — RAG 检索 Tool

```go
// tools/search_policy_tool.go — v4 新增
//
// 将知识库检索封装为 Eino Tool，供 ComplianceMiniAgent 调用。
// 与 v3 的 compliance.ComplianceBiz.CheckCompliance 不同：
//   此工具只负责"检索政策文档"，不负责"判定合规性"，
//   判定由 ComplianceMiniAgent 的 LLM 完成。

type SearchPolicyInput struct {
    Query string `json:"query" jsonschema:"required" jsonschema_description:"检索查询（自然语言，如'差旅住宿标准 500元'）"`
    Limit int    `json:"limit" jsonschema:"default=5" jsonschema_description:"返回结果数上限"`
}

type SearchPolicyOutput struct {
    Chunks []PolicyChunk `json:"chunks"` // 检索到的政策文档片段
}

type PolicyChunk struct {
    RuleID  string  `json:"rule_id"`   // 规则ID（如 RULE-TRAVEL-002）
    Title   string  `json:"title"`     // 政策标题
    Content string  `json:"content"`   // 政策原文片段
    Source  string  `json:"source"`    // 来源文档名
    Score   float64 `json:"score"`     // 语义相似度分数
}

func NewSearchPolicyTool(
    kb *compliance.KnowledgeBase, // 复用 v3 知识库
    logger *log.Logger,
) tool.InvokableTool {
    t, err := utils.InferTool[SearchPolicyInput, SearchPolicyOutput](
        "search_policy",
        "检索企业报销政策知识库。根据自然语言查询，返回最相关的政策文档片段，包括费用标准、审批流程、特殊规定等。",
        func(ctx context.Context, input SearchPolicyInput) (SearchPolicyOutput, error) {
            if input.Limit <= 0 {
                input.Limit = 5
            }
            // 调用向量检索
            chunks, err := kb.Search(ctx, input.Query, input.Limit)
            if err != nil {
                return SearchPolicyOutput{}, fmt.Errorf("政策检索失败: %w", err)
            }
            // 转换为输出结构
            result := make([]PolicyChunk, 0, len(chunks))
            for _, c := range chunks {
                result = append(result, PolicyChunk{
                    RuleID:  c.RuleID,
                    Title:   c.Source,
                    Content: c.Content,
                    Source:  c.Source,
                    Score:   c.Score,
                })
            }
            logger.Debug("政策检索完成",
                zap.String("查询", input.Query),
                zap.Int("命中数", len(result)))
            return SearchPolicyOutput{Chunks: result}, nil
        },
    )
    if err != nil {
        panic("创建search_policy工具失败: " + err.Error())
    }
    return t
}
```

#### 层 2: ComplianceMiniAgent — 审核 ChatModelAgent

```go
// tools/compliance_agent_tool.go — v4 新增
//
// 创建一个 ChatModelAgent 作为合规审核子 Agent，
// 持有 search_policy 工具，通过 ReAct 循环完成检索→审核→输出。

func NewComplianceAgentTool(
    ctx context.Context,
    chatModel model.ToolCallingChatModel,
    searchPolicyTool tool.InvokableTool,
    logger *log.Logger,
) *ComplianceAgentTool {

    // ── 创建合规审核 Mini-Agent ──
    complianceAgent, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
        Name:        "compliance_reviewer",
        Description: "企业合规审核专家。接收票据信息（金额、类别、日期），调用 search_policy 检索相关政策，然后判定合规性并给出建议。",
        Instruction: buildComplianceAgentInstruction(),
        Model:       chatModel,
        ToolsConfig: adk.ToolsConfig{
            ToolsNodeConfig: compose.ToolsNodeConfig{
                Tools: []tool.BaseTool{searchPolicyTool}, // ★ RAG 作为工具
            },
        },
        MaxIterations: 5, // search→review→done（最多2-3轮，5为安全上限）
    })
    if err != nil {
        panic("创建合规审核Agent失败: " + err.Error())
    }

    // ── 包装为 AgentTool ──
    agentTool := adk.NewAgentTool(ctx, complianceAgent)
    logger.Debug("合规审核AgentTool创建完成")

    return &ComplianceAgentTool{InvokableTool: agentTool}
}

// ComplianceAgentTool 命名类型（Wire DI 区分类键）
type ComplianceAgentTool struct{ tool.InvokableTool }

// buildComplianceAgentInstruction 合规审核 Agent 的系统指令
func buildComplianceAgentInstruction() string {
    return `你是一个企业财务合规审核专家。你的工作流程：

1. **检索政策**：收到票据信息后，先调用 search_policy 工具检索相关企业报销政策。
   - 按费用类别检索（如"差旅住宿标准"）
   - 必要时多次检索（如先查类别标准，再查城市级别、职级限制等特殊规则）

2. **审核票据**：根据检索到的政策原文，审核票据的：
   - 金额是否在标准范围内
   - 类别是否允许报销
   - 日期是否在有效期内（通常为90天）
   - 是否有特殊条件（如一线城市上浮、高级别员工额度等）

3. **返回结果**：审核完成后，以 JSON 格式输出结论，格式如下：
{
    "result": "pass|warning|error",
    "message": "检查结果描述（中文，引用具体政策条款）",
    "rule_id": "触发的规则ID（从search_policy返回结果中提取）",
    "standard": "政策标准值（如 500元/晚，超标时必填）",
    "suggestion": "处理建议（超标时必填，如'建议提交部门负责人审批'）",
    "reference": "引用的政策原文摘要（溯源用，不超过100字）"
}

## 判定标准
- **pass**: 票据完全符合所有相关政策标准
- **warning**: 金额为标准值的80%-100%，或日期距过期不足30天
- **error**: 金额超过标准值，或日期已过期，或类别不在允许报销范围内

## 注意事项
- 必须以检索到的政策原文为准，不要凭记忆猜测标准值
- 如果一次检索未找到相关规则，尝试换用不同关键词再次检索
- 金额统一以"元"为单位展示和比较`
}

```

#### 层 3: AgentTool 注册到主 Agent 工具集

```go
// tools/provider.go — v4 ToolSet 聚合
type ToolSet struct {
    OCR            tool.InvokableTool
    Compliance     tool.InvokableTool  // ★ 类型改为 AgentTool
    Budget         tool.InvokableTool
    CreateReimb    tool.InvokableTool
    SubmitReimb    tool.InvokableTool
    PDF            tool.InvokableTool
    Email          tool.InvokableTool
    Progress       tool.InvokableTool
    QueryRecords   tool.InvokableTool
}
```

### 7.3 完整调用链

```
用户: "我要报销差旅住宿500元"

主 ReimburseAgent (ReAct):
  ├── LLM 思考: "需要检查合规性"
  ├── 调用 check_compliance 工具 ──────────────────────┐
  │                                                     │
  │  ┌── ComplianceMiniAgent (ReAct) ──────────────┐   │
  │  │  ① LLM: "先查差旅住宿标准"                    │   │
  │  │  ② 调用 search_policy("差旅住宿 标准 元")      │   │
  │  │     RAG: 向量检索 → 返回3条政策片段            │   │
  │  │  ③ LLM: "标准为500元/晚，票据500元，判定pass"  │   │
  │  │  ④ 输出 JSON: {"result":"pass",...}           │   │
  │  └──────────────────────────────────────────────┘   │
  │                                                     │
  └── 返回: {"result":"pass","message":"符合标准",...}

主 Agent LLM: "合规检查通过，金额符合差旅住宿标准500元/晚"
```

### 7.4 架构对比

```
┌─── v3 RAG 版 ────────────────────────────────┐
│  代码直接调 kb.Search() → 程序化阈值比对       │
│  if amount > 500 → error                      │
│  无法理解 "一线城市上浮至800" 等自然语言政策    │
└───────────────────────────────────────────────┘

┌─── v4 AgentTool 版 ──────────────────────────┐
│  check_compliance = AgentTool(                │
│      ComplianceMiniAgent {                    │
│          LLM + [search_policy tool]           │
│      }                                        │
│  )                                            │
│                                               │
│  LLM 自主决定: 查什么、查几次、如何判定       │
│  能理解自然语言政策中的条件和例外              │
│  完全 Eino 原生模式，无手工 Pipeline          │
└───────────────────────────────────────────────┘
```

### 7.5 配置

```yaml
# config.yaml — 合规审核配置
compliance:
  # 知识库（复用 v3 RAG 基础设施，供 search_policy tool 使用）
  knowledge_base:
    embedding_model: "text-embedding-v3"

  # 审核 Agent（独立 LLM 配置，可与主 Agent 不同）
  agent:
    model: "gpt-4o"            # 审核需要较强推理能力
    temperature: 0.0           # 零温度确保判定一致性
    max_iterations: 5          # ReAct 上限（search→review 通常2-3轮）
    max_tokens: 1024
```

---

## 8. 数据流

### 8.1 完整请求-响应流（v4）

```
HTTP POST /api/chat/stream?session_id=X&message=Y
  │
  ▼ AgentService.HandleChat()
  │ 1. 解析参数 → 提取 JWT 身份
  │ 2. 查询/创建 session_meta
  │ 3. 创建 GinSSEWriter
  │ 4. InjectUserIdentity → 更新 session_meta
  │ 5. PushMessage → doneCh 阻塞等待
  ▼
LoopManager.PushMessage
  │ GetOrCreate → createSessionLoop (启用 Store + CheckpointID)
  │ Push(message)
  ▼
TurnLoop 三回调:
  │
  ├─ ① GenInput:
  │   1. 查 session_meta → message_count
  │   2. 查 session_messages ORDER BY seq ASC LIMIT N
  │   3. 还原为 []*schema.Message
  │   4. 构建 AgentInput{Messages: [历史+本轮], EnableStreaming: true}
  │   5. 持久化用户消息: INSERT session_messages (seq=message_count+1)
  │
  ├─ ② PrepareAgent:
  │   始终返回 m.reimburseAgent（唯一 Agent）
  │
  ├─ ③ ChatModelAgent ReAct 循环 (MaxIterations=15):
  │   LLM 思考 → 工具调用 → 工具执行 → 结果注入 → 循环
  │   （工具如 submit_reimbursement 会触发 Interrupt）
  │
  └─ ④ OnAgentEvents:
      消费 AgentEvent 流 → SSE 推送
      → 持久化 assistant/tool 消息到 session_messages
      → clearActiveWriter → doneCh ← nil → HTTP 返回
```

### 8.2 中断恢复流

```
正常 Turn (TurnLoop A):
  LLM 调用 submit_reimbursement → Interrupt 触发
  → Checkpoint 保存到 MySQLCheckpointStore (key=sessionID)
  → TurnLoop A 退出（ExitReason = InterruptError）
  → OnAgentEvents 发送 confirm_required SSE 事件
  → doneCh ← InterruptError → HTTP handler 返回

用户确认后:
  POST /api/chat/approve?session_id=X {"approved":true}
  → 创建新 TurnLoop B（相同 CheckpointID）
  → TurnLoop B.Run() 检测到 Checkpoint 存在
  → 调用 GenResume（而非 GenInput）
  → Push 审批结果 item → GenResume 构建 ResumeParams
  → ResumeParams.Targets[sessionID] = ApprovalResult
  → 恢复 Agent 执行，submit_reimbursement 工具重新运行
  → isResume=true → 读取 ResumeInfo → 执行提交
  → 正常完成后清除 Checkpoint
```

---

## 9. API 变更

### 9.1 新增端点

```
POST /api/chat/approve?session_id=X
  请求体: {"approved": true, "reason": ""}
  响应: SSE 流（恢复执行的后续事件）
  用途: 用户在 UI 点击确认按钮后调用
```

### 9.2 不变端点

```
GET /api/chat/stream?session_id=X&message=Y    — 对话（v4 行为不变）
POST /api/reimbursements/upload                 — 票据上传（不变）
POST /api/sessions                               — 创建会话（新增）
GET /api/sessions                                — 会话列表（新增）
DELETE /api/sessions/:id                         — 删除会话（新增）
```

### 9.3 删除/废弃

- `confirm_invoice` / `confirm_submit` 相关的前端逻辑（不再需要主动调确认工具）
- `phase_change` SSE 事件类型（不再有 Phase 概念）

---

## 10. 文件变更清单

### 10.1 新增文件

```
model/session_meta.go                              — SessionMeta GORM 模型
internal/domain/agent/tools/search_policy_tool.go  — RAG 检索 Tool（供 ComplianceMiniAgent 调用）
internal/domain/agent/tools/compliance_agent_tool.go — ComplianceMiniAgent + AgentTool 包装
docs/agent-v4-design.md                            — 本文档
```

### 10.2 修改文件

```
# Agent 核心
internal/domain/agent/loop_manager.go    — 单 Agent 持有、Checkpoint 启用
internal/domain/agent/session_loop.go    — PrepareAgent 简化、GenResume 新增
internal/domain/agent/phase_agents.go    — 改为单 Agent 创建
internal/domain/agent/service.go         — HandleApprove 新增
internal/domain/agent/prompt.go          — 简化为单系统 Prompt
internal/domain/agent/dto.go             — 移除 IntentResult/WorkflowRoute/GuardResult
internal/domain/agent/config.go          — 新增 InterruptTimeout 配置项
internal/domain/agent/sse.go             — 新增 Interrupted 事件类型
internal/domain/agent/runner.go          — 移除不必要的 re-export
internal/domain/agent/provider.go        — Wire 更新（移除旧 Factory、新增 GenResume）

# 工具层
internal/domain/agent/tools/provider.go  — ToolSet 简化，移除 Phase 分组
internal/domain/agent/tools/submit_reimb_tool.go  — 重写：内建 Interrupt
internal/domain/agent/tools/ocr_tool.go  — 简化：移除 SessionStore 依赖

# 类型
internal/domain/agent/types/state.go     — 简化 ReimbursementState

# Session 持久化
model/session_message.go                 — 重构：拆分为结构化字段
infra/session.go                         — 接口更新
infra/session_mysql.go                   — 重写：meta + messages 分表操作
infra/session_state.go                   — 标记废弃（不再使用 session_states 表）
infra/session_redis.go                   — 适配新格式

# 路由
internal/router/root.go                  — 新增 approve 路由
```

### 10.3 删除文件

```
internal/domain/agent/tools/confirm_invoice_tool.go   — Phase 确认由 LLM 对话完成
internal/domain/agent/tools/confirm_submit_tool.go    — 替换为 Interrupt
internal/domain/agent/tools/compliance_tool.go        — 替换为 search_policy_tool.go + compliance_agent_tool.go
```

---

## 11. 实施计划

### Phase 1: Session 持久化重构（基础）

| 步骤 | 内容 | 验证 |
|------|------|------|
| 1.1 | 创建 `model/session_meta.go`，定义 SessionMeta GORM 模型 | go build |
| 1.2 | 重构 `model/session_message.go`，拆分为结构化字段 | go build |
| 1.3 | 重写 `infra/session_mysql.go`，实现 meta + messages 分离操作 | 单元测试 |
| 1.4 | 创建 `infra/session_meta_repo.go`，封装会话 CRUD | 单元测试 |
| 1.5 | 更新 `infra/session.go` 接口定义 | go build |
| 1.6 | 适配 `infra/session_redis.go` 新格式 | 集成测试 |

### Phase 2: Agent 简化

| 步骤 | 内容 | 验证 |
|------|------|------|
| 2.1 | 修改 `phase_agents.go` → 单 Agent 创建 | go build |
| 2.2 | 简化 `session_loop.go`：移除意图分类、PrepareAgent 返回单 Agent | go build |
| 2.3 | 简化 `prompt.go`：单一系统 Prompt | go build |
| 2.4 | 简化 `dto.go`：移除废弃类型 | go build |
| 2.5 | 创建 `compliance_llm_tool.go`：LLM 合规审核工具 | 单元测试 |
| 2.6 | 更新 `tools/provider.go`：ToolSet 简化 | go build |

### Phase 3: Interrupt 确认

| 步骤 | 内容 | 验证 |
|------|------|------|
| 3.1 | 重写 `submit_reimb_tool.go`：内建 Interrupt | 单元测试 |
| 3.2 | 修改 `loop_manager.go`：启用 Checkpoint | go build |
| 3.3 | 实现 `makeGenResume()` 回调 | 集成测试 |
| 3.4 | 实现 `HandleApprove` 端点 | curl 测试 |
| 3.5 | 新增 SSE `interrupted` 事件类型 | 集成测试 |
| 3.6 | 前端确认 UI 对话框 | Playwright E2E |

### Phase 4: 清理与 Wire

| 步骤 | 内容 | 验证 |
|------|------|------|
| 4.1 | 删除 confirm_invoice/confirm_submit/compliance(RAG) 工具文件 | go build |
| 4.2 | 更新 `provider.go`：Wire ProviderSet | make wire |
| 4.3 | 更新 `config.yaml`：调整 Agent 配置项 | 配置校验 |

### Phase 5: 全面测试

| 步骤 | 内容 | 验证 |
|------|------|------|
| 5.1 | 单元测试：所有工具、Session 持久化 | go test ./... |
| 5.2 | 集成测试：完整报销流程（含 Interrupt 确认） | go test -tags=integration |
| 5.3 | E2E 测试：前端 UI 交互 | Playwright |

---

## 附录 A：v3 → v4 类型对照

| v3 类型/函数 | v4 处理 |
|-------------|--------|
| `selectPhaseAgent()` | 删除 |
| `classifyIntentByLLM()` | 删除 |
| `classifyByKeywords()` | 删除 |
| `ReimbursementState.CurrentPhase` | 删除 |
| `ReimbursementState.UserConfirmed` | 删除 |
| `ReimbursementState.FinalConfirmed` | 删除 |
| `ReimbursementState.Phase1Turns/2Turns/3Turns` | 删除 |
| `IntentResult` | 删除 |
| `WorkflowRoute` / `Route*` 常量 | 删除 |
| `GuardResult` | 删除 |
| `AgentInput` / `AgentOutput` (dto) | 删除（使用 Eino 内置） |
| `ConfirmInvoiceTool` / `ConfirmSubmitTool` | 删除 |
| `OCRTool` / `ComplianceTool` / etc 命名类型 | 保留，简化为直接类型 |

## 附录 B：配置变更

```yaml
# config.yaml — v4 新增/变更项
agent:
  # v3 保留
  session_ttl_minutes: 30
  max_history_turns: 20
  max_phase_turns: 10         # v4: 不再使用，可移除
  llm_max_retries: 3
  tool_timeout_seconds: 30
  
  # v4 新增
  interrupt_timeout_seconds: 300  # Interrupt 等待用户确认超时（秒）
  enable_checkpoint: true         # 启用 Checkpoint（中断恢复必需）

# v4 新增：合规审核配置
compliance:
  knowledge_base:
    embedding_model: "text-embedding-v3"  # 复用 v3 RAG 基础设施
  llm:
    model: "gpt-4o"             # 审核需要较强推理能力
    temperature: 0.0            # 零温度确保判定一致性
```
