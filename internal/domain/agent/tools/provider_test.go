package tools_test

import (
	"testing"

	"github.com/CycleZero/Reimbee/internal/domain/agent/tools"
	"github.com/CycleZero/Reimbee/internal/testutil"
	"github.com/CycleZero/Reimbee/log"
	"go.uber.org/zap"
)

func TestToolSetV4TypesExist(t *testing.T) {
	logger := &log.Logger{Logger: zap.NewNop()}
	mockTool := testutil.NewNamedMockTool("mock_ocr", "ok")

	ocrTool := &tools.OCRTool{}
	ocrTool.InvokableTool = mockTool

	ts := &tools.ToolSet{}
	ts.OCR = mockTool
	ts.Budget = mockTool
	ts.Progress = mockTool

	if ts.OCR == nil || ts.Budget == nil || ts.Progress == nil {
		t.Error("ToolSet 字段不应为 nil")
	}
	_ = logger
}
