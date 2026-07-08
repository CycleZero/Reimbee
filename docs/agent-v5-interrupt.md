# Reimbee 中断机制设计

## 1. 设计目标

LLM 决定调用提交类工具时，**在工具实际执行前**被系统拦截，用户通过 GUI 显式确认后再继续执行。

关键约束：
- 确认动作来自用户 GUI，不是 agent 对话
- 工具调用前拦截，不是调用后
- 拦截期间会话状态完整保存，可恢复

## 2. 核心原理

Blades 执行模型：

```
model.Generate() → ToolPart 消息 → yield() → executeTools() → tool result → model.Generate() → ...
```

yield 和 executeTools 之间是唯一的拦截窗口。通过 **ActionLoopExit** 机制在工具执行后立即退出循环，工具本身在确认未完成时只做标记不执行实际操作。

## 3. 架构分层

```
┌─────────────────────────────────────────────────────┐
│ 工具层（tools/）                                      │
│   Handle() 内部检查 Session.State["approval"]        │
│   未确认 → SetAction("await_approval") + 返回 pending │
│   已确认 → 执行实际提交逻辑                            │
└────────────────────┬────────────────────────────────┘
                     │ msg.Actions["await_approval"]
┌────────────────────▼────────────────────────────────┐
│ 业务层（biz.go）                                      │
│   Run() 检测 msg.Actions → SSE interrupted → 保存    │
│   HandleApprove() → 写 state → 重新 Run              │
└─────────────────────────────────────────────────────┘
```

## 4. 工具层实现

### 4.1 submit_reimbursement_tool（典型示例）

```go
func (t *SubmitReimbTool) Handle(ctx context.Context, input string) (string, error) {
    var req SubmitReimbInput
    json.Unmarshal([]byte(input), &req)

    tc, _ := tools.FromContext(ctx)

    // 检查是否已通过审批
    approved := t.getApprovalStatus(ctx)

    if !approved {
        // 第一阶段：请求审批，不执行实际操作
        tc.SetAction("await_approval", "确认提交报销单？金额："+formatAmount(req.TotalAmount))
        tc.SetAction(tools.ActionLoopExit, true)
        return `{"status":"pending_approval"}`, nil
    }

    // 第二阶段：审批通过，执行提交
    rm, err := t.reimbursementBiz.Submit(req.ReimbursementID)
    if err != nil {
        return "", err
    }
    return json.Marshal(SubmitReimbOutput{
        Status:            rm.Status,
        ReimbursementNo:   rm.ReimbursementNo,
        NeedSpecialApproval: rm.NeedSpecialApproval,
    })
}
```

### 4.2 通用中断模式

任何需要用户确认的工具都遵循同一模式：

```
func ToolHandle(ctx, input):
    if 未确认:
        SetAction("await_approval", "提示文案")
        SetAction(ActionLoopExit, true)
        return {"status":"pending"}

    已确认，执行实际操作:
        return 实际结果
```

## 5. 业务层实现

### 5.1 Run() — 检测中断

```go
func (a *ReimburseAgent) Run(ctx context.Context, params RunParams, writer *GinSSEWriter) error {
    session := GetOrCreate(ctx, params.SessionID, a.repo)
    session.InjectUser(...)

    stream := a.runner.RunStream(ctx, msg, blades.WithSession(session))

    for msg, err := range stream {
        switch msg.Role {
        case blades.RoleTool:
            // 检测中断信号
            if reason, ok := msg.Actions["await_approval"]; ok {
                writer.WriteEvent(NewInterruptedEvent(fmt.Sprint(reason)))
                writer.Flush()
                a.repo.Save(ctx, session.Snapshot())
                return nil  // 退出循环，等待审批恢复
            }
            // 正常工具结果
            for _, part := range msg.Parts {
                if tp, ok := any(part).(blades.ToolPart); ok && tp.Completed {
                    writer.WriteEvent(NewToolResultEvent(tp.Name, tp.Response))
                    writer.Flush()
                }
            }
        }
    }
    // ...
}
```

### 5.2 HandleApprove — 恢复执行

```go
func (a *ReimburseAgent) HandleApprove(ctx context.Context, sessionID string, approved bool, reason string, writer *GinSSEWriter) error {
    // 写入审批结果到 Session State
    session := GetOrCreate(ctx, sessionID, a.repo)
    session.SetState("approval", map[string]any{
        "approved": approved,
        "reason":   reason,
    })
    a.repo.Save(ctx, session.Snapshot())

    // 以 continue 消息重新运行 Agent
    // 工具将检测到 approval 状态并执行第二阶段
    return a.Run(ctx, RunParams{
        SessionID: sessionID,
        Message:   "继续",
    }, writer)
}
```

## 6. SSE 事件流

### 6.1 正常流程

```
event: thinking
data: {"text":"正在处理..."}

event: message
data: {"text":"好的，我来","delta":true}
...

event: done
data: {}
```

### 6.2 中断流程

```
event: thinking
data: {"text":"正在处理..."}

event: message
data: {"text":"正在提交审批","delta":true}
...

event: interrupted
data: {"reason":"确认提交报销单？金额：1,234.56元"}

// 流结束，前端显示确认按钮
```

### 6.3 恢复流程

```
POST /api/chat/approve
Body: {"session_id":"xxx","approved":true,"reason":"确认"}

event: thinking
data: {"text":"正在处理..."}

event: tool_result
data: {"name":"submit_reimbursement","output":"{\"status\":\"submitted\"}"}

event: message
data: {"text":"报销单已提交成功","delta":false}

event: done
data: {}
```

## 7. 文件清单

| 文件 | 变更 |
|------|------|
| `tools/submit_reimb_tool.go` | Handle 内检查审批状态，未确认时设 ActionLoopExit |
| `agent/biz.go` | Run() 检测 msg.Actions["await_approval"] → interrupted 事件 + 保存退出 |
| `agent/biz.go` | 新增 HandleApprove() 方法 |
| `agent/service.go` | 新增 HandleApprove HTTP 端点 |
| `agent/sse.go` | 已有 NewInterruptedEvent |
| `router/root.go` | 已有 /chat/approve 路由 |

## 8. 为什么不用中间件

Blades 中间件拦截点在 yield 之后：

```
中间件能看到 ToolPart → 但此时 executeTools() 即将执行，无法阻止
```

工具自身 Handle 是唯一能决定"执行还是不执行"的地方。两阶段提交模式（工具内部分支 + ActionLoopExit）是最小侵入方案。

## 9. 状态管理

```
session_states 表（已有 SessionRepo.State 机制）:
  sessionID:approval → {"approved":true,"reason":"确认"}

工具通过 Session.State["approval"] 读取审批结果
HandleApprove 通过 Session.SetState 写入审批结果
```
