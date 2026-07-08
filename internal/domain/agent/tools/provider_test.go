package tools_test

import (
	"testing"

	"github.com/CycleZero/Reimbee/internal/domain/agent/tools"
	"github.com/CycleZero/Reimbee/log"
	"go.uber.org/zap"
)

func TestToolSetBladesTypesExist(t *testing.T) {
	logger := &log.Logger{Logger: zap.NewNop()}

	// 验证 ToolSet 结构体能正确初始化
	// 各工具实例由 Wire DI 自动注入，此处仅验证类型正确
	ts := &tools.ToolSet{
		PDF:          &tools.PDFTool{},
		Email:        &tools.EmailTool{},
		Progress:     &tools.ProgressTool{},
		QueryRecords: &tools.QueryTool{},
		SearchPolicy: &tools.SearchPolicyTool{},
		CreateReimb:  &tools.CreateReimbTool{},
	}

	if ts.PDF == nil || ts.Email == nil || ts.Progress == nil ||
		ts.QueryRecords == nil || ts.SearchPolicy == nil || ts.CreateReimb == nil {
		t.Error("ToolSet 字段不应为 nil")
	}
	_ = logger
}
