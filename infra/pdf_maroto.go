package infra

import (
	"bytes"
	"fmt"
	"os"

	"github.com/CycleZero/Reimbee/log"
	"github.com/CycleZero/Reimbee/model"
	"github.com/jung-kurt/gofpdf"
	"go.uber.org/zap"
)

// 中文字体文件路径（按优先级查找）
var chineseFontPaths = []string{
	"./data/simhei.ttf",                              // 项目内置（最高优先级）
	"/usr/share/fonts/truetype/wqy/wqy-zenhei.ttc",   // Linux 文泉驿
}

// findChineseFont 查找可用的中文字体文件
func findChineseFont() string {
	for _, p := range chineseFontPaths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// GofpdfPDFGenerator 基于 gofpdf 的 PDF 生成器，轻量零重度依赖（仅 ~200KB）
type GofpdfPDFGenerator struct {
	logger   *log.Logger
	fontPath string
}

// NewGofpdfPDFGenerator 创建 gofpdf PDF 生成器
func NewGofpdfPDFGenerator(logger *log.Logger) *GofpdfPDFGenerator {
	fontPath := findChineseFont()
	logger.Debug("初始化 gofpdf PDF 生成器", zap.String("中文字体", fontPath))
	return &GofpdfPDFGenerator{logger: logger, fontPath: fontPath}
}

// GenerateReimbursementPDF 根据报销单生成标准格式的 PDF 报销单文件
func (g *GofpdfPDFGenerator) GenerateReimbursementPDF(rm *model.Reimbursement) ([]byte, error) {
	g.logger.Debug("开始生成报销单 PDF", zap.String("报销单号", rm.ReimbursementNo))

	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetAutoPageBreak(true, 15)

	// 注册中文字体（SimHei 只有常规体，粗体通过字号模拟）
	fontName := "Helvetica"
	fontBold := "Helvetica"
	if g.fontPath != "" {
		pdf.AddUTF8Font("simhei", "", g.fontPath)
		fontName = "simhei"
		fontBold = "simhei"
	}

	pdf.AddPage()

	// 标题（粗体用大字号模拟）
	pdf.SetFont(fontBold, "", 16)
	pdf.CellFormat(0, 10, "Reimbee 费用报销单", "", 1, "C", false, 0, "")

	// 生成时间
	pdf.SetFont(fontName, "", 9)
	pdf.SetTextColor(120, 120, 120)
	pdf.CellFormat(0, 6, "生成时间: "+rm.CreatedAt.Format("2006-01-02 15:04"), "", 1, "C", false, 0, "")
	pdf.SetTextColor(0, 0, 0)
	pdf.Ln(4)

	// 基本信息区
	g.drawSectionTitle(pdf, fontName, "基本信息")
	pdf.Ln(2)

	drawInfoLine(pdf, fontName, "报销单号", rm.ReimbursementNo, "申请人", rm.EmployeeName+" ("+rm.EmployeeID+")")
	drawInfoLine(pdf, fontName, "部门", getDeptName(rm), "总金额", fmt.Sprintf("%.2f元", float64(rm.TotalAmount)/100.0))
	drawInfoLine(pdf, fontName, "状态", statusText(rm.Status), "事由", rm.SubmitNote)
	if rm.NeedSpecialApproval {
		drawInfoLine(pdf, fontName, "特殊审批", "是（预算不足触发）", "", "")
	}
	pdf.Ln(3)

	// 票据明细表
	g.drawSectionTitle(pdf, fontName, "票据明细")
	pdf.Ln(2)

	colW := []float64{12, 36, 28, 26, 36, 32}
	drawHeader(pdf, fontName, colW, []string{"序号", "费用类别", "开票日期", "金额(元)", "OCR原始值/修正", "合规"})

	total := int64(0)
	for i, inv := range rm.Invoices {
		ocr := "-"
		if inv.IsUserModified {
			ocr = fmt.Sprintf("OCR:%.2f元->%.2f元", float64(inv.OCRRawAmount)/100.0, float64(inv.Amount)/100.0)
		} else if inv.OCRRawAmount > 0 {
			ocr = fmt.Sprintf("OCR:%.2f元", float64(inv.OCRRawAmount)/100.0)
		}
		drawRow(pdf, fontName, colW, []string{
			fmt.Sprintf("%d", i+1),
			inv.Category,
			inv.InvoiceDate,
			fmt.Sprintf("%.2f元", float64(inv.Amount)/100.0),
			ocr,
			inv.CheckResult,
		}, i%2 == 0)
		total += inv.Amount
	}

	// 合计
	pdf.SetFont(	fontName, "", 9)
	pdf.SetFillColor(230, 230, 230)
	pdf.CellFormat(12+36+28+26, 7, "", "", 0, "C", true, 0, "")
	pdf.CellFormat(38, 7, fmt.Sprintf("%.2f元", float64(total)/100.0), "", 0, "R", true, 0, "")
	pdf.CellFormat(30, 7, "", "", 1, "C", true, 0, "")
	pdf.Ln(5)

	// 审批记录
	if len(rm.Approvals) > 0 {
		g.drawSectionTitle(pdf, fontName, "审批记录")
		pdf.Ln(2)
		acw := []float64{30, 24, 52, 64}
		drawHeader(pdf, fontName, acw, []string{"审批人", "状态", "审批时间", "审批意见"})
		for i, a := range rm.Approvals {
			actionAt := "-"
			if a.ActionAt != nil {
				actionAt = a.ActionAt.Format("2006-01-02 15:04")
			}
			drawRow(pdf, fontName, acw, []string{a.ApproverName, a.Action, actionAt, a.Comment}, i%2 == 0)
		}
	}

	// 页脚
	if fontName == "simhei" {
		pdf.SetFont(fontName, "", 8)
	} else {
		pdf.SetFont(fontName, "", 8)
	}
	pdf.SetTextColor(150, 150, 150)
	pdf.CellFormat(0, 5, "本报销单由 Reimbee 智能报销系统自动生成", "", 0, "C", false, 0, "")

	var buf bytes.Buffer
	err := pdf.Output(&buf)
	if err != nil {
		g.logger.Error("PDF輸出失敗", zap.String("報銷單號", rm.ReimbursementNo), zap.Error(err))
		return nil, fmt.Errorf("PDF生成失敗: %w", err)
	}

	g.logger.Info("PDF生成成功",
		zap.String("報銷單號", rm.ReimbursementNo),
		zap.Int("文件大小(bytes)", buf.Len()))
	return buf.Bytes(), nil
}

// drawSectionTitle 绘制区域标题
func (g *GofpdfPDFGenerator) drawSectionTitle(pdf *gofpdf.Fpdf, fontName, title string) {
	pdf.SetFont(	fontName, "", 11)
	pdf.SetFillColor(240, 240, 240)
	pdf.CellFormat(0, 8, "  "+title, "", 1, "L", true, 0, "")
}

// drawInfoLine 绘制信息行
func drawInfoLine(pdf *gofpdf.Fpdf, fontName, l1, v1, l2, v2 string) {
	pdf.SetFont(fontName, "", 9)
	pdf.CellFormat(28, 7, l1+":", "", 0, "L", false, 0, "")
	pdf.CellFormat(62, 7, v1, "", 0, "L", false, 0, "")
	if l2 != "" {
		pdf.CellFormat(22, 7, l2+":", "", 0, "L", false, 0, "")
		pdf.CellFormat(58, 7, v2, "", 1, "L", false, 0, "")
	} else {
		pdf.Ln(7)
	}
}

func drawHeader(pdf *gofpdf.Fpdf, fontName string, widths []float64, headers []string) {
	pdf.SetFont(	fontName, "", 9)
	pdf.SetFillColor(220, 220, 220)
	for i, h := range headers {
		pdf.CellFormat(widths[i], 7, h, "", 0, "C", true, 0, "")
	}
	pdf.Ln(7)
}

func drawRow(pdf *gofpdf.Fpdf, fontName string, widths []float64, vals []string, alt bool) {
	pdf.SetFont(fontName, "", 8)
	if alt {
		pdf.SetFillColor(248, 248, 248)
	}
	for i, v := range vals {
		align := "L"
		if i == 0 || i == 3 {
			align = "C"
		}
		pdf.CellFormat(widths[i], 6, v, "", 0, align, alt, 0, "")
	}
	pdf.Ln(6)
}

func getDeptName(rm *model.Reimbursement) string {
	if rm.Department != nil {
		return rm.Department.Name
	}
	return "-"
}

func statusText(status string) string {
	switch status {
	case "draft":
		return "草稿"
	case "pending":
		return "待审批"
	case "reviewing":
		return "审批中"
	case "approved":
		return "已通过"
	case "rejected":
		return "已驳回"
	default:
		return status
	}
}
