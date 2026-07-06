package infra

import (
	"fmt"

	"github.com/spf13/viper"
)

// NewOCRRecognizer 根据配置创建对应的 OCR 识别器实例
// 通过 config.yaml 中的 ocr.driver 字段切换实现:
//   - "multimodal": 多模态大模型（推荐）
//   - "paddle": PaddleOCR 微服务
//   - "mock": 模拟数据（演示/测试）
func NewOCRRecognizer(vc *viper.Viper) (OCRRecognizer, error) {
	driver := vc.GetString("ocr.driver")
	if driver == "" {
		driver = "mock" // 默认使用 Mock 确保演示不出错
	}

	switch driver {
	case "multimodal":
		return NewMultimodalLLMRecognizer(vc), nil
	case "paddle":
		return NewPaddleOCRRecognizer(vc.GetString("ocr.paddle.endpoint"),
			vc.GetDuration("ocr.paddle.timeout")), nil
	case "mock":
		return NewMockOCRRecognizer(), nil
	default:
		return nil, fmt.Errorf("未知 OCR 驱动: %s (可选: multimodal, paddle, mock)", driver)
	}
}
