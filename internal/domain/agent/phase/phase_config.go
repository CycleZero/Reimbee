// Package phase 三阶段 Agent 配置，定义每个阶段的行为约束和工具集
// Phase 1：信息收集——OCR 票据识别 + 规则查询 + 用户确认
// Phase 2：校验确认——合规检查 + 预算检查 + 最终确认
// Phase 3：执行提交——PDF 生成 + 邮件发送 + 进度告知
package phase

// ============================================
// 阶段标识常量
// ============================================

const (
	Phase1Collect  = "phase1_collect"  // 信息收集
	Phase2Validate = "phase2_validate"  // 校验确认
	Phase3Execute  = "phase3_execute"   // 执行提交
)

// ============================================
// Phase 1：信息收集阶段配置
// ============================================

// Phase1Config Phase 1（信息收集）阶段的运营参数
type Phase1Config struct {
	MaxTurns        int     // 最大交互轮次（防死循环），默认 10
	MinInvoices     int     // 最少票据数量，默认 1
	RequireCategory bool    // 是否要求票据有类别，默认 true
	ConfidenceThreshold float64 // OCR 置信度阈值，低于此值建议用户核对
}

// DefaultPhase1Config 返回 Phase 1 的默认配置
func DefaultPhase1Config() *Phase1Config {
	return &Phase1Config{
		MaxTurns:            10,
		MinInvoices:         1,
		RequireCategory:     true,
		ConfidenceThreshold: 0.7,
	}
}

// ============================================
// Phase 2：校验确认阶段配置
// ============================================

// Phase2Config Phase 2（校验确认）阶段的运营参数
type Phase2Config struct {
	MaxTurns         int  // 最大交互轮次
	RequireCompliance bool // 是否要求合规检查完成
	RequireBudget     bool // 是否要求预算检查完成
	RequireFinalConfirm bool // 是否要求用户最终确认
}

// DefaultPhase2Config 返回 Phase 2 的默认配置
func DefaultPhase2Config() *Phase2Config {
	return &Phase2Config{
		MaxTurns:          10,
		RequireCompliance: true,
		RequireBudget:     true,
		RequireFinalConfirm: true,
	}
}

// ============================================
// Phase 3：执行提交阶段配置
// ============================================

// Phase3Config Phase 3（执行提交）阶段的运营参数
type Phase3Config struct {
	MaxTurns      int  // 最大交互轮次
	RequirePDF    bool // 是否要求 PDF 生成
	AllowSkipEmail bool // 邮件失败是否允许跳过（默认 true）
}

// DefaultPhase3Config 返回 Phase 3 的默认配置
func DefaultPhase3Config() *Phase3Config {
	return &Phase3Config{
		MaxTurns:      5,
		RequirePDF:    true,
		AllowSkipEmail: true,
	}
}
