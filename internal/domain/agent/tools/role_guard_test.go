// Package tools_test RoleGuard 外部黑盒测试
//
// 使用外部测试包（tools_test）模拟真实使用场景，
// 通过 stubTool 验证 RoleGuard 的拦截与委托行为。
package tools_test

import (
	"context"
	"strings"
	"testing"

	"github.com/CycleZero/Reimbee/internal/common"
	"github.com/CycleZero/Reimbee/internal/domain/agent/tools"
	btools "github.com/CycleZero/blades/tools"
	"github.com/google/jsonschema-go/jsonschema"
)

// stubTool 是一个最小化的 tools.Tool 实现，用于测试包装器行为。
// called 字段跟踪 Handle 是否被实际调用。
type stubTool struct {
	called bool
}

func (s *stubTool) Name() string                  { return "stub" }
func (s *stubTool) Description() string            { return "stub tool" }
func (s *stubTool) InputSchema() *jsonschema.Schema  { return nil }
func (s *stubTool) OutputSchema() *jsonschema.Schema { return nil }

func (s *stubTool) Handle(_ context.Context, input string) (string, error) {
	s.called = true
	return input, nil
}

func TestRoleGuard(t *testing.T) {
	tests := []struct {
		name     string
		setupCtx func() context.Context // 准备带元数据的 context（nil 表示无元数据）
		allowed  []string               // RoleGuard 允许的角色列表
		wantCall bool                   // 期望 inner.Handle 是否被调用
		wantMsg  string                 // 期望输出包含的子串（空字符串表示不检查）
	}{
		{
			name: "allowed_role",
			setupCtx: func() context.Context {
				return common.SetRequestMetadata(context.Background(), &common.RequestMetadata{
					Role: "employee",
				})
			},
			allowed:  []string{"employee"},
			wantCall: true,
			wantMsg:  "hello",
		},
		{
			name: "denied_role",
			setupCtx: func() context.Context {
				return common.SetRequestMetadata(context.Background(), &common.RequestMetadata{
					Role: "employee",
				})
			},
			allowed:  []string{"approver"},
			wantCall: false,
			wantMsg:  "forbidden",
		},
		{
			name:    "nil_metadata",
			setupCtx: func() context.Context {
				return context.Background()
			},
			allowed:  []string{"employee"},
			wantCall: false,
			wantMsg:  "forbidden",
		},
		{
			name: "admin_admitted",
			setupCtx: func() context.Context {
				return common.SetRequestMetadata(context.Background(), &common.RequestMetadata{
					Role: "admin",
				})
			},
			allowed:  []string{"approver", "admin"},
			wantCall: true,
			wantMsg:  "admin_input",
		},
		{
			name: "delegation",
			setupCtx: func() context.Context {
				return common.SetRequestMetadata(context.Background(), &common.RequestMetadata{
					Role: "employee",
				})
			},
			allowed:  []string{"employee"},
			wantCall: true,
			wantMsg:  "", // 委托测试不关心输出内容，只验证接口委托
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stub := &stubTool{}
			guard := tools.NewRoleGuard(stub, tt.allowed...)

			// 委托测试：验证 Name/Description 正确委托给 stub
			if tt.name == "delegation" {
				if got := guard.Name(); got != "stub" {
					t.Errorf("Name() = %q, want %q", got, "stub")
				}
				if got := guard.Description(); got != "stub tool" {
					t.Errorf("Description() = %q, want %q", got, "stub tool")
				}
			}

			ctx := tt.setupCtx()
			output, err := guard.Handle(ctx, tt.wantMsg)

			if err != nil {
				t.Fatalf("Handle() unexpected error: %v", err)
			}

			if stub.called != tt.wantCall {
				t.Errorf("stub.called = %v, want %v", stub.called, tt.wantCall)
			}

			if tt.wantMsg != "" && !strings.Contains(output, tt.wantMsg) {
				t.Errorf("output = %q, want substring %q", output, tt.wantMsg)
			}

			// denied_role 和 nil_metadata 额外验证输出包含 "forbidden"
			if tt.name == "denied_role" || tt.name == "nil_metadata" {
				if !strings.Contains(output, "forbidden") {
					t.Errorf("expected 'forbidden' in output, got: %s", output)
				}
			}
		})
	}
}

// 编译时检查 stubTool 实现 tools.Tool 接口
var _ btools.Tool = (*stubTool)(nil)
