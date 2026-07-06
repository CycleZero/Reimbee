package compliance

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/CycleZero/Reimbee/log"
	"github.com/CycleZero/Reimbee/model"
	"go.uber.org/zap"
)

// ComplianceBiz 合规检查业务逻辑层，基于知识库检索 + 规则阈值评估
type ComplianceBiz struct {
	logger *log.Logger
	kb     *KnowledgeBase
}

// NewComplianceBiz 创建合规检查业务逻辑层实例
func NewComplianceBiz(logger *log.Logger, kb *KnowledgeBase) *ComplianceBiz {
	logger.Debug("初始化合规检查业务逻辑层（RAG模式）")
	return &ComplianceBiz{
		logger: logger,
		kb:     kb,
	}
}

// CheckCompliance 执行合规检查：检索策略知识库 → 提取阈值规则 → 比对判定
func (b *ComplianceBiz) CheckCompliance(ctx context.Context, input *ComplianceInput) (*ComplianceOutput, error) {
	b.logger.Debug("开始合规检查",
		zap.Int64("金额(分)", input.Amount),
		zap.String("类别", input.Category),
		zap.String("开票日期", input.InvoiceDate))

	amountYuan := float64(input.Amount) / 100.0

	// 构建检索查询："类别 金额元"
	query := fmt.Sprintf("%s %.0f元", input.Category, amountYuan)

	// 检索相关知识库分块
	chunks, err := b.kb.Search(ctx, query, 5)
	if err != nil {
		b.logger.Error("知识库检索失败", zap.Error(err))
		return &ComplianceOutput{
			Result:  model.CheckResultPass,
			Level:   "pass",
			Message: "知识库检索异常，默认通过",
			RuleID:  "default-pass",
		}, nil
	}

	if len(chunks) == 0 {
		b.logger.Debug("未找到相关合规规则，默认通过")
		return &ComplianceOutput{
			Result:  model.CheckResultPass,
			Level:   "pass",
			Message: "未找到相关合规规则，默认通过",
			RuleID:  "no-rule",
		}, nil
	}

	b.logger.Debug("检索到相关规则分块", zap.Int("分块数量", len(chunks)))

	// 评估所有匹配的规则
	return b.evaluateRules(input, chunks, amountYuan), nil
}

// evaluateRules 对检索到的策略分块逐一评估，合并为最严重的结果
func (b *ComplianceBiz) evaluateRules(input *ComplianceInput, chunks []*model.PolicyChunk, amountYuan float64) *ComplianceOutput {
	var worstResult = model.CheckResultPass
	var worstMessage string
	var worstRuleID string
	worstLevel := "pass"

	for _, chunk := range chunks {
		rules := extractRules(chunk.Content)

		for _, rule := range rules {
			// 检查规则类别是否与输入匹配
			if !categoryMatches(rule.label, input.Category) {
				continue
			}

			result, msg := rule.evaluate(amountYuan)
			ruleID := fmt.Sprintf("chunk-%d-%s", chunk.ID, rule.label)

			b.logger.Debug("规则评估完成",
				zap.String("规则标签", rule.label),
				zap.Float64("阈值(元)", rule.threshold),
				zap.String("结果", result))

			if isWorse(result, worstResult) {
				worstResult = result
				worstMessage = msg
				worstRuleID = ruleID
				switch result {
				case model.CheckResultError:
					worstLevel = "error"
				case model.CheckResultWarning:
					worstLevel = "warning"
				default:
					worstLevel = "pass"
				}
			}
		}
	}

	// 额外检查发票有效期（如果存在策略提到天数限制）
	for _, chunk := range chunks {
		if strings.Contains(chunk.Content, "天") && strings.Contains(chunk.Content, "发票") {
			ok, msg := checkInvoiceExpiry(input.InvoiceDate, chunk.Content)
			if !ok {
				if isWorse(model.CheckResultError, worstResult) {
					worstResult = model.CheckResultError
					worstMessage = msg
					worstRuleID = fmt.Sprintf("chunk-%d-expiry", chunk.ID)
					worstLevel = "error"
				}
			}
		}
	}

	if worstResult == model.CheckResultPass {
		worstMessage = "合规检查通过"
		worstRuleID = "all-pass"
	}

	return &ComplianceOutput{
		Result:  worstResult,
		Level:   worstLevel,
		Message: worstMessage,
		RuleID:  worstRuleID,
	}
}

// ============================================================
// 规则定义与提取
// ============================================================

// policyRule 从策略文本中提取的单条规则
type policyRule struct {
	label     string  // 规则标签（如"差旅-住宿"）
	threshold float64 // 金额阈值（元），0 表示无金额限制
	limitDays int     // 天数限制（如90天有效期），0 表示无天数限制
	rawText   string  // 原始规则文本片段
}

// evaluate 评估规则是否通过
func (r *policyRule) evaluate(amountYuan float64) (result, message string) {
	if r.threshold == 0 {
		return model.CheckResultPass, fmt.Sprintf("规则'%s'无金额限制，通过", r.label)
	}

	if amountYuan > r.threshold {
		return model.CheckResultError,
			fmt.Sprintf("超出%s标准(≤%.0f元)，实际%.0f元", r.label, r.threshold, amountYuan)
	}

	// 金额在阈值 90% 以上给出警告（接近上限）
	if amountYuan > r.threshold*0.9 {
		return model.CheckResultWarning,
			fmt.Sprintf("接近%s上限(≤%.0f元)，实际%.0f元", r.label, r.threshold, amountYuan)
	}

	return model.CheckResultPass,
		fmt.Sprintf("%s符合标准(≤%.0f元)，实际%.0f元", r.label, r.threshold, amountYuan)
}

// extractRules 从分块文本中提取所有金额阈值规则
func extractRules(content string) []policyRule {
	var rules []policyRule

	labelMap := map[string]string{
		"住宿":   "差旅-住宿",
		"交通":   "差旅-交通",
		"补助":   "差旅-补助",
		"招待":   "招待费",
		"人均":   "招待费",
		"礼品":   "招待费",
		"采购":   "办公用品",
		"办公用品": "办公用品",
		"办公设备": "办公用品",
		"电脑配件": "办公用品",
		"单张发票": "通用-单张限额",
		"单次报销": "通用-单次限额",
	}

	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		threshold := extractAmountFromLine(line)
		if threshold <= 0 {
			continue
		}

		for keyword, label := range labelMap {
			if strings.Contains(line, keyword) {
				rules = append(rules, policyRule{
					label:     label,
					threshold: threshold,
					rawText:   line,
				})
				break
			}
		}
	}

	return rules
}

// extractAmountFromLine 从一行文本中提取金额（元），支持多种格式：
//
//	"不超过 **300元**"、"≤ 200 元"、"不超过1500元"、"不超过 5000 元"
func extractAmountFromLine(line string) float64 {
	// 清理 Markdown 加粗标记
	cleaned := strings.NewReplacer(
		"**", "",
		"*", "",
		"不超过", "",
		"不超过 ", "",
		"≤", "",
		"≤ ", "",
		"不高于", "",
		"不高于 ", "",
	).Replace(line)
	cleaned = strings.TrimSpace(cleaned)

	// 找到"元"字并向前提取数字
	yuanIdx := strings.Index(cleaned, "元")
	if yuanIdx == -1 {
		return 0
	}

	// 从"元"向前扫描，截取连续的数字字符（含小数点）
	numEnd := yuanIdx
	numStart := yuanIdx
	for numStart > 0 && isDigitOrDot(rune(cleaned[numStart-1])) {
		numStart--
	}
	if numStart == numEnd {
		return 0
	}

	numStr := cleaned[numStart:numEnd]
	val, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return 0
	}
	return val
}

// extractDayLimitFromLine 从一行文本中提取天数限制
func extractDayLimitFromLine(line string) int {
	cleaned := strings.NewReplacer(
		"**", "",
		"*", "",
		"有效", "",
		"有效期", "",
		"内", "",
	).Replace(line)

	tianIdx := strings.Index(cleaned, "天")
	if tianIdx == -1 {
		return 0
	}

	// 向前提取数字
	numEnd := tianIdx
	numStart := tianIdx
	for numStart > 0 && isDigit(cleaned[numStart-1]) {
		numStart--
	}
	if numStart == numEnd {
		return 0
	}

	days, err := strconv.Atoi(cleaned[numStart:numEnd])
	if err != nil {
		return 0
	}
	return days
}

// ============================================================
// 辅助函数
// ============================================================

// categoryMatches 判断规则标签是否与输入的费用类别匹配
func categoryMatches(ruleLabel, inputCategory string) bool {
	// 精确匹配
	if ruleLabel == inputCategory {
		return true
	}
	// 前缀匹配（如"差旅-住宿"匹配"差旅"开头的类别）
	if strings.HasPrefix(ruleLabel, "差旅") && strings.HasPrefix(inputCategory, "差旅") {
		return true
	}
	// 通用规则匹配所有类别
	if strings.HasPrefix(ruleLabel, "通用") {
		return true
	}
	return false
}

// isWorse 判断结果 a 是否比结果 b 更严重
func isWorse(a, b string) bool {
	severity := map[string]int{
		model.CheckResultPass:    0,
		model.CheckResultPending: 0,
		model.CheckResultWarning: 1,
		model.CheckResultError:   2,
	}
	return severity[a] > severity[b]
}

// checkInvoiceExpiry 检查发票是否在有效期内
// content 为包含天数限制的策略分块文本
func checkInvoiceExpiry(invoiceDate, content string) (ok bool, message string) {
	limitDays := extractDayLimitFromLine(content)
	if limitDays <= 0 {
		return true, "" // 未提取到有效天数限制，跳过
	}

	parsed, err := time.Parse("2006-01-02", invoiceDate)
	if err != nil {
		return true, "" // 日期解析失败，不阻塞
	}

	elapsed := int(time.Since(parsed).Hours() / 24)
	if elapsed > limitDays {
		return false, fmt.Sprintf("发票已过期%d天（有效期%d天，开票日期%s）",
			elapsed-limitDays, limitDays, invoiceDate)
	}
	if elapsed > limitDays-10 {
		return true, fmt.Sprintf("发票即将过期（剩余%d天，有效期%d天）",
			limitDays-elapsed, limitDays)
	}
	return true, ""
}

func isDigitOrDot(r rune) bool {
	return (r >= '0' && r <= '9') || r == '.'
}

func isDigit(b byte) bool {
	return b >= '0' && b <= '9'
}
