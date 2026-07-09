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

type ComplianceBiz struct {
	logger *log.Logger
	kb     *KnowledgeBase
}

func NewComplianceBiz(logger *log.Logger, kb *KnowledgeBase) *ComplianceBiz {
	logger.Debug("初始化合规检查业务逻辑层（RAG模式）")
	return &ComplianceBiz{logger: logger, kb: kb}
}

func (b *ComplianceBiz) CheckCompliance(ctx context.Context, input *ComplianceInput) (*ComplianceOutput, error) {
	invoices := input.GetItems()
	if len(invoices) == 0 {
		return &ComplianceOutput{
			Result: model.CheckResultPass,
			Message: "无待审核票据", RuleID: "no-input",
		}, nil
	}

	b.logger.Debug("开始合规检查", zap.Int("明细数", len(invoices)))

	var allResults []ComplianceItemResult
	worstResult := model.CheckResultPass

	for _, item := range invoices {
		for _, rct := range item.Receipts {
			out := b.checkSingle(ctx, rct)
			allResults = append(allResults, ComplianceItemResult{
				Result:   out.Result,
				Message:  out.Message,
				RuleID:   out.RuleID,
				Amount:   rct.Amount,
				Category: rct.Category,
			})
			if isWorse(out.Result, worstResult) {
				worstResult = out.Result
			}
		}
	}

	return &ComplianceOutput{
		Result:  worstResult,
		Message: fmt.Sprintf("共审核%d张票据，%d张通过，%d张警告，%d张违规",
			len(allResults), countBy(allResults, model.CheckResultPass),
			countBy(allResults, model.CheckResultWarning), countBy(allResults, model.CheckResultError)),
		RuleID: worstRuleID(allResults),
		Items:  allResults,
	}, nil
}

func (b *ComplianceBiz) checkSingle(ctx context.Context, inv ComplianceReceiptItem) *ComplianceOutput {
	b.logger.Debug("合规检查(单张)", zap.Int64("金额(分)", inv.Amount), zap.String("类别", inv.Category))
	amountYuan := float64(inv.Amount) / 100.0

	query := fmt.Sprintf("%s %.0f元", inv.Category, amountYuan)
	chunks, err := b.kb.Search(ctx, query, 5)
	if err != nil {
		b.logger.Error("知识库检索失败", zap.Error(err))
		return &ComplianceOutput{Result: model.CheckResultPass, Message: "知识库检索异常，默认通过", RuleID: "default-pass"}
	}
	if len(chunks) == 0 {
		return &ComplianceOutput{Result: model.CheckResultPass, Message: "未找到相关合规规则，默认通过", RuleID: "no-rule"}
	}
	return b.evaluateRules(inv.Category, inv.InvoiceDate, chunks, amountYuan)
}

func (b *ComplianceBiz) evaluateRules(category, invoiceDate string, chunks []*model.PolicyChunk, amountYuan float64) *ComplianceOutput {
	var worstResult = model.CheckResultPass
	var worstMessage, worstRuleID string

	for _, chunk := range chunks {
		rules := extractRules(chunk.Content)
		for _, rule := range rules {
			if !categoryMatches(rule.label, category) {
				continue
			}
			result, msg := rule.evaluate(amountYuan)
			ruleID := fmt.Sprintf("chunk-%d-%s", chunk.ID, rule.label)
			if isWorse(result, worstResult) {
				worstResult = result
				worstMessage = msg
				worstRuleID = ruleID
			}
		}
	}

	for _, chunk := range chunks {
		if strings.Contains(chunk.Content, "天") && strings.Contains(chunk.Content, "发票") {
			ok, msg := checkInvoiceExpiry(invoiceDate, chunk.Content)
			if !ok && isWorse(model.CheckResultError, worstResult) {
				worstResult = model.CheckResultError
				worstMessage = msg
			worstRuleID = fmt.Sprintf("chunk-%d-expiry", chunk.ID)
			}
		}
	}

	if worstResult == model.CheckResultPass {
		worstMessage = "合规检查通过"
		worstRuleID = "all-pass"
	}

	return &ComplianceOutput{
		Result: worstResult,
		Message: worstMessage, RuleID: worstRuleID,
	}
}

type policyRule struct {
	label     string
	threshold float64
	limitDays int
	rawText   string
}

func (r *policyRule) evaluate(amountYuan float64) (string, string) {
	if r.threshold == 0 {
		return model.CheckResultPass, fmt.Sprintf("规则'%s'无金额限制，通过", r.label)
	}
	if amountYuan > r.threshold {
		return model.CheckResultError, fmt.Sprintf("超出%s标准(≤%.0f元)，实际%.0f元", r.label, r.threshold, amountYuan)
	}
	if amountYuan > r.threshold*0.9 {
		return model.CheckResultWarning, fmt.Sprintf("接近%s上限(≤%.0f元)，实际%.0f元", r.label, r.threshold, amountYuan)
	}
	return model.CheckResultPass, fmt.Sprintf("%s符合标准(≤%.0f元)，实际%.0f元", r.label, r.threshold, amountYuan)
}

func extractRules(content string) []policyRule {
	labelMap := map[string]string{
		"住宿": "差旅-住宿", "交通": "差旅-交通", "补助": "差旅-补助",
		"招待": "招待费", "人均": "招待费", "礼品": "招待费",
		"采购": "办公用品", "办公用品": "办公用品", "办公设备": "办公用品", "电脑配件": "办公用品",
		"单张发票": "通用-单张限额", "单次报销": "通用-单次限额",
	}
	var rules []policyRule
	for _, line := range strings.Split(content, "\n") {
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
				rules = append(rules, policyRule{label: label, threshold: threshold, rawText: line})
				break
			}
		}
	}
	return rules
}

func extractAmountFromLine(line string) float64 {
	cleaned := strings.NewReplacer("**", "", "*", "", "不超过", "", "≤", "", "≤ ", "", "不高于", "").Replace(line)
	cleaned = strings.TrimSpace(cleaned)
	yuanIdx := strings.Index(cleaned, "元")
	if yuanIdx == -1 {
		return 0
	}
	numEnd := yuanIdx
	numStart := yuanIdx
	for numStart > 0 && isDigitOrDot(rune(cleaned[numStart-1])) {
		numStart--
	}
	if numStart == numEnd {
		return 0
	}
	val, err := strconv.ParseFloat(cleaned[numStart:numEnd], 64)
	if err != nil {
		return 0
	}
	return val
}

func extractDayLimitFromLine(line string) int {
	cleaned := strings.NewReplacer("**", "", "*", "", "有效", "", "有效期", "", "内", "").Replace(line)
	tianIdx := strings.Index(cleaned, "天")
	if tianIdx == -1 {
		return 0
	}
	numEnd, numStart := tianIdx, tianIdx
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

func categoryMatches(ruleLabel, inputCategory string) bool {
	if ruleLabel == inputCategory {
		return true
	}
	if strings.HasPrefix(ruleLabel, "差旅") && strings.HasPrefix(inputCategory, "差旅") {
		return true
	}
	if strings.HasPrefix(ruleLabel, "通用") {
		return true
	}
	return false
}

func isWorse(a, b string) bool {
	severity := map[string]int{
		model.CheckResultPass: 0, model.CheckResultPending: 0,
		model.CheckResultWarning: 1, model.CheckResultError: 2,
	}
	return severity[a] > severity[b]
}

func checkInvoiceExpiry(invoiceDate, content string) (bool, string) {
	limitDays := extractDayLimitFromLine(content)
	if limitDays <= 0 {
		return true, ""
	}
	parsed, err := time.Parse("2006-01-02", invoiceDate)
	if err != nil {
		return true, ""
	}
	elapsed := int(time.Since(parsed).Hours() / 24)
	if elapsed > limitDays {
		return false, fmt.Sprintf("发票已过期%d天（有效期%d天，开票日期%s）", elapsed-limitDays, limitDays, invoiceDate)
	}
	if elapsed > limitDays-10 {
		return true, fmt.Sprintf("发票即将过期（剩余%d天，有效期%d天）", limitDays-elapsed, limitDays)
	}
	return true, ""
}

func isDigitOrDot(r rune) bool { return (r >= '0' && r <= '9') || r == '.' }
func isDigit(b byte) bool     { return b >= '0' && b <= '9' }

func countBy(items []ComplianceItemResult, result string) int {
	n := 0
	for _, it := range items {
		if it.Result == result { n++ }
	}
	return n
}

func worstRuleID(items []ComplianceItemResult) string {
	for _, it := range items {
		if it.Result == model.CheckResultError || it.Result == model.CheckResultWarning {
			return it.RuleID
		}
	}
	return "all-pass"
}
