# Agent 层完整数据流追踪

> **日期**: 2026-07-06  
> **覆盖范围**: HTTP 端点 → JWT → Runner → Graph → SSE → SessionStore

---

## 1. 全链路总览

```
Layer 1: HTTP 入口
  GET /api/chat/stream?session_id=X&message=Y
    │  携带: Authorization: Bearer <JWT>
    │
    ▼
Layer 2: JWT 中间件 (middleware/auth.go:16-71)
    │  解析 JWT → 注入 gin.Context:
    │    c.Set("user_id", uint(uid))
    │    c.Set("employee_id", empID)
    │    c.Set("role", role)
    │
    ▼
Layer 3: AgentService (service.go:44-88)
    │  c.Query("session_id")  → sessionID
    │  c.Query("message")     → message
    │  c.Get("user_id")       → userID      (从 JWT claims)
    │  c.Get("employee_id")   → employeeID
    │  c.Get("role")          → role
    │
    │  → BuildAgentInput(sessionID, message, employeeID, userID, role)
    │  → AgentInput{ SessionID, UserID, EmployeeID, Role, Message }
    │
    ▼
Layer 4: AgentRunner (runner.go:58-131)
    │  ① SSE: thinking 事件
    │  ② loadSessionState() → SessionStore.GetState("reimbursement") → ctx注入
    │  ③ SessionStore.GetHistory(sessionID) → []*Message (对话历史)
    │  ④ SessionStore.SaveMessages(userMsg) → 持久化
    │  ⑤ rootGraph.Stream(ctx, AgentInput) → *StreamReader[*Message]
    │  ⑥ chunk-by-chunk → SSE message(delta=true)
    │  ⑦ saveSessionState() → ProcessState → SessionStore.SaveState("reimbursement")
    │  ⑧ SSE: done 事件
    │
    ▼
Layer 5: Root Graph (graph/root.go:43-68)
    │  Type: AgentInput → *Message
    │  Node: "dispatcher" (StreamableLambda)
    │    ├── classifyIntent() → "new_reimbursement" | "query_progress" | ...
    │    └── dispatchToWorkflow()
    │          ├── ctx注入: userContextKey{} → AgentInput
    │          ├── switch(route):
    │          │   "new_reimbursement" → ReimbRunnable.Invoke(ctx, *Message)
    │          │   "query_progress"    → ProgressRunnable.Invoke(ctx, *Message)
    │          │   ...
    │          └── 返回 *Message
    │
    ▼
Layer 6: Reimbursement Sub-Graph (graph/reimbursement.go)
    │  Type: []*Message → *Message
    │  State: *ReimbursementState (WithGenLocalState)
    │
    │  [父图节点]
    │  START → Phase1 → phase1_guard → [Branch]
    │                                     ├── pass → Phase2 → phase2_guard → [Branch]
    │                                     │                                   ├── pass → Phase3 → END
    │                                     │                                   └── fail → Phase2 (loop)
    │                                     └── fail → Phase1 (loop)
    │
    │  [每个 Phase = ReAct 子图]
    │  Type: []*Message → *Message
    │  State: *phaseState { Messages []*Message }
    │
    │  ChatModelNode ←── [Branch: ToolCalls?] ──→ ToolsNode
    │       │               YES → tools              │
    │       │               NO  → END                └── (always) → ChatModelNode
    │       │
    │       └── StatePreHandler: system prompt + message history
    │
    ▼
Layer 7: SSE 输出 (sse.go)
    │  event: thinking\ndata: {type:"thinking", data:{message:"正在理解..."}}
    │  event: message\ndata: {type:"message", data:{content:"请上传票据", delta:true}}
    │  event: done\ndata:    {type:"done"}
    │
    ▼
Layer 8: SessionStore 持久化
    │  SaveMessages(sessionID, [userMsg, assistantMsg]) → MySQL + Redis
    │  SaveState(sessionID, "reimbursement", ReimbursementState) → MySQL
```

---

## 2. 逐层详细分析

### 2.1 HTTP → JWT 中间件

```
入: GET /api/chat/stream?session_id=abc&message=我要报销
    Authorization: Bearer eyJhbG... (JWT token)

JWT Claims (签发时写入):
{
  "user_id": 42,         // float64 → JWT标准数字类型
  "employee_id": "EMP001",
  "role": "employee",
  "exp": 1750262400
}

中间件处理后 gin.Context:
  c.Keys["user_id"]     = uint(42)
  c.Keys["employee_id"] = "EMP001"
  c.Keys["role"]        = "employee"

出: gin.Context (带 JWT claims)
```

### 2.2 AgentService.HandleChat

```go
// 输入: gin.Context
// 输出: SSE 流 (通过 c.Writer)

① 解析查询参数:
   sessionID := c.Query("session_id")   // "abc"
   message   := c.Query("message")      // "我要报销"

② 提取 JWT claims:
   userID     := c.Get("user_id")       // uint(42)  ← 需要类型断言 float64→uint
   employeeID := c.Get("employee_id")   // "EMP001"
   role       := c.Get("role")          // "employee"

③ 创建 GinSSEWriter:
   c.Header("Content-Type", "text/event-stream")
   c.Header("Cache-Control", "no-cache")
   c.Header("Connection", "keep-alive")
   c.Header("X-Accel-Buffering", "no")  // 禁用 nginx 缓冲
   c.Writer.WriteHeader(200)
   flusher.Flush()                       // 立即推送响应头

④ 构建 AgentInput:
   input := AgentInput{
       SessionID:  "abc",
       UserID:     42,
       EmployeeID: "EMP001",
       Role:       "employee",
       Message:    "我要报销",
   }
```

### 2.3 AgentRunner.StreamChat

```go
// 输入: context.Context + AgentInput + SSEWriter
// 核心: "加载 → 执行 Graph → 流式输出 → 持久化" 八步循环

━━ Step ①: thinking 事件 ━━
   SSE: event:thinking data:{"type":"thinking","data":{"message":"正在理解您的需求..."}}

━━ Step ②: 加载业务状态 (v2.1 新增) ━━
   SessionStore.GetState(ctx, "abc", "reimbursement", &ReimbursementState{})
   ┌─ 首次请求: found=false → 不注入
   └─ 后续请求: found=true  → ctx = context.WithValue(ctx, StateContextKey{}, &state)
                                 ↓
                            Graph 的 WithGenLocalState 读取此值恢复状态

━━ Step ③: 加载对话历史 ━━
   SessionStore.GetHistory(ctx, "abc", limit=40)
   ┌─ 缓存命中 (Redis): 直接返回, 按 limit 截取最近消息
   └─ 缓存未命中: MySQL 查询 → 反序列化 → 异步预热 Redis
   返回: []*schema.Message{
       {Role: "user", Content: "我要报销", ...},
       {Role: "assistant", Content: "请上传票据", ...},
   }
   ⚠️ 历史消息仅用于 SSE 回复后的语义注入——Graph 内部通过自己的 State 管理消息

━━ Step ④: 持久化用户消息 ━━
   userMsg := schema.UserMessage("我要报销")
   SessionStore.SaveMessages(ctx, "abc", [userMsg])
   ┌─ MySQL: INSERT INTO session_messages(session_id, role, content, raw_json)
   └─ Redis: 异步更新缓存 (goroutine)

━━ Step ⑤: 执行 Graph ━━
   stream, err := rootGraph.Stream(ctx, AgentInput{...})
   ┌─ 成功: 获取 *StreamReader[*Message] (逐chunk读取)
   └─ 失败: invokeFallback() → 整个回复作为单个消息

━━ Step ⑥: 流式 SSE 推送 ━━
   for {
       chunk, err := stream.Recv()     // 从 Eino Graph 流获取 chunk
       if err == io.EOF → break
       fullContent += chunk.Content
       SSE: event:message data:{"type":"message","data":{"content":"请上传","delta":true}}
       SSE: event:message data:{"type":"message","data":{"content":"票据图片","delta":true}}
   }

━━ Step ⑦: 持久化 assistant 回复 ━━
   SessionStore.SaveMessages(ctx, "abc", [assistantMsg])

━━ Step ⑧: 保存业务状态 (v2.1) ━━
   compose.ProcessState(ctx, func(rs *ReimbursementState) {
       SessionStore.SaveState(ctx, "abc", "reimbursement", rs)
   })
   ⚠️ 注意: 由于 Eino 状态作用域限制，此步无法从 Runner 层读取嵌套图的 ReimbursementState。
   当前 saveSessionState 返回 nil（无错误，但实际未保存）
   → 需在 Graph 内部增加状态导出钩子

━━ Step ⑨: done 事件 ━━
   SSE: event:done data:{"type":"done"}
```

### 2.4 Root Graph — dispatcher

```go
// Type: AgentInput → *Message
// 单个 Lambda 节点: "dispatcher"

代理输入 → StreamableLambda {
    result := dispatchToWorkflow(ctx, input, deps)
    return StreamReaderFromArray([result])  // 单消息包装为流
}

dispatchToWorkflow 内部:

━━ ① 用户上下文注入 ━━
   ctx = context.WithValue(ctx, userContextKey{}, AgentInput{...})
   → 子图通过 agentInputAdapter.Invoke() 中的 ctx.Value(userContextKey{}) 恢复

━━ ② 意图分类 ━━
   route := classifyIntent(ctx, input, deps)
   ┌─ ChatModel 优先: LLM 返回 JSON → 解析 confidence → 与 AgentConfig.IntentConfidenceThreshold 比较
   └─ 降级: classifyByKeywords() — 按优先级匹配中文关键词

━━ ③ 路由分发 ━━
   msg := schema.UserMessage(input.Message)  // *Message{Role:user, Content:"我要报销"}

   switch route {
   case "new_reimbursement":
       ReimbursementRunnable.Invoke(ctx, msg) → *Message
       // 此 Runnable 的 Invoke 接收 *Message 输入，返回 *Message 输出
       // 内部实现: AgentInput{Message: input.Content} → 新 compile 的子图执行
   case "query_progress":
       ProgressRunnable.Invoke(ctx, msg) → *Message
   ...
   case "general_chat":
       ChatModel.Generate(ctx, [systemPrompt, userMsg]) → *Message
   }

   返回: *Message{Role:assistant, Content:"请上传票据图片..."}
```

### 2.5 Reimbursement Sub-Graph — 父图

```go
// Type: []*Message → *Message
// State: *ReimbursementState (共享跨阶段状态)

入口数据: []*schema.Message{ userMsg }

━━ 节点 1: START → phase1_collect ━━
   AddGraphNode("phase1_collect", phase1Graph)
   ┌─ StatePreHandler: rs.CurrentPhase = "phase1_collect"
   └─ 子图输入: []*Message (用户的原始消息)
   子图输出: *Message (Phase 1 LLM 的最终回复)


━━ 节点 2: phase1_collect → phase1_guard ━━
   Guard Lambda (InvokableLambda):
   ┌─ ProcessState → rs.Phase1Turns++
   │  Phase1Guard(rs):
   │    ├─ len(Invoices)==0        → Passed=false, "请上传票据"
   │    ├─ Invoice.amount==0       → Passed=false, "请补充金额"
   │    ├─ !UserConfirmed          → Passed=false, "请确认票据"
   │    └─ all pass                → Passed=true
   └─ 返回: []*Message{msg}  (包装为数组)
       ⚠️ msg 内容 = Phase 1 LLM 的最终回复文本


━━ 节点 3: phase1_guard → Branch ━━
   AddBranch("phase1_guard", GraphBranch):
   ┌─ ProcessState → 重新调用 Phase1Guard(rs)
   │  ├─ pass → "phase2_validate"  (进入校验阶段)
   │  └─ fail → "phase1_collect"   (循环回 Phase1)
   └─ 被路由到的节点收到: []*Message (guard 的输出)


━━ 节点 4: Phase2 → ... (重复 Phase1 模式) ━━
   phase2 Graph 收到: []*Message{guard输出}
   → 子图 StatePreHandler: rs.CurrentPhase = "phase2_validate"
   → 子图 ReAct: ChatNode ↔ ToolsNode
   → Guard: Phase2Guard(rs) — 检查 ComplianceResult + BudgetResult + FinalConfirmed
   → Branch: pass → phase3 | fail → phase2


━━ 节点 5: Phase3 → END ━━
   phase3 Graph 收到: []*Message{guard输出}
   → 子图 ReAct: ChatNode ↔ ToolsNode (CreateReimb, SubmitReimb, PDF, Email)
   → 直接连到 END（Phase3 无 Guard）


━━ 守护机制 ━━
   compose.WithMaxRunSteps(100)
   → Phase Guard 重试次数 = Phase1Turns + Phase2Turns 各自独立计数
   → 每个 ReAct 子图的内部循环也计入总步数
   → 超过 100 步 → [GraphRunError] exceeds max steps
```

### 2.6 ReAct Phase Sub-Graph — 子图内部

```go
// Type: []*Message → *Message
// State: *phaseState { Messages []*Message }
// 注意: phaseState 是子图私有的，非父图的 ReimbursementState!

━━ 子图入口: []*Message ━━
   父图通过 AddEdge 传递的 guard 输出消息数组


━━ 内部拓扑 ━━
   START → ChatModelNode → [Branch] → ToolsNode → [Branch] → ChatModelNode
                                │ NO                       │ (always loop)
                                ▼                          ▼
                               END ← 文本回复          → ChatModelNode

━━ ChatModelNode 的 StatePreHandler ━━
   func(ctx, input []*Message, ps *phaseState) ([]*Message, error) {
       ps.Messages = append(ps.Messages, input...)   // ← 累积入口消息
       return append(SystemMessage(prompt), ps.Messages...) // ← 返回完整历史给 LLM
   }

   ⚠️ 问题: 当 Guard 失败、Branch 路由回 Phase1 时，子图被重新进入，
   phaseState 中的 Messages 被清空（WithGenLocalState 创建新实例）
   → LLM 收到的消息: [SystemPrompt, Guard消息]
   → 丢失了: 上一轮的 userMsg 和 assistantMsg

━━ ChatModelNode 执行 ━━
   输入: [SystemPrompt(phase1规则), userMsg, assistantMsg(含ToolCalls), toolResults...]
         ↑ 由 StatePreHandler 构建的完整消息列表
   输出: *Message { Role:assistant, Content:"...", ToolCalls:[...] OR Content:"请上传票据" }

━━ Branch 1: ChatModel → 检查 ToolCalls ━━
   StreamGraphBranch:
   读取 ChatModel 输出流 → 检查首个有效 chunk:
   ├─ ToolCalls 非空 → "tools" → 工具被 ToolsNode 执行
   ├─ Content 非空   → END    → 阶段结束, 返回 *Message
   └─ 流结束(EOF)     → END    → 阶段结束

━━ ToolsNode 执行 ━━
   输入: *Message (含 []ToolCall, 由 LLM 生成)
   内部:
   ┌─ 遍历 ToolCalls, 按名称匹配配置的工具
   ├─ 逐个调用 tool.InvokableRun(ctx, argumentsJSON)
   ├─ 每个结果封装为 ToolMessage(tool_call_id, tool_name, result)
   └─ 输出: []*Message (每条 tool 结果一个消息)

━━ Branch 2: ToolsNode → 回到 ChatModel ━━
   工具结果通过 StatePreHandler 追加到 phaseState.Messages
   → ChatModel 下一轮调用时看到: 系统提示词 + 对话历史 + assistant的tool_call + tool结果
   → LLM 根据工具结果继续推理

━━ 子图出口: END ━━
   ChatModel 返回纯文本（无ToolCalls）时的最后一个 *Message
   → 传递给父图的 Guard 节点
```

### 2.7 简单子流程 (progress/budget/policy/modify)

```go
// 这些子图没有工具调用循环，是简单的 ChatModel 链

━━ buildProgressGraph ━━
   Type: AgentInput → *Message
   拓扑: START → build_prompt(Lambda) → progress_agent(ChatModel) → END

   Lambda: func(AgentInput) → []*Message {
       return [SystemMessage("查询进度..."), UserMessage(input.Message)]
   }
   ChatModel: 输出 *Message

   ⚠️ 问题: build_prompt Lambda 的 AgentInput 来自 agentInputAdapter,
   当前 adapter 已修复(B1): ctx.Value(userContextKey{}) 恢复完整 AgentInput
   → EmployeeID, UserID 等上下文可用
```

### 2.8 SSE 输出

```go
// 每条 SSE 事件格式:
//   event: <type>\n
//   data: <json>\n
//   \n

━━ 事件类型 ━━
thinking:      {type:"thinking",  data:{message:"正在理解您的需求..."}}
tool_call:     {type:"tool_call", data:{tool:"recognize_invoice", input:{...}}}
tool_result:   {type:"tool_result", data:{tool:"recognize_invoice", output:{...}}}
message:       {type:"message",   data:{content:"请上传票据", delta:true}}
phase_change:  {type:"phase_change", data:{from:"phase1", to:"phase2", summary:"..."}}
confirm_required: {type:"confirm_required", data:{prompt:"确认提交?", action:"confirm"}}
error:         {type:"error",     data:{message:"OCR失败", retry:true, code:"OCR_FAILED"}}
done:          {type:"done",      data:null}

⚠️ tool_call 和 tool_result 事件目前由工具实现内部发送，Graph 层不自动生成
```

---

## 3. 节点携带的数据汇总

| 节点 | 输入类型 | 输出类型 | 携带的核心数据 |
|------|---------|---------|---------------|
| JWT Middleware | HTTP Header | gin.Context | user_id, employee_id, role |
| AgentService | gin.Context | AgentInput | SessionID, UserID, EmployeeID, Role, Message |
| AgentRunner | AgentInput | SSE events | 加载 State + 历史消息 + 执行 Graph + 保存 |
| Root dispatcher | AgentInput | *Message | intent route + userContextKey |
| Reimb parent | []*Message | *Message | ReimbursementState (跨阶段共享) |
| Phase guard | *Message (END输出) | []*Message | PhaseXTurns++, GuardResult |
| ReAct phase | []*Message | *Message | phaseState.Messages (子图私有消息历史) |
| ReAct ChatNode | []*Message | *Message | SystemPrompt + ToolCall or Content |
| ReAct ToolsNode | *Message | []*Message | ToolResults (每条tool一个消息) |
| SessionStore | — | — | Messages(MySQL+Redis) + State(MySQL) |

---

## 4. 消息历史在各阶段的生命周期

```
请求 N:
  SessionStore.GetHistory("abc") → [msg1, msg2, msg3, ...]
    │
    ├── 这些消息被加载，但 ⚠️ 不直接注入 Graph
    │
    ▼
  Graph 执行:
    ReAct 子图内部通过 phaseState.Messages 累积本轮消息
    ├── 第1次 ChatModel: [SystemPrompt, userMsg]
    ├── 第2次 ChatModel: [SystemPrompt, userMsg, assistant(tool_call), tool_result]
    └── 第N次 ChatModel: [SystemPrompt, userMsg, ...all previous...]
    
    ⚠️ phaseState 在子图每次被重新进入时创建新的空实例
    → Guard 重试会导致消息历史丢失
```

---

## 5. 已识别的数据流缺陷

| ID | 缺陷 | 影响 | 位置 |
|----|------|------|------|
| D1 | phaseState.Messages 在 Guard 重试时丢失 | LLM 丢失上一轮的对话上下文 | react_phase.go:150 |
| D2 | saveSessionState 无法读取嵌套图 State | State 未实际持久化 | runner.go saveSessionState() |
| D3 | dispatchToWorkflow 将 *Message 传递给 []*Message 类型子图 | 类型桥接依赖 agentInputAdapter | root.go:91 |
| D4 | 工具调用事件 (tool_call/tool_result) 未通过 SSE 发送 | 前端无法展示工具执行进度 | — |
