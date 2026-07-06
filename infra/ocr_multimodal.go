package infra

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/spf13/viper"
)

// MultimodalLLMRecognizer 使用 OpenAI 兼容多模态 API 识别票据
// 通过 Vision API 将图片 + Prompt 发给大模型，直接返回结构化 JSON
type MultimodalLLMRecognizer struct {
	baseURL    string
	apiKey     string
	model      string
	timeout    time.Duration
	client     *http.Client
}

// NewMultimodalLLMRecognizer 创建多模态识别器实例
func NewMultimodalLLMRecognizer(vc *viper.Viper) *MultimodalLLMRecognizer {
	timeout := vc.GetDuration("ocr.multimodal.timeout")
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	return &MultimodalLLMRecognizer{
		baseURL: vc.GetString("ocr.multimodal.base_url"),
		apiKey:  vc.GetString("ocr.multimodal.api_key"),
		model:   vc.GetString("ocr.multimodal.model"),
		timeout: timeout,
		client:  &http.Client{Timeout: timeout},
	}
}

func (r *MultimodalLLMRecognizer) Name() string { return "多模态大模型" }

func (r *MultimodalLLMRecognizer) HealthCheck(ctx context.Context) error {
	if r.apiKey == "" {
		return fmt.Errorf("多模态 API Key 未配置")
	}
	return nil
}

func (r *MultimodalLLMRecognizer) Recognize(ctx context.Context, imageData []byte, mimeType string) (*InvoiceResult, error) {
	// 将图片编码为 Base64
	base64Image := base64.StdEncoding.EncodeToString(imageData)

	// 构建 OpenAI 兼容 Vision API 请求体
	reqBody := map[string]any{
		"model": r.model,
		"messages": []map[string]any{
			{
				"role": "user",
				"content": []map[string]any{
					{
						"type": "image_url",
						"image_url": map[string]string{
							"url": "data:" + mimeType + ";base64," + base64Image,
						},
					},
					{
						"type": "text",
						"text": invoiceRecognitionPrompt,
					},
				},
			},
		},
		"max_tokens":  1024,
		"temperature": 0.1,
	}

	bodyBytes, _ := json.Marshal(reqBody)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.baseURL+"/v1/chat/completions", bytes.NewReader(bodyBytes))
	if err != nil {
		return &InvoiceResult{Error: "构建识别请求失败", Retry: false}, nil
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+r.apiKey)

	resp, err := r.client.Do(req)
	if err != nil {
		return &InvoiceResult{
			Error: fmt.Sprintf("多模态识别服务调用失败: %v", err),
			Retry: true,
		}, nil
	}
	defer resp.Body.Close()

	respBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return &InvoiceResult{
			Error: fmt.Sprintf("多模态 API 返回错误 (HTTP %d): %s", resp.StatusCode, string(respBytes)),
			Retry: resp.StatusCode >= 500,
		}, nil
	}

	// 解析 OpenAI 格式响应，提取 JSON
	var apiResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBytes, &apiResp); err != nil {
		return &InvoiceResult{Error: "解析识别响应失败", Retry: false}, nil
	}
	if len(apiResp.Choices) == 0 {
		return &InvoiceResult{Error: "模型未返回识别结果", Retry: true}, nil
	}

	// 从模型回复中提取 JSON
	content := apiResp.Choices[0].Message.Content

	// 尝试直接解析为 InvoiceResult
	var result InvoiceResult
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		// 不是纯 JSON，尝试提取 JSON 块
		result = extractJSONFromContent(content)
	}
	return &result, nil
}

// invoiceRecognitionPrompt 票据识别的系统提示词，要求模型输出结构化 JSON
const invoiceRecognitionPrompt = `你是一个专业的发票识别助手。请仔细分析这张票据图片，提取以下信息并以 JSON 格式返回。

要求:
1. 金额: 识别票据上的总金额（大写或小写均可），单位：元
2. 日期: 票据开具日期，格式 YYYY-MM-DD
3. 类别: 根据票据内容判断费用类别，可选值: 差旅-交通/差旅-住宿/招待费/办公用品/印刷费/其他
4. 销售方: 票据开具单位名称
5. 发票代码: 票据上的发票代码（通常在右上角）
6. 发票号码: 票据上的发票号码
7. confidence: 对整体识别结果的置信度，0~1

如果某个字段无法识别，将其值设为空字符串，confidence 设为 0。
只返回 JSON，不要添加任何其他文本或解释。

返回格式示例:
{
  "invoice_code": "044001900111",
  "invoice_number": "12345678",
  "amount": 1500.00,
  "date": "2026-07-01",
  "seller_name": "XX科技有限公司",
  "category": "办公用品",
  "confidence": 0.95
}`

// extractJSONFromContent 从模型返回的文本中提取 JSON 块
func extractJSONFromContent(content string) InvoiceResult {
	var result InvoiceResult
	// 尝试找到 { 开头 } 结尾的 JSON 块
	start := -1
	end := -1
	for i, c := range content {
		if c == '{' && start == -1 {
			start = i
		}
	}
	for i := len(content) - 1; i >= 0; i-- {
		if content[i] == '}' {
			end = i + 1
			break
		}
	}
	if start >= 0 && end > start {
		jsonStr := content[start:end]
		json.Unmarshal([]byte(jsonStr), &result)
	}
	return result
}
