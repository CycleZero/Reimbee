# Agent 层详细设计 v2.0 — 流程编排架构

> 版本: v2.2 | 日期: 2026-07-04 | 状态: 设计评审
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

### 3.1 混合架构：阶段护栏 + 阶段内 Agent 自主

```
┌─────────────────────────────────────────────────────────┐
│                   Graph 定义的阶段护栏                     │
│                                                         │
│  ┌──────────┐     ┌──────────┐     ┌──────────┐        │
│  │ 信息收集  │ ──▶ │ 校验确认  │ ──▶ │ 执行提交  │        │
│  │ Phase 1  │     │ Phase 2  │     │ Phase 3  │        │
│  └────┬─────┘     └────┬─────┘     └────┬─────┘        │
│       │                │                │               │
│  Guard: 票据        Guard: 合规      Guard: 用户        │
│  信息完整+确认       +预算通过        最终确认           │
│                                                         │
│  ┌────┴────────────────┴────────────────┴────┐          │
│  │         每个阶段内部：Agent 自主决策           │          │
│  │                                             │          │
│  │  ┌─────────┐  ┌─────────┐  ┌─────────────┐ │          │
│  │  │ LLM 对话 │  │ 工具调用  │  │ 阶段内路由   │ │          │
│  │  │ 理解意图 │  │ 选择调用  │  │ 多轮交互    │ │          │
│  │  └─────────┘  └─────────┘  └─────────────┘ │          │
│  └─────────────────────────────────────────────┘          │
└─────────────────────────────────────────────────────────┘
```

**核心思想**：Graph 定义"什么阶段"（宏观不可变），Agent 决定"怎么完成"（微观可自主）。

### 3.2 三阶段架构

```
Phase 1: 信息收集                  Phase 2: 校验确认              Phase 3: 执行提交
─────────────────                 ─────────────────             ─────────────────
Agent 可用工具:                    Agent 可用工具:                Agent 可用工具:
├── recognize_invoice (OCR)        ├── check_compliance           ├── generate_pdf
├── check_compliance (查规则)       ├── check_budget               ├── send_email
│                                  │                              └── query_progress
LLM 自主决策:                      LLM 自主决策:
├── 用户传图 → 调 OCR              ├── 自动调合规检查              LLM 自主决策:
├── 用户说金额 → 跳过 OCR          ├── warning → 展示+询问         ├── PDF 成功 → 自动发邮件
├── 用户问"住宿标准" → 查规则       ├── 预算不足 → 告知选项          ├── 发邮件失败 → 告知但已完成
├── 用户中途加票 → 继续收集         ├── 用户问细节 → 展开规则        └── 用户问后续 → 告知审批人
└── 用户取消 → 结束流程             └── 用户确认 → 最终确认
                                   │
                                   Guard: 合规通过 + 预算充足
                                   + 用户最终确认
                                   │
Guard: 信息完整 + 用户确认          │
        │                          │
        └──────────▶───────────────┘
```

### 3.3 阶段过渡的护卫条件

每个阶段结束时，Graph 检查护卫条件——不满足则拒绝进入下一阶段：

```go
// Phase 1 → Phase 2 的护卫条件
// 核心原则：票据图片是审计依据，必须有至少一张图片上传
func phase1Guard(state *ReimbursementState) (bool, string) {
    // 1. 必须有至少一张票据图片（审计凭证）
    if len(state.Invoices) == 0 {
        return false, "请至少上传一张票据图片作为报销凭证"
    }
    
    // 2. 每张票据必须有金额（OCR 自动填充 或 用户手动输入）
    for i, inv := range state.Invoices {
        if inv.Amount <= 0 {
            return false, fmt.Sprintf("第 %d 张票据缺少金额，请补充", i+1)
        }
    }
    
    // 3. 用户必须确认过票据信息
    if !state.UserConfirmed {
        return false, "请先确认票据信息"
    }
    
    return true, ""
}

// Phase 2 → Phase 3 的护卫条件
func phase2Guard(state *ReimbursementState) (bool, string) {
    if state.ComplianceResult == nil {
        return false, "尚未完成合规检查"
    }
    if state.BudgetResult == nil {
        return false, "尚未完成预算检查"
    }
    if state.ComplianceResult.Level == "error" {
        return false, state.ComplianceResult.Message
    }
    if !state.FinalConfirmed {
        return false, "请先确认提交（此操作不可撤销）"
    }
    return true, ""
}
```

### 3.4 Phase 1 详细设计：信息收集 Agent

**核心约束**：**票据图片是报销的法定凭证，必须上传。** OCR 识别是辅助填单的加速手段，不是必选路径。

```
用户进入 Phase 1
       │
       ▼
┌──────────────────────────────────────┐
│        Phase 1 Agent                 │
│                                      │
│  可用工具: recognize_invoice (OCR)     │
│           check_compliance (规则查询)  │
│                                      │
│  图片是必须的，OCR 是可选的:            │
│  ┌──────────────────────────────────┐│
│  │ 用户: "报销一张1500的差旅发票"      ││
│  │ → Agent: "请先上传发票图片"         ││
│  │ → 口头说金额不能代替凭证             ││
│  └──────────────────────────────────┘│
│  ┌──────────────────────────────────┐│
│  │ 用户: [上传 photo.jpg]             ││
│  │ → 图片已上传 → 调 OCR（自动加速）    ││
│  │ → 成功: "识别到机票 ¥2,580, 正确吗?"││
│  │ → 失败: "图片不够清晰，但已保存。     ││
│  │          请手动输入金额和类别"        ││
│  └──────────────────────────────────┘│
│  ┌──────────────────────────────────┐│
│  │ 用户: "等等，再加一张餐费发票"       ││
│  │ → "好的，请上传餐费发票"             ││
│  │ → 必须上传图片，不能口头说           ││
│  └──────────────────────────────────┘│
│  ┌──────────────────────────────────┐│
│  │ 用户: "住宿标准是多少？"            ││
│  │ → 查规则 → 回答，同时提醒"请上传票据" ││
│  └──────────────────────────────────┘│
│                                      │
│  退出 Guard 条件:                     │
│  ① ≥1 张票据图片已上传（硬性）          │
│  ② 每张票据有金额（OCR 或手动）         │
│  ③ 用户确认了票据信息                  │
└──────────────────────────────────────┘
       │
       ▼
  Graph Guard 检查
       │
       ▼
  Phase 2
```

### 3.5 OCR 数据完整性：凭证链与审批留痕

#### 3.5.1 核心原则

```
OCR 机器读取值 = 客观基准
用户输入值     = 人工修正（可能偏离凭证）
审批人         = 最终裁决者，可选择采纳 OCR 值或用户修正值
```

**为什么这么设计**：
- 用户可能因 OCR 识别错误而修正金额——这是正当行为，但不能悄无声息
- 审批人需要看到"机器读的是什么"和"用户改成了什么"，才能做出判断
- 如果用户改动了金额但没有标注原因，审批人应该能发现并质疑

#### 3.5.2 数据模型扩展

`InvoiceItem` 模型需要额外字段：

```go
// OCR 原始值与用户修正值——完整审计链
type InvoiceItem struct {
    // ... 原有字段 ...
    
    // OCR 识别结果（客观基准）
    OCRRawAmount    int64  `json:"ocr_raw_amount"`     // OCR 读到的原始金额（分）
    OCRRawDate      string `json:"ocr_raw_date"`       // OCR 读到的原始日期
    OCRRawCategory  string `json:"ocr_raw_category"`   // OCR 推断的原始类别
    OCRConfidence   float64 `json:"ocr_confidence"`    // OCR 识别置信度
    
    // 用户修正标记
    IsUserModified  bool   `json:"is_user_modified"`   // 用户是否修改了 OCR 结果
    ModificationNote string `json:"modification_note"`  // 用户填写的修改原因
    
    // 最终审批决定
    ApproverChoice  string `json:"approver_choice"`    // "ocr" | "user" | null（审批时填写）
}
```

**关键设计**：`Amount` 字段始终存用户确认后的值。OCR 原始值独立存储在 `OCRRawAmount` 中。审批人可以通过比较这两个字段发现差异。

#### 3.5.3 Phase 1 Agent 的修正处理流程

```
用户上传发票 → OCR 识别 → Agent 展示结果

┌──────────────────────────────────────────────────┐
│ 正常情况（OCR 值 = 最终值）:                       │
│                                                   │
│ Agent: "识别到机票 ¥2,580，开票日期 2026-07-01，   │
│         销售方: 中国国际航空。正确吗？"              │
│ 用户: "对"                                         │
│ → Amount = 258000, OCRRawAmount = 258000           │
│ → IsUserModified = false                           │
└──────────────────────────────────────────────────┘

┌──────────────────────────────────────────────────┐
│ 用户修正（OCR 值 ≠ 最终值，需留痕）:                │
│                                                   │
│ Agent: "识别到金额 ¥2,580"                         │
│ 用户: "不对，金额应该是 ¥2,380"                     │
│ Agent: "已将金额修正为 ¥2,380。请简要说明修正原因"   │
│ 用户: "实际支付金额与票面不一致"                      │
│ → Amount = 238000, OCRRawAmount = 258000           │
│ → IsUserModified = true                            │
│ → ModificationNote = "实际支付金额与票面不一致"       │
└──────────────────────────────────────────────────┘

┌──────────────────────────────────────────────────┐
│ OCR 失败，用户手动输入:                             │
│                                                   │
│ Agent: "图片不够清晰，识别失败。请手动输入信息"       │
│ 用户: "金额 1500，差旅交通费"                        │
│ → Amount = 150000, OCRRawAmount = 0                │
│ → OCRConfidence = 0                                │
│ → IsUserModified = true (手动输入等同于修正)          │
│ → ModificationNote = "OCR 识别失败，手动输入"        │
└──────────────────────────────────────────────────┘
```

#### 3.5.4 Phase 2 合规检查的修正感知

合规检查时，Agent 需要将"用户修正"作为风险因素纳入考量：

```
Phase 2 Agent 调用 check_compliance:
    检查结果: pass
    但有修正记录:
    
Agent 告知用户:
"✅ 合规检查通过。但请注意，以下票据的金额经您手动修正，
  审批人将看到 OCR 原始值与修正值的差异：
  
  票据 1: 机票
    OCR 识别: ¥2,580 → 您修正为: ¥2,380
    修正原因: 实际支付金额与票面不一致
  
  审批人可能会就修正项向您确认。"
```

#### 3.5.5 PDF 报销单中的修正展示

```
┌─────────────────────────────────────────────────┐
│               费用报销单                          │
│                                                  │
│  票据明细                                        │
│ ┌──────┬──────────┬──────────┬──────────┬──────┐│
│ │ 序号  │ 费用类别  │ OCR 金额  │ 申报金额  │ 备注  ││
│ ├──────┼──────────┼──────────┼──────────┼──────┤│
│ │  1   │ 差旅-交通  │ ¥2,580   │ ¥2,580   │  —   ││
│ ├──────┼──────────┼──────────┼──────────┼──────┤│
│ │  2   │ 办公用品   │ ¥1,500   │ ¥1,200   │ ⚠️   ││
│ │      │          │          │          │ 修正  ││
│ └──────┴──────────┴──────────┴──────────┴──────┘│
│                                                  │
│  ⚠️ 票据 2 经申报人修正:                          │
│     OCR 识别: ¥1,500                             │
│     申报金额: ¥1,200                             │
│     修正原因: 实际支付金额与票面不一致               │
│                                                  │
│  审批人裁决（请勾选）:                             │
│     □ 采纳申报金额 ¥1,200                         │
│     □ 采纳 OCR 金额 ¥1,500                       │
└─────────────────────────────────────────────────┘
```

#### 3.5.6 审批人对修正项的处理

审批人查看报销单时，对于 `IsUserModified = true` 的票据：

```
审批人看到的选项:
  
  票据 2: 办公用品
  ┌─────────────────────────────────────┐
  │ OCR 识别金额: ¥1,500                │
  │ 申报人修正为: ¥1,200                │
  │ 修正原因: 实际支付金额与票面不一致      │
  │                                     │
  │ 请选择批准的金额:                     │
  │ ○ 采纳申报金额 ¥1,200               │
  │ ○ 采纳 OCR 金额 ¥1,500             │
  │ ○ 驳回，请申报人说明                 │
  └─────────────────────────────────────┘
  
  → 审批人选择后，ApproverChoice 字段记录
  → 若选择驳回，报销单回到 rejected 状态
```

---

### 3.6 Phase 2 详细设计：校验确认 Agent

```
进入 Phase 2
       │
       ▼
┌──────────────────────────────────────┐
│        Phase 2 Agent                 │
│                                      │
│  可用工具: check_compliance (校验)     │
│           check_budget               │
│                                      │
│  修正感知: 如有 IsUserModified=true    │
│  的票据，Agent 在合规结果中额外标注     │
│                                      │
│  LLM 决策示例:                        │
│  ┌──────────────────────────────────┐│
│  │ 合规 pass + 无修正                ││
│  │ → "✅ 合规检查通过"               ││
│  │ → 自动进入预算检查                 ││
│  └──────────────────────────────────┘│
│  ┌──────────────────────────────────┐│
│  │ 合规 pass + 有修正（新增风险提示）   ││
│  │ → "合规检查通过。⚠️ 您修正了      ││
│  │    1 项票据金额，审批人将对此复核。"  ││
│  │ → 引导进入预算检查                 ││
│  └──────────────────────────────────┘│
│  ┌──────────────────────────────────┐│
│  │ 合规检查 warning: 住宿超标          ││
│  │ → "⚠️ 住宿费 ¥350 超出标准 ¥300   ││
│  │    超出的 ¥50 需审批人确认"         ││
│  │ → 询问用户: "是否继续提交？"        ││
│  │ → 用户确认 → 继续                  ││
│  │ → 用户拒绝 → 回 Phase 1 修改       ││
│  └──────────────────────────────────┘│
│  ┌──────────────────────────────────┐│
│  │ 预算不足，触发特殊审批              ││
│  │ → "⚠️ 部门预算仅剩 ¥500，          ││
│  │    本次报销 ¥1,500 超出预算，       ││
│  │    将触发特殊审批流程。是否继续？"    ││
│  └──────────────────────────────────┘│
│                                      │
│  退出条件: 合规通过 + 预算可接受        │
│           + 用户最终确认               │
└──────────────────────────────────────┘
       │
       ▼
  最终确认（不可逆操作前最后一道防线）
       │
       ▼
  Phase 3
```

### 3.7 Phase 3 详细设计：执行提交 Agent

```
进入 Phase 3
       │
       ▼
┌──────────────────────────────────────┐
│        Phase 3 Agent                 │
│                                      │
│  可用工具: generate_pdf               │
│           send_email                 │
│           query_progress             │
│                                      │
│  LLM 决策示例:                        │
│  ┌──────────────────────────────────┐│
│  │ 正常流程                          ││
│  │ → 调 generate_pdf                ││
│  │ → 调 send_email                  ││
│  │ → "报销单 REIMB-2026-0001 已提交"  ││
│  └──────────────────────────────────┘│
│  ┌──────────────────────────────────┐│
│  │ PDF 生成成功，邮件发送失败          ││
│  │ → "报销单已生成，但邮件发送失败"     ││
│  │ → "稍后可以手动通知审批人"          ││
│  │ → 不做自动重试（由用户决定）         ││
│  └──────────────────────────────────┘│
│  ┌──────────────────────────────────┐│
│  │ 用户: "审批大概多久？"              ││
│  │ → 不属于 Phase 3 范围              ││
│  │ → 但可以告诉用户审批人信息           ││
│  └──────────────────────────────────┘│
│                                      │
│  退出条件: PDF 已生成（邮件可失败）     │
└──────────────────────────────────────┘
       │
       ▼
      END
```

### 3.8 阶段切换 vs 阶段内路由

| 谁决定 | 什么 | 机制 |
|-------|------|------|
| **Graph** | 阶段之间的切换 | `graph.AddEdge(phase1, guard → phase2)` |
| **Graph** | 护卫条件（不满足拒绝切换） | `graph.AddBranch(phase, guardFn, {nextPhase, currentPhase})` |
| **Agent** | 阶段内调用哪个工具 | ChatModelAgent 的 tool_choice |
| **Agent** | 阶段内如何回应用户 | LLM 自然语言生成 |
| **Agent** | 阶段内是否多轮交互 | LLM 判断是否需要更多信息 |

### 3.9 对比：纯编排 vs 混合架构

| 场景 | 纯编排 (v2.0) | 混合架构 (v2.1) |
|------|:---:|:---:|
| 用户说了金额，跳过 OCR | ❌ 必走 OCR 节点 | ✅ Agent 决定，但**必须上传图片** |
| 用户中途想加一张票 | ❌ 无法回退 | ✅ Agent 继续留在 Phase 1 |
| 合规 warning，用户确认继续 | ❌ 僵化的 yes/no | ✅ Agent 自然对话引导 |
| 用户问"标准是多少" | ❌ 不在流程中，无法回答 | ✅ Agent 调 check_compliance 查规则 |
| 合规 error，引导修改 | ✅ 可以 | ✅ 可以 |
| 提交前必须合规+预算检查 | ✅ Graph 保证 | ✅ Graph Guard 保证 |
| 不可逆操作前必须确认 | ✅ 硬编码节点 | ✅ Guard 检查 FinalConfirmed 标志 |

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

### 3.10 完整对话生命周期：提交之后

Phase 3 提交报销单只是"员工侧动作"的终点，不是业务的终点。提交之后是审批流转，Agent 需要覆盖**驳回→重提**和**进度追踪→等待通知**两条后续路径。

**生命周期全景**：

```
                              ┌─────────────────────────────┐
                              │       完整对话生命周期        │
                              └─────────────────────────────┘
                                             │
                      ┌──────────────────────┼──────────────────────┐
                      ▼                      ▼                      ▼
              ┌──────────────┐      ┌──────────────┐      ┌──────────────┐
              │ 发起报销      │      │ 进度追踪      │      │ 政策咨询      │
              │ Phase 1→2→3  │      │              │      │              │
              └──────┬───────┘      └──────┬───────┘      └──────────────┘
                     │                     │
                     ▼                     │
              ┌──────────────┐             │
              │ 提交成功      │             │
              │ "已发送给审批人"│◀────────────┤
              └──────┬───────┘             │
                     │                     │
          ┌──────────┼──────────┐          │
          ▼          ▼          ▼          │
    ┌─────────┐ ┌───────┐ ┌─────────┐     │
    │ 审批通过  │ │ 驳回   │ │ 用户主动 │     │
    │         │ │       │ │ 查进度   │─────┘
    └────┬────┘ └───┬───┘ └─────────┘
         │          │
         ▼          ▼
    ┌─────────┐ ┌──────────────┐
    │ 通知用户  │ │ Agent 告知   │
    │ "已通过"  │ │ 驳回原因      │
    └─────────┘ │ "是否修改重提?"│
                └──────┬───────┘
                       │ 用户选择重提
                       ▼
                回到 Phase 1
                (从已驳回的报销单恢复)
```

**关键设计决策**：系统不主动推送通知（超出范围），Agent 在用户下次对话时感知状态变化。

### 3.11 进度追踪子流程（query_progress）

```
用户: "我的报销批了吗"
        │
        ▼
┌──────────────────────────────────────┐
│       Query Progress Agent            │
│                                      │
│  可用工具: query_progress             │
│           query_reimbursements        │
│                                      │
│  LLM 决策示例:                        │
│  ┌──────────────────────────────────┐│
│  │ 用户: "REIMB-2026-0001 到哪了"    ││
│  │ → 调 query_progress               ││
│  │ → 返回审批链状态                   ││
│  │ → "当前在【财务复核】阶段，         ││
│  │    已通过: 部门主管张三"            ││
│  └──────────────────────────────────┘│
│  ┌──────────────────────────────────┐│
│  │ 用户: "我最近提交的报销怎么样了"    ││
│  │ → 调 query_reimbursements          ││
│  │ → 返回最近 5 条报销单              ││
│  │ → "您最近有 3 条报销:               ││
│  │    ① REIMB-2026-0001 审批中        ││
│  │    ② REIMB-2026-0002 已通过        ││
│  │    ③ REIMB-2026-0003 已驳回"       ││
│  └──────────────────────────────────┘│
└──────────────────────────────────────┘
```

### 3.12 驳回重提路径

```
用户: "REIMB-2026-0003 被驳回了，为什么"

Agent 调 query_progress
    │
    ▼
"报销单 REIMB-2026-0003 被【财务复核李四】驳回。
 驳回原因: 发票金额与系统记录不一致，请核实后重新提交。"
    │
    ├── 用户: "帮我改一下金额重新提交"
    │        │
    │        ▼
    │   Agent 识别意图: modify_reimbursement
    │        │
    │        ▼
    │   ① 调 query_progress → 获取原报销单信息
    │   ② "原报销单信息: 交通费 ¥1,500, 开票日期 2026-07-01"
    │   ③ "请告诉我需要修改的内容"
    │   ④ 用户: "金额改成 1200"
    │   ⑤ 更新报销单（draft 状态重新进入 Phase 1）
    │   ⑥ 走完整三阶段流程重新提交
    │
    └── 用户: "知道了，我找财务确认一下"
             │
             ▼
        "好的，有问题随时找我。"
```

### 3.13 Agent 的状态感知能力

Agent 不能被动等用户问——在对话开始时，如果 Session 中有未完结的报销单，**主动告知状态**：

```
用户: 打开聊天（新 Session 或恢复 Session）
        │
        ▼
Agent 检查: Session 上下文中是否有进行中的报销单？
        │
        ├── 有 → "您有一笔报销单 REIMB-2026-0001 正在审批中。
        │         当前在【财务复核】阶段。需要查看详情吗？"
        │
        └── 无 → 正常等待用户指令
```

**实现方式**：`IntentClassify` 节点在分类意图前，先检查 Session 中是否存在未完结的报销单（通过 Checkpoint 中的 `ReimbursementID` + 查数据库确认状态），如有则优先引导用户关注。

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
// SessionStore 会话持久化接口
// 调用方只需要"存消息、取历史、清会话"，不关心底层是 MySQL 还是 Redis
type SessionStore interface {
    // SaveMessage 保存一条消息到会话（持久化到 MySQL，同时更新 Redis 热缓存）
    SaveMessage(ctx context.Context, sessionID string, msg Message) error

    // GetHistory 获取会话最近的消息（先查缓存，未命中回源 MySQL）
    GetHistory(ctx context.Context, sessionID string, limit int) ([]Message, error)

    // Clear 清除会话所有消息（支持报销完成后清理，实现层负责 MySQL DELETE + Redis DEL）
    Clear(ctx context.Context, sessionID string) error
}

type Message struct {
    ID        int64     `json:"id"`
    SessionID string    `json:"session_id"`
    Role      string    `json:"role"`    // "user" | "assistant"
    Content   string    `json:"content"`
    Timestamp time.Time `json:"timestamp"`
}
```

**接口设计原则**：

| 原则 | 体现 |
|------|------|
| 动词一致 | Save / Get / Clear，全 CRUD 风格，无 business-specific 命名 |
| 单一职责 | 每个方法只做一件事。`Clear` 内部处理 MySQL DELETE + Redis DEL，调用方不感知 |
| 不泄露实现 | 接口注释提到 MySQL/Redis 仅作为文档参考，调用方不需要知道缓存层 |
| 对称性 | Save → Get → Clear 形成完整的生命周期闭环 |

### 5.5 MySQL 实现（默认）

```go
type MySQLSessionStore struct {
    db    *gorm.DB
    cache *RedisSessionCache  // 可选，nil 时降级纯 MySQL
}

func (s *MySQLSessionStore) SaveMessage(ctx context.Context, sessionID string, msg Message) error {
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
        _ = s.cache.Prepend(ctx, sessionID, msg)
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

// Prepend 追加单条消息到缓存列表头部
func (c *RedisSessionCache) Prepend(ctx context.Context, sessionID string, msg Message) error {
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
    a.session.SaveMessage(ctx, sessionID, Message{Role: "user", Content: userMsg})

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
    a.session.SaveMessage(ctx, sessionID, Message{Role: "assistant", Content: fullResponse})

    // 5. 如果报销已完成，清理会话
    if eventIndicatesCompletion(iter.LastEvent()) {
        a.session.Clear(ctx, sessionID)
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
├── runner.go                # AgentRunner（Graph 编译 + SSE 流式）
├── prompt.go                # 三阶段各自 Prompt 模板
├── session.go               # SessionStore（MySQL + Redis 缓存）
├── dto.go                   # Message, PhaseState, GuardResult 等
│
├── graph/                   # Graph 定义
│   ├── root.go              # 顶层 Graph：意图分类 → 路由 → 子流程入口
│   ├── reimbursement.go     # 报销子流程：Phase1 → Guard → Phase2 → Guard → Phase3
│   ├── progress.go          # 进度查询（简单 ChatModelAgent，2 个工具）
│   ├── budget.go            # 预算查询（简单 ChatModelAgent，1 个工具）
│   ├── policy.go            # 政策咨询（LLM 直接回复，无工具）
│   └── modify.go            # 修改报销（简单 ChatModelAgent）
│
├── phase/                   # 三阶段 Agent 定义
│   ├── phase1_collect.go    # 信息收集 Agent (tools: OCR + 规则查询)
│   ├── phase2_validate.go   # 校验确认 Agent (tools: 合规 + 预算)
│   ├── phase3_execute.go    # 执行提交 Agent (tools: PDF + 邮件 + 进度)
│   └── guard.go             # 阶段护卫条件函数
│
└── tools/                   # 7 个 Tool 定义
    ├── provider.go          # ToolProviderSet + ToolSet 聚合
    ├── ocr_tool.go          # recognize_invoice
    ├── compliance_tool.go   # check_compliance
    ├── budget_tool.go       # check_budget
    ├── pdf_tool.go          # generate_pdf
    ├── email_tool.go        # send_email
    ├── progress_tool.go     # query_progress
    └── query_tool.go        # query_reimbursements
```

---

## 八、架构演进对比

| 维度 | v1.0 ChatModelAgent | v2.0 纯 Graph 编排 | v2.2 混合架构（当前） |
|------|:---:|:---:|:---:|
| **流程控制** | LLM 全权决定 | Graph 硬编码每步 | Graph 管阶段, Agent 管阶段内 |
| **合规保证** | ❌ 可能跳过 | ✅ 必经节点 | ✅ Guard 强制 |
| **灵活性** | ✅ 极高 | ❌ 死板 | ✅ 阶段内 LLM 自主 |
| **跳过 OCR** | ✅ 可以 | ❌ 不能 | ✅ Agent 决定 |
| **中途加票** | ✅ 可以 | ❌ 不能回退 | ✅ Agent 留在 Phase 1 |
| **自然对话感** | ✅ 好 | ❌ 僵化 | ✅ 好 |
| **可审计性** | ❌ 路径不定 | ✅ 完全可追溯 | ✅ 阶段级可追溯 |
| **代码复杂度** | 低 | 高（9 个节点+路由） | 中（3 阶段+Guard） |
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
| **P1** | 顶层 Graph + 意图分类 + 路由 | 能识别意图并路由到子流程 |
| **P2** | Phase 1 Agent（信息收集）+ Guard | 能收集票据信息并确认 |
| **P3** | Phase 2 Agent（校验确认）+ Phase 3（执行提交） | 核心报销流程可用 |
| **P4** | 进度查询、预算查询子流程 | 辅助功能 |
| **P5** | SSE 流式端点 + 前端联调 | 完整端到端 |

---

*文档结束*
