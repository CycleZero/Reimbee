// Package infra 业务状态持久化接口
package infra

import "context"

// StateStore 工具读写业务状态的统一接口。
// 通过 sessionID + key 读写 session_states 表中的 JSON 快照。
//
// 实现: *SessionRepo（SaveState/GetState/DeleteState 方法）
type StateStore interface {
	SaveState(ctx context.Context, sessionID string, key string, state any) error
	GetState(ctx context.Context, sessionID string, key string, target any) (bool, error)
	DeleteState(ctx context.Context, sessionID string, key string) error
}
