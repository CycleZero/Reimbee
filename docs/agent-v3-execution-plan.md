# Agent 层 v3.0 — 执行计划

> **日期**: 2026-07-07  
> **前置阅读**: [agent-v3-spec.md](./agent-v3-spec.md)（必读）  
> **原则**: Eino 已有的不自己写，Eino 没有的业务逻辑自己写  
> **状态**: 就绪，待实施

---

## 目录

1. [spec 修正清单](#1-spec-修正清单)
2. [总览：变更地图](#2-总览变更地图)
3. [Phase 1: 新建文件（保持旧代码可用）](#3-phase-1-新建文件)
4. [Phase 2: 修改现有文件](#4-phase-2-修改现有文件)
5. [Phase 3: 编译 + Wire + 基础测试](#5-phase-3-编译--wire--基础测试)
6. [Phase 4: 删除旧文件](#6-phase-4-删除旧文件)
7. [Phase 5: 全面测试](#7-phase-5-全面测试)
8. [回滚策略](#8-回滚策略)
9. [附录: Eino API 验证清单](#9-附录-eino-api-验证清单)

---

## 1. spec 修正清单

基于 Eino v0.9.12 源码阅读和官方文档核实，`agent-v3-spec.md` 需修正以下问题：

### 修正 1: OnAgentEvents 中的 CancelError 处理 ⚠️ 重要

**问题**: spec §7.5 在 `event.Err != nil` 时直接 `return event.Err`。但 Eino TurnLoop 源码明确：框架通过 proxy iterator **自动捕获** `*CancelError`，回调不应传播它。

**修正**: 区分 CancelError 和真实错误：
```go
if event.Err != nil {
    // 框架已自动捕获 CancelError（Preempt/Stop），回调不应传播
    if errors.Is(event.Err, new(adk.CancelError)) {
        // 静默返回，框架会处理后续流程（Preempt→新Turn，Stop→退出）
        return nil
    }
    // 真实错误才传播并在 SSE 中展示
    _ = sseWriter.WriteEvent(NewErrorEvent(event.Err.Error(), false, "agent_error"))
    _ = sseWriter.Flush()
    return event.Err
}
```

### 修正 2: SSE Writer 生命周期问题 ⚠️ 重要

**问题**: spec §7.7 的 `HandleChat` 通过 `sseWriterFactory(sessionID)` 在 OnAgentEvents 回调中创建 SSE Writer，但 SSE Writer 需要 `gin.Context`（HTTP 请求级别的对象），回调运行在 TurnLoop 后台 goroutine 中。

**分析**: 
- Gin SSE Writer 必须绑定到当前 HTTP 请求的 `gin.Context`，请求结束后失效
- OnAgentEvents 运行在 TurnLoop 的 goroutine 中，与 HTTP handler goroutine 不同
- `sseWriterFactory` 按 sessionID 创建无法获取当前请求的 `gin.Context`

**修正方案**: 采用"每请求注册 + 通道通知完成"模式：

```
HTTP Handler:
  1. 创建 GinSSEWriter（绑定当前请求）
  2. 创建 doneCh（信号通道）
  3. 调用 loopManager.PushMessage(sessionID, message, sseWriter, doneCh)
  4. 阻塞等待 <-doneCh
  5. HTTP handler 返回，SSE 连接关闭

LoopManager.PushMessage:
  1. 将 (sseWriter, doneCh) 存入 SessionLoop 的活跃请求映射
  2. 调用 turnLoop.Push(message)

OnAgentEvents:
  1. 从 SessionLoop 取出当前活跃请求的 sseWriter
  2. 事件写入 sseWriter
  3. 完成后关闭 doneCh，清除活跃请求映射
```

### 修正 3: GenInput 的 Remaining 语义

**问题**: spec §7.3 中 `Consumed: items` 消费全部 items，未设置 Remaining。TurnLoop 源码中 `planTurn()` 会将 Remaining 通过 `buffer.PushFront(plan.remaining)` 放回队列。

**结论**: 对于我们的场景（每次 Push 一条消息），`Consumed: items` + `Remaining: nil` 是正确的。但如果未来一次 Push 多条消息，需要注意 Remaining。

**无需代码修改**，但需在注释中注明语义。

### 修正 4: ChatModelAgentConfig 字段名核实

**核实结果**: spec 中所有字段名均与源码一致。`MaxIterations` 是顶层字段，不在 `ToolsConfig` 内。默认值为 20。

**无需修改**。

### 修正 5: Checkpoint 可选性

**问题**: spec §7.2 在 `TurnLoopConfig` 中始终设置了 `Store` 和 `CheckpointID`。但源码证实：这两个字段为可选，不设置则不启用 Checkpoint。

**建议**: v3.0 首个版本**不启用** Checkpoint（简化实施，减少变量）。后续版本按需启用。Checkpoint 用于审批中断/恢复场景，我们当前的流程在一个 HTTP 请求内完成，不需要跨请求恢复 ReAct 循环状态。

**修改 `createSessionLoop`**: 移除 `Store` 和 `CheckpointID`，在注释中标注为"可选，后续版本启用"。

### 修正 6: TurnLoop 类型参数确认

**核实**: TurnLoop 定义为 `TurnLoop[T any, M MessageType]`，其中 `MessageType` 是 `*schema.Message | *schema.AgenticMessage`。

**确认**: `T = string`, `M = *schema.Message` 正确。`string` 天然支持 `encoding/gob`（如果启用 Checkpoint）。

### 修正 7: 意图分类的 LLM 路径

**问题**: spec §7.4 的 `makePrepareAgent` 中只使用了关键词分类 `classifyByKeywords`，删除了 v2.1 的 ChatModel 优先分类。

**分析**: 关键词分类是足够可靠的降级方案，且减少了不必要的 LLM 调用延迟。**保留 spec 当前方案**。

---

## 2. 总览：变更地图

```
新文件 (3):
  internal/domain/agent/loop_manager.go     — LoopManager + 会话生命周期管理
  internal/domain/agent/session_loop.go     — SessionLoop + TurnLoop 三回调
  internal/domain/agent/phase_agents.go     — 8 个 ChatModelAgent 的初始化

修改文件 (7):
  internal/domain/agent/config.go           — 新增 LoopConfig
  internal/domain/agent/service.go          — HandleChat 改为 loopManager + Push 模式
  internal/domain/agent/provider.go         — Wire 绑定更新
  internal/domain/agent/runner.go           — 删除 Graph 执行逻辑，改为 LoopManager 包装
  internal/domain/agent/tools/provider.go   — 工具构造函数增加 SessionStore 参数
  wire.go                                   — 移除 graph.ProviderSet，新增 agent 绑定
  infra/provider.go                         — (按需微调)

删除文件 (Phase 4, 编译通过后):
  internal/domain/agent/graph/react_phase.go, reimbursement.go, root.go, provider.go
  internal/domain/agent/graph/budget.go, progress.go, policy.go, modify.go
  internal/domain/agent/phase/guard.go, phase_config.go
  internal/domain/agent/graph/*_test.go     — 对应的测试文件

保留不变 (~20 文件):
  tools/*.go (9个工具), dto.go, sse.go, prompt.go, llm.go, checkpoint.go
  infra/session*.go (4个), infra/provider.go
```

---

## 3. Phase 1: 新建文件

> **目标**: 创建三个新文件，不删除、不修改任何旧文件。`go build ./...` 仍应通过。

### 步骤 1.1: `agent/loop_manager.go`

**职责**: Session 生命周期管理。创建/销毁 TurnLoop，超时清理。

**关键实现点**:
- `LoopManager` 结构体：持有 `map[string]*SessionLoop`、`SessionStore`、预创建的 8 个 Agent
- `NewLoopManager(ctx, deps)`：调用 `initAgents` 创建所有 Agent，启动后台 `cleanupLoop` goroutine
- `GetOrCreate(sessionID)`: 获取或创建 SessionLoop，原子操作
- `PushMessage(sessionID, message, sseWriter, doneCh)`: 向 TurnLoop 推送消息，注册当前请求的 SSE writer
- `cleanupLoop()`: 定时检查超时会话，调用 `TurnLoop.Stop(WithGracefulTimeout)` 优雅关闭
- `Shutdown()`: 遍历所有会话，优雅关闭全部 TurnLoop

**LoopManagerDeps 结构体**:
```go
type LoopManagerDeps struct {
    Store            infra.SessionStore
    ChatModel        model.ToolCallingChatModel
    ToolSet          *tools.ToolSet
    Logger           *log.Logger
    Config           *LoopConfig
}
```

### 步骤 1.2: `agent/session_loop.go`

**职责**: 单个 Session 的 TurnLoop 生命周期。包含三个核心回调的闭包生成方法。

**关键实现点**:
- `SessionLoop` 结构体：持有 `*adk.TurnLoop[string, *schema.Message]`、`cancel`、`lastActive`、活跃请求的 SSE writer 映射
- `createSessionLoop(sessionID)`: 创建 `TurnLoopConfig`，构建三个回调，调用 `NewTurnLoop` + `Run`
- `makeGenInput(sessionID)`: 从 SessionStore 加载历史 + 状态，保存用户消息，构建消息列表
- `makePrepareAgent(sessionID)`: 意图分类（关键词）→ 简单意图直接返回对应 Agent → 报销流程调用 `selectPhaseAgent`
- `makeOnAgentEvents(sessionID)`: 消费 AgentEvent 流 → SSE 推送 → 持久化 assistant 消息
- `selectPhaseAgent(rs)`: 根据 ReimbursementState 字段返回对应 Phase Agent
- SSE writer 管理：`registerWriter` / `unregisterWriter` / `getWriter`

**注意**: `TurnLoopConfig` 的 Item 类型 `T = string`，消息类型 `M = *schema.Message`。不设置 `Store` 和 `CheckpointID`（首个版本不启用 Checkpoint）。

### 步骤 1.3: `agent/phase_agents.go`

**职责**: 创建所有 8 个 ChatModelAgent（3 个 Phase + 5 个子流程）。

**关键实现点**:
- `initAgents(ctx, deps)`: 初始化全部 Agent
- `mustNewAgent(ctx, deps, name, desc, instruction, tools)`: 统一工厂方法
- 每个 Agent 配置 `MaxIterations: 10`（防死循环）
- 工具通过 `ToolSet` 的 `GetPhaseNBaseTools()` 获取，确保物理隔离

**Phase Agent 工具分配**:
- Phase1Agent: `[OCR, Compliance]` — 信息收集
- Phase2Agent: `[Compliance, Budget]` — 校验确认
- Phase3Agent: `[CreateReimb, SubmitReimb, PDF, Email, Progress]` — 执行提交
- ChatAgent: 无工具 — 通用对话
- ProgressAgent: `[Progress, QueryRecords]` — 进度查询
- BudgetAgent: `[Budget]` — 预算查询
- PolicyAgent: 无工具 — 政策咨询
- ModifyAgent: `[Progress, QueryRecords]` — 修改报销

**意图路由表** (在 `makePrepareAgent` 中使用):
```
"query_progress"       → ProgressAgent
"query_budget"         → BudgetAgent
"policy_question"      → PolicyAgent
"modify_reimbursement" → ModifyAgent
"general_chat"         → ChatAgent
"new_reimbursement"    → selectPhaseAgent(rs) → Phase1Agent / Phase2Agent / Phase3Agent / ChatAgent
```

---

## 4. Phase 2: 修改现有文件

### 步骤 2.1: `agent/config.go`

- 新增 `LoopConfig` 结构体:
  ```go
  type LoopConfig struct {
      SessionTTL      time.Duration  // 默认 30min
      MaxHistoryTurns int            // 默认 20
      CleanupInterval time.Duration  // 默认 60s
  }
  ```
- `AgentConfig` 保持不变（旧配置暂时保留，Phase 4 清理）
- 新增 `LoadLoopConfig(vc)` 或从 `AgentConfig` 中读取对应的值

### 步骤 2.2: `agent/service.go`

- `AgentService` 将 `*AgentRunner` 替换为 `*LoopManager`
- `HandleChat` 重写：创建 SSE writer → 注册请求 → Push 消息 → 等待完成
  ```go
  func (s *AgentService) HandleChat(c *gin.Context) {
      // 1. 解析参数
      // 2. 提取 JWT claims
      // 3. 创建 GinSSEWriter
      // 4. 创建 doneCh
      // 5. 调用 loopManager.PushMessage(sessionID, message, sseWriter, doneCh)
      // 6. 阻塞 <-doneCh
  }
  ```
- 保留 `BuildAgentInput`、`getStringFromContext`、`getUintFromContext` 等辅助函数

### 步骤 2.3: `agent/provider.go`

- 移除 `NewAgentRunner`（旧 Runner）
- 新增 `NewLoopManager`
- 新增 `LoadLoopConfig`
- 保留 `LoadAgentConfig`（供旧代码引用，Phase 4 清理）
- 保留 `MustNewChatModel`、`tools.ProviderSet`、`NewMySQLCheckpointStore`

### 步骤 2.4: `agent/runner.go`

- 简化：删除 `StreamChat`、`invokeFallback`、`loadSessionState`、`saveSessionState`、`truncateStr`
- 保留 `StateContextKey{}`（供 GenInput 注入 State 到 context）
- 保留 `BuildAgentInput`（供 service.go 使用）
- 或者：将 `BuildAgentInput` 移到 `service.go`，删除 `runner.go`（当 `AgentRunner` 不再被引用后）

### 步骤 2.5: `agent/tools/provider.go`

- 工具构造函数增加 `infra.SessionStore` 参数，使工具能直接调用 `store.SaveState()` 更新 `ReimbursementState`
- `NewToolSet` 增加 `store` 参数
- 各工具类型（`OCRTool` 等）的 `New*Tool` 函数增加 `store` 参数

### 步骤 2.6: `agent/tools/*.go`（按需修改）

- 核心修改：在 `InvokableRun` 中，工具执行完成后调用 `store.SaveState()` 更新 `ReimbursementState`
- 优先级：先改 Phase 1/2 工具（OCR, Compliance, Budget），Phase 3 工具稍后
- Phase 3 工具（CreateReimb, SubmitReimb）已经调用 biz 层，会写入 DB，不需要额外 SaveState
- 具体修改：
  - **OCR Tool**: 识别完成后 `store.SaveState("reimbursement")` 更新 `Invoices` 列表
  - **Compliance Tool**: 检查完成后 `store.SaveState("reimbursement")` 更新 `ComplianceResult`
  - **Budget Tool**: 检查完成后 `store.SaveState("reimbursement")` 更新 `BudgetResult`
  - 每个工具需要从 context 获取 `sessionID`（通过 `StateContextKey{}` 或显式注入）

### 步骤 2.7: `wire.go`

- 移除 `agentgraph.ProviderSet` 的导入和绑定
- 移除 `wire.Bind(new(compose.Runnable[...]), new(*agentgraph.RootGraphRunnable))`
- `infra.ProviderSet` 和 `internal.ProviderSet` 保持

### 步骤 2.8: `internal/domain/provider.go`（按需）

- 确认 `agent.ProviderSet` 已包含所有新组件的 Wire 绑定

---

## 5. Phase 3: 编译 + Wire + 基础测试

### 步骤 3.1: 编译验证

```bash
go build ./...
```
- 修复所有编译错误（类型不匹配、未定义引用等）
- 特别关注：`adk.ChatModelAgent` 返回类型、`TurnLoopConfig` 泛型参数、SSE writer 接口

### 步骤 3.2: Wire 重新生成

```bash
make wire
```
- 验证所有依赖可解析
- 确认不再有对 `graph.ProviderSet` 或 `agentgraph` 的引用
- 确认 `LoopManager` 正确注入

### 步骤 3.3: 基础单元测试

```bash
go test ./internal/domain/agent/... -v -count=1
```
- 编写 `loop_manager_test.go`: 测试 `GetOrCreate` 创建/复用
- 编写 `session_loop_test.go`: 测试 `selectPhaseAgent` 四个分支
- 编写 `phase_agents_test.go`: 测试所有 Agent 可成功创建
- Mock: `SessionStore`、`ChatModel`（使用现有 `testutil/mock_*.go`）

### 步骤 3.4: 修复旧测试

- 删除对 `graph/` 和 `phase/` 包的测试引用
- 更新 `agent/provider_test.go`（如存在）
- 更新 `agent/e2e_test.go`（如存在）

---

## 6. Phase 4: 删除旧文件

> **前置条件**: Phase 3 全部通过（`go build` + `make wire` + `go test` 全绿）

```bash
# 删除 Graph 层（被 ChatModelAgent + PrepareAgent 替代）
rm internal/domain/agent/graph/react_phase.go
rm internal/domain/agent/graph/reimbursement.go
rm internal/domain/agent/graph/root.go
rm internal/domain/agent/graph/provider.go
rm internal/domain/agent/graph/budget.go
rm internal/domain/agent/graph/progress.go
rm internal/domain/agent/graph/policy.go
rm internal/domain/agent/graph/modify.go

# 删除 Phase 守卫（被 selectPhaseAgent 替代）
rm internal/domain/agent/phase/guard.go
rm internal/domain/agent/phase/phase_config.go

# 删除对应测试文件
rm internal/domain/agent/graph/*_test.go
rm internal/domain/agent/phase/guard_test.go
```

删除后立即验证：
```bash
go build ./... && make wire && go test ./internal/domain/... -race
```

---

## 7. Phase 5: 全面测试

### 7.1 单元测试

| 测试目标 | 测试内容 | 预期 |
|---------|---------|------|
| `LoopManager.GetOrCreate` | 创建 + 复用 + 并发安全 | 相同 ID 返回同一实例 |
| `LoopManager.cleanupLoop` | 超时会话被清理 | TTL 过期后 loop 被 Stop |
| `selectPhaseAgent` | 4 种状态组合 | 正确返回对应 Agent |
| `makeGenInput` | 首次请求（无历史）+ 后续请求（有历史） | 消息列表正确构建 |
| `makePrepareAgent` | 6 种意图分类 | 正确路由到对应 Agent |
| `OnAgentEvents` | SSE 事件消费 + CancelError 处理 | SSE 正确推送，Preempt 正确处理 |
| `mustNewAgent` | 有工具 + 无工具 | ChatModelAgent 正确创建 |

### 7.2 集成测试

| 场景 | 步骤 | 验证点 |
|------|------|--------|
| Phase1 → Phase2 → Phase3 完整流程 | 4 轮对话 | 阶段正确切换，工具正确调用 |
| Preempt 打断 | Push 消息时 Preempt | 当前输出中断，新消息开始处理 |
| 多 Session 并发 | 10 个并发会话 | 无竞态，各自独立 |
| 工具更新 ReimbursementState | OCR 识别后查状态 | State 正确持久化和恢复 |
| 意图分类 | 6 种意图各 3 种表达 | 正确分类 |

### 7.3 回归测试

```bash
# 全部领域模块
go test ./internal/domain/... -race -count=1

# 基础设施层
go test ./infra/... -race -count=1

# 编译验证
go build ./...
make wire
```

---

## 8. 回滚策略

如果 Phase 3 编译失败超过 1 小时仍无法解决，或 Phase 5 测试发现根本性架构问题：

1. **回滚到 v2.1**: `git checkout HEAD~1`（假设实施前提交了当前状态）
2. **保留新文件**: 不删除新增的三个文件，供后续分析
3. **恢复 wire.go**: 还原 `graph.ProviderSet` 和 `agentgraph` 的引用
4. **分析失败原因**: 记录到 `docs/agent-v3-postmortem.md`

---

## 9. 附录: Eino API 验证清单

| API | spec 中使用 | 源码确认 | 状态 |
|-----|-----------|---------|------|
| `adk.NewChatModelAgent(ctx, *ChatModelAgentConfig)` | ✅ | 返回 `(*TypedChatModelAgent[*Message], error)` | ✅ |
| `ChatModelAgentConfig.MaxIterations` | ✅ `10` | 顶层字段，默认 `20` | ✅ |
| `ChatModelAgentConfig.ToolsConfig.ToolsNodeConfig.Tools` | ✅ `[]tool.BaseTool` | 内嵌 `compose.ToolsNodeConfig` | ✅ |
| `ChatModelAgentConfig.Handlers` | 未使用 | `[]ChatModelAgentMiddleware`（推荐） | — |
| `adk.NewTurnLoop(TurnLoopConfig[T, M])` | ✅ | 泛型，`T=string, M=*schema.Message` | ✅ |
| `TurnLoopConfig.GenInput` | ✅ | `func(ctx, *TurnLoop[T,M], []T) (*GenInputResult[T,M], error)` | ✅ |
| `TurnLoopConfig.PrepareAgent` | ✅ | `func(ctx, *TurnLoop[T,M], []T) (TypedAgent[M], error)` | ✅ |
| `TurnLoopConfig.OnAgentEvents` | ✅ | `func(ctx, *TurnContext[T,M], *AsyncIterator[*AgentEvent[M]]) error` | ✅ |
| `TurnLoopConfig.Store` | 移除 | `CheckPointStore`，可选 | 首个版本不启用 |
| `TurnLoopConfig.CheckpointID` | 移除 | `string`，可选 | 首个版本不启用 |
| `TurnLoop.Push(item, ...PushOption)` | ✅ | 返回 `(bool, <-chan struct{})` | ✅ |
| `TurnLoop.Run(ctx)` | ✅ | 非阻塞启动 | ✅ |
| `TurnLoop.Stop(...StopOption)` | ✅ | `WithGracefulTimeout(d)` | ✅ |
| `AgentInput{Messages, EnableStreaming}` | ✅ spec §7.3 | `TypedAgentInput[M]` 结构体 | ✅ |
| `AgentEvent.Output.MessageOutput.Role` | ✅ `schema.Assistant/Tool` | `schema.RoleType` | ✅ |
| `AgentEvent.Output.MessageOutput.IsStreaming` | ✅ | `bool` | ✅ |
| `AgentEvent.Output.MessageOutput.MessageStream.Recv()` | ✅ | `*schema.StreamReader[M].Recv()` | ✅ |
| `AsyncIterator.Next()` | ✅ | 返回 `(T, bool)`，阻塞读取 | ✅ |
| `TurnContext.Preempted` | ✅ | `<-chan struct{}` | ✅ |
| `TurnContext.Stopped` | ✅ | `<-chan struct{}` | ✅ |
| `adk.WithPreempt[T,M](SafePoint)` | ✅ spec §7.7 | `PushOption` | ✅ |
| `adk.AnySafePoint` | ✅ | `AfterChatModel \| AfterToolCalls` | ✅ |
| `adk.WithGracefulTimeout(d)` | ✅ | `StopOption` | ✅ |
| `adk.Agent` | ✅ 作为 PrepareAgent 返回类型 | `TypedAgent[*schema.Message]` 别名 | ✅ |

---

> **实施就绪。** 按 Phase 1 → 2 → 3 → 4 → 5 顺序执行。每个 Phase 结束需 `go build ./...` 通过。Phase 4 只有在 Phase 3 全部通过后才能执行。
