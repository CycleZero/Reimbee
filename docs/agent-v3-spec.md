# Agent 层 v3.0 — 完整设计规范

> **目标读者**: 执行实现的 Agent  
> **原则**: Eino 已有的不自己写，Eino 没有的业务逻辑自己写  
> **前置**: 需先读完本文再动手

---

## 目录

1. [背景与动机](#1-背景与动机)
2. [v2.1 缺陷清单 (为什么要改)](#2-v21-缺陷清单)
3. [方案取舍记录](#3-方案取舍记录)
4. [最终架构](#4-最终架构)
5. [组件对照: 删除 vs 保留 vs 新增](#5-组件对照)
6. [核心数据结构](#6-核心数据结构)
7. [详细实现](#7-详细实现)
8. [流控机制](#8-流控机制)
9. [多 Session 管理](#9-多-session-管理)
10. [文件变更完整清单](#10-文件变更完整清单)
11. [实施步骤](#11-实施步骤)
12. [测试策略](#12-测试策略)

---

## 1. 背景与动机

### 1.1 项目目标

Reimbee 是一个企业报销智能助手。核心流程是员工通过对话完成报销申报：

```
员工: "我要报销差旅费"  →  Agent引导上传票据 → OCR识别 → 员工确认
      → 合规检查 → 预算检查 → 员工确认提交 → 生成PDF → 发送审批邮件
```

这个流程有三个阶段，阶段之间有**硬性约束**（没上传票据不能进入校验，没确认不能提交）。系统需要在"流程控制"和"LLM自主决策"之间取得平衡。

### 1.2 项目使用的框架

[cloudwego/eino](https://github.com/cloudwego/eino) v0.9.12，字节跳动的 Go AI Agent 框架。

关键能力：
- `adk.ChatModelAgent`: 内置 ReAct 循环（LLM↔Tool 往复）的 Agent
- `adk.TurnLoop`: 多轮对话运行时（Push/Preempt/Resume/Checkpoint）
- `adk.Runner`: Agent 生命周期管理
- `compose.Graph`: 确定性流程编排（节点+边）
- `compose.ToolsNode`: 工具执行器
- `compose.WithGenLocalState`: 图级状态管理

### 1.3 为什么需要 v3.0

v2.1 用 compose.Graph 手工构建了 ReAct 循环（`buildReActPhase()`），这在根本上与 Eino 的设计理念冲突——Eino 官方文档明确说：

> "flow/react is essentially using Graph's approach to simulate an Agent — using deterministic orchestration to carry dynamic decision-making. As Agent complexity grows, this mismatch creates systemic problems."
>
> "When the core problem is autonomous decision-making + runtime enhancement, the correct abstraction is ChatModelAgent + ChatModelAgentMiddleware."

v3.0 的目标：**用 Eino 原生组件替代所有手工实现，只保留 Eino 不提供的业务逻辑。**

---

## 2. v2.1 缺陷清单

### 2.1 数据流缺陷 (D1-D4)

| ID | 缺陷 | 根因 | 表现 |
|----|------|------|------|
| D1 | Phase 消息历史在 Guard 重试时丢失 | 子图 `phaseState` 每次重入被 `WithGenLocalState` 创建新实例 | LLM 丢失上一轮对话上下文 |
| D2 | 跨请求 State 持久化未生效 | `saveSessionState()` 通过 `ProcessState` 读取嵌套图 State，因 Eino 词法作用域读不到 | ReimbursementState 跨 HTTP 请求丢失 |
| D3 | 类型桥接复杂 | 父图 `*Message` ↔ 子图 `[]*Message`，依赖 adapter | 扩展受限 |
| D4 | tool_call/tool_result 事件未通过 SSE 发送 | 手工 ReAct 没有 event sender | 前端看不到工具执行进度 |

### 2.2 架构缺陷 (A1-A3)

| ID | 缺陷 | 说明 |
|----|------|------|
| A1 | 手工实现 ReAct 循环 (257 行) | Eino 已提供 `adk.ChatModelAgent`，我们重复造轮子 |
| A2 | Grid 层与业务耦合 | `graph/reimbursement.go` (303行) 混合了流程编排和 Eino 底层 API |
| A3 | 无法使用 Eino 生态 | ChatModelAgent 的 middleware（Summarization, PatchToolCalls 等）都无法接入 |

---

## 3. 方案取舍记录

### 3.1 选型: 谁来管流控

| 方案 | 描述 | 取舍 |
|------|------|------|
| Graph Guard (v2.1) | `AddBranch` + `Phase1Guard` 检查 | ❌ 需要手工管理子图 State |
| BeforeModel 过滤工具 | 同一个 Agent，middleware 中过滤工具列表 | ❌ LLM 可能"幻觉"已执行过被过滤的工具 |
| **PrepareAgent 选 Agent (v3.0)** | **每个 Phase 独立 Agent, PrepareAgent 决定用哪个** | ✅ **物理隔离, LLM 根本不知道其他阶段的工具** |

### 3.2 选型: 谁来管多轮对话

| 方案 | 描述 | 取舍 |
|------|------|------|
| 手动 StreamChat 循环 (v2.1) | 加载历史 → 执行 Graph → SSE → 保存 | ❌ 无 Preempt/Resume/Checkpoint |
| **adk.TurnLoop (v3.0)** | Eino 内置多轮运行时 | ✅ 原生 Preempt/Resume/Checkpoint |

### 3.3 选型: TurnLoop 与 Session 的关系

| 方案 | 描述 | 取舍 |
|------|------|------|
| 一个 TurnLoop 管所有 Session | items[] 里带 sessionID | ❌ 无法独立 Checkpoint, 无法独立销毁 |
| **一个 Session 一个 TurnLoop** | LoopManager 管理 map[sessionID]→TurnLoop | ✅ 独立生命周期, 独立 Checkpoint, 可超时清理 |

### 3.4 选型: ReimbursementState 如何跨请求持久化

| 方案 | 描述 | 取舍 |
|------|------|------|
| ProcessState → saveSessionState (v2.1) | Runner 层尝试读取嵌套图 State | ❌ Eino 词法作用域导致读不到 |
| **工具直接写 SessionStore** | 工具在 InvokableRun 内调 `store.SaveState()` | ✅ 工具已有 store 依赖 |

### 3.5 选型: 保留什么

| 保留 | 原因 |
|------|------|
| `MySQLSessionStore` + `RedisSessionCache` | Eino 明确: Session 是业务层概念，框架不提供 |
| `MySQLCheckpointStore` | Eino 提供 `CheckPointStore` 接口，不提供实现 |
| `tools/*.go` (全部9个) | 报销业务专属，Eino 不提供 |
| `sse.go` | SSE 格式是前后端协议，不是 Eino 的概念 |
| `dto.go` (ReimbursementState, InvoiceState) | 报销业务专属数据结构 |
| `prompt.go` (BuildSystemPrompt 等) | 业务提示词 |

---

## 4. 最终架构

### 4.1 组件关系

```
┌────────────────────────────────────────────────────────────┐
│                      HTTP 层                               │
│  service.go: HandleChat() → loopManager.GetOrCreate(id)    │
│                         → sessionLoop.turnLoop.Push(msg)    │
└──────────────────────┬─────────────────────────────────────┘
                       │
         ┌─────────────▼──────────────┐
         │     LoopManager            │
         │  map[sessionID]→SessionLoop│
         │  GetOrCreate / cleanup     │
         └─────────────┬──────────────┘
                       │
         ┌─────────────▼──────────────┐
         │  SessionLoop (每 session 1个)│
         │  ┌───────────────────────┐ │
         │  │ TurnLoop (Eino内置)   │ │
         │  │                       │ │
         │  │ GenInput ←── 加载历史+State
         │  │ PrepareAgent ←── 选Phase Agent
         │  │ OnAgentEvents ←── SSE输出
         │  │ Checkpoint ←── 自动gob
         │  └───────────────────────┘ │
         └─────────────┬──────────────┘
                       │
         ┌─────────────▼──────────────┐
         │  Phase Agents (Eino内置)   │
         │  ┌───────────────────────┐ │
         │  │ ChatModelAgent        │ │
         │  │  ReAct循环            │ │
         │  │  消息历史 State       │ │
         │  │  工具事件 SSE         │ │
         │  └───────────────────────┘ │
         │  Phase1: [OCR, Compliance] │
         │  Phase2: [Compliance, Budget]
         │  Phase3: [Create,Submit,PDF,Email,Progress]
         └─────────────────────────────┘
                       │
         ┌─────────────▼──────────────┐
         │  工具层 (我们的代码)       │
         │  ocr_tool.go               │
         │  compliance_tool.go        │
         │  budget_tool.go            │
         │  create_reimb_tool.go      │
         │  submit_reimb_tool.go      │
         │  pdf_tool.go               │
         │  email_tool.go             │
         │  progress_tool.go          │
         │  query_tool.go             │
         └─────────────┬──────────────┘
                       │
         ┌─────────────▼──────────────┐
         │  持久化 (我们的代码)       │
         │  SessionStore (MySQL+Redis)│
         │  CheckpointStore (MySQL)   │
         └────────────────────────────┘
```

### 4.2 一句话总结

**TurnLoop 管对话轮次，PrepareAgent 管阶段选择（物理隔离工具集），ChatModelAgent 管阶段内自主决策，SessionStore 管业务状态持久化。自己什么多余的都不写。**

### 4.3 流控保证

用户在任何时候都不可能跳过阶段，因为：

```
用户处于 Phase1 → PrepareAgent 返回 Phase1Agent → 工具集没有 submit_reimbursement
→ LLM 的 WithTools() 传入的 schema 中就没有 submit
→ LLM 不可能生成 submit_reimbursement 的 ToolCall
→ 物理上不可能
```

---

## 5. 组件对照

### 5.1 删除（被 Eino 替代）

| 文件 | 行数 | 替代 |
|------|------|------|
| `graph/react_phase.go` | 257 | `adk.ChatModelAgent` |
| `graph/reimbursement.go` | 303 | `PrepareAgent`选 Agent |
| `graph/root.go` | 248 | TurnLoop GenInput 中的意图分类 |
| `graph/provider.go` | 110 | Wire 绑定简化 |
| `graph/budget.go` | 81 | 独立 ChatModelAgent |
| `graph/progress.go` | 82 | 独立 ChatModelAgent |
| `graph/policy.go` | 86 | 独立 ChatModelAgent |
| `graph/modify.go` | 86 | 独立 ChatModelAgent |
| `phase/guard.go` + `phase/phase_config.go` | 185 | `selectPhaseAgent()` 函数 |
| **小计** | **~1440行** | |

### 5.2 修改（保留但调整）

| 文件 | 变更 |
|------|------|
| `agent/runner.go` | 简化为 LoopManager 的包装 |
| `agent/provider.go` | Wire 绑定更新 |
| `agent/service.go` | HandleChat 改为 loopManager.Push |
| `agent/config.go` | 清理 Graph 相关配置 |
| `graph/ (目录)` | 只保留测试文件（或完全移除） |

### 5.3 新增

| 文件 | 说明 | 行数 |
|------|------|------|
| `agent/loop_manager.go` | `LoopManager` — map[sessionID]→SessionLoop | ~80 |
| `agent/session_loop.go` | `SessionLoop` — TurnLoop 的 GenInput/PrepareAgent/OnAgentEvents | ~200 |
| `agent/phase_agents.go` | `initPhaseAgents()` — 创建 3+4 个 ChatModelAgent | ~120 |
| **小计** | | **~400行** |

### 5.4 保留不变

| 文件 | 原因 |
|------|------|
| `tools/*.go` (全部9个) | 报销业务工具 |
| `dto.go` | ReimbursementState 等 |
| `sse.go` | SSE 格式 |
| `prompt.go` | 提示词 |
| `llm.go` | ChatModel 工厂 |
| `checkpoint.go` | MySQL CheckpointStore |
| `infra/session*.go` | MySQL SessionStore |
| `infra/provider.go` | Wire (微调) |

---

## 6. 核心数据结构

```go
// ── ReimbursementState (已有, dto.go) ──
// 工具通过 ProcessState 或 SessionStore.SaveState 更新
// PrepareAgent 读取 CurrentPhase 决定用哪个 Phase Agent

// ── LoopManager ──
type LoopManager struct {
    mu              sync.Mutex
    loops           map[string]*SessionLoop
    store           infra.SessionStore      // 消息+State 持久化
    checkpointStore agent.CheckpointStore   // Eino Checkpoint 持久化
    phase1Agent     *adk.ChatModelAgent
    phase2Agent     *adk.ChatModelAgent
    phase3Agent     *adk.ChatModelAgent
    chatAgent       *adk.ChatModelAgent     // 通用对话
    // progressAgent, budgetAgent, policyAgent, modifyAgent...
    sseWriterFactory func(sessionID string) SSEWriter
    logger           *log.Logger
    config           *LoopConfig
}

// ── SessionLoop ──
type SessionLoop struct {
    turnLoop   *adk.TurnLoop[string, *schema.Message]
    cancel     context.CancelFunc
    lastActive time.Time
    sessionID  string
}

// ── LoopConfig ──
type LoopConfig struct {
    SessionTTL         time.Duration  // 超时空闲 Session 自动销毁
    MaxHistoryTurns    int            // 每次注入 LLM 的历史轮数
    CleanupInterval    time.Duration  // 清理检查间隔
}
```

---

## 7. 详细实现

### 7.1 LoopManager

```go
// agent/loop_manager.go

package agent

import (
    "context"
    "sync"
    "time"

    "github.com/CycleZero/Reimbee/infra"
    "github.com/CycleZero/Reimbee/log"
    "github.com/cloudwego/eino/adk"
    "go.uber.org/zap"
)

type LoopManager struct {
    mu                sync.Mutex
    loops             map[string]*SessionLoop
    store             infra.SessionStore
    checkpointStore   CheckpointStore

    // 预创建的 Agent 实例
    phase1Agent     *adk.ChatModelAgent
    phase2Agent     *adk.ChatModelAgent
    phase3Agent     *adk.ChatModelAgent
    chatAgent       *adk.ChatModelAgent
    progressAgent   *adk.ChatModelAgent
    budgetAgent     *adk.ChatModelAgent
    policyAgent     *adk.ChatModelAgent
    modifyAgent     *adk.ChatModelAgent

    sseWriterFactory func(sessionID string) SSEWriter
    logger           *log.Logger
    config           *LoopConfig
}

type LoopConfig struct {
    SessionTTL      time.Duration
    MaxHistoryTurns int
    CleanupInterval time.Duration
}

// NewLoopManager 创建并启动 LoopManager
// 创建所有 Phase Agent, 启动后台清理 goroutine
func NewLoopManager(ctx context.Context, deps LoopManagerDeps) *LoopManager {
    m := &LoopManager{
        loops:            make(map[string]*SessionLoop),
        store:            deps.Store,
        checkpointStore:  deps.Checkpoint,
        sseWriterFactory: deps.SSEWriterFactory,
        logger:           deps.Logger,
        config:           deps.Config,
    }
    m.initAgents(ctx, deps)
    go m.cleanupLoop()
    return m
}

// GetOrCreate 获取或创建 SessionLoop
func (m *LoopManager) GetOrCreate(sessionID string) *SessionLoop {
    m.mu.Lock()
    defer m.mu.Unlock()

    if sl, ok := m.loops[sessionID]; ok {
        sl.lastActive = time.Now()
        return sl
    }

    sl := m.createSessionLoop(sessionID)
    m.loops[sessionID] = sl
    m.logger.Info("创建新会话TurnLoop", zap.String("sessionID", sessionID),
        zap.Int("活跃会话数", len(m.loops)))
    return sl
}

// cleanupLoop 后台清理超时会话
func (m *LoopManager) cleanupLoop() {
    ticker := time.NewTicker(m.config.CleanupInterval)
    for range ticker.C {
        m.mu.Lock()
        for id, sl := range m.loops {
            if time.Since(sl.lastActive) > m.config.SessionTTL {
                m.logger.Info("清理超时会话", zap.String("sessionID", id))
                sl.turnLoop.Stop(adk.WithGracefulTimeout(5 * time.Second))
                sl.cancel()
                delete(m.loops, id)
            }
        }
        m.mu.Unlock()
    }
}

// Shutdown 优雅关闭所有会话
func (m *LoopManager) Shutdown() {
    m.mu.Lock()
    defer m.mu.Unlock()
    for id, sl := range m.loops {
        sl.turnLoop.Stop(adk.WithGracefulTimeout(10 * time.Second))
        sl.cancel()
        m.logger.Info("关闭会话", zap.String("sessionID", id))
    }
}
```

### 7.2 createSessionLoop

```go
// agent/session_loop.go (LoopManager 的方法)

func (m *LoopManager) createSessionLoop(sessionID string) *SessionLoop {
    ctx, cancel := context.WithCancel(context.Background())

    sl := &SessionLoop{
        sessionID:  sessionID,
        cancel:     cancel,
        lastActive: time.Now(),
    }

    cfg := adk.TurnLoopConfig[string, *schema.Message]{
        GenInput:      m.makeGenInput(sessionID),
        PrepareAgent:  m.makePrepareAgent(sessionID),
        OnAgentEvents: m.makeOnAgentEvents(sessionID),
        Store:         m.checkpointStore,
        CheckpointID:  sessionID,
    }

    sl.turnLoop = adk.NewTurnLoop(cfg)
    sl.turnLoop.Run(ctx)

    return sl
}
```

### 7.3 GenInput — 加载历史 + State + 保存用户消息

```go
func (m *LoopManager) makeGenInput(sessionID string) func(
    ctx context.Context,
    loop *adk.TurnLoop[string, *schema.Message],
    items []string,
) (*adk.GenInputResult[string, *schema.Message], error) {

    return func(ctx context.Context, _ *adk.TurnLoop[string, *schema.Message],
        items []string) (*adk.GenInputResult[string, *schema.Message], error) {

        // ── 1. 加载对话历史 ──
        history, err := m.store.GetHistory(ctx, sessionID, m.config.MaxHistoryTurns*2)
        if err != nil {
            m.logger.Warn("加载对话历史失败", zap.String("sessionID", sessionID), zap.Error(err))
            history = nil
        }

        // ── 2. 加载业务状态 (首次为空, 不需要提前创建) ──
        var rs ReimbursementState
        found, _ := m.store.GetState(ctx, sessionID, infra.StateKeyReimbursement, &rs)
        // 注入 context 供工具通过 ProcessState 访问
        if found {
            ctx = context.WithValue(ctx, StateContextKey{}, &rs)
        }

        // ── 3. 保存用户消息 ──
        // items 是 Push 传入的字符串数组, 通常本轮只有一个
        for _, item := range items {
            userMsg := schema.UserMessage(item)
            if err := m.store.SaveMessages(ctx, sessionID,
                []*schema.Message{userMsg}); err != nil {
                m.logger.Warn("保存用户消息失败", zap.Error(err))
            }
        }

        // ── 4. 构建消息列表 ──
        msgs := make([]*schema.Message, 0, len(history)+1)
        msgs = append(msgs, history...)
        for _, item := range items {
            msgs = append(msgs, schema.UserMessage(item))
        }

        m.logger.Debug("GenInput完成",
            zap.String("sessionID", sessionID),
            zap.Int("历史消息数", len(history)),
            zap.Int("本轮消息数", len(items)),
            zap.String("当前阶段", rs.CurrentPhase))

        return &adk.GenInputResult[string, *schema.Message]{
            Input:    &adk.AgentInput{Messages: msgs, EnableStreaming: true},
            Consumed: items,
        }, nil
    }
}
```

### 7.4 PrepareAgent — 意图分类 + 阶段选择（流控核心）

```go
func (m *LoopManager) makePrepareAgent(sessionID string) func(
    ctx context.Context,
    loop *adk.TurnLoop[string, *schema.Message],
    consumed []string,
) (adk.Agent, error) {

    return func(ctx context.Context, _ *adk.TurnLoop[string, *schema.Message],
        consumed []string) (adk.Agent, error) {

        // ── 1. 意图分类 ──
        route := classifyByKeywords(consumed[0])
        m.logger.Debug("意图分类", zap.String("sessionID", sessionID),
            zap.String("意图", route), zap.String("消息", consumed[0]))

        // ── 2. 简单意图 → 直接返回对应 Agent ──
        switch route {
        case "query_progress":
            return m.progressAgent, nil
        case "query_budget":
            return m.budgetAgent, nil
        case "policy_question":
            return m.policyAgent, nil
        case "modify_reimbursement":
            return m.modifyAgent, nil
        case "general_chat":
            return m.chatAgent, nil
        // case "new_reimbursement": → 继续, 需要根据阶段选 Agent
        }

        // ── 3. 报销流程: 根据 ReimbursementState 选 Phase Agent ──
        var rs ReimbursementState
        m.store.GetState(ctx, sessionID, infra.StateKeyReimbursement, &rs)

        agent := m.selectPhaseAgent(&rs)
        m.logger.Info("选择Phase Agent",
            zap.String("sessionID", sessionID),
            zap.String("当前阶段", rs.CurrentPhase),
            zap.String("Agent", agent.Name(ctx)))
        return agent, nil
    }
}

// selectPhaseAgent 根据 ReimbursementState 决定返回哪个 Phase Agent
// 这是流控的核心: 物理隔离工具集
func (m *LoopManager) selectPhaseAgent(rs *ReimbursementState) adk.Agent {
    switch {
    case rs.ReimbursementNo != "":
        // 已提交 → 后续查询走通用对话
        return m.chatAgent
    case rs.FinalConfirmed:
        // 用户已确认提交 → Phase 3: 执行
        return m.phase3Agent
    case rs.UserConfirmed:
        // 票据已确认 → Phase 2: 校验
        return m.phase2Agent
    default:
        // Phase 1: 信息收集
        return m.phase1Agent
    }
}
```

### 7.5 OnAgentEvents — AgentEvent → SSE 输出

```go
func (m *LoopManager) makeOnAgentEvents(sessionID string) func(
    ctx context.Context,
    tc *adk.TurnContext[string, *schema.Message],
    events *adk.AsyncIterator[*adk.AgentEvent],
) error {

    return func(ctx context.Context, tc *adk.TurnContext[string, *schema.Message],
        events *adk.AsyncIterator[*adk.AgentEvent]) error {

        sseWriter := m.sseWriterFactory(sessionID)

        // 发送 thinking 事件
        _ = sseWriter.WriteEvent(NewThinkingEvent("正在处理..."))
        _ = sseWriter.Flush()

        var fullContent string

        for {
            // ── Preempt/Stop 检测 ──
            select {
            case <-tc.Preempted:
                m.logger.Debug("当前Turn被Preempt", zap.String("sessionID", sessionID))
                return nil
            case <-tc.Stopped:
                m.logger.Debug("TurnLoop被Stop", zap.String("sessionID", sessionID))
                return nil
            default:
            }

            event, ok := events.Next()
            if !ok {
                break
            }

            if event.Err != nil {
                _ = sseWriter.WriteEvent(NewErrorEvent(
                    event.Err.Error(), false, "agent_error"))
                _ = sseWriter.Flush()
                return event.Err
            }

            if event.Output == nil || event.Output.MessageOutput == nil {
                continue
            }

            mv := event.Output.MessageOutput

            switch mv.Role {
            case schema.Assistant:
                // LLM 文本输出
                if mv.IsStreaming {
                    // 流式: chunk by chunk 推送
                    for {
                        chunk, err := mv.MessageStream.Recv()
                        if err != nil { break }
                        if chunk.Content != "" {
                            fullContent += chunk.Content
                            _ = sseWriter.WriteEvent(NewMessageEvent(chunk.Content, true))
                            _ = sseWriter.Flush()
                        }
                    }
                } else if mv.Message != nil {
                    fullContent = mv.Message.Content
                    _ = sseWriter.WriteEvent(NewMessageEvent(mv.Message.Content, false))
                    _ = sseWriter.Flush()
                }

            case schema.Tool:
                // 工具调用结果
                _ = sseWriter.WriteEvent(NewToolResultEvent(
                    mv.ToolName, mv.Message.Content))
                _ = sseWriter.Flush()
            }
        }

        // ── 持久化 assistant 回复 ──
        if fullContent != "" {
            assistantMsg := schema.AssistantMessage(fullContent, nil)
            if err := m.store.SaveMessages(ctx, sessionID,
                []*schema.Message{assistantMsg}); err != nil {
                m.logger.Warn("保存assistant消息失败", zap.Error(err))
            }
        }

        // ── 持久化业务状态 ──
        // 工具在执行时已通过 store.SaveState 写入, 这里不需要额外操作

        // ── done 事件 ──
        _ = sseWriter.WriteEvent(NewDoneEvent())
        _ = sseWriter.Flush()

        m.logger.Debug("Turn事件消费完成",
            zap.String("sessionID", sessionID),
            zap.Int("回复长度", len(fullContent)))
        return nil
    }
}
```

### 7.6 Phase Agent 初始化

```go
// agent/phase_agents.go

type LoopManagerDeps struct {
    Store            infra.SessionStore
    Checkpoint       CheckpointStore
    ChatModel        model.ToolCallingChatModel
    ToolSet          *tools.ToolSet
    Logger           *log.Logger
    Config           *LoopConfig
    SSEWriterFactory func(sessionID string) SSEWriter
}

func (m *LoopManager) initAgents(ctx context.Context, deps LoopManagerDeps) {
    m.phase1Agent = mustNewAgent(ctx, deps, "phase1_collect", "收集票据信息",
        BuildSystemPrompt("phase1_collect", nil),
        []tool.BaseTool{deps.ToolSet.OCR, deps.ToolSet.Compliance})

    m.phase2Agent = mustNewAgent(ctx, deps, "phase2_validate", "合规与预算校验",
        BuildSystemPrompt("phase2_validate", nil),
        []tool.BaseTool{deps.ToolSet.Compliance, deps.ToolSet.Budget})

    m.phase3Agent = mustNewAgent(ctx, deps, "phase3_execute", "创建并提交报销单",
        BuildSystemPrompt("phase3_execute", nil),
        []tool.BaseTool{
            deps.ToolSet.CreateReimb, deps.ToolSet.SubmitReimb,
            deps.ToolSet.PDF, deps.ToolSet.Email, deps.ToolSet.Progress,
        })

    m.chatAgent = mustNewAgent(ctx, deps, "general_chat", "通用对话",
        BuildGeneralChatPrompt(), nil)

    // 简单子流程 (不需要工具)
    m.progressAgent = mustNewAgent(ctx, deps, "query_progress", "查询进度",
        progressSystemPrompt,
        []tool.BaseTool{deps.ToolSet.Progress, deps.ToolSet.QueryRecords})
    m.budgetAgent = mustNewAgent(ctx, deps, "query_budget", "查询预算",
        budgetSystemPrompt,
        []tool.BaseTool{deps.ToolSet.Budget})
    m.policyAgent = mustNewAgent(ctx, deps, "policy_question", "政策咨询",
        BuildGeneralChatPrompt(), nil)
    m.modifyAgent = mustNewAgent(ctx, deps, "modify_reimbursement", "修改报销",
        modifySystemPrompt,
        []tool.BaseTool{deps.ToolSet.Progress, deps.ToolSet.QueryRecords})

    deps.Logger.Info("全部Agent初始化完成",
        zap.Int("Phase Agent数", 3),
        zap.Int("子流程Agent数", 5))
}

func mustNewAgent(ctx context.Context, deps LoopManagerDeps,
    name, desc, instruction string, tools []tool.BaseTool) *adk.ChatModelAgent {

    agent, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
        Name:        name,
        Description: desc,
        Instruction: instruction,
        Model:       deps.ChatModel,
        ToolsConfig: adk.ToolsConfig{
            ToolsNodeConfig: compose.ToolsNodeConfig{Tools: tools},
        },
        MaxIterations: 10,
    })
    if err != nil {
        deps.Logger.Error("创建Agent失败", zap.String("name", name), zap.Error(err))
        panic("创建Agent失败: " + name + ": " + err.Error())
    }
    deps.Logger.Debug("Agent创建成功", zap.String("name", name), zap.Int("工具数", len(tools)))
    return agent
}
```

### 7.7 Service 层适配

```go
// agent/service.go — 修改 HandleChat

type AgentService struct {
    loopManager *LoopManager
    logger      *log.Logger
}

func (s *AgentService) HandleChat(c *gin.Context) {
    sessionID := c.Query("session_id")
    message   := c.Query("message")

    if sessionID == "" || message == "" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "缺少参数"})
        return
    }

    // 获取或创建 Session 专属的 TurnLoop
    sl := s.loopManager.GetOrCreate(sessionID)

    // Push 消息 (带 Preempt: 用户连发消息时打断当前执行)
    accepted, ack := sl.turnLoop.Push(message,
        adk.WithPreempt[string, *schema.Message](adk.AnySafePoint),
    )
    if !accepted {
        c.JSON(http.StatusServiceUnavailable, gin.H{"error": "服务繁忙"})
        return
    }
    <-ack // 等待 Preempt 确认

    // SSE 流在 OnAgentEvents 中通过 sseWriter 输出
    // 前端通过 EventSource 接收
    c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
```

### 7.8 工具适配 — 直接写 SessionStore

```go
// 示例: OCR 工具 — 在 InvokableRun 中更新 ReimbursementState

func (t *OCRTool) InvokableRun(ctx context.Context, argsJSON string) (string, error) {
    result := doOCR(argsJSON)

    // 通过 ProcessState 或直接写 SessionStore 更新业务状态
    // 方式 A: 从 context 读取 sessionID → store.SaveState
    sessionID := getSessionIDFromCtx(ctx)
    var rs agent.ReimbursementState
    t.store.GetState(ctx, sessionID, "reimbursement", &rs)

    rs.Invoices = append(rs.Invoices, InvoiceState{
        Amount:   result.Amount,
        Category: result.Category,
    })
    rs.TotalAmount += result.Amount

    t.store.SaveState(ctx, sessionID, "reimbursement", &rs)

    return result.ToJSON(), nil
}
```

---

## 8. 流控机制

### 8.1 流程状态机

```
Phase1 (信息收集)
  │  工具: OCR, Compliance
  │  条件: UserConfirmed = true → Phase2
  │
  ▼
Phase2 (校验确认)
  │  工具: Compliance, Budget
  │  条件: FinalConfirmed = true → Phase3
  │
  ▼
Phase3 (执行提交)
  │  工具: CreateReimb, SubmitReimb, PDF, Email, Progress
  │  条件: ReimbursementNo != "" → 回到 chatAgent
  │
  ▼
ChatAgent (通用对话)
    工具: 无 (或 Progress/Query)
    "进度到哪了?" / "再帮我报销一笔"
```

### 8.2 阶段切换由 PrepareAgent 保证

```go
selectPhaseAgent(rs):
  rs.UserConfirmed == false   → Phase1Agent  (物理隔离 Phase2/Phase3 工具)
  rs.FinalConfirmed == false  → Phase2Agent  (物理隔离 Phase3 工具)
  rs.ReimbursementNo == ""    → Phase3Agent  (物理隔离查询工具)
  rs.ReimbursementNo != ""    → chatAgent     (通用对话)
```

### 8.3 防死循环

每个 ChatModelAgent 配置 `MaxIterations: 10`，超过 10 轮 ReAct 循环自动 `ErrExceedMaxIterations` 退出。

---

## 9. 多 Session 管理

### 9.1 生命周期

```
创建: LoopManager.GetOrCreate(sessionID) → createSessionLoop → TurnLoop.Run()
活跃: HTTP 请求 → sl.lastActive = now()
清理: cleanupLoop goroutine → 超过 TTL → TurnLoop.Stop → delete
关闭: LoopManager.Shutdown() → 遍历所有 → Stop
```

### 9.2 Checkpoint 隔离

每个 Session 的 CheckpointID 就是 sessionID：

```
TurnLoop("abc123"):
  CheckpointID: "abc123"
  CheckpointStore: MySQL, key="abc123"

TurnLoop("xyz789"):
  CheckpointID: "xyz789"
  CheckpointStore: MySQL, key="xyz789"

→ 完全隔离
```

### 9.3 Config 配置

```yaml
# config.yaml
agent:
  session_ttl_minutes: 30      # Session 超时 (LoopManager 清理)
  max_history_turns: 20         # 每次注入 LLM 的历史轮数
  cleanup_interval_seconds: 60  # 清理检查间隔
```

---

## 10. 文件变更完整清单

### 删除

```
internal/domain/agent/graph/react_phase.go
internal/domain/agent/graph/reimbursement.go
internal/domain/agent/graph/root.go
internal/domain/agent/graph/provider.go
internal/domain/agent/graph/budget.go
internal/domain/agent/graph/progress.go
internal/domain/agent/graph/policy.go
internal/domain/agent/graph/modify.go
internal/domain/agent/phase/guard.go
internal/domain/agent/phase/phase_config.go
```

### 新增

```
internal/domain/agent/loop_manager.go      — LoopManager + cleanupLoop
internal/domain/agent/session_loop.go      — createSessionLoop + GenInput + PrepareAgent + OnAgentEvents
internal/domain/agent/phase_agents.go      — initAgents + mustNewAgent
```

### 修改

```
internal/domain/agent/runner.go           — 删除 StreamChat 等, 保留 Wire 兼容
internal/domain/agent/service.go          — HandleChat 改为 loopManager.Push
internal/domain/agent/provider.go         — Wire 绑定更新
internal/domain/agent/config.go           — 新增 LoopConfig
internal/domain/agent/tools/provider.go   — 工具构造函数增加 SessionStore 参数
wire.go                                   — 移除 graph.ProviderSet
```

### 保留（不变）

```
internal/domain/agent/dto.go
internal/domain/agent/sse.go
internal/domain/agent/prompt.go
internal/domain/agent/llm.go
internal/domain/agent/checkpoint.go
internal/domain/agent/tools/ocr_tool.go
internal/domain/agent/tools/compliance_tool.go
internal/domain/agent/tools/budget_tool.go
internal/domain/agent/tools/pdf_tool.go
internal/domain/agent/tools/email_tool.go
internal/domain/agent/tools/progress_tool.go
internal/domain/agent/tools/query_tool.go
internal/domain/agent/tools/create_reimb_tool.go
internal/domain/agent/tools/submit_reimb_tool.go
infra/session.go
infra/session_mysql.go
infra/session_redis.go
infra/session_state.go
infra/provider.go
```

---

## 11. 实施步骤

### Phase 1: 创建新文件 (不删旧文件)

1. `agent/loop_manager.go` — 参考 §7.1
2. `agent/session_loop.go` — 参考 §7.2-7.5
3. `agent/phase_agents.go` — 参考 §7.6

### Phase 2: 修改现有文件

1. `agent/config.go` — 新增 `LoopConfig`
2. `agent/service.go` — `HandleChat` 改为 `loopManager.Push`
3. `agent/provider.go` — Wire 绑定更新
4. `agent/tools/provider.go` — 工具增加 SessionStore 依赖
5. `agent/tools/*.go` — 工具内部调用 `store.SaveState()` 更新 ReimbursementState
6. `wire.go` — 移除图相关绑定

### Phase 3: 编译 + 测试

1. `go build ./...` → 修复编译错误
2. `make wire` → 重新生成
3. `go test ./internal/domain/agent/... -race`
4. 修复测试用例（mockSessionStore 等）

### Phase 4: 删除旧文件

确认 Phase 3 全部通过后删除 §10 中列出的文件。

---

## 12. 测试策略

### 12.1 单元测试

- `LoopManager.GetOrCreate()` — 创建/复用/超时清理
- `selectPhaseAgent()` — 各阶段正确选择 Agent
- `makeGenInput()` — 历史加载 + State 恢复
- `makePrepareAgent()` — 意图分类 + 阶段路由

### 12.2 集成测试

- 完整 4 轮对话: Phase1 → Phase2 → Phase3 → Chat
- Preempt 打断当前执行
- 多 Session 并发安全

### 12.3 回归测试

- `go test ./internal/domain/... -race` 全部通过
- 旧有 Graph 测试在删除旧文件前仍需通过

