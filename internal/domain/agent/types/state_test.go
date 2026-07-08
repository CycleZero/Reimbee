package types

import (
	"encoding/json"
	"testing"
)

func TestInvoiceState_RoundTrip(t *testing.T) {
	original := InvoiceState{
		Date: "2026-01-01",
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}

	var decoded InvoiceState
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}

	if decoded.Date != "2026-01-01" {
		t.Errorf("Date 字段不匹配: 期望 %q, 实际 %q", "2026-01-01", decoded.Date)
	}
}
