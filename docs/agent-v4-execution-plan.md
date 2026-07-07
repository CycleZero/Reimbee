# Agent 层 v4.0 — 详细执行方案

> **日期**: 2026-07-07  
> **前置阅读**: [agent-v4-design.md](./agent-v4-design.md)（必读）  
> **原则**: 每个步骤可独立编译、可验证、可回滚  
> **总计**: 6 个 Phase，36 个步骤

---

## 目录

1. [总览：变更地图](#1-总览变更地图)
2. [Phase 1: Session 持久化重构](#2-phase-1-session-持久化重构)
3. [Phase 2: 合规审核 AgentTool](#3-phase-2-合规审核-agenttool)
4. [Phase 3: Agent 层简化](#4-phase-3-agent-层简化)
5. [Phase 4: Interrupt 确认机制](#5-phase-4-interrupt-确认机制)
6. [Phase 5: 清理、Wire、路由](#6-phase-5-清理wire路由)
7. [Phase 6: 全面测试](#7-phase-6-全面测试)
8. [回滚策略](#8-回滚策略)

---

## 1. 总览：变更地图

```
操作统计:
  新增文件:  5 个
  重写文件:  8 个
  修改文件:  6 个
  删除文件:  3 个
  不变文件:  ~18 个
```

### 文件级变更清单

```
━━━ 新增 ━━━
model/session_meta.go
infra/session_meta_repo.go
internal/domain/agent/tools/search_policy_tool.go
internal/domain/agent/tools/compliance_agent_tool.go
docs/agent-v4-design.md                              (✓ 已完成)
docs/agent-v4-execution-plan.md                      (✓ 本文档)

━━━ 重写 ━━━
model/session_message.go                             → 结构化字段
infra/session_mysql.go                               → meta + messages 分表
infra/session_redis.go                               → 适配新格式
internal/domain/agent/phase_agents.go                → 单 Agent 创建
internal/domain/agent/prompt.go                      → 单系统 Prompt
internal/domain/agent/session_loop.go                → 简化 PrepareAgent + 新增 GenResume
internal/domain/agent/tools/provider.go              → ToolSet 简化
internal/domain/agent/tools/submit_reimb_tool.go     → 内建 Interrupt

━━━ 修改 ━━━
internal/domain/agent/loop_manager.go                → 单 Agent 持有 + Checkpoint 启用
internal/domain/agent/service.go                     → HandleApprove 端点
internal/domain/agent/config.go                      → 新增配置项
internal/domain/agent/sse.go                         → Interrupted 事件类型
internal/domain/agent/dto.go                         → 移除废弃类型
internal/domain/agent/provider.go                    → Wire 更新
internal/domain/agent/types/state.go                 → 简化 ReimbursementState
internal/router/root.go                              → /api/chat/approve 路由
infra/session.go                                     → 接口更新

━━━ 删除 ━━━
internal/domain/agent/tools/confirm_invoice_tool.go
internal/domain/agent/tools/confirm_submit_tool.go
internal/domain/agent/tools/compliance_tool.go
```

---

## 2. Phase 1: Session 持久化重构

> **目标**: 将当前单表 `session_messages` + `session_states` 重构为 `session_meta` + `session_messages` 双表。
> **原则**: 旧代码仍可编译，新表与旧表并存，最后切换引用后删除旧逻辑。

### Step 1.1 — 创建 SessionMeta 模型

**文件**: `model/session_meta.go` (新增)

```go
// Package model Session 元数据持久化模型
package model

import "time"

// SessionMeta 会话元数据
// 分离自 session_messages，存储会话级别信息（用户身份、状态、CheckpointID）
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

func (SessionMeta) TableName() string {
	return "session_meta"
}
```

**验证**: `go build ./model/...`

---

### Step 1.2 — 重构 SessionMessage 模型

**文件**: `model/session_message.go` (重写)

```go
package model

import "time"

// SessionMessage 会话消息明细（v4 重构）
// 拆分 RawJSON 为结构化字段：content / tool_name / tool_input / tool_output / message_meta
type SessionMessage struct {
	ID          uint      `gorm:"primaryKey;autoIncrement"`
	SessionID   string    `gorm:"type:varchar(36);index:idx_session_seq,priority:1;not null;comment:会话ID"`
	Seq         uint      `gorm:"index:idx_session_seq,priority:2;not null;comment:消息序号(会话内递增)"`
	Role        string    `gorm:"type:varchar(20);index:idx_session_role,priority:2;not null;comment:user/assistant/tool"`
	
	// ── 结构化内容 ──
	Content     *string   `gorm:"type:text;comment:消息文本(user=用户输入/assistant=LLM回复/tool=工具返回摘要)"`
	ToolName    string    `gorm:"type:varchar(64);not null;default:'';comment:工具名称(仅tool角色)"`
	ToolInput   *string   `gorm:"type:text;comment:工具输入参数JSON(仅tool角色)"`
	ToolOutput  *string   `gorm:"type:text;comment:工具输出结果JSON(仅tool角色)"`
	
	// ── Eino 框架元数据（仅框架消费，业务不查询）──
	MessageMeta *string   `gorm:"type:json;comment:Eino Message元数据(ToolCalls/ResponseMeta等)"`
	
	CreatedAt   time.Time `gorm:"autoCreateTime"`
}

func (SessionMessage) TableName() string {
	return "session_messages"
}
```

**与 v3 的兼容**：`RawJSON` 替换为 `MessageMeta`（内容更精准）。v3 的历史数据可保留 `RawJSON` 字段（新增字段不影响已有数据）。

**验证**: `go build ./model/...`

---

### Step 1.3 — 重写 infra/session.go 接口

**文件**: `infra/session.go` (修改)

```go
package infra

import (
	"context"
	"github.com/cloudwego/eino/schema"
)

// SessionStore 会话持久化接口（v4 重构）
type SessionStore interface {
	// ── 消息持久化 ──
	SaveMessages(ctx context.Context, sessionID string, msgs []*schema.Message) error
	GetHistory(ctx context.Context, sessionID string, limit int) ([]*schema.Message, error)
	Clear(ctx context.Context, sessionID string) error

	// ── v4 新增：会话元数据 ──
	CreateSession(ctx context.Context, meta *SessionMeta) error
	GetSession(ctx context.Context, sessionID string) (*SessionMeta, error)
	UpdateSession(ctx context.Context, sessionID string, updates map[string]any) error
	DeleteSession(ctx context.Context, sessionID string) error
	ListSessions(ctx context.Context, userID uint, status string) ([]*SessionMeta, error)

	// ── v4 保留：Checkpoint 状态（仅 Interrupt 恢复使用）──
	SaveCheckpointState(ctx context.Context, sessionID string, key string, state any) error
	GetCheckpointState(ctx context.Context, sessionID string, key string, target any) (bool, error)
	DeleteCheckpointState(ctx context.Context, sessionID string, key string) error
}

// SessionMeta 会话元数据（从 model 包导入，此处为接口引用）
type SessionMeta struct {
	SessionID    string
	UserID       uint
	EmployeeID   string
	Role         string
	Status       string
	Summary      string
	CheckpointID string
	MessageCount uint
}
```

**变更说明**：
- 移除 `StateKeyReimbursement` / `StateKeyUserIdentity` 常量（不再使用 session_states 表存储业务状态）
- 新增 `CreateSession` / `GetSession` / `UpdateSession` / `DeleteSession` / `ListSessions`
- 保留 `SaveCheckpointState` / `GetCheckpointState` / `DeleteCheckpointState`（仅用于 Interrupt 恢复时的临时状态快照）

**验证**: `go build ./infra/...`

---

### Step 1.4 — 创建 SessionMetaRepo

**文件**: `infra/session_meta_repo.go` (新增)

```go
package infra

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

// SessionMetaRepo 会话元数据仓储（薄层，直接操作 GORM）
type SessionMetaRepo struct {
	db *gorm.DB
}

func NewSessionMetaRepo(db *gorm.DB) *SessionMetaRepo {
	return &SessionMetaRepo{db: db}
}

func (r *SessionMetaRepo) Create(ctx context.Context, meta *model.SessionMeta) error {
	return r.db.WithContext(ctx).Create(meta).Error
}

func (r *SessionMetaRepo) GetBySessionID(ctx context.Context, sessionID string) (*model.SessionMeta, error) {
	var meta model.SessionMeta
	err := r.db.WithContext(ctx).Where("session_id = ?", sessionID).First(&meta).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("查询会话元数据失败: %w", err)
	}
	return &meta, nil
}

func (r *SessionMetaRepo) Update(ctx context.Context, sessionID string, updates map[string]any) error {
	return r.db.WithContext(ctx).Model(&model.SessionMeta{}).
		Where("session_id = ?", sessionID).Updates(updates).Error
}

func (r *SessionMetaRepo) Delete(ctx context.Context, sessionID string) error {
	return r.db.WithContext(ctx).
		Where("session_id = ?", sessionID).Delete(&model.SessionMeta{}).Error
}

func (r *SessionMetaRepo) List(ctx context.Context, userID uint, status string) ([]*model.SessionMeta, error) {
	var metas []*model.SessionMeta
	q := r.db.WithContext(ctx).Where("user_id = ?", userID)
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if err := q.Order("updated_at DESC").Limit(50).Find(&metas).Error; err != nil {
		return nil, fmt.Errorf("查询会话列表失败: %w", err)
	}
	return metas, nil
}
```

**验证**: `go build ./infra/...`

---

### Step 1.5 — 重写 infra/session_mysql.go

**文件**: `infra/session_mysql.go` (重写)

关键变更点：
1. `SaveMessages`: 拆分 `*schema.Message` → 结构化字段写入
2. `GetHistory`: 从结构化字段还原 `*schema.Message`
3. 新增 `CreateSession` / `GetSession` / `UpdateSession` / `DeleteSession` / `ListSessions`

```go
// SaveMessages v4: 拆分为结构化字段批量写入
func (s *MySQLSessionStore) SaveMessages(ctx context.Context, sessionID string, msgs []*schema.Message) error {
	if len(msgs) == 0 { return nil }

	// ── 1. 获取当前最大 seq ──
	var maxSeq uint
	s.db.WithContext(ctx).Model(&model.SessionMessage{}).
		Where("session_id = ?", sessionID).
		Select("COALESCE(MAX(seq), 0)").Scan(&maxSeq)

	// ── 2. 构建结构化记录 ──
	records := make([]*model.SessionMessage, 0, len(msgs))
	for i, msg := range msgs {
		rec := &model.SessionMessage{
			SessionID: sessionID,
			Seq:       maxSeq + uint(i) + 1,
			Role:      string(msg.Role),
		}
		
		switch msg.Role {
		case schema.User, schema.Assistant:
			content := msg.Content
			rec.Content = &content
		case schema.Tool:
			// 工具消息：提取 tool_name + 摘要
			rec.ToolName = msg.ToolName
			// 工具输出截断为摘要（全量存 message_meta）
			summary := truncateContent(msg.Content, 500)
			rec.Content = &summary
			output := msg.Content
			rec.ToolOutput = &output
		}
		
		// Eino 完整元数据（ToolCalls / ResponseMeta 等）
		metaBytes, _ := json.Marshal(msg)
		metaStr := string(metaBytes)
		rec.MessageMeta = &metaStr
		
		records = append(records, rec)
	}

	// ── 3. 批量写入 + 更新计数器 ──
	if err := s.db.WithContext(ctx).Create(records).Error; err != nil {
		return fmt.Errorf("保存消息失败: %w", err)
	}

	s.metaRepo.Update(ctx, sessionID, map[string]any{
		"message_count": maxSeq + uint(len(msgs)),
		"updated_at":    time.Now(),
	})

	// 缓存更新（省略，与 v3 相同模式）
	return nil
}

// GetHistory v4: 从结构化字段还原 message
func (s *MySQLSessionStore) GetHistory(ctx context.Context, sessionID string, limit int) ([]*schema.Message, error) {
	var records []model.SessionMessage
	
	query := s.db.WithContext(ctx).
		Where("session_id = ?", sessionID).
		Order("seq ASC")
	if limit > 0 {
		// 取最近的 N 条
		subQuery := s.db.Model(&model.SessionMessage{}).
			Select("id").Where("session_id = ?", sessionID).
			Order("seq DESC").Limit(limit)
		query = query.Where("id IN (?)", subQuery).Order("seq ASC")
	}
	if err := query.Find(&records).Error; err != nil {
		return nil, err
	}

	// 还原为 schema.Message（优先从 MessageMeta 完整还原，降级到结构化字段拼装）
	msgs := make([]*schema.Message, 0, len(records))
	for _, rec := range records {
		if rec.MessageMeta != nil {
			var msg schema.Message
			if err := json.Unmarshal([]byte(*rec.MessageMeta), &msg); err == nil {
				msgs = append(msgs, &msg)
				continue
			}
		}
		// 降级：从结构化字段重建
		msg := &schema.Message{Role: schema.RoleType(rec.Role)}
		if rec.Content != nil { msg.Content = *rec.Content }
		if rec.ToolName != "" { msg.ToolName = rec.ToolName }
		msgs = append(msgs, msg)
	}
	return msgs, nil
}
```

**验证**: 单元测试 `infra/session_mysql_test.go`（测试保存/读取结构化消息的往返一致性）

---

### Step 1.6 — 适配 infra/session_redis.go

**文件**: `infra/session_redis.go` (修改)

**变更**: 缓存键格式从 `session:{id}:messages` → 适配新结构化格式。缓存值使用 JSON 序列化 `[]*SessionMessage` 而非 `[]*schema.Message`。

**验证**: 集成测试 `infra/session_redis_test.go`

---

### Step 1.7 — Wire + 编译

更新 `infra/provider.go`：
```go
var ProviderSet = wire.NewSet(
	// ... 现有 ...
	NewSessionMetaRepo,       // 新增
)
```

**验证**: `go build ./...`（此时旧 Agent 代码可能引用旧 SessionStore 接口，暂不修复——Phase 3 统一处理）

---

## 3. Phase 2: 合规审核 AgentTool

> **目标**: 实现 `search_policy` Tool + `ComplianceMiniAgent` + AgentTool 包装。
> **独立可测**: 此 Phase 不依赖 Phase 1，可先于 Session 重构完成。

### Step 2.1 — 创建 search_policy Tool

**文件**: `internal/domain/agent/tools/search_policy_tool.go` (新增)

```go
package tools

import (
	"context"
	"fmt"

	"github.com/CycleZero/Reimbee/internal/domain/compliance"
	"github.com/CycleZero/Reimbee/log"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"go.uber.org/zap"
)

// ── 输入/输出类型 ──
type SearchPolicyInput struct {
	Query string `json:"query" jsonschema:"required" jsonschema_description:"检索查询(自然语言如'差旅住宿标准')"`
	Limit int    `json:"limit" jsonschema:"default=5" jsonschema_description:"返回数量上限"`
}

type PolicyChunk struct {
	RuleID  string  `json:"rule_id"`
	Title   string  `json:"title"`
	Content string  `json:"content"`
	Score   float64 `json:"score"`
}

type SearchPolicyOutput struct {
	Chunks []PolicyChunk `json:"chunks"`
}

// ── 构造函数 ──
type SearchPolicyTool struct{ tool.InvokableTool }

func NewSearchPolicyTool(kb *compliance.KnowledgeBase, logger *log.Logger) *SearchPolicyTool {
	t, err := utils.InferTool[SearchPolicyInput, SearchPolicyOutput](
		"search_policy",
		"检索企业报销政策知识库。输入自然语言查询(如'差旅住宿标准 500元')，返回最相关的政策文档片段(含费用标准、审批流程、特殊规定)。",
		func(ctx context.Context, input SearchPolicyInput) (SearchPolicyOutput, error) {
			if input.Limit <= 0 { input.Limit = 5 }
			chunks, err := kb.Search(ctx, input.Query, input.Limit)
			if err != nil {
				return SearchPolicyOutput{}, fmt.Errorf("政策检索失败: %w", err)
			}
			result := make([]PolicyChunk, 0, len(chunks))
			for _, c := range chunks {
				result = append(result, PolicyChunk{
					RuleID: c.RuleID, Title: c.Source,
					Content: c.Content, Score: c.Score,
				})
			}
			logger.Debug("政策检索完成", zap.String("查询", input.Query), zap.Int("命中数", len(result)))
			return SearchPolicyOutput{Chunks: result}, nil
		},
	)
	if err != nil { panic("创建search_policy工具失败: " + err.Error()) }
	return &SearchPolicyTool{t}
}
```

**验证**: `go build ./internal/domain/agent/tools/`

---

### Step 2.2 — 创建 ComplianceAgentTool

**文件**: `internal/domain/agent/tools/compliance_agent_tool.go` (新增)

```go
package tools

import (
	"context"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"go.uber.org/zap"
)

type ComplianceAgentTool struct{ tool.InvokableTool }

func NewComplianceAgentTool(
	ctx context.Context,
	complianceModel model.ToolCallingChatModel, // 审核专用 LLM
	searchPolicyTool tool.InvokableTool,
	logger *zap.Logger,
) *ComplianceAgentTool {
	agent, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:        "compliance_reviewer",
		Description: "企业合规审核专家。接收票据信息，检索政策后判定合规性。",
		Instruction: buildComplianceAgentPrompt(),
		Model:       complianceModel,
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools: []tool.BaseTool{searchPolicyTool},
			},
		},
		MaxIterations: 5,
	})
	if err != nil {
		panic("创建合规审核Agent失败: " + err.Error())
	}

	agentTool := adk.NewAgentTool(ctx, agent)
	logger.Info("合规审核AgentTool创建成功")
	return &ComplianceAgentTool{InvokableTool: agentTool}
}

func buildComplianceAgentPrompt() string {
	return `你是企业财务合规审核专家。工作流程：
1. 收到票据信息后，调用 search_policy 检索相关政策
2. 必要时多次检索（不同角度查询）
3. 根据政策原文判定合规性
4. 以 JSON 格式返回结果

输出格式（严格 JSON，无其他内容）：
{
  "result": "pass|warning|error",
  "message": "检查结果描述（引用政策条款）",
  "rule_id": "触发的规则ID",
  "standard": "标准值（超标时必填）",
  "suggestion": "处理建议（超标时必填）",
  "reference": "引用的政策原文摘要"
}

判定标准：
- pass: 完全符合政策标准
- warning: 金额为标准80%-100%，或日期距过期不足30天
- error: 金额超标，或日期失效，或类别不允许

金额以"元"为单位比较。日期格式为 YYYY-MM-DD。`
}
```

**验证**: `go build ./internal/domain/agent/tools/`

---

### Step 2.3 — 更新 tools/provider.go

**文件**: `internal/domain/agent/tools/provider.go` (修改)

```go
// ProviderSet v4: 新增 search_policy + compliance_agent
var ProviderSet = wire.NewSet(
	NewToolSet,
	NewOCRTool,
	NewSearchPolicyTool,           // ★ 新增
	NewComplianceAgentTool,        // ★ 新增（替换 NewComplianceTool）
	NewBudgetTool,
	NewPDFTool,
	NewEmailTool,
	NewProgressTool,
	NewQueryTool,
	NewCreateReimbTool,
	NewSubmitReimbTool,
	// 删除: NewConfirmInvoiceTool, NewConfirmSubmitTool, NewComplianceTool
)

// ToolSet v4: 简化，移除 Phase 分组
type ToolSet struct {
	OCR            tool.InvokableTool
	Compliance     tool.InvokableTool  // ★ AgentTool 类型
	Budget         tool.InvokableTool
	CreateReimb    tool.InvokableTool
	SubmitReimb    tool.InvokableTool
	PDF            tool.InvokableTool
	Email          tool.InvokableTool
	Progress       tool.InvokableTool
	QueryRecords   tool.InvokableTool
}

func NewToolSet(
	ocr *OCRTool,
	compliance *ComplianceAgentTool,  // ★ 类型变更
	budget *BudgetTool,
	pdf *PDFTool,
	email *EmailTool,
	progress *ProgressTool,
	query *QueryTool,
	createReimb *CreateReimbTool,
	submitReimb *SubmitReimbTool,
	store infra.SessionStore,
	logger *log.Logger,
) *ToolSet {
	return &ToolSet{
		OCR:          ocr.InvokableTool,
		Compliance:   compliance.InvokableTool,
		Budget:       budget.InvokableTool,
		PDF:          pdf.InvokableTool,
		Email:        email.InvokableTool,
		Progress:     progress.InvokableTool,
		QueryRecords: query.InvokableTool,
		CreateReimb:  createReimb.InvokableTool,
		SubmitReimb:  submitReimb.InvokableTool,
	}
}

// ★ 删除所有 GetPhase* 方法
// 删除: GetPhase1Tools, GetPhase2Tools, GetPhase3Tools
// 删除: GetPhase1BaseTools, GetPhase2BaseTools, GetPhase3BaseTools
// 删除: GetProgressBaseTools, GetBudgetBaseTools
```

**验证**: `go build ./internal/domain/agent/tools/`（此时 tool 文件仍未删除，兼容性编译）

---

## 4. Phase 3: Agent 层简化

> **目标**: 8 个 Agent → 1 个，移除 Phase 状态机、意图分类、confirm 工具。

### Step 3.1 — 重写 phase_agents.go → 单 Agent

**文件**: `internal/domain/agent/phase_agents.go` (重写)

```go
package agent

import (
	"context"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"go.uber.org/zap"
)

// initAgent v4: 创建唯一的 ReimburseAgent（不再区分 Phase）
func (m *LoopManager) initAgent(ctx context.Context, deps LoopManagerDeps) {
	deps.Logger.Debug("初始化唯一 ReimburseAgent")

	m.reimburseAgent = mustNewAgent(ctx, deps,
		"reimburse_agent",
		"企业报销全流程智能助手",
		BuildSystemPromptV4(),  // ★ 新 Prompt（见 Step 3.3）
		[]tool.BaseTool{
			deps.ToolSet.OCR,
			deps.ToolSet.Compliance,     // ★ AgentTool
			deps.ToolSet.Budget,
			deps.ToolSet.CreateReimb,
			deps.ToolSet.SubmitReimb,    // ★ 内建 Interrupt
			deps.ToolSet.PDF,
			deps.ToolSet.Email,
			deps.ToolSet.Progress,
			deps.ToolSet.QueryRecords,
		},
	)

	deps.Logger.Info("ReimburseAgent初始化完成",
		zap.Int("工具数", 9))
}
```

**LoopManager 结构体变更**：

```go
// loop_manager.go — v4 简化
type LoopManager struct {
	mu    sync.Mutex
	loops map[string]*SessionLoop
	store infra.SessionStore

	reimburseAgent *adk.ChatModelAgent  // ★ 唯一 Agent（替换 8 个字段）
	
	complianceModel model.ToolCallingChatModel  // ★ 合规审核专用 LLM
	chatModel       model.ToolCallingChatModel  // 主 Agent LLM
	
	logger *log.Logger
	config *LoopConfig
}
```

**验证**: `go build ./internal/domain/agent/`

---

### Step 3.2 — 简化 session_loop.go

**文件**: `internal/domain/agent/session_loop.go` (重写)

**PrepareAgent 简化**：

```go
// makePrepareAgent v4: 始终返回唯一 Agent（无意图分类，无 Phase 选择）
func (m *LoopManager) makePrepareAgent(sessionID string) func(...) (adk.Agent, error) {
	return func(ctx context.Context, loop *adk.TurnLoop[string, *schema.Message],
		consumed []string) (adk.Agent, error) {
		// v4: 单一 Agent，无需选择
		return m.reimburseAgent, nil
	}
}
```

**GenInput 简化**：移除 `ReimbursementState` 加载 → 不再需要 `StateContextKey` 注入。历史消息从结构化 `session_messages` 表加载。

**新增 GenResume**：参见 Step 5.2。

**删除函数**：
- `classifyIntentByLLM()`
- `classifyByKeywords()`
- `selectPhaseAgent()`
- `containsAnyStr()`
- `truncateStr()`
- `min()`

**验证**: `go build ./internal/domain/agent/`

---

### Step 3.3 — 重写 prompt.go

**文件**: `internal/domain/agent/prompt.go` (重写)

```go
package agent

// BuildSystemPromptV4 v4 单系统 Prompt
func BuildSystemPromptV4() string {
	return `你是 Reimbee，企业财务报销智能助手。帮助员工完成报销全流程。

## 报销流程

**1. 信息收集**
- 引导用户上传票据图片
- 用户告知图片路径后，调用 recognize_invoice 进行 OCR 识别
- 展示识别结果（金额、类别、日期），请用户核对
- 用户可继续添加票据，或告知"完成了"

**2. 合规与预算检查**
- 用户确认票据后，逐张调用 check_compliance 检查合规性
- 展示每张票据的检查结果（通过/超标/违规及处理建议）
- 调用 check_budget 检查部门预算

**3. 提交确认**
- 合规和预算通过后，汇总全部信息
- 明确告知用户"请确认以上信息，我将为您提交报销单"
- 用户确认后调用 submit_reimbursement

## 行为规范
- 逐步引导，一次只问一个问题
- 涉及金额时必须让用户确认
- 合规问题明确告知标准值和实际值
- 专业、友好、简洁

## 其他能力
- 查询审批进度：直接调用 query_progress
- 查询历史报销：直接调用 query_reimbursements
- 查询部门预算：直接调用 check_budget`
}

// ★ 删除所有 Phase 相关函数
// 删除: BuildSystemPrompt(phase, state)
// 删除: getPhaseInstruction()
// 删除: BuildStateSummary()
// 删除: BuildModifiedInvoicesWarning()
// 删除: BuildGeneralChatPrompt()
// 保留: BuildIntentClassifyPrompt() — 可后续按需使用
```

**验证**: `go build ./internal/domain/agent/`

---

### Step 3.4 — 简化 dto.go

**文件**: `internal/domain/agent/dto.go` (修改)

```go
package agent

import "github.com/CycleZero/Reimbee/internal/domain/agent/types"

// ── 类型别名 ──
type ReimbursementState = types.ReimbursementState
type InvoiceState = types.InvoiceState
type ComplianceCheckResult = types.ComplianceCheckResult
type BudgetCheckResult = types.BudgetCheckResult

// ★ 删除以下类型:
// 删除: IntentResult, WorkflowRoute, Route* 常量
// 删除: GuardResult, AgentInput, AgentOutput
```

**验证**: `go build ./internal/domain/agent/`

---

### Step 3.5 — 简化 types/state.go

**文件**: `internal/domain/agent/types/state.go` (修改)

```go
// ReimbursementState v4: 移除 Phase 相关字段
type ReimbursementState struct {
	ReimbursementID uint   `json:"reimbursement_id"`
	ReimbursementNo string `json:"reimbursement_no"`
	DepartmentID    uint   `json:"department_id"`
	EmployeeID      string `json:"employee_id"`
	EmployeeName    string `json:"employee_name"`

	Invoices      []InvoiceState `json:"invoices"`
	TotalAmount   int64          `json:"total_amount"`

	ComplianceResult    *ComplianceCheckResult `json:"compliance_result,omitempty"`
	BudgetResult        *BudgetCheckResult     `json:"budget_result,omitempty"`
	NeedSpecialApproval bool                   `json:"need_special_approval"`

	// ★ 删除:
	// CurrentPhase, UserConfirmed, FinalConfirmed
	// Phase1Turns, Phase2Turns, Phase3Turns
}

// InvoiceState: 保留所有字段，移除 UserConfirmed
type InvoiceState struct {
	Index          int     `json:"index"`
	ImagePath      string  `json:"image_path"`
	OCRRawAmount   int64   `json:"ocr_raw_amount"`
	OCRRawCategory string  `json:"ocr_raw_category"`
	OCRRawDate     string  `json:"ocr_raw_date"`
	OCRConfidence  float64 `json:"ocr_confidence"`
	Amount         int64   `json:"amount"`
	Category       string  `json:"category"`
	InvoiceDate    string  `json:"invoice_date"`
	IsModified     bool    `json:"is_modified"`
	ModifyReason   string  `json:"modify_reason"`
	// ★ 删除: UserConfirmed
}
```

**验证**: `go build ./internal/domain/agent/types/`

---

## 5. Phase 4: Interrupt 确认机制

> **目标**: `submit_reimbursement` 内建 Interrupt，Checkpoint 启用，GenResume 回调，HTTP approve 端点。
> **核心依赖**: Phase 3（Agent 简化完成）。

### Step 5.1 — 重写 submit_reimb_tool.go

**文件**: `internal/domain/agent/tools/submit_reimb_tool.go` (重写)

```go
package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/CycleZero/Reimbee/internal/domain/agent/types"
	"github.com/CycleZero/Reimbee/internal/domain/reimbursement"
	"github.com/CycleZero/Reimbee/log"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"go.uber.org/zap"
)

type SubmitReimbInput struct {
	// 空结构体：LLM 调用此工具表示用户已口头确认，实际提交需 Interrupt 确认
}

type SubmitConfirmInfo struct {
	Invoices      []types.InvoiceState          `json:"invoices"`
	TotalAmount   int64                         `json:"total_amount"`
	Budget        *types.BudgetCheckResult      `json:"budget,omitempty"`
	Compliance    *types.ComplianceCheckResult  `json:"compliance,omitempty"`
}

type SubmitReimbOutput struct {
	ReimbursementNo     string `json:"reimbursement_no"`
	Status              string `json:"status"`
	NeedSpecialApproval bool   `json:"need_special_approval"`
}

type SubmitReimbTool struct{ tool.InvokableTool }

func NewSubmitReimbTool(
	reimbursementBiz *reimbursement.ReimbursementBiz,
	logger *log.Logger,
) *SubmitReimbTool {
	t, err := utils.InferTool[SubmitReimbInput, SubmitReimbOutput](
		"submit_reimbursement",
		"提交报销单进入审批流程。此操作将触发用户在UI界面显式确认，确认后不可撤销。"+
			"调用前确保已创建报销单草稿(create_reimbursement)。",
		func(ctx context.Context, input SubmitReimbInput) (SubmitReimbOutput, error) {

			// ── 1. 从 context 加载状态 ──
			var state types.ReimbursementState
			if raw, ok := ctx.Value(types.StateContextKey{}).(*types.ReimbursementState); ok {
				state = *raw
			}

			// ── 2. 判断是否为恢复执行 ──
			resumeInfo, isResume := adk.GetResumeInfo(ctx)
			if !isResume {
				// 首次执行 → 触发 Interrupt
				confirmData := SubmitConfirmInfo{
					Invoices:    state.Invoices,
					TotalAmount: state.TotalAmount,
					Budget:      state.BudgetResult,
					Compliance:  state.ComplianceResult,
				}
				logger.Info("触发提交确认中断",
					zap.Int("票据数", len(state.Invoices)),
					zap.Int64("总金额(分)", state.TotalAmount))

				// ★ Interrupt: 框架暂停 → 保存 Checkpoint → 前端弹确认
				event := adk.Interrupt(ctx, confirmData)
				// 返回中断信号
				return SubmitReimbOutput{}, fmt.Errorf("%w", event.Action.Interrupted)
			}

			// ── 3. 恢复执行：用户已确认 ──
			logger.Info("用户已确认，执行提交流程")

			// 解析审批结果
			var approval struct {
				Approved bool   `json:"approved"`
				Reason   string `json:"reason"`
			}
			if data, err := json.Marshal(resumeInfo.ResumeData); err == nil {
				json.Unmarshal(data, &approval)
			}
			if !approval.Approved {
				return SubmitReimbOutput{}, fmt.Errorf("用户取消提交")
			}

			// 执行提交
			rm, err := reimbursementBiz.Submit(state.ReimbursementID, state.TotalAmount)
			if err != nil {
				return SubmitReimbOutput{}, fmt.Errorf("提交失败: %w", err)
			}

			logger.Info("报销单提交成功",
				zap.String("单号", rm.ReimbursementNo),
				zap.String("状态", rm.Status))

			return SubmitReimbOutput{
				ReimbursementNo:     rm.ReimbursementNo,
				Status:              rm.Status,
				NeedSpecialApproval: rm.NeedSpecialApproval,
			}, nil
		},
	)
	if err != nil { panic("创建submit_reimbursement(Interrupt)工具失败: " + err.Error()) }
	return &SubmitReimbTool{t}
}
```

**验证**: `go build ./internal/domain/agent/tools/`

---

### Step 5.2 — 启用 Checkpoint + GenResume

**文件**: `internal/domain/agent/session_loop.go` (修改)

```go
// createSessionLoop v4: 启用 Store + CheckpointID
func (m *LoopManager) createSessionLoop(sessionID string) *SessionLoop {
	ctx, cancel := context.WithCancel(context.Background())
	sl := &SessionLoop{sessionID: sessionID, cancel: cancel, lastActive: time.Now()}

	cfg := adk.TurnLoopConfig[string, *schema.Message]{
		GenInput:      m.makeGenInput(sessionID),
		PrepareAgent:  m.makePrepareAgent(sessionID),
		OnAgentEvents: m.makeOnAgentEvents(sessionID),
		// ★ v4: 启用 Checkpoint
		GenResume:    m.makeGenResume(sessionID),
		Store:        m.checkpointStore,
		CheckpointID: sessionID,
	}

	sl.turnLoop = adk.NewTurnLoop(cfg)
	sl.turnLoop.Run(ctx)
	return sl
}

// makeGenResume v4: 从 Push 的审批 item 构建 ResumeParams
func (m *LoopManager) makeGenResume(sessionID string) func(
	ctx context.Context,
	loop *adk.TurnLoop[string, *schema.Message],
	interruptedItems, unhandledItems, newItems []string,
) (*adk.GenResumeResult[string, *schema.Message], error) {

	return func(ctx context.Context, loop *adk.TurnLoop[string, *schema.Message],
		interruptedItems, unhandledItems, newItems []string,
	) (*adk.GenResumeResult[string, *schema.Message], error) {

		// 从 newItems 提取审批结果
		var approval struct {
			Approved bool   `json:"approved"`
			Reason   string `json:"reason"`
		}
		for _, item := range newItems {
			if err := json.Unmarshal([]byte(item), &approval); err == nil {
				break
			}
		}

		return &adk.GenResumeResult[string, *schema.Message]{
			ResumeParams: &adk.ResumeParams{
				Targets: map[string]any{
					sessionID: approval, // InterruptID = sessionID
				},
			},
			Consumed:  newItems,
			Remaining: unhandledItems,
		}, nil
	}
}
```

**验证**: `go build ./internal/domain/agent/`

---

### Step 5.3 — 新增 HandleApprove 端点

**文件**: `internal/domain/agent/service.go` (修改)

```go
// HandleApprove 处理用户审批确认（Interrupt 恢复）
//
// @Summary 审批确认（Interrupt 恢复）
// @Description 用户在 UI 点击确认按钮后调用，恢复被 Interrupt 暂停的 Agent 执行。
// @Tags Agent对话
// @Accept json
// @Produce text/event-stream
// @Param session_id query string true "会话ID"
// @Param request body object true "审批结果" 
// @Success 200 {string} string "SSE 事件流"
// @Router /api/chat/approve [post]
func (s *AgentService) HandleApprove(c *gin.Context) {
	sessionID := c.Query("session_id")
	var req struct {
		Approved bool   `json:"approved"`
		Reason   string `json:"reason,omitempty"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	sseWriter, err := NewGinSSEWriter(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "不支持流式响应"})
		return
	}

	// 构造审批 item（JSON 序列化后 Push）
	approvalItem, _ := json.Marshal(map[string]any{
		"approved": req.Approved,
		"reason":   req.Reason,
	})

	doneCh := make(chan error, 1)
	// 使用相同 sessionID 创建新 TurnLoop → 自动触发 GenResume
	s.loopManager.PushMessage(sessionID, string(approvalItem), sseWriter, doneCh)

	if err := <-doneCh; err != nil {
		s.logger.Error("审批恢复执行失败", zap.Error(err))
	}
}
```

**验证**: `go build ./internal/domain/agent/`

---

### Step 5.4 — SSE Interrupted 事件类型

**文件**: `internal/domain/agent/sse.go` (修改)

```go
// ★ 新增事件类型
const (
	EventTypeInterrupted SSEEventType = "interrupted" // Agent 暂停等待确认
)

// ★ 新增数据结构
type InterruptedData struct {
	InterruptID string `json:"interrupt_id"`
	Action      string `json:"action"`       // confirm_submit
	Context     any    `json:"context"`      // SubmitConfirmInfo
}

// ★ 新增工厂函数
func NewInterruptedEvent(interruptID, action string, context any) SSEEvent {
	return SSEEvent{
		Type: EventTypeInterrupted,
		Data: InterruptedData{
			InterruptID: interruptID,
			Action:      action,
			Context:     context,
		},
	}
}
```

**OnAgentEvents 中检测 Interrupt**：

```go
// session_loop.go — OnAgentEvents 回调中新增
if event.Action != nil && event.Action.Interrupted != nil {
	info := event.Action.Interrupted
	_ = writer.WriteEvent(NewInterruptedEvent(
		sessionID, "confirm_submit", info.Data,
	))
	_ = writer.Flush()
	// Interrupt 后 TurnLoop 会退出，通过 GenResume 恢复
}
```

**验证**: `go build ./internal/domain/agent/`

---

### Step 5.5 — 新增路由

**文件**: `internal/router/root.go` (修改)

```go
// 在 SSE 对话接口旁新增
api.POST("/chat/approve", hub.AgentService.HandleApprove)
```

**验证**: `go build ./internal/router/`

---

## 6. Phase 5: 清理、Wire、路由

> **目标**: 删除旧文件，更新 Wire ProviderSet，更新配置，`make wire` 通过。

### Step 6.1 — 删除旧工具文件

```bash
rm internal/domain/agent/tools/confirm_invoice_tool.go
rm internal/domain/agent/tools/confirm_submit_tool.go
rm internal/domain/agent/tools/compliance_tool.go
```

### Step 6.2 — 更新 infra/provider.go

```go
var ProviderSet = wire.NewSet(
	NewData,
	NewRedisClient,
	NewMySQLSessionStore,
	NewSessionMetaRepo,          // ★ 新增
	// ... 其余不变 ...
)
```

### Step 6.3 — 更新 agent/provider.go

```go
var ProviderSet = wire.NewSet(
	LoadAgentConfig,
	LoadLoopConfig,
	MustNewComplianceChatModel,   // ★ 合规审核专用 LLM
	tools.ProviderSet,
	MustNewChatModel,
	NewMySQLCheckpointStore,
	wire.Bind(new(CheckpointStore), new(*MySQLCheckpointStore)),
	NewLoopManager,
	NewAgentService,
)
```

### Step 6.4 — 更新 config.go

**文件**: `internal/domain/agent/config.go` (修改)

```go
type AgentConfig struct {
	// ... v3 保留 ...
	InterruptTimeoutSeconds int  // ★ 新增：Interrupt 等待超时（秒）
}

func applyDefaults(cfg *AgentConfig) {
	// ... v3 保留 ...
	if cfg.InterruptTimeoutSeconds <= 0 {
		cfg.InterruptTimeoutSeconds = 300 // 5 分钟默认
	}
}
```

### Step 6.5 — Wire 生成

```bash
make wire
```

### Step 6.6 — 全量编译

```bash
go build ./...
```

---

## 7. Phase 6: 全面测试

### Step 7.1 — 单元测试

```
测试对象:
  ✓ model/session_meta_test.go         — GORM 模型 CRUD
  ✓ infra/session_mysql_test.go        — SaveMessages ↔ GetHistory 往返一致性
  ✓ tools/search_policy_tool_test.go   — RAG 检索 Tool
  ✓ tools/submit_reimb_tool_test.go    — Interrupt 触发 + 恢复逻辑
  ✓ session_loop_test.go               — GenResume 回调
  
运行: go test ./... -count=1
```

### Step 7.2 — 集成测试

```
场景 1: 完整报销流程（无 Interrupt 触发）
  用户: "我要报销差旅费" → OCR → 合规 → 预算 → 确认 → 提交成功
  验证: session_messages 结构化字段正确，meta.message_count 递增

场景 2: Interrupt 确认流程
  用户: "确认提交" → submit_reimbursement 触发 Interrupt
  → HandleApprove 恢复 → 提交成功
  验证: Checkpoint 保存/恢复正确，GenResume 正确解析审批结果

场景 3: 用户取消提交
  用户: "确认提交" → Interrupt → HandleApprove(approved=false)
  → 返回"用户取消提交"
  验证: 报销单未创建，Checkpoint 被清理

场景 4: 合规超标
  用户: "报销差旅住宿800元" → 合规检查 → warning → LLM 告知超标 → 用户决定继续
```

### Step 7.3 — E2E 测试（Playwright）

```
1. 前端发起完整报销对话
2. 确认弹窗出现（SSE interrupted 事件）
3. 点击确认按钮
4. 验证最终提交成功消息
```

---

## 8. 回滚策略

| 场景 | 回滚方式 |
|------|---------|
| Phase 1 失败（Session 持久化） | `git checkout` model/session_message.go + infra/session*.go，旧表未删除 |
| Phase 2 失败（合规 AgentTool） | 删除新文件，保留旧 compliance_tool.go |
| Phase 3 失败（Agent 简化） | `git checkout` phase_agents.go + session_loop.go + prompt.go |
| Phase 4 失败（Interrupt） | 最复杂——需同时回滚 Phase 3 + 删除 submit_reimb_tool.go 修改 |
| Wire 编译失败 | `git checkout wire_gen.go` → 修复 → `make wire` |

### 安全措施

1. **每步可编译**: 每个 Step 结束时 `go build` 必须通过
2. **旧文件先保留**: 删除操作集中在 Phase 5，前 4 个 Phase 零删除
3. **分支策略**: 在 `feature/agent-v4` 分支开发，`develop` 保持稳定

---

## 附录 A: 步骤依赖图

```
Phase 1 (Session) ─────────────────────────────────────┐
  1.1 → 1.2 → 1.3 → 1.4 → 1.5 → 1.6 → 1.7              │
                                                         │
Phase 2 (合规 AgentTool) ───────────────────────┐       │
  2.1 → 2.2 → 2.3                               │       │
                                                  │       │
Phase 3 (Agent 简化) ←────────────────────────────┼───────┘
  3.1 → 3.2 → 3.3 → 3.4 → 3.5                    │
                                                  │
Phase 4 (Interrupt) ←─────────────────────────────┘
  5.1 → 5.2 → 5.3 → 5.4 → 5.5

Phase 5 (清理 + Wire)
  6.1 → 6.2 → 6.3 → 6.4 → 6.5 → 6.6

Phase 6 (测试)
  7.1 → 7.2 → 7.3
```

**并行机会**: Phase 1 和 Phase 2 可并行执行（互不依赖）。

---

## 附录 B: 配置变更

```yaml
# config.yaml v4 新增项

agent:
  # v3 保留
  session_ttl_minutes: 30
  max_history_turns: 20
  llm_max_retries: 3
  tool_timeout_seconds: 30
  
  # v4 新增
  interrupt_timeout_seconds: 300

# v4 新增：合规审核 Agent 配置
compliance:
  knowledge_base:
    embedding_model: "text-embedding-v3"
  agent:
    model: "gpt-4o"
    temperature: 0.0
    max_iterations: 5

# v4 新增：Checkpoint 存储
checkpoint:
  enabled: true
  cleanup_hours: 24  # 孤儿 Checkpoint 清理周期
```
