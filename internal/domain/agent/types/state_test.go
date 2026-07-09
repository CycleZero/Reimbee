package types

import (
	"encoding/json"
	"testing"
)

func TestReceiptState_RoundTrip(t *testing.T) {
	original := ReceiptState{
		ImagePath:    "/path/to/invoice.png",
		Amount:       150000,
		Category:     "差旅-交通",
		Date:         "2026-06-20",
		InvoiceCode:  "031002200111",
		InvoiceNo:    "98765432",
		OCRConfidence: 0.95,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}

	var decoded ReceiptState
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}

	if decoded.Amount != original.Amount {
		t.Errorf("Amount: 期望 %d, 实际 %d", original.Amount, decoded.Amount)
	}
	if decoded.ImagePath != original.ImagePath {
		t.Errorf("ImagePath 不匹配")
	}
}

func TestReimbursementState_JSONRoundTrip(t *testing.T) {
	original := ReimbursementState{
		Items: []ItemState{{
			Category:    "差旅-交通",
			Amount:      150000,
			Description: "北京→上海机票",
			Receipts: []ReceiptState{{
				ImagePath: "/path/img.png",
				Amount:    150000,
				Category:  "差旅-交通",
				Date:      "2026-06-20",
			}},
		}},
		PendingReceipts: []ReceiptState{{
			ImagePath: "/path/pending.png",
			Amount:    50000,
			Category:  "办公用品",
			Date:      "2026-07-01",
		}},
		TotalAmount:     150000,
		CurrentPhase:    "phase3_submit",
		ReimbursementID: 1,
		EmployeeID:      "E001",
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal 失败: %v", err)
	}

	var decoded ReimbursementState
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal 失败: %v", err)
	}

	if len(decoded.Items) != 1 {
		t.Errorf("Items 数量: 期望 1, 实际 %d", len(decoded.Items))
	}
	if len(decoded.PendingReceipts) != 1 {
		t.Errorf("PendingReceipts 数量: 期望 1, 实际 %d", len(decoded.PendingReceipts))
	}
}
