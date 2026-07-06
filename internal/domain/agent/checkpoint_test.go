package agent_test

import (
	"context"
	"sync"
	"testing"

	"github.com/CycleZero/Reimbee/internal/domain/agent"
	"github.com/CycleZero/Reimbee/log"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// newTestStore 创建基于 SQLite 内存数据库的 MySQLCheckpointStore
func newTestStore(t *testing.T) *agent.MySQLCheckpointStore {
	t.Helper()

	// 使用共享缓存 URI 确保并发测试中所有 goroutine 共享同一内存数据库
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
		Logger:                                   gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Fatalf("创建测试数据库失败: %v", err)
	}

	// 限制为单个连接，避免 SQLite 内存数据库的并发写锁冲突
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("获取 sql.DB 失败: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)

	logger := &log.Logger{Logger: zap.NewNop()}
	return agent.NewMySQLCheckpointStore(db, logger)
}

func TestMySQLCheckpointStore_SetAndGet(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// 设置 checkpoint
	key := "test:123"
	value := []byte(`{"name":"test","count":42}`)
	if err := store.Set(ctx, key, value); err != nil {
		t.Fatalf("Set 失败: %v", err)
	}

	// 获取已存在的 checkpoint
	got, exists, err := store.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get 失败: %v", err)
	}
	if !exists {
		t.Fatal("期望 checkpoint 存在，但 exists=false")
	}
	if string(got) != string(value) {
		t.Fatalf("数据不匹配: got=%s, want=%s", string(got), string(value))
	}

	// 获取不存在的 key
	got, exists, err = store.Get(ctx, "nonexistent:999")
	if err != nil {
		t.Fatalf("Get 不存在的 key 返回错误: %v", err)
	}
	if exists {
		t.Fatal("不存在的 key 期望 exists=false，但返回了 true")
	}
	if got != nil {
		t.Fatalf("不存在的 key 期望 data=nil，但返回了 %s", string(got))
	}
}

func TestMySQLCheckpointStore_Overwrite(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	key := "graph:session1"

	// 第一次写入
	v1 := []byte(`{"version":1}`)
	if err := store.Set(ctx, key, v1); err != nil {
		t.Fatalf("第一次 Set 失败: %v", err)
	}

	// 第二次写入相同 key，不同数据
	v2 := []byte(`{"version":2,"extra":"updated"}`)
	if err := store.Set(ctx, key, v2); err != nil {
		t.Fatalf("第二次 Set 失败: %v", err)
	}

	// 读取应返回最新数据
	got, exists, err := store.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get 失败: %v", err)
	}
	if !exists {
		t.Fatal("期望 checkpoint 存在，但 exists=false")
	}
	if string(got) != string(v2) {
		t.Fatalf("覆盖后数据不匹配: got=%s, want=%s", string(got), string(v2))
	}
}

func TestMySQLCheckpointStore_Delete(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	key := "delete:test"

	// 先写入
	value := []byte(`{"data":"to_delete"}`)
	if err := store.Set(ctx, key, value); err != nil {
		t.Fatalf("Set 失败: %v", err)
	}

	// 删除
	if err := store.Delete(ctx, key); err != nil {
		t.Fatalf("Delete 失败: %v", err)
	}

	// 读取应返回不存在
	_, exists, err := store.Get(ctx, key)
	if err != nil {
		t.Fatalf("删除后 Get 返回错误: %v", err)
	}
	if exists {
		t.Fatal("删除后仍然可以读到 checkpoint")
	}

	// 删除不存在的 key 不应报错
	if err := store.Delete(ctx, "nonexistent:999"); err != nil {
		t.Fatalf("删除不存在的 key 不应报错: %v", err)
	}
}

func TestMySQLCheckpointStore_Concurrent(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	const numGoroutines = 5
	var wg sync.WaitGroup

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			key := "concurrent:" + string(rune('0'+idx))
			value := []byte(`{"goroutine":` + string(rune('0'+idx)) + `}`)
			if err := store.Set(ctx, key, value); err != nil {
				t.Errorf("goroutine %d: Set 失败: %v", idx, err)
			}
		}(i)
	}

	wg.Wait()

	// 验证所有 key 都可读取
	for i := 0; i < numGoroutines; i++ {
		key := "concurrent:" + string(rune('0'+i))
		got, exists, err := store.Get(ctx, key)
		if err != nil {
			t.Errorf("读取 key=%s 失败: %v", key, err)
		}
		if !exists {
			t.Errorf("key=%s 不存在", key)
		}
		if got == nil {
			t.Errorf("key=%s 数据为 nil", key)
		}
	}
}

func TestMarshalUnmarshalCheckpoint(t *testing.T) {
	type testData struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}

	original := testData{Name: "hello", Value: 42}

	// Marshal
	data, err := agent.MarshalCheckpoint(original)
	if err != nil {
		t.Fatalf("MarshalCheckpoint 失败: %v", err)
	}
	if data == nil {
		t.Fatal("MarshalCheckpoint 返回 nil")
	}
	expectedJSON := `{"name":"hello","value":42}`
	if string(data) != expectedJSON {
		t.Fatalf("JSON 不匹配: got=%s, want=%s", string(data), expectedJSON)
	}

	// Unmarshal
	var restored testData
	if err := agent.UnmarshalCheckpoint(data, &restored); err != nil {
		t.Fatalf("UnmarshalCheckpoint 失败: %v", err)
	}
	if restored.Name != original.Name {
		t.Fatalf("Name 不匹配: got=%s, want=%s", restored.Name, original.Name)
	}
	if restored.Value != original.Value {
		t.Fatalf("Value 不匹配: got=%d, want=%d", restored.Value, original.Value)
	}
}
