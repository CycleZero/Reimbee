# Agent 层 v2 重构 — 执行计划

> **状态**: 待执行  
> **日期**: 2026-07-06  
> **前置文档**: [agent-redesign-v2.md](./agent-redesign-v2.md)  
> **预计工期**: 5 Waves，约 8-12 小时

---

## 目录

1. [执行概览](#1-执行概览)
2. [Wave 0 — 基础设施准备](#2-wave-0--基础设施准备)
3. [Wave 1 — 工具层 + ReAct 阶段构建器](#3-wave-1--工具层--react-阶段构建器)
4. [Wave 2 — 报销子流程重写](#4-wave-2--报销子流程重写)
5. [Wave 3 — 根路由 + Runner 适配](#5-wave-3--根路由--runner-适配)
6. [Wave 4 — 简单子流程修复 + Bug 清零](#6-wave-4--简单子流程修复--bug-清零)
7. [Wave 5 — 集成测试 + 回归验证](#7-wave-5--集成测试--回归验证)
8. [回滚策略](#8-回滚策略)

---

## 1. 执行概览

```
Wave 0  → 基础设施准备（无行为变更）
Wave 1  → 工具层 + ReAct 阶段构建器（新增模块，独立可测）
Wave 2  → 报销子流程重写（核心变更）
Wave 3  → 根路由 + Runner 适配（连接层）
Wave 4  → 简单子流程修复 + Bug 清零（收尾）
Wave 5  → 集成测试 + 回归验证（质量门）
```

**每个 Wave 结束条件**: 该 Wave 所有 TODO 标记 `completed`，`go build ./...` 和 `go test ./internal/domain/agent/...` 通过。

**TDD 强制要求**: 每个生产代码变更前，先写对应的测试用例，见证 RED → 实施 → GREEN。

---

## 2. Wave 0 — 基础设施准备

> **目标**: 建立测试基础设施，无行为变更，确保后续 Waves 有安全网

### 2.1 任务清单

| # | 任务 | 文件 | 场景 |
|---|------|------|------|
| 0.1 | **创建 `react_phase_test.go`** —— 先写 ReAct 阶段构建器的最小测试用例 | `internal/domain/agent/graph/react_phase_test.go` | **S0.1.1**: `buildReActPhase` 编译成功（有效 chatModel + 工具列表）→ graph 非 nil<br>**S0.1.2**: 无工具时编译成功（ChatModel 直出，无 ToolsNode）→ graph 非 nil<br>**S0.1.3**: chatModel 为 nil 时返回错误 → 错误包含 "chatModel" |
| 0.2 | **扩展 `testutil`** —— 新增 mock `ToolCallingChatModel` | `internal/testutil/mock_chatmodel.go` | **S0.2.1**: mock 实现 `model.ToolCallingChatModel` 接口<br>**S0.2.2**: `Generate()` 返回指定文本（无 ToolCall）<br>**S0.2.3**: `Generate()` 返回含 ToolCall 的消息<br>**S0.2.4**: `Stream()` 逐 chunk 返回<br>**S0.2.5**: `WithTools()` 返回自身（保持工具绑定） |
| 0.3 | **扩展 `testutil`** —— 新增 mock `BaseTool` | `internal/testutil/mock_tool.go` | **S0.3.1**: `Info()` 返回有效 `*schema.ToolInfo`<br>**S0.3.2**: `InvokableRun()` 返回预设结果 |

### 2.2 验证 Gate

```bash
make build        # 编译通过
go test ./internal/testutil/... -v   # 新增 mock 测试通过
```

---

## 3. Wave 1 — 工具层 + ReAct 阶段构建器

> **目标**: 新增 ReAct 阶段构建器 `react_phase.go`，按 TDD 驱动实现

### 3.1 RED: react_phase_test.go（先写测试）

| # | 测试场景 | 期望结果 |
|---|---------|---------|
| 1.1 | `buildReActPhase` 正常参数（有效 chatModel + 2工具） | 编译成功，返回非 nil 图 |
| 1.2 | chatModel 为 nil | 返回 error |
| 1.3 | 工具列表为空 | 编译成功（ChatModel 直出，无 ToolsNode 分支） |
| 1.4 | 工具列表为 nil | 编译成功（同1.3） |
| 1.5 | 图执行：用户消息 → ChatModel 无 ToolCall | 返回文本回复 |
| 1.6 | 图执行：用户消息 → ChatModel 含 ToolCall → ToolsNode 执行 → 回到 ChatModel → 文本回复 | 工具被执行，最终返回文本回复 |
| 1.7 | 图执行：连续3次 ToolCall 后得到文本回复 | 工具被正确执行3次，消息历史包含全部 tool 消息 |

### 3.2 GREEN: react_phase.go（实现）

**文件**: `internal/domain/agent/graph/react_phase.go`

```
职责单一：构建一个 [*schema.Message → *schema.Message] 的 ReAct 阶段子图

func buildReActPhase(ctx, chatModel, logger, config) (*compose.Graph, error)
  ├── 获取 ToolInfos
  ├── chatModel.WithTools(toolInfos)           // 绑定工具
  ├── compose.NewToolNode(tools)               // 创建工具执行器
  ├── compose.NewGraph[*Message, *Message](    // 创建图
  │     compose.WithGenLocalState(phaseState))  // 消息历史累积
  ├── AddChatModelNode("chat", ...)            // ChatModel + state pre-handler
  ├── AddToolsNode("tools", ...)               // ToolsNode + state pre-handler
  ├── AddBranch("chat", StreamBranch)          // ToolCall? → tools : END
  ├── AddBranch("tools", StreamBranch)         // 始终 → chat (loop)
  └── return g, nil
```

**PhaseConfig 结构体**:

```go
type PhaseConfig struct {
    Name         string           // 阶段名（日志/调试用）
    SystemPrompt string           // 系统提示词
    Tools        []tool.BaseTool  // 工具列表
}
```

### 3.3 GREEN: tools/provider.go（新增方法）

**文件**: `internal/domain/agent/tools/provider.go`

新增方法（不删旧方法，保持兼容）：

```go
func (ts *ToolSet) GetPhase1BaseTools() []tool.BaseTool { ... }
func (ts *ToolSet) GetPhase2BaseTools() []tool.BaseTool { ... }
func (ts *ToolSet) GetPhase3BaseTools() []tool.BaseTool { ... }
func (ts *ToolSet) GetProgressBaseTools() []tool.BaseTool { ... }
func (ts *ToolSet) GetBudgetBaseTools()   []tool.BaseTool { ... }
```

### 3.4 验证 Gate

```bash
go test ./internal/domain/agent/graph/ -run TestBuildReActPhase -v  # 7个场景全部 GREEN
go test ./internal/domain/agent/tools/ -run TestGetBaseTools -v     # 新方法正确返回
```

---

## 4. Wave 2 — 报销子流程重写

> **目标**: 完全重写 `reimbursement.go`，用 ReAct 模式替代手动 Lambda 循环

### 4.1 RED: reimbursement_test.go（先写测试）

**复用的 mock**：`testutil.MockChatModel` + `testutil.MockTool`

| # | 测试场景 | 期望结果 |
|---|---------|---------|
| 2.1 | `NewReimbursementGraph` 有效参数编译 | 返回非 nil runnable，无 error |
| 2.2 | Phase 1 → Phase 2 过渡（Guard 通过） | 执行后 ReimbursementState.CurrentPhase == "phase2_validate" |
| 2.3 | Phase 1 Guard 失败时循环（无票据） | Phase1Turns > 1（说明循环了） |
| 2.4 | Phase 2 → Phase 3 过渡（合规通过 + 确认） | CurrentPhase == "phase3_execute" |
| 2.5 | Phase 3 执行后到达 END | 最终返回的 Message 非空 |
| 2.6 | 完整三阶段端到端（mock 驱动） | 所有 Phase 正确过渡，最终返回成功消息 |
| 2.7 | chatModel 为 nil 时的降级行为 | 返回 error |

### 4.2 GREEN: reimbursement.go（完全重写）

**文件**: `internal/domain/agent/graph/reimbursement.go`

```
NewReimbursementGraph(ctx, deps) → compose.Runnable[*Message, *Message]
  │
  ├── buildReActPhase(ctx, chatModel, logger, PhaseConfig{...})
  │     └── Phase1: ChatModel + [OCR, Compliance] tools
  │
  ├── buildReActPhase(ctx, chatModel, logger, PhaseConfig{...})
  │     └── Phase2: ChatModel + [Compliance, Budget] tools
  │
  ├── buildReActPhase(ctx, chatModel, logger, PhaseConfig{...})
  │     └── Phase3: ChatModel + [PDF, Email, Progress] tools
  │
  ├── compose.NewGraph[*Message, *Message](
  │     compose.WithGenLocalState(ReimbursementState))  ← 共享状态
  │
  ├── AddLambdaNode("phase1_guard")  ← Phase1Guard 检查
  ├── AddLambdaNode("phase2_guard")  ← Phase2Guard 检查
  │
  ├── AddGraphNode("phase1", phase1Graph)  ← 子图嵌套
  ├── AddGraphNode("phase2", phase2Graph)
  ├── AddGraphNode("phase3", phase3Graph)
  │
  ├── 连线: START → phase1 → phase1_guard
  ├── Branch(phase1_guard): pass→phase2 | fail→phase1
  ├── 连线: phase2 → phase2_guard
  ├── Branch(phase2_guard): pass→phase3 | fail→phase2
  ├── 连线: phase3 → END
  │
  └── Compile(MaxRunSteps=100, GraphName="reimb_workflow")
```

**关键实现细节**:
- `WithStatePreHandler` 在每个子图被调用时更新 `CurrentPhase`
- Guard Lambda 通过 `ProcessState` 读写 `ReimbursementState`
- Phase1/Phase2 Guard 递增各自的 `PhaseXTurns`（修复 B3）
- Phase3 无 Guard（其内部的 ReAct 循环自然验证完成——修复 B5）

### 4.3 验证 Gate

```bash
go test ./internal/domain/agent/graph/ -run TestNewReimbursementGraph -v   # 7个场景 GREEN
go test ./internal/domain/agent/graph/ -run TestReimbursement -v           # 旧测试适配后 GREEN
```

---

## 5. Wave 3 — 根路由 + Runner 适配

> **目标**: 修改 `root.go`、`provider.go`、`runner.go`，使新报销子流程接入现有系统

### 5.1 RED: 先写测试

| # | 测试场景 | 文件 |
|---|---------|------|
| 3.1 | `classifyIntent` 读取 Config 阈值 0.5（LLM 返回 confidence=0.6）→ 路由到正确意图 | `graph/root_test.go` |
| 3.2 | `classifyIntent` 读取 Config 阈值 0.9（LLM 返回 confidence=0.6）→ 降级到关键词 | `graph/root_test.go` |
| 3.3 | `dispatchToWorkflow` 将 AgentInput 注入 context → 子图可读取 | `graph/root_test.go` |
| 3.4 | `StreamableLambda` dispatcher 的 `Stream()` 返回有效流 | `graph/root_test.go` |
| 3.5 | `agentInputAdapter` 从 context 恢复完整 AgentInput | `graph/provider_test.go` |
| 3.6 | `StreamChat` 真正流式路径不触发 invokeFallback（mock stream 成功） | `runner_test.go` |

### 5.2 GREEN: 逐个文件修改

**文件 1**: `internal/domain/agent/graph/root.go`

| 变更 | Bug | 描述 |
|------|-----|------|
| `RootGraphDeps` 增加 `Config *agent.AgentConfig` | B4 | 新增字段 |
| `classifyIntent` 读取 `deps.Config.IntentConfidenceThreshold` | B4 | 替换硬编码 0.7 |
| `dispatchToWorkflow` 注入 `ctx = context.WithValue(ctx, userContextKey{}, input)` | B1, B7 | 传递用户上下文 |
| `dispatcher` 改为 `StreamableLambda` | B6 | 支持真流式 |
| `truncate` 替换为 `truncateStr`（rune 截断） | B8 | 修复中文截断 |

**文件 2**: `internal/domain/agent/graph/provider.go`

| 变更 | Bug | 描述 |
|------|-----|------|
| `NewRootGraphRunnable` 传入 `config` 到 `RootGraphDeps` | B4 | 传递 Config |
| `agentInputAdapter.Invoke` 从 context 恢复完整 AgentInput | B1 | 修复元数据丢失 |
| `agentInputAdapter.Stream` 同样恢复 | B1 | 流式路径 |

**文件 3**: `internal/domain/agent/runner.go`

| 变更 | Bug | 描述 |
|------|-----|------|
| `StreamChat` 移除无效的 stream-fallback 判断 | B6 | 现在真正流式 |

### 5.3 验证 Gate

```bash
go test ./internal/domain/agent/graph/ -run TestRoot -v     # 根图测试 GREEN
go test ./internal/domain/agent/ -run TestAgentRunner -v    # Runner 测试 GREEN  
go test ./internal/domain/agent/ -run TestStreamChat -v     # 流式测试 GREEN
go build ./...                                               # 编译通过
make wire                                                    # Wire 重新生成通过
```

---

## 6. Wave 4 — 简单子流程修复 + Bug 清零

> **目标**: 修复剩余低级 Bug，适配简单子流程

### 6.1 任务清单

| # | 文件 | 变更 | Bug |
|---|------|------|-----|
| 4.1 | `graph/root.go` | `truncate()` → `truncateStr()`（rune 安全） | B8 |
| 4.2 | `tools/email_tool.go:70` | `context.Background()` → `time.Now().UnixNano()` | B9 |
| 4.3 | `config.yaml` | 所有 `${}` 替换为实际值或移除示例中的占位符 | B10 |
| 4.4 | `graph/progress.go` | `build_prompt` Lambda 中利用 context 恢复的 AgentInput.EmployeeID | B1 |
| 4.5 | `graph/budget.go` | 同上 | B1 |
| 4.6 | `graph/policy.go` | 无需修改（不依赖用户上下文） | — |
| 4.7 | `graph/modify.go` | 同上 B1 | B1 |

### 6.2 验证 Gate

```bash
go test ./internal/domain/agent/tools/ -v          # 工具测试 GREEN
go test ./internal/domain/agent/graph/ -v          # 所有子图测试 GREEN
go test ./internal/domain/agent/ -v                # 全 Agent 层测试 GREEN
```

---

## 7. Wave 5 — 集成测试 + 回归验证

> **目标**: 端到端验证、性能回归、清理

### 7.1 RED: 新增集成测试

| # | 测试场景 | 文件 |
|---|---------|------|
| 5.1 | 完整报销流程集成测试：模拟用户→Phase1(OCR)→Phase2(Compliance+Budget)→Phase3(PDF+Email) | `graph/reimbursement_integration_test.go` |
| 5.2 | 意图分类集成测试：多种中文输入 → 正确路由 | `graph/root_integration_test.go` |
| 5.3 | 流式输出端到端测试：SSE 事件顺序/格式验证 | `service_integration_test.go` |
| 5.4 | Session 隔离测试：两个并发 session 不互相污染 | `runner_test.go`（已有，确认通过） |

### 7.2 回归验证

```bash
# 全量编译
go build ./...

# Wire 生成
make wire

# Agent 层全量测试（含集成测试）
go test ./internal/domain/agent/... -v -count=1 -race

# 全项目测试
go test ./internal/domain/... -v -count=1

# 静态分析
go vet ./...
```

### 7.3 验证 Gate（最终质量门）

| 检查项 | 命令 | 通过条件 |
|--------|------|---------|
| 编译 | `go build ./...` | exit 0 |
| Wire | `make wire` | exit 0，`wire_gen.go` 无差异 |
| Agent 层测试 | `go test ./internal/domain/agent/... -race` | 全部 PASS |
| 全项目测试 | `go test ./internal/domain/... -race` | 全部 PASS |
| Vet | `go vet ./...` | 无警告 |
| LSP Diagnostics | `lsp_diagnostics ./internal/domain/agent/` | 无 Error |

### 7.4 手动 QA

| 场景 | 操作 | 预期结果 |
|------|------|---------|
| 服务启动 | `make rebuild && ./bin/app` | 无 panic，路由表打印正常 |
| 健康检查 | `curl http://localhost:8080/health` | `{"status":"ok"}` |
| SSE 连接 | `curl -N "http://localhost:8080/api/chat/stream?session_id=test&message=你好"` | SSE 事件流：thinking → message → done |

---

## 8. 回滚策略

### 8.1 每 Wave 原子提交

```
Wave 0 → commit: "feat(agent): add test infrastructure for v2 refactor"
Wave 1 → commit: "feat(agent): add ReAct phase builder + tool base methods"
Wave 2 → commit: "refactor(agent): rewrite reimbursement graph with ReAct pattern"
Wave 3 → commit: "refactor(agent): adapt root graph and runner for ReAct"
Wave 4 → commit: "fix(agent): resolve P1-P3 bugs in simple sub-flows"
Wave 5 → commit: "test(agent): add integration tests + regression verification"
```

### 8.2 回滚方式

任一波失败 → `git reset --hard <前一波 commit>` 回到已知良好状态。

### 8.3 不可回滚的操作

- 无。所有变更在本地仓库内，不影响外部依赖。

---

## 附录 A：完整 TODO 清单（按执行顺序）

```
Wave 0:
□ testutil/mock_chatmodel.go: 实现 MockToolCallingChatModel — 验证 RED
□ testutil/mock_tool.go: 实现 MockBaseTool — 验证 RED
□ graph/react_phase_test.go: 编写 RED 测试用例（7 场景）

Wave 1:
□ graph/react_phase.go: 实现 buildReActPhase() — 验证 GREEN
□ tools/provider.go: 新增 GetPhaseXBaseTools() 方法
□ tools/provider_test.go: 验证新方法

Wave 2:
□ graph/reimbursement_test.go: 编写 RED 测试用例（7 场景）
□ graph/reimbursement.go: 完全重写为 ReAct 模式 — 验证 GREEN

Wave 3:
□ graph/root.go: RootGraphDeps 增加 Config 字段
□ graph/root.go: classifyIntent 读取配置阈值
□ graph/root.go: dispatchToWorkflow 注入用户上下文
□ graph/root.go: dispatcher → StreamableLambda
□ graph/root.go: truncate → truncateStr
□ graph/provider.go: agentInputAdapter 修复元数据恢复
□ graph/provider.go: NewRootGraphRunnable 传递 config
□ runner.go: StreamChat 适配真流式
□ root_test.go: 新增 RED 测试

Wave 4:
□ tools/email_tool.go: 修复 MessageID 格式化
□ graph/progress.go: build_prompt 利用用户上下文
□ graph/budget.go: 同上
□ graph/modify.go: 同上

Wave 5:
□ graph/reimbursement_integration_test.go: 端到端集成测试
□ graph/root_integration_test.go: 意图分类集成测试
□ 全量回归测试
□ 手动 QA
```

---

## 附录 B：风险缓解矩阵

| 风险 | 可能性 | 影响 | 缓解 |
|------|--------|------|------|
| `AddGraphNode` 嵌套子图 state 不可见 | 中 | 高 | Wave 2 阶段先验证 state 传递，失败则改用 `AppendGraph` |
| `chatModel.WithTools()` 返回值错误 | 低 | 高 | 使用 mock 在 Wave 1 中完全覆盖此路径 |
| `AddBranch` 条件函数 panic | 低 | 中 | Branch 函数内 defer recover |
| Wire 依赖注入与新参数不兼容 | 低 | 中 | 每 Wave 后 `make wire` 验证 |
| 流式输出被上游代理缓冲 | 低 | 低 | X-Accel-Buffering 已在 SSE Writer 中设置 |
