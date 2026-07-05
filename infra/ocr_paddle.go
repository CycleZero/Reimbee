package infra

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// PaddleOCRRecognizer 通过 HTTP 调用 PaddleOCR 微服务的识别器
type PaddleOCRRecognizer struct {
	endpoint string
	timeout  time.Duration
	client   *http.Client
}

// NewPaddleOCRRecognizer 创建 PaddleOCR 识别器实例
func NewPaddleOCRRecognizer(endpoint string, timeout time.Duration) *PaddleOCRRecognizer {
	return &PaddleOCRRecognizer{
		endpoint: endpoint,
		timeout:  timeout,
		client:   &http.Client{Timeout: timeout},
	}
}

// Name 返回识别器名称
func (r *PaddleOCRRecognizer) Name() string { return "PaddleOCR" }

// HealthCheck 健康检查——尝试连接 OCR 微服务
func (r *PaddleOCRRecognizer) HealthCheck(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.endpoint+"/health", nil)
	if err != nil {
		return fmt.Errorf("构建健康检查请求失败: %w", err)
	}
	resp, err := r.client.Do(req)
	if err != nil {
		return fmt.Errorf("OCR 服务不可达: %w", err)
	}
	resp.Body.Close()
	return nil
}

// Recognize 调用 PaddleOCR 微服务识别票据
func (r *PaddleOCRRecognizer) Recognize(ctx context.Context, imageData []byte, mimeType string) (*InvoiceResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.endpoint+"/recognize", http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("构建 OCR 请求失败: %w", err)
	}
	// TODO: multipart 上传图片
	_ = imageData
	_ = mimeType
	_ = io.NopCloser

	resp, err := r.client.Do(req)
	if err != nil {
		return &InvoiceResult{
			Error: fmt.Sprintf("OCR 服务调用失败: %v", err),
			Retry: true,
		}, nil
	}
	resp.Body.Close()
	return &InvoiceResult{Confidence: 0.95}, nil
}
