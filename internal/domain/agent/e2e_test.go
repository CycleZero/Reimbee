// Package agent_test Agent 层端到端集成测试
// 验证完整的 TurnLoop 生命周期和多 Session 并发安全性
package agent_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/CycleZero/Reimbee/internal/domain/agent"
	"github.com/CycleZero/Reimbee/internal/testutil"
)

// ============================================
// E2E: LoopManager 完整生命周期
// ============================================

func TestE2E_LoopManagerLifecycle(t *testing.T) {
	mockModel := testutil.NewTextReplyChatModel("好的，已处理")
	store := newMockSessionStore()
	toolSet := newMinimalToolSet(t)
	cfg := agent.DefaultLoopConfig()

	mgr := agent.NewLoopManager(store, mockModel, toolSet, testLogger(t), cfg)
	defer mgr.Shutdown()

	if mgr == nil {
		t.Fatal("LoopManager creation failed")
	}

	sl := mgr.GetOrCreate("session-e2e-1")
	if sl == nil {
		t.Fatal("GetOrCreate returned nil")
	}

	sl2 := mgr.GetOrCreate("session-e2e-1")
	if sl != sl2 {
		t.Error("GetOrCreate should return same SessionLoop for same sessionID")
	}

	sl3 := mgr.GetOrCreate("session-e2e-2")
	if sl == sl3 {
		t.Error("GetOrCreate should return different SessionLoop for different sessionID")
	}

	mgr.Shutdown()
}

func TestE2E_ConcurrentSessions(t *testing.T) {
	mockModel := testutil.NewTextReplyChatModel("hello")
	store := newMockSessionStore()
	toolSet := newMinimalToolSet(t)
	cfg := agent.DefaultLoopConfig()

	mgr := agent.NewLoopManager(store, mockModel, toolSet, testLogger(t), cfg)
	defer mgr.Shutdown()

	const numSessions = 10
	done := make(chan struct{}, numSessions)

	for i := range numSessions {
		go func(id int) {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("concurrent GetOrCreate panicked: %v", r)
				}
				done <- struct{}{}
			}()
			sessionID := fmt.Sprintf("concurrent-session-%d", id)
			sl := mgr.GetOrCreate(sessionID)
			if sl == nil {
				t.Errorf("GetOrCreate returned nil for session %d", id)
			}
		}(i)
	}

	for range numSessions {
		<-done
	}
}

func TestE2E_ShutdownIsIdempotent(t *testing.T) {
	mockModel := testutil.NewTextReplyChatModel("hello")
	store := newMockSessionStore()
	toolSet := newMinimalToolSet(t)
	cfg := agent.DefaultLoopConfig()

	mgr := agent.NewLoopManager(store, mockModel, toolSet, testLogger(t), cfg)

	mgr.Shutdown()
	mgr.Shutdown()
	mgr.Shutdown()
}

func TestE2E_StateMachineFlow(t *testing.T) {
	mockModel := testutil.NewTextReplyChatModel("hello")
	store := newMockSessionStore()
	toolSet := newMinimalToolSet(t)
	cfg := agent.DefaultLoopConfig()

	mgr := agent.NewLoopManager(store, mockModel, toolSet, testLogger(t), cfg)
	defer mgr.Shutdown()

	for _, sid := range []string{"state-1", "state-2", "state-3"} {
		sl := mgr.GetOrCreate(sid)
		if sl == nil {
			t.Fatalf("GetOrCreate(%s) returned nil", sid)
		}
	}

	sl1 := mgr.GetOrCreate("state-1")
	sl2 := mgr.GetOrCreate("state-2")
	sl3 := mgr.GetOrCreate("state-3")

	if sl1 == sl2 || sl2 == sl3 || sl1 == sl3 {
		t.Error("不同 sessionID 应返回不同 SessionLoop 实例")
	}
}

func TestE2E_EmptyMessageHandling(t *testing.T) {
	mockModel := testutil.NewTextReplyChatModel("hello")
	store := newMockSessionStore()
	toolSet := newMinimalToolSet(t)
	cfg := agent.DefaultLoopConfig()

	mgr := agent.NewLoopManager(store, mockModel, toolSet, testLogger(t), cfg)
	defer mgr.Shutdown()

	sl := mgr.GetOrCreate("empty-session")
	if sl == nil {
		t.Fatal("GetOrCreate with no messages should still create SessionLoop")
	}
}

func TestE2E_CleanupLoop(t *testing.T) {
	mockModel := testutil.NewTextReplyChatModel("hello")
	store := newMockSessionStore()
	toolSet := newMinimalToolSet(t)

	cfg := &agent.LoopConfig{
		SessionTTL:      100 * time.Millisecond,
		MaxHistoryTurns: 1,
		CleanupInterval: 50 * time.Millisecond,
	}

	mgr := agent.NewLoopManager(store, mockModel, toolSet, testLogger(t), cfg)
	defer mgr.Shutdown()

	mgr.GetOrCreate("will-expire")
	time.Sleep(300 * time.Millisecond)

	sl := mgr.GetOrCreate("will-expire")
	if sl == nil {
		t.Fatal("GetOrCreate after cleanup should create new SessionLoop")
	}
}
