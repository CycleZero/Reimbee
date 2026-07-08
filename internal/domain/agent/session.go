// Package agent 会话对象
//
// Session 实现 blades.Session 接口，纯数据对象，不持有持久化逻辑。
// 持久化由调用方通过 infra.SessionRepo 控制：
//
//	session := NewSession(sessionID)
//	session.Load(ctx, repo)             // 从 DB 恢复
//	session.SetState("key", value)      // 写内存
//	session.Append(ctx, msg)            // 追加消息到内存
//	repo.Save(ctx, session.Snapshot())  // 持久化
package agent

import (
	"context"
	"sync"

	"github.com/CycleZero/Reimbee/infra"
	"github.com/CycleZero/blades"
	"github.com/go-kratos/kit/container/maps"
	"github.com/go-kratos/kit/container/slices"
)

// Session 实现 blades.Session 接口，纯数据，无持久化逻辑
type Session struct {
	id       string
	meta     *infra.SessionMeta
	metaMu   sync.RWMutex                  // 仅保护 meta（maps.Map/slices.Slice 自带并发安全）
	state    maps.Map[string, any]
	messages slices.Slice[*blades.Message]
}

// NewSession 创建空 Session 实例
func NewSession(sessionID string) *Session {
	return &Session{
		id:   sessionID,
		meta: &infra.SessionMeta{SessionID: sessionID, Status: infra.SessionStatusActive},
	}
}

// ── blades.Session 接口 ──

func (s *Session) ID() string { return s.id }

func (s *Session) State() blades.State {
	return s.state.ToMap()
}

func (s *Session) SetState(key string, value any) {
	s.state.Store(key, value)
}

func (s *Session) Append(ctx context.Context, msg *blades.Message) error {
	if _, ok := msg.Actions["await_approval"]; ok {
		// 中断消息：克隆并剥离中断信号后正常落库，保持 reasoning+text+tool_call 结构完整
		clone := msg.Clone()
		delete(clone.Actions, "await_approval")
		delete(clone.Actions, "loop_exit")
		s.messages.Append(clone)
		s.metaMu.Lock()
		s.meta.MessageCount = s.messages.Len()
		s.metaMu.Unlock()
		return nil
	}
	s.messages.Append(msg)
	s.metaMu.Lock()
	s.meta.MessageCount = s.messages.Len()
	s.metaMu.Unlock()
	return nil
}

func (s *Session) History(ctx context.Context) ([]*blades.Message, error) {
	return s.messages.ToSlice(), nil
}

// ── 持久化辅助 ──

// Load 从 SessionRepo 恢复数据到内存
func (s *Session) Load(ctx context.Context, repo *infra.SessionRepo) error {
	loaded, err := repo.Load(ctx, s.id)
	if err != nil {
		return err
	}
	if loaded == nil {
		return nil
	}

	s.metaMu.Lock()
	s.meta = loaded.Meta
	s.metaMu.Unlock()

	for k, v := range loaded.State {
		s.state.Store(k, v)
	}
	for _, msg := range loaded.Messages {
		s.messages.Append(msg)
	}
	return nil
}

// GetOrCreate 从 DB 加载已有会话，不存在则创建新的（无 DB 调用方）
func GetOrCreate(ctx context.Context, sessionID string, repo *infra.SessionRepo) (*Session, error) {
	s := NewSession(sessionID)
	if err := s.Load(ctx, repo); err != nil {
		return nil, err
	}
	return s, nil
}

// Snapshot 导出完整数据快照，供 repo.Save 使用
func (s *Session) Snapshot() *infra.Session {
	s.metaMu.RLock()
	meta := s.meta
	s.metaMu.RUnlock()

	return &infra.Session{
		Meta:     meta,
		Messages: s.messages.ToSlice(),
		State:    s.state.ToMap(),
	}
}

// ── 身份注入 ──

func (s *Session) InjectUser(userID uint, employeeID, employeeName, role string) {
	s.metaMu.Lock()
	s.meta.UserID = userID
	s.meta.EmployeeID = employeeID
	s.meta.EmployeeName = employeeName
	s.meta.Role = role
	s.metaMu.Unlock()
	s.state.Store("user_id", userID)
	s.state.Store("employee_id", employeeID)
	s.state.Store("employee_name", employeeName)
	s.state.Store("role", role)
}

var _ blades.Session = (*Session)(nil)
