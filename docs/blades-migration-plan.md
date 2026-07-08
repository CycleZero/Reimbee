# Eino → Blades Agent 层迁移执行计划

> 创建日期: 2026-07-07
> 状态: 待确认

## 1. 背景与目标

### 当前状态
Reimbee 的 Agent 层使用 **cloudwego/eino v0.9.12** (ADK 模式)，存在以下问题：
- Eino 流式事件（OnAgentEvents）丢失 assistant 消息中的 `tool_calls` 结构
- 需要 `messageCollector` 中间件 workaround（AfterModelRewriteState hook）
- TurnLoop 的 Push/Pull + 三回调模式过于复杂
- 后台 goroutine 管理 + SSE writer 注册机制耦合度高

### 目标
用 **github.com/go-kratos/blades v0.5.0** 完全重写 Agent 层：
- 消除 Eino 依赖
- Blades 原生处理 tool_calls → 删除 messageCollector
- Runner.RunStream() 替代 TurnLoop → 大幅简化
- 保持对外接口（SSE 端点、SessionStore、Wire DI）不变

## 2. 框架对比

| 概念 | Eino (当前) | Blades (目标) |
|------|-------------|---------------|
| 入口 | `TurnLoop.Push()` + 回调 | `Runner.Run() / RunStream()` |
| Agent | `adk.ChatModelAgent` | `blades.NewAgent(name, opts...)` |
| LLM | `openaimodel.NewChatModel` (eino-ext) | `openai.NewModel` (blades/contrib) |
| Tool | `tool.InvokableTool` (Eino schema) | `tools.Tool` 接口 (`Handle(ctx, input)`) |
| 多轮历史 | 自定义 SessionStore (双表) | `Session.History() / Append()` |
| 流式 | event stream → SSE writer | `Runner.RunStream()` → `Generator[*Message, error]` |
| 中断 | `Checkpoint + GenResume` | `ActionInterrupted` + actions |
| 中间件 | `AfterModelRewriteState` hook | 洋葱模型 `Middleware` |
| 嵌套Agent | `adk.NewAgentTool` (ChatModelAgent) | `blades.NewAgentTool` |
| 消息持久化 | 手动 SaveMessages + collector | Blades 原生 `session.Append()` 自动处理 |

## 3. 影响范围

### 文件删除 (5个)
- `internal/domain/agent/message_collector.go` — 中间件 workaround 不再需要
- `internal/domain/agent/checkpoint.go` — 中断机制改为 Blades actions
- `internal/domain/agent/checkpoint_test.go` — 对应测试
- `internal/domain/agent/runner.go` — context key 移到 types 包
- `internal/domain/agent/llm_test.go` — 重写为 Blades 模型测试

### 文件完全重写 (14个)
- `internal/domain/agent/session_loop.go` → `agent_loop.go` (Runner + SSE bridge)
- `internal/domain/agent/phase_agents.go` → Blades NewAgent 初始化
- `internal/domain/agent/loop_manager.go` → 大幅简化 (移除 TurnLoop 复杂度)
- `internal/domain/agent/llm.go` → 切换到 blades/contrib/openai
- `internal/domain/agent/tools/` 目录下 **9 个工具** → 接口改为 tools.NewFunc

### 文件修改 (7个)
- `internal/domain/agent/service.go` — SSE 集成对接 RunStream
- `internal/domain/agent/provider.go` — Wire 绑定更新
- `internal/domain/agent/config.go` — 移除 checkpoint 相关
- `infra/session.go` — SessionStore 接口 Message 类型改为 blades.Message
- `infra/session_mysql.go` — 实现 blades.Message 序列化
- `infra/session_redis.go` — 实现 blades.Message 序列化
- `infra/session_mysql_test.go` — 更新测试

### 新文件 (2个)
- `internal/domain/agent/session_adapter.go` — 基于 SessionStore 的 blades.Session 实现
- `internal/domain/agent/interrupt_handler.go` — 基于 Blades actions 的审批中断

## 4. 关键设计决策 (待确认)

| # | 决策 | 推荐方案 | 待确认 |
|---|------|---------|--------|
| D1 | SessionStore Message 类型 | 改为 `blades.Message`，保持方法和 DB 列不变 | ⏳ |
| D2 | 消息持久化 | 删除 messageCollector，依赖 Blades 原生 session.Append() | ✅ |
| D3 | 会话生命周期 | 每请求同步 RunStream，无需后台 goroutine | ✅ |
| D4 | ReimbursementState 访问 | 保持 ctx.Value + store.GetState/SaveState 模式 | ✅ |
| D5 | 历史加载 | WithContext(true) + DB 后端 Session.History() | ✅ |
| D6 | 审批中断 | Blades Actions (interrupt_handler.go) vs 纯对话确认 | ⏳ |
| D7 | 依赖兼容性 | contribute/openai 版本不兼容时允许 vendoring | ⏳ |

## 5. 任务分解 (35 步, 9 个 Wave)

### Wave 1: 基础依赖 (并行)
| ID | 任务 | 类别 | 依赖 |
|----|------|------|------|
| T1 | 添加 blades + contrib/openai 到 go.mod，版本兼容验证 | deep | 无 |
| T9 | 验证 context_helper.go ctx 传播模式在 blades 中可用 | quick | 无 |

### Wave 2: 接口层变更 (并行, 依赖 T1)
| ID | 任务 | 类别 | 依赖 |
|----|------|------|------|
| T2 | infra.SessionStore 接口改为 blades.Message | unspecified-low | T1 |
| T6 | llm.go 改为 blades contrib/openai | unspecified-low | T1 |
| T8 | tools/provider.go 类型改为 blades.Tool | unspecified-low | T1 |

### Wave 3: 实现层 (并行)
| ID | 任务 | 类别 | 依赖 |
|----|------|------|------|
| T3 | MySQLSessionStore → blades.Message | ultrabrain | T2 |
| T4 | RedisSessionCache → blades.Message | quick | T2 |
| T5 | infra session 测试更新 | unspecified-low | T2 |
| T7 | llm_test.go 更新 | quick | T6 |
| T10–T18 | 9 个 leaf 工具重写 (并行) | quick | T8, T9 |
| T19 | search_policy_tool 重写 | quick | T8 |

### Wave 4: 核心组件 (并行)
| ID | 任务 | 类别 | 依赖 |
|----|------|------|------|
| T20 | compliance_agent_tool → NewAgent+NewAgentTool | unspecified-high | T6, T19 |
| T21 | tools/provider_test.go 更新 | quick | 工具层 |
| T22 | session_adapter.go (NEW) | ultrabrain | T2, T3 |

### Wave 5: Agent 核心 (并行)
| ID | 任务 | 类别 | 依赖 |
|----|------|------|------|
| T23 | phase_agents.go → blades.NewAgent | unspecified-high | T6, T8, T20 |
| T24 | interrupt_handler.go (NEW) | ultrabrain | T22, T8 |

### Wave 6: Agent 循环
| ID | 任务 | 类别 | 依赖 |
|----|------|------|------|
| T25 | agent_loop.go (替换 session_loop.go) | ultrabrain | T22, T23, T24 |
| T30 | config/dto/prompt 微调 | quick | T26 |

### Wave 7: 集成层
| ID | 任务 | 类别 | 依赖 |
|----|------|------|------|
| T26 | loop_manager.go 简化 | unspecified-high | T23, T25 |
| T28 | service.go SSE 集成 | unspecified-high | T25, T26 |

### Wave 8: 清理与 DI
| ID | 任务 | 类别 | 依赖 |
|----|------|------|------|
| T27 | 删除 message_collector/checkpoint/runner.go | quick | T25, T26 |
| T29 | provider.go Wire 重新绑定 | unspecified-low | T6, T22, T26, T28 |

### Wave 9: 验证门 (严格顺序)
| ID | 任务 | 类别 | 依赖 |
|----|------|------|------|
| T31 | go.mod 移除 eino + tidy | quick | T27 |
| T32 | make wire | quick | T29, T31 |
| T33 | go build ./... | quick | T32 |
| T34 | go test ./... + 修复 | unspecified-high | T33 |
| T35 | 最终 review (review-work) | unspecified-high | T34 |

## 6. 关键路径

```
T1 → T2 → T3 → T22 → T23/T24 → T25 → T26/T28 → T29 → T32 → T33 → T34 → T35
```

## 7. 风险与缓解

| 风险 | 严重程度 | 缓解措施 |
|------|---------|---------|
| contrib/openai 版本不兼容 blades v0.5.0 | HIGH | T1 先 spike 验证；允许 vendoring 或 replace |
| blades.Message ↔ DB 列映射 (tool_calls/tool_results) | HIGH | T5 先写测试锚定预期行为 |
| 中断/恢复语义差异 (Checkpoint→Actions) | HIGH | T24 先写独立单元测试 |
| jsonschema tag 语义差异 (eino→google/jsonschema-go) | MED | 先在 OCR tool 上验证 tag 映射 |
| 双重消息追加 (blades 自动 Append + 手动 SaveMessages) | HIGH | session_adapter 明确职责边界 |
| SSE 流式事件映射 (blades 增量 chunk → SSE delta 事件) | HIGH | T25 需要在 agent_loop 中正确映射 StatusInProgress |

## 8. 成功标准

- `grep -r "cloudwego/eino"` → 无结果
- `go build ./...`、`make wire`、`go test ./...` 全部通过
- 中文注释/日志/错误规范保持不变
- DDD 分层 + Wire DI + 金额规范 (int64分/float64元) 不变
- 审批中断/恢复通过 Blades actions 工作
- 嵌套合规 Agent 通过 NewAgentTool 工作

## 9. Blades API 速查

```go
// Agent 创建
agent, err := blades.NewAgent("name",
    blades.WithModel(model),                           // ModelProvider (必选)
    blades.WithInstruction("system prompt..."),         // 系统指令
    blades.WithTools(tool1, tool2),                    // 工具列表
    blades.WithMaxIterations(15),                      // 最大 tool-calling 循环
    blades.WithDescription("description"),              // 描述
    blades.WithMiddleware(mw1, mw2),                   // 洋葱模型中间件
    blades.WithContext(true),                          // 从 session 加载完整历史
)

// Runner
runner := blades.NewRunner(agent)
result, _ := runner.Run(ctx, msg, blades.WithSession(session))           // 非流式
stream := runner.RunStream(ctx, msg, blades.WithSession(session))        // 流式

// 模型 (contrib/openai)
model := openai.NewModel("gpt-4o", openai.Config{
    APIKey:  key,
    BaseURL: url,
})

// 工具
tool, _ := tools.NewFunc[Input, Output]("name", "desc", handler)

// 嵌套 Agent
complianceTool := blades.NewAgentTool(complianceAgent)

// Session 接口
type Session interface {
    ID() string
    State() State
    SetState(string, any)
    History(ctx context.Context) ([]*Message, error)
    Append(context.Context, *Message) error
}
```
