// Package infra 会话持久化仓储
//
// SessionRepo 提供 Session 对象的完整 CRUD，封装三表拆分细节：
//   session_meta、session_messages、session_states。
//
// 业务层持有一个 Session 对象，需要时丢给 SessionRepo 持久化。
package infra

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/CycleZero/Reimbee/log"
	"github.com/CycleZero/Reimbee/model"
	"github.com/CycleZero/blades"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// ============================================
// Session 业务对象 — 纯数据，无持久化逻辑
// ============================================

// Session 一个完整会话的数据快照
type Session struct {
	Meta     *SessionMeta
	Messages []*blades.Message
	State    map[string]any
}

// SessionMeta 会话元数据
type SessionMeta struct {
	SessionID    string
	UserID       uint
	EmployeeID   string
	EmployeeName string
	Role         string
	Status       string
	Summary      string
	MessageCount int
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// ============================================
// SessionRepo 接口
// ============================================

// SessionRepo CRUD 风格会话仓储
type SessionRepo struct {
	db     *gorm.DB
	cache  *RedisSessionCache
	logger *log.Logger
}

// NewSessionRepo 创建会话仓储实例，自动迁移表结构
func NewSessionRepo(db *gorm.DB, cache *RedisSessionCache, logger *log.Logger) *SessionRepo {
	r := &SessionRepo{db: db, cache: cache, logger: logger}

	if err := db.AutoMigrate(&model.SessionMessage{}); err != nil {
		panic(fmt.Errorf("会话消息表自动迁移失败: %w", err))
	}
	if err := db.AutoMigrate(&model.SessionMeta{}); err != nil {
		panic(fmt.Errorf("会话元数据表自动迁移失败: %w", err))
	}
	db.AutoMigrate(&SessionState{})

	logger.Info("会话仓储初始化完成")
	return r
}

// ============================================
// Save — 持久化整个 Session
// ============================================

// Save 将 Session 完整持久化
//   - Meta: upsert（INSERT ON DUPLICATE KEY UPDATE）
//   - Messages: diff save（只写入内存中有但 DB 中没有的新消息）
//   - State: 每个 key 单独 upsert
func (r *SessionRepo) Save(ctx context.Context, session *Session) error {
	if session == nil || session.Meta == nil {
		return fmt.Errorf("session 或 meta 不能为空")
	}
	sessionID := session.Meta.SessionID

	// 1. Upsert meta
	if err := r.saveMeta(ctx, session.Meta); err != nil {
		return fmt.Errorf("保存会话元数据失败: %w", err)
	}

	// 2. Diff save messages — 只写增量
	if len(session.Messages) > 0 {
		if err := r.saveMessages(ctx, sessionID, session.Messages); err != nil {
			return fmt.Errorf("保存会话消息失败: %w", err)
		}
	}

	// 3. Upsert states
	for key, value := range session.State {
		if err := r.saveState(ctx, sessionID, key, value); err != nil {
			return fmt.Errorf("保存会话状态[%s]失败: %w", key, err)
		}
	}

	return nil
}

// ============================================
// Load — 从 DB 恢复完整 Session
// ============================================

// Load 加载完整 Session（meta + messages + states）
// 返回 nil 表示会话不存在
func (r *SessionRepo) Load(ctx context.Context, sessionID string) (*Session, error) {
	meta, err := r.LoadMeta(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if meta == nil {
		return nil, nil
	}

	session := &Session{
		Meta:     meta,
		Messages: make([]*blades.Message, 0),
		State:    make(map[string]any),
	}

	// 加载消息（走缓存→MySQL）
	msgs, err := r.loadMessages(ctx, sessionID, 0)
	if err != nil {
		return nil, err
	}
	session.Messages = msgs

	// 加载业务状态
	states, err := r.loadStates(ctx, sessionID)
	if err != nil {
		r.logger.Warn("加载会话状态失败，继续", zap.Error(err))
	} else {
		session.State = states
	}

	return session, nil
}

// LoadMeta 仅加载会话元数据（不加载 messages 和 states）
// 返回 nil 表示会话不存在
func (r *SessionRepo) LoadMeta(ctx context.Context, sessionID string) (*SessionMeta, error) {
	var rec model.SessionMeta
	err := r.db.WithContext(ctx).Where("session_id = ?", sessionID).First(&rec).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("查询会话元数据失败: %w", err)
	}
	return &SessionMeta{
		SessionID:    rec.SessionID,
		UserID:       rec.UserID,
		EmployeeID:   rec.EmployeeID,
		EmployeeName: rec.EmployeeName,
		Role:         rec.Role,
		Status:       rec.Status,
		Summary:      rec.Summary,
		MessageCount: int(rec.MessageCount),
		CreatedAt:    rec.CreatedAt,
		UpdatedAt:    rec.UpdatedAt,
	}, nil
}

// ============================================
// List — 会话列表
// ============================================

// List 查询用户会话列表（仅 meta，不加载消息体）
func (r *SessionRepo) List(ctx context.Context, userID uint, status string) ([]*SessionMeta, error) {
	var recs []model.SessionMeta
	q := r.db.WithContext(ctx).Where("user_id = ?", userID)
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if err := q.Order("updated_at DESC").Limit(50).Find(&recs).Error; err != nil {
		return nil, fmt.Errorf("查询会话列表失败: %w", err)
	}

	result := make([]*SessionMeta, 0, len(recs))
	for i := range recs {
		result = append(result, &SessionMeta{
			SessionID:    recs[i].SessionID,
			UserID:       recs[i].UserID,
			EmployeeID:   recs[i].EmployeeID,
			EmployeeName: recs[i].EmployeeName,
			Role:         recs[i].Role,
			Status:       recs[i].Status,
			Summary:      recs[i].Summary,
			MessageCount: int(recs[i].MessageCount),
			CreatedAt:    recs[i].CreatedAt,
			UpdatedAt:    recs[i].UpdatedAt,
		})
	}
	return result, nil
}

// ============================================
// ListCursor — 游标分页会话列表
// ============================================

// ListCursorResult 游标分页结果
type ListCursorResult struct {
	Sessions  []*SessionMeta
	NextCursor string // 下一页游标（空表示无更多数据）
	HasMore   bool
}

// ListCursor 游标分页查询用户会话列表（按 updated_at DESC）
// cursor: 上一页最后一条的 updated_at (RFC3339)，首次传空
// limit: 每页数量
func (r *SessionRepo) ListCursor(ctx context.Context, userID uint, cursor string, limit int) (*ListCursorResult, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}

	q := r.db.WithContext(ctx).Where("user_id = ?", userID)

	if cursor != "" {
		t, err := time.Parse(time.RFC3339, cursor)
		if err != nil {
			return nil, fmt.Errorf("游标格式错误: %w", err)
		}
		q = q.Where("updated_at < ?", t)
	}

	var recs []model.SessionMeta
	if err := q.Order("updated_at DESC").Limit(limit + 1).Find(&recs).Error; err != nil {
		return nil, fmt.Errorf("查询会话列表失败: %w", err)
	}

	hasMore := len(recs) > limit
	if hasMore {
		recs = recs[:limit]
	}

	result := &ListCursorResult{
		Sessions: make([]*SessionMeta, 0, len(recs)),
		HasMore:  hasMore,
	}

	for i := range recs {
		result.Sessions = append(result.Sessions, &SessionMeta{
			SessionID:    recs[i].SessionID,
			UserID:       recs[i].UserID,
			EmployeeID:   recs[i].EmployeeID,
			EmployeeName: recs[i].EmployeeName,
			Role:         recs[i].Role,
			Status:       recs[i].Status,
			Summary:      recs[i].Summary,
			MessageCount: int(recs[i].MessageCount),
			CreatedAt:    recs[i].CreatedAt,
			UpdatedAt:    recs[i].UpdatedAt,
		})
	}

	if hasMore && len(recs) > 0 {
		result.NextCursor = recs[len(recs)-1].UpdatedAt.Format(time.RFC3339)
	}

	return result, nil
}

// ============================================
// LoadMessagesCursor — 游标分页消息历史
// ============================================

// MessagesCursorResult 消息游标分页结果
type MessagesCursorResult struct {
	Messages   []MessageWithSeq
	NextCursor uint // 下一页游标（0 表示无更多）
	HasMore   bool
}

// MessageWithSeq 消息与 DB 序号
type MessageWithSeq struct {
	Seq     uint
	Msg     *blades.Message
	CreatedAt time.Time
}

// LoadMessagesCursor 游标分页加载会话消息（按 seq ASC）
// cursor: 上次拉取的最后一条消息 seq（首次传 0）
// limit: 每页数量
func (r *SessionRepo) LoadMessagesCursor(ctx context.Context, sessionID string, cursor uint, limit int) (*MessagesCursorResult, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	q := r.db.WithContext(ctx).Where("session_id = ?", sessionID)
	if cursor > 0 {
		q = q.Where("seq > ?", cursor)
	}

	var records []model.SessionMessage
	if err := q.Order("seq ASC").Limit(limit + 1).Find(&records).Error; err != nil {
		return nil, fmt.Errorf("查询会话消息失败: %w", err)
	}

	hasMore := len(records) > limit
	if hasMore {
		records = records[:limit]
	}

	msgs := make([]MessageWithSeq, 0, len(records))
	for _, rec := range records {
		if msg := restoreBladesMessage(&rec); msg != nil {
			msgs = append(msgs, MessageWithSeq{Seq: rec.Seq, Msg: msg, CreatedAt: rec.CreatedAt})
		}
	}

	result := &MessagesCursorResult{
		Messages: msgs,
		HasMore:  hasMore,
	}
	if hasMore && len(records) > 0 {
		result.NextCursor = records[len(records)-1].Seq
	}

	return result, nil
}

// ============================================
// Delete — 级联删除
// ============================================

// Delete 删除整个会话（meta + messages + states）
func (r *SessionRepo) Delete(ctx context.Context, sessionID string) error {
	_ = r.db.WithContext(ctx).Where("session_id = ?", sessionID).Delete(&model.SessionMessage{}).Error
	_ = r.db.WithContext(ctx).Where("session_id = ?", sessionID).Delete(&SessionState{}).Error
	if err := r.db.WithContext(ctx).Where("session_id = ?", sessionID).Delete(&model.SessionMeta{}).Error; err != nil {
		return fmt.Errorf("删除会话元数据失败: %w", err)
	}
	if r.cache != nil {
		r.cache.Del(ctx, sessionID)
	}
	return nil
}

// SaveState 持久化业务状态（实现 StateStore 接口）
func (r *SessionRepo) SaveState(ctx context.Context, sessionID string, key string, state any) error {
	return r.saveState(ctx, sessionID, key, state)
}

// GetState 读取业务状态（实现 StateStore 接口）
func (r *SessionRepo) GetState(ctx context.Context, sessionID string, key string, target any) (bool, error) {
	states, err := r.loadStates(ctx, sessionID)
	if err != nil {
		return false, err
	}
	raw, ok := states[key]
	if !ok {
		return false, nil
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return false, err
	}
	return true, json.Unmarshal(data, target)
}

// DeleteState 删除指定业务状态
func (r *SessionRepo) DeleteState(ctx context.Context, sessionID string, key string) error {
	return r.db.WithContext(ctx).
		Where("id = ?", sessionID+":"+key).
		Delete(&SessionState{}).Error
}

// ============================================
// 内部方法
// ============================================

// saveMeta upsert 会话元数据
func (r *SessionRepo) saveMeta(ctx context.Context, m *SessionMeta) error {
	rec := model.SessionMeta{
		SessionID:    m.SessionID,
		UserID:       m.UserID,
		EmployeeID:   m.EmployeeID,
		EmployeeName: m.EmployeeName,
		Role:         m.Role,
		Status:       m.Status,
		Summary:      m.Summary,
		MessageCount: uint(m.MessageCount),
	}
	if rec.Status == "" {
		rec.Status = model.SessionStatusActive
	}

	// 基于 SessionID 做 upsert（非自增 ID），避免后续更新被唯一索引冲突
	var existing model.SessionMeta
	err := r.db.WithContext(ctx).Where("session_id = ?", rec.SessionID).First(&existing).Error
	if err == nil {
		rec.ID = existing.ID
	}
	return r.db.WithContext(ctx).Save(&rec).Error
}

// saveMessages diff save 消息（仅写增量）
func (r *SessionRepo) saveMessages(ctx context.Context, sessionID string, msgs []*blades.Message) error {
	// 获取当前DB中最大seq，只写入新增消息（索引从已有数量+1开始）
	var dbCount int64
	r.db.WithContext(ctx).Model(&model.SessionMessage{}).
		Where("session_id = ?", sessionID).Count(&dbCount)

	if int(dbCount) >= len(msgs) {
		r.logger.Debug("消息已全部持久化，跳过", zap.Int("内存", len(msgs)), zap.Int64("DB", dbCount))
		return nil
	}

	// 只写增量部分
	newMsgs := msgs[dbCount:]

	var maxSeq uint
	r.db.WithContext(ctx).Model(&model.SessionMessage{}).
		Where("session_id = ?", sessionID).
		Select("COALESCE(MAX(seq), 0)").Scan(&maxSeq)

	records := make([]*model.SessionMessage, 0, len(newMsgs))
	for i, msg := range newMsgs {
		rec := &model.SessionMessage{
			SessionID: sessionID,
			Seq:       maxSeq + uint(i) + 1,
			Role:      string(msg.Role),
		}
		rec.Content = extractBladesContent(msg)
		rec.ToolName = extractBladesToolName(msg)
		rec.ToolCallID = extractBladesToolCallID(msg)
		rec.ToolRequest = extractBladesToolRequest(msg)
		rec.ToolOutput = extractBladesToolOutput(msg)

		if data, err := json.Marshal(msg); err == nil {
			s := string(data)
			rec.BladesJSON = &s
		}
		records = append(records, rec)
	}

	if err := r.db.WithContext(ctx).Create(records).Error; err != nil {
		return err
	}

	// 失效缓存
	if r.cache != nil {
		go r.cache.Del(context.Background(), sessionID)
	}
	return nil
}

// loadMessages 加载消息历史（优先 Redis 缓存，未命中回源 MySQL）
func (r *SessionRepo) loadMessages(ctx context.Context, sessionID string, limit int) ([]*blades.Message, error) {
	if r.cache != nil {
		if msgs, ok := r.cache.Get(ctx, sessionID); ok {
			if limit > 0 && len(msgs) > limit {
				return msgs[len(msgs)-limit:], nil
			}
			return msgs, nil
		}
	}

	var records []model.SessionMessage
	if limit > 0 {
		if err := r.db.WithContext(ctx).Raw(
			"SELECT * FROM (SELECT * FROM session_messages WHERE session_id = ? ORDER BY seq DESC LIMIT ?) AS recent ORDER BY seq ASC",
			sessionID, limit,
		).Scan(&records).Error; err != nil {
			return nil, fmt.Errorf("查询会话历史失败: %w", err)
		}
	} else {
		if err := r.db.WithContext(ctx).
			Where("session_id = ?", sessionID).
			Order("seq ASC").Find(&records).Error; err != nil {
			return nil, fmt.Errorf("查询会话历史失败: %w", err)
		}
	}

	msgs := make([]*blades.Message, 0, len(records))
	for _, rec := range records {
		if msg := restoreBladesMessage(&rec); msg != nil {
			msgs = append(msgs, msg)
		}
	}

	if r.cache != nil && len(msgs) > 0 {
		go r.cache.Set(context.Background(), sessionID, msgs)
	}
	return msgs, nil
}

// saveState upsert 业务状态
func (r *SessionRepo) saveState(ctx context.Context, sessionID, key string, state any) error {
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("序列化状态[%s]失败: %w", key, err)
	}
	rec := SessionState{
		ID:        sessionID + ":" + key,
		SessionID: sessionID,
		StateKey:  key,
		Data:      string(data),
		UpdatedAt: time.Now(),
	}
	return r.db.WithContext(ctx).Save(&rec).Error
}

// loadStates 加载所有业务状态
func (r *SessionRepo) loadStates(ctx context.Context, sessionID string) (map[string]any, error) {
	var records []SessionState
	if err := r.db.WithContext(ctx).Where("session_id = ?", sessionID).Find(&records).Error; err != nil {
		return nil, err
	}
	states := make(map[string]any, len(records))
	for _, rec := range records {
		var val any
		if err := json.Unmarshal([]byte(rec.Data), &val); err != nil {
			r.logger.Warn("反序列化状态失败", zap.String("key", rec.StateKey), zap.Error(err))
			continue
		}
		states[rec.StateKey] = val
	}
	return states, nil
}

// ============================================
// 辅助函数
// ============================================

func restoreBladesMessage(rec *model.SessionMessage) *blades.Message {
	if rec.BladesJSON == nil {
		return nil
	}
	var msg blades.Message
	if err := json.Unmarshal([]byte(*rec.BladesJSON), &msg); err != nil {
		return nil
	}
	return &msg
}

func extractBladesContent(msg *blades.Message) *string {
	var parts []string
	for _, p := range msg.Parts {
		if t, ok := any(p).(blades.TextPart); ok {
			parts = append(parts, t.Text)
		}
	}
	if len(parts) == 0 {
		return nil
	}
	s := strings.Join(parts, "\n")
	return &s
}

func extractBladesToolName(msg *blades.Message) string {
	for _, p := range msg.Parts {
		if t, ok := any(p).(blades.ToolPart); ok {
			return t.Name
		}
	}
	return ""
}

func extractBladesToolCallID(msg *blades.Message) string {
	for _, p := range msg.Parts {
		if t, ok := any(p).(blades.ToolPart); ok {
			return t.ID
		}
	}
	return ""
}

func extractBladesToolRequest(msg *blades.Message) *string {
	for _, p := range msg.Parts {
		if t, ok := any(p).(blades.ToolPart); ok && t.Request != "" {
			s := t.Request
			return &s
		}
	}
	return nil
}

func extractBladesToolOutput(msg *blades.Message) *string {
	for _, p := range msg.Parts {
		if t, ok := any(p).(blades.ToolPart); ok && t.Response != "" {
			s := t.Response
			return &s
		}
	}
	return nil
}

// UpdateMessageBladesJSON 更新指定消息的 BladesJSON（用于修改已持久化的消息后重新落库）
func (r *SessionRepo) UpdateMessageBladesJSON(ctx context.Context, sessionID, toolName string, msg *blades.Message) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("序列化消息失败: %w", err)
	}
	s := string(data)
	return r.db.WithContext(ctx).Model(&model.SessionMessage{}).
		Where("session_id = ? AND tool_name = ?", sessionID, toolName).
		Update("blades_json", s).Error
}
