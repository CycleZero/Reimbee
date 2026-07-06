# PDF + SMTP 真实实现 — 技术选型与设计方案

> 版本: v1.0 | 日期: 2026-07-06

---

## 一、PDF 生成 — `johnfercher/maroto/v2`

### 选型理由

| 候选 | 结论 |
|------|------|
| `go-pdf/fpdf` | ❌ 底层 API，需手写坐标绘制 |
| `signintech/gopdf` | ❌ 同上 |
| **`johnfercher/maroto/v2`** | ✅ **采纳** — 基于 fpdf 的 Grid 布局引擎，适合表格型文档 |
| `wkhtmltopdf` | ❌ 太重，需外部二进制 |
| Chrome headless | ❌ 太重 |

**maroto v2** 特点：
- **Grid 布局**（12 列栅格）：Row → Col → 填充文本/图片/表格
- **内建表格组件**：自动列宽、表头样式、斑马纹
- **中文支持**：可注册中文字体（如 `simhei.ttf`）
- **轻量**：纯 Go，零外部依赖（与 fpdf 同底层）
- **导入路径**：`github.com/johnfercher/maroto/v2`

### PDF 报销单布局

```
┌──────────────────────────────────────────┐
│         Reimbee 费用报销单                │
│                                         │
│ 报销单号  REIMB-2026-0001                │
│ 申请人    张三（EMP001）                  │
│ 部门      技术部                          │
│ 提交日期  2026-07-06                     │
│ 状态      待审批                          │
│                                         │
├──────────────────────────────────────────┤
│ 票据明细                                 │
│ ┌──────┬──────────┬──────────┬─────────┐│
│ │ 序号  │ 费用类别  │ 金额(元)  │ 备注    ││
│ ├──────┼──────────┼──────────┼─────────┤│
│ │  1   │ 差旅-交通  │ ¥1,500   │ OCR: ¥1,500││
│ │  2   │ 办公用品   │ ¥500     │ —       ││
│ ├──────┼──────────┼──────────┼─────────┤│
│ │ 合计  │          │ ¥2,000   │         ││
│ └──────┴──────────┴──────────┴─────────┘│
│                                         │
│ 合规检查：通过                           │
│ 预算余额：¥50,000（使用率 30%）           │
│                                         │
├──────────────────────────────────────────┤
│ 审批记录                                 │
│ ┌──────────┬──────────┬──────────┬─────┐│
│ │ 审批人    │ 状态      │ 时间      │ 意见 ││
│ ├──────────┼──────────┼──────────┼─────┤│
│ │ 李主管    │ 待审批    │ —        │ —   ││
│ │ 张财务    │ 待审批    │ —        │ —   ││
│ └──────────┴──────────┴──────────┴─────┘│
│                                         │
│ 审批人裁决（待签署）                       │
│ ○ 批准    ○ 驳回                         │
│ 审批意见：_____________                   │
└──────────────────────────────────────────┘
```

---

## 二、邮件发送 — `jordan-wright/email`

### 选型理由

| 候选 | 结论 |
|------|------|
| `net/smtp` | ❌ 不支持 HTML / MIME / 附件 |
| `gomail` | ❌ 停止维护 |
| **`jordan-wright/email`** | ✅ **采纳** — 2k+ stars，API 简洁，支持 HTML + 附件 + CC/BCC |
| SendGrid API | ❌ 需外部服务 |

**jordan-wright/email** 特点：
- `email.NewEmail()` → `.From/.To/.Subject/.HTML/.Attach()` → `.Send()`
- 支持 TLS/STARTTLS 自动协商
- 附件支持 `[]byte` + filename（与现有接口签名完美匹配）
- 导入路径：`github.com/jordan-wright/email`

### 接口映射

```go
// 现有接口
type EmailSender interface {
    SendReimbursementNotification(ctx, to []string, subject, body string, attachment []byte, filename string) error
}

// 实现
func (s *SMTPEmailSender) SendReimbursementNotification(...) error {
    e := email.NewEmail()
    e.From    = s.config.From
    e.To      = to
    e.Subject = subject
    e.HTML    = []byte(body)
    e.Attach(bytes.NewReader(attachment), filename, "application/pdf")
    return e.Send(s.config.Addr, s.auth)
}
```

---

## 三、配置新增

```yaml
# config.yaml
smtp:
  host: "${SMTP_HOST:-smtp.qq.com}"
  port: "${SMTP_PORT:-587}"
  user: "${SMTP_USER:-}"
  password: "${SMTP_PASSWORD:-}"
  from: "${SMTP_FROM:-noreply@reimbee.com}"

# PDF 配置
pdf:
  font_path: "./data/fonts/simhei.ttf"    # 中文字体路径（可空，空时使用内嵌字体但中文乱码）
```

---

## 四、文件变更清单

| 文件 | 操作 | 说明 |
|------|:--:|------|
| `infra/pdf_maroto.go` | **新建** | `MarotoPDFGenerator` — maroto 实现 |
| `infra/smtp_sender.go` | **新建** | `SMTPEmailSender` — jordan-wright/email 实现 |
| `infra/provider.go` | 修改 | 替换 Mock 绑定为真实实现 |
| `infra/pdf_mock.go` | 保留 | 测试用 |
| `infra/smtp_mock.go` | 保留 | 测试用 |
| `config.yaml` | 修改 | `pdf.font_path` 配置项 |
| `go.mod` | 修改 | 新增 2 个依赖 |

---

## 五、Wire 切换策略

```go
// 开发环境：Mock（默认）
// 生产环境：配置 smtp.host 后自动切换为真实实现

// provider.go:
if vc.GetString("smtp.host") != "" {
    // 使用真实 SMTP
    wire.Bind(new(EmailSender), new(*SMTPEmailSender))
} else {
    // 降级 Mock
    wire.Bind(new(EmailSender), new(*MockEmailSender))
}
```

> Wire 不支持条件编译，实际通过 `config.yaml` 的 `smtp.host` 是否为空，在 `NewSMTPEmailSender` 中决定：空 → 返回 nil，Wire fallback 到 Mock。或使用 `wire.Value` 提供 Mock 兜底。

---

## 六、实施步骤

| 步骤 | 内容 | 估时 |
|:--:|------|:--:|
| 1 | `go get` maroto + email 依赖 | 1min |
| 2 | `infra/pdf_maroto.go` — PDF 生成器 | 30min |
| 3 | `infra/smtp_sender.go` — SMTP 发送器 | 15min |
| 4 | `infra/provider.go` — Wire 切换 | 10min |
| 5 | `go build` + `make wire` 验证 | 5min |

---

*设计结束，开始实施。*
