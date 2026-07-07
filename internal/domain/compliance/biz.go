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

// TODO: 后续审查合规检查逻辑

// ComplianceBiz 合规检查业务逻辑层，基于知识库检索 + 规则阈值评估
type ComplianceBiz struct {
	logger *log.Logger
	kb     *KnowledgeBase
}

// NewComplianceBiz 创建合规检查业务逻辑层实例
func NewComplianceBiz(logger *log.Logger, kb *KnowledgeBase) *ComplianceBiz {
	// 记录初始化事件，标记合规模块采用RAG模式启动
	logger.Debug("初始化合规检查业务逻辑层（RAG模式）")
	return &ComplianceBiz{
		logger: logger,
		kb:     kb,
	}
}

// CheckCompliance 执行合规检查：检索策略知识库 → 提取阈值规则 → 比对判定
func (b *ComplianceBiz) CheckCompliance(ctx context.Context, input *ComplianceInput) (*ComplianceOutput, error) {
	// 记录合规检查的入口参数，包括金额、类别和开票日期
	b.logger.Debug("开始合规检查",
		zap.Int64("金额(分)", input.Amount),
		zap.String("类别", input.Category),
		zap.String("开票日期", input.InvoiceDate))

	// 将金额从分转换为元，便于后续与阈值比对（阈值单位为元）
	amountYuan := float64(input.Amount) / 100.0
	b.logger.Debug("金额单位转换完成", zap.Float64("金额(元)", amountYuan))

	// 构建检索查询字符串，格式为 "类别 金额元"，用于语义匹配策略知识库
	query := fmt.Sprintf("%s %.0f元", input.Category, amountYuan)
	b.logger.Debug("构建知识库检索查询", zap.String("检索字符串", query))

	// 向知识库发起语义检索，获取最相关的5条策略分块
	chunks, err := b.kb.Search(ctx, query, 5)
	if err != nil {
		// 检索失败时采用降级策略：直接放行，避免因知识库不可用而阻塞业务流程
		b.logger.Error("知识库检索失败", zap.Error(err))
		b.logger.Debug("知识库检索异常，降级为默认通过")
		return &ComplianceOutput{
			Result:  model.CheckResultPass,
			Level:   "pass",
			Message: "知识库检索异常，默认通过",
			RuleID:  "default-pass",
		}, nil
	}
	b.logger.Debug("知识库检索成功", zap.Int("返回分块数", len(chunks)))

	// 若检索结果为空，说明知识库中没有与该费用类别匹配的策略规则
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

	// 对检索到的所有策略分块进行规则评估，取最严重的判定结果
	output := b.evaluateRules(input, chunks, amountYuan)
	b.logger.Debug("合规检查完成",
		zap.String("最终结果", output.Result),
		zap.String("匹配规则ID", output.RuleID))
	return output, nil
}

// evaluateRules 对检索到的策略分块逐一评估，合并为最严重的结果
func (b *ComplianceBiz) evaluateRules(input *ComplianceInput, chunks []*model.PolicyChunk, amountYuan float64) *ComplianceOutput {
	// 初始化最坏结果为"通过"，后续逐步升级到更严重级别
	var worstResult = model.CheckResultPass
	var worstMessage string
	var worstRuleID string
	worstLevel := "pass"

	b.logger.Debug("开始评估检索到的策略规则",
		zap.Int("分块总数", len(chunks)),
		zap.Float64("报销金额(元)", amountYuan))

	// 第一轮遍历：对每个分块提取规则，并逐一评估
	for _, chunk := range chunks {
		b.logger.Debug("处理策略分块", zap.Uint("分块ID", chunk.ID))

		// 从策略分块的文本内容中提取金额阈值规则
		rules := extractRules(chunk.Content)
		b.logger.Debug("分块规则提取完成",
			zap.Int("分块ID", int(chunk.ID)),
			zap.Int("提取规则数", len(rules)))

		// 对每条规则逐一评估，判断该报销金额是否合规
		for _, rule := range rules {
			// 检查规则所属类别是否与用户输入的报销类别匹配
			// 不匹配的规则直接跳过（如差旅规则不应用于办公用品报销）
			if !categoryMatches(rule.label, input.Category) {
				b.logger.Debug("规则类别不匹配，跳过",
					zap.String("规则标签", rule.label),
					zap.String("输入类别", input.Category))
				continue
			}

			// 根据规则阈值评估报销金额，返回评估结果和描述信息
			result, msg := rule.evaluate(amountYuan)
			// 生成唯一规则ID，格式为 "chunk-分块ID-规则标签"
			ruleID := fmt.Sprintf("chunk-%d-%s", chunk.ID, rule.label)

			b.logger.Debug("规则评估完成",
				zap.String("规则标签", rule.label),
				zap.Float64("阈值(元)", rule.threshold),
				zap.String("结果", result))

			// 如果当前规则的评估结果比已记录的最坏结果更严重，则更新最坏结果
			if isWorse(result, worstResult) {
				b.logger.Debug("检测到更严重的合规问题",
					zap.String("原结果", worstResult),
					zap.String("新结果", result),
					zap.String("规则ID", ruleID))
				worstResult = result
				worstMessage = msg
				worstRuleID = ruleID
				// 将字符串结果映射为可读的严重等级标识
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

	// 第二轮遍历：额外检查发票有效期（判断开票日期是否超出策略规定的天数）
	for _, chunk := range chunks {
		// 仅处理同时包含"天"和"发票"关键字的策略分块，表明该策略涉及发票时效性
		if strings.Contains(chunk.Content, "天") && strings.Contains(chunk.Content, "发票") {
			b.logger.Debug("检测到发票有效期相关策略，检查发票时效",
				zap.Uint("分块ID", chunk.ID),
				zap.String("开票日期", input.InvoiceDate))
			// 检查发票是否仍处于有效期或即将过期
			ok, msg := checkInvoiceExpiry(input.InvoiceDate, chunk.Content)
			if !ok {
				b.logger.Debug("发票已过期",
					zap.String("描述", msg))
				// 过期是最严重的违规类型，直接设为 error 级别
				if isWorse(model.CheckResultError, worstResult) {
					worstResult = model.CheckResultError
					worstMessage = msg
					worstRuleID = fmt.Sprintf("chunk-%d-expiry", chunk.ID)
					worstLevel = "error"
				}
			}
		}
	}

	// 若遍历完所有规则后仍为"通过"，填充默认的成功提示信息
	if worstResult == model.CheckResultPass {
		worstMessage = "合规检查通过"
		worstRuleID = "all-pass"
	}

	b.logger.Debug("规则评估汇总完成",
		zap.String("最严重结果", worstResult),
		zap.String("等级", worstLevel))

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
	// 阈值为0表示该规则无金额限制，直接判定为通过
	if r.threshold == 0 {
		return model.CheckResultPass, fmt.Sprintf("规则'%s'无金额限制，通过", r.label)
	}

	// 金额超过阈值：直接判定为违规（error），不可提交报销
	if amountYuan > r.threshold {
		return model.CheckResultError,
			fmt.Sprintf("超出%s标准(≤%.0f元)，实际%.0f元", r.label, r.threshold, amountYuan)
	}

	// 金额在阈值的 90%~100% 之间：给出警告，提示接近上限，审批人需关注
	if amountYuan > r.threshold*0.9 {
		return model.CheckResultWarning,
			fmt.Sprintf("接近%s上限(≤%.0f元)，实际%.0f元", r.label, r.threshold, amountYuan)
	}

	// 金额在安全范围内，判定为通过
	return model.CheckResultPass,
		fmt.Sprintf("%s符合标准(≤%.0f元)，实际%.0f元", r.label, r.threshold, amountYuan)
}

// extractRules 从分块文本中提取所有金额阈值规则
func extractRules(content string) []policyRule {
	// labelMap 建立关键词到标准规则标签的映射关系
	// 策略文本中可能使用口语化描述，需要标准化为系统可识别的规则标签
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

	var rules []policyRule

	// 逐行解析策略文本，提取每行中的金额阈值并匹配对应类别
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		// 去除行首尾空白字符，处理可能的缩进和对齐
		line = strings.TrimSpace(line)
		if line == "" {
			continue // 跳过空行，提高解析效率
		}

		// 从当前行中提取金额阈值（元），支持多种格式如"不超过300元"、"≤ 5000 元"
		threshold := extractAmountFromLine(line)
		if threshold <= 0 {
			continue // 未提取到有效金额，跳过该行
		}

		// 根据关键词匹配规则标签，一行最多匹配一个标签（break 机制保证）
		for keyword, label := range labelMap {
			if strings.Contains(line, keyword) {
				// 成功匹配到类别，创建规则并加入结果集
				rules = append(rules, policyRule{
					label:     label,
					threshold: threshold,
					rawText:   line,
				})
				break // 一行只匹配第一个命中的关键词，避免一条文本生成多条重复规则
			}
		}
	}

	return rules
}

// extractAmountFromLine 从一行文本中提取金额（元），支持多种格式：
//
//	"不超过 **300元**"、"≤ 200 元"、"不超过1500元"、"不超过 5000 元"
func extractAmountFromLine(line string) float64 {
	// 清理 Markdown 加粗标记和常见的中文金额限制前缀词，简化后续数字提取
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

	// 找到"元"字的位置，以此作为数字提取的锚点
	yuanIdx := strings.Index(cleaned, "元")
	if yuanIdx == -1 {
		return 0 // 没有"元"字则无法定位金额，返回0表示无效
	}

	// 从"元"字向前扫描，截取连续的数字字符（含小数点）作为金额数值
	numEnd := yuanIdx   // 数字结束位置 = "元"字的位置
	numStart := yuanIdx // 从"元"字向前找数字起始位置
	for numStart > 0 && isDigitOrDot(rune(cleaned[numStart-1])) {
		numStart-- // 向前移动直到遇到非数字/非小数点字符
	}
	// 未找到任何数字字符，表示"元"字之前没有金额
	if numStart == numEnd {
		return 0
	}

	// 将截取的数字字符串解析为 float64 类型的金额值
	numStr := cleaned[numStart:numEnd]
	val, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return 0 // 解析失败，返回0表示提取无效
	}
	return val
}

// extractDayLimitFromLine 从一行文本中提取天数限制
func extractDayLimitFromLine(line string) int {
	// 清理策略文本中的 Markdown 标记和中文描述前缀，保留纯数字和"天"字
	cleaned := strings.NewReplacer(
		"**", "",
		"*", "",
		"有效", "",
		"有效期", "",
		"内", "",
	).Replace(line)

	// 找到"天"字作为天数提取的锚点
	tianIdx := strings.Index(cleaned, "天")
	if tianIdx == -1 {
		return 0 // 没有"天"字，该行不包含天数限制
	}

	// 从"天"字向前提取数字字符（不含小数点，天数不存在小数情况）
	numEnd := tianIdx
	numStart := tianIdx
	// 向前扫描直到遇到非数字字符，确定数字起始位置
	for numStart > 0 && isDigit(cleaned[numStart-1]) {
		numStart--
	}
	if numStart == numEnd {
		return 0 // 未找到数字字符
	}

	// 将数字字符串解析为整数天数
	days, err := strconv.Atoi(cleaned[numStart:numEnd])
	if err != nil {
		return 0 // 解析失败，返回0表示提取无效
	}
	return days
}

// ============================================================
// 辅助函数
// ============================================================

// categoryMatches 判断规则标签是否与输入的费用类别匹配
func categoryMatches(ruleLabel, inputCategory string) bool {
	// 精确匹配：规则标签与输入类别完全一致
	if ruleLabel == inputCategory {
		return true
	}
	// 前缀匹配：规则和输入都以"差旅"开头时视为匹配
	// 例如"差旅-住宿"规则也能应用于"差旅-交通"报销
	if strings.HasPrefix(ruleLabel, "差旅") && strings.HasPrefix(inputCategory, "差旅") {
		return true
	}
	// 通用规则匹配所有类别：以"通用"开头的规则适用于任何费用类型
	if strings.HasPrefix(ruleLabel, "通用") {
		return true
	}
	return false
}

// isWorse 判断结果 a 是否比结果 b 更严重
// 严重程度排序：error(2) > warning(1) > pass/pending(0)
func isWorse(a, b string) bool {
	// severity 定义各检查结果的严重程度权重，数值越大表示越严重
	severity := map[string]int{
		model.CheckResultPass:    0,
		model.CheckResultPending: 0, // pending 与 pass 同级，表示待处理但不主动干预
		model.CheckResultWarning: 1,
		model.CheckResultError:   2,
	}
	return severity[a] > severity[b]
}

// checkInvoiceExpiry 检查发票是否在有效期内
// content 为包含天数限制的策略分块文本
func checkInvoiceExpiry(invoiceDate, content string) (ok bool, message string) {
	// 从策略文本中提取有效期天数限制
	limitDays := extractDayLimitFromLine(content)
	if limitDays <= 0 {
		return true, "" // 未提取到有效天数限制，视为无需检查，直接通过
	}

	// 解析开票日期字符串为 time.Time 对象，用于计算经过天数
	parsed, err := time.Parse("2006-01-02", invoiceDate)
	if err != nil {
		return true, "" // 日期解析失败时不阻塞流程，避免因格式问题误拦截
	}

	// 计算从开票日到今天已经过的天数
	elapsed := int(time.Since(parsed).Hours() / 24)
	// 已过天数超过有效期限制：发票已过期，返回 false 并附带过期提示
	if elapsed > limitDays {
		return false, fmt.Sprintf("发票已过期%d天（有效期%d天，开票日期%s）",
			elapsed-limitDays, limitDays, invoiceDate)
	}
	// 已过天数接近有效期上限（距过期还剩10天以内）：发出即将过期警告
	if elapsed > limitDays-10 {
		return true, fmt.Sprintf("发票即将过期（剩余%d天，有效期%d天）",
			limitDays-elapsed, limitDays)
	}
	// 发票在有效期内，无需特殊提示
	return true, ""
}

// isDigitOrDot 判断一个 rune 是否为数字或小数点
// 用于金额提取时的字符类型判断，支持小数金额的解析
func isDigitOrDot(r rune) bool {
	return (r >= '0' && r <= '9') || r == '.'
}

// isDigit 判断一个 byte 是否为数字字符（0-9）
// 用于天数提取时的字符类型判断，天数不含小数点
func isDigit(b byte) bool {
	return b >= '0' && b <= '9'
}
