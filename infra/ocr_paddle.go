package infra

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"time"
)

// PaddleOCRRecognizer 通过 HTTP multipart 调用 PaddleOCR 微服务
type PaddleOCRRecognizer struct {
	endpoint string
	timeout  time.Duration
	client   *http.Client
}

// NewPaddleOCRRecognizer 创建 PaddleOCR 识别器实例
func NewPaddleOCRRecognizer(endpoint string, timeout time.Duration) *PaddleOCRRecognizer {
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	return &PaddleOCRRecognizer{
		endpoint: endpoint,
		timeout:  timeout,
		client:   &http.Client{Timeout: timeout},
	}
}

func (r *PaddleOCRRecognizer) Name() string { return "PaddleOCR" }

func (r *PaddleOCRRecognizer) HealthCheck(ctx context.Context) error {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, r.endpoint+"/health", nil)
	resp, err := r.client.Do(req)
	if err != nil {
		return fmt.Errorf("PaddleOCR 服务不可达: %w", err)
	}
	resp.Body.Close()
	return nil
}

func (r *PaddleOCRRecognizer) Recognize(ctx context.Context, imageData []byte, _ string) (*InvoiceResult, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("image", "invoice.jpg")
	if err != nil {
		return &InvoiceResult{Error: "构建 OCR 请求失败", Retry: false}, nil
	}
	part.Write(imageData)
	writer.Close()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.endpoint+"/recognize", &body)
	if err != nil {
		return &InvoiceResult{Error: "构建 OCR 请求失败", Retry: false}, nil
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := r.client.Do(req)
	if err != nil {
		return &InvoiceResult{
			Error: fmt.Sprintf("PaddleOCR 服务调用失败: %v", err),
			Retry: true,
		}, nil
	}
	defer resp.Body.Close()

	respBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return &InvoiceResult{
			Error: fmt.Sprintf("PaddleOCR 返回错误 (HTTP %d): %s", resp.StatusCode, string(respBytes)),
			Retry: resp.StatusCode >= 500,
		}, nil
	}

	var result InvoiceResult
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return &InvoiceResult{Error: "解析 PaddleOCR 响应失败", Retry: false}, nil
	}
	return &result, nil
}
