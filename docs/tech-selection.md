# 企业财务报销助手 — 技术选型报告

> 版本: v1.0 | 日期: 2026-07-03 | 编制: 技术选型组

---

## 文档修订记录

| 版本 | 日期 | 修订说明 |
|:--:|------|------|
| v1.0 | 2026-07-03 | 初始版本，全栈技术选型 |

---

## 一、选型原则

1. **继承优先**：gin-template 已有且合适的组件直接沿用，不替换
2. **生产级优先**：选择在生产环境中经过验证的方案，避免实验性项目
3. **中文支持优先**：所有涉及文本输出的组件必须原生支持 CJK
4. **维护活跃优先**：最近 12 个月内有实质性更新的项目
5. **简单优先**：在满足需求的前提下，选择最简单的方案

---

## 二、已确定组件（继承自 gin-template）

| 类别 | 组件 | 版本 | 说明 |
|------|------|:--:|------|
| 编程语言 | Go | ≥ 1.23 | gin-template 要求 |
| HTTP 框架 | Gin | v1.10 | 高性能 HTTP 框架 |
| ORM | GORM | v1.25 | 全功能 ORM，支持 AutoMigrate |
| 数据库驱动 | go-sql-driver/mysql | v1.7 | GORM MySQL 驱动 |
| 缓存 | go-redis | v9.7 | Redis 客户端 |
| 依赖注入 | Google Wire | v0.6 | 编译期 DI，无运行时反射 |
| 配置管理 | Viper | v1.19 | 多格式配置，环境变量覆盖 |
| 日志 | Zap | v1.27 | 高性能结构化日志 |
| 日志轮转 | lumberjack | v2.2 | 日志文件自动切割 |
| JWT | golang-jwt | v5.2 | JWT 鉴权 |
| 性能分析 | gin-contrib/pprof | v1.5 | 内置 pprof 端点 |

---

## 三、新选型：Agent 框架与 LLM

### 3.1 Agent 框架

| 候选方案 | 语言 | 维护者 | 特点 |
|---------|:--:|------|------|
| **Eino ADK** | Go | 字节跳动 | ChatModelAgent + ReAct + 工具编排 + 流式 |
| LangChainGo | Go | 社区 | LangChain 的 Go 移植，API 不稳定 |
| 自研 Agent 循环 | Go | 自行开发 | 完全可控，但工作量大 |

**选定**: **Eino ADK** (`github.com/cloudwego/eino` v0.9.x)

**理由**:
- 字节跳动内部 200+ 服务验证过的生产级框架
- ChatModelAgent 内置 ReAct 循环，工具调用全自动
- `runner.Query()` 返回迭代器，天然适配 SSE 流式输出
- 中间件体系成熟（摘要、文件系统、工具搜索等）
- 与 Gin 的并发模型（goroutine per request）天然兼容

**注意事项**:
- Eino 的工具定义使用 `utils.InferTool()`，依赖 Go struct tags
- 工具依赖注入需通过工厂函数 + 闭包模式实现（Wire 兼容）
- 参考项目: [aggo](https://github.com/CoolBanHub/aggo) — Eino + SSE 企业级 Agent 框架

### 3.2 LLM 提供商

| 候选方案 | 提供商 | 定价 | 中文能力 | Go SDK |
|---------|------|:--:|:--:|:--:|
| **OpenAI 兼容 API** | 任意 | 按 token | 取决于模型 | eino-ext/openai |
| 火山方舟（ARK） | 字节跳动 | 按 token | 优秀（豆包系列） | eino-ext/ark（官方） |

**选定**: **OpenAI 兼容模式**（`eino-ext/openai`），支持任意兼容端点

**理由**:
- 不锁定单一供应商，可接入火山方舟/DeepSeek/OpenAI 等任意兼容 API
- `eino-ext/components/model/openai` 提供标准 Go SDK
- 通过 `base_url` 配置切换，零代码改动
- 备选方案确保演示不受单一供应商影响

**模型选择**: 取决于所接 API，推荐 32K+ 上下文模型以支持多轮对话

---

## 四、新选型：基础设施

### 4.1 PDF 生成

| 候选方案 | 类型 | CJK 支持 | 维护状态 | 备注 |
|---------|------|:--:|:--:|------|
| **gpdf** | 纯 Go | ✅ 原生 | ✅ 2026 年活跃 | 零依赖，10-30x 更快，12 列网格布局 |
| gofpdf | 纯 Go | ⚠️ 手动字体 | ❌ 已归档 | 经典但停止维护 |
| maroto v2 | 纯 Go | ⚠️ 通过 gofpdf | ✅ 活跃 | 抽象层更高，但底层依赖 gofpdf |
| unidoc/unipdf | 纯 Go | ✅ | ✅ 活跃 | 商业授权，需付费 |

**选定**: **gpdf** (`github.com/gpdf-dev/gpdf`)，备选 maroto v2

**理由**:
- 2026 年新库，专为 Go 现代化 PDF 生成设计
- 零外部依赖（不依赖 C 库或系统字体）
- `AddUTF8Font` 原生支持中文 TrueType 字体
- Bootstrap 式 12 列网格布局，适合报销单表格
- 速度是 gofpdf 的 10-30 倍

**风险与缓解**:
- gpdf 是较新的库（2026），社区案例较少
- 缓解: 提前验证中文表格渲染效果；若不满足，降级到 maroto v2
- 备选: 若两种方案均失败，用 Go `html/template` 生成 HTML → wkhtmltopdf 转换

**字体方案**:
- 使用 Noto Sans SC（思源黑体）作为默认中文字体
- 字体文件内嵌在二进制中（`embed.FS`），不依赖系统字体

### 4.2 邮件发送

| 候选方案 | 维护状态 | 附件支持 | 认证方式 | 备注 |
|---------|:--:|:--:|------|------|
| **go-mail** | ✅ 2026.05 | ✅ 内置 | 8 种 | MIT，OpenSSF 最佳实践徽章 |
| gomail | ❌ 2017 | ✅ | 有限 | 110 个未关闭 issue |
| net/smtp | 🔒 冻结 | ❌ 需手写 MIME | PLAIN | Go 标准库，功能受限 |

**选定**: **go-mail** (`github.com/wneessen/go-mail` v0.7)

**理由**:
- 2026 年 5 月仍在活跃更新
- 附件支持从 `io.Reader`、文件系统、`embed.FS` 多种来源
- 8 种 SMTP 认证方式，覆盖国内主流邮箱（QQ、163、企业微信）
- HTML 邮件 + 附件一行代码
- UTF-8 中文标题自动 RFC 2047 编码

**使用示例**:
```go
m := mail.NewMsg()
m.From("noreply@company.com")
m.To("approver@company.com")
m.Subject("报销审批通知")
m.SetBodyString(mail.TypeTextHTML, "<h2>报销单</h2>...")
m.AttachFile("reimbursement.pdf")
c, _ := mail.NewClient("smtp.qq.com", mail.WithPort(587), ...)
c.DialAndSend(m)
```

### 4.3 OCR 方案

#### 4.3.1 架构设计：接口抽象 + 策略模式

OCR 模块采用 Go interface 抽象，支持运行时切换实现。这是整个系统中最需要"可替换性"的组件——OCR 引擎的选择取决于部署环境、准确率需求、成本预算。

**核心接口定义**：

```go
// infra/ocr.go — OCR 能力抽象接口

// InvoiceResult OCR 识别后返回的结构化票据信息
type InvoiceResult struct {
    InvoiceCode   string  `json:"invoice_code"`   // 发票代码
    InvoiceNumber string  `json:"invoice_number"` // 发票号码
    Amount        float64 `json:"amount"`         // 金额（元）
    Date          string  `json:"date"`           // 开票日期 YYYY-MM-DD
    SellerName    string  `json:"seller_name"`    // 销售方名称
    SellerTaxID   string  `json:"seller_tax_id"`  // 销售方税号
    BuyerName     string  `json:"buyer_name"`     // 购买方名称
    Category      string  `json:"category"`       // 费用类别（由 OCR + 规则推断）
    Confidence    float64 `json:"confidence"`     // 识别置信度 0~1
    RawText       string  `json:"raw_text"`       // OCR 原始文本
    Error         string  `json:"error,omitempty"` // 错误信息
    Retry         bool    `json:"retry"`          // 是否建议重试
}

// OCRRecognizer OCR 识别器接口
// 所有 OCR 实现必须满足此接口
type OCRRecognizer interface {
    // Recognize 识别单张票据图片
    Recognize(ctx context.Context, imageData []byte, mimeType string) (*InvoiceResult, error)
    
    // Name 返回识别器名称（用于日志和诊断）
    Name() string
    
    // HealthCheck 健康检查，判断服务是否可用
    HealthCheck(ctx context.Context) error
}
```

#### 4.3.2 实现方案对比与选型

| 实现方案 | 类型 | 中文能力 | 部署方式 | 适用场景 |
|---------|------|:--:|------|------|
| **PaddleOCR 微服务** | gRPC 调用 Python 服务 | ⭐⭐⭐⭐⭐ | Docker，独立进程 | **默认方案**：高准确率，需 GPU/CPU |
| Tesseract 本地 | 系统命令调用 | ⭐⭐ | apt install tesseract | 降级方案：零网络依赖 |
| 云端 OCR API | HTTP API | ⭐⭐⭐ | 云服务 | 备选：无 GPU 时用云服务 |
| Mock 实现 | 返回固定数据 | — | 纯内存 | 单元测试 / 演示离线模式 |

**默认实现**: PaddleOCR 微服务（`PaddleOCRRecognizer`）
**降级链**: PaddleOCR → Tesseract 本地 → Mock（演示保底）

#### 4.3.3 实现切换机制

通过 Viper 配置 + Wire 注入实现运行时切换：

```yaml
# config.yaml — OCR 配置段
ocr:
  driver: "paddle"          # paddle | tesseract | cloud | mock
  paddle:
    endpoint: "localhost:50051"
    timeout: 30s
  tesseract:
    binary: "/usr/bin/tesseract"
    lang: "chi_sim"
  cloud:
    endpoint: "https://your-ocr-api.example.com"
```

```go
// infra/provider.go — 根据配置选择实现
func NewOCRRecognizer(vc *viper.Viper) (OCRRecognizer, error) {
    switch vc.GetString("ocr.driver") {
    case "paddle":
        return NewPaddleOCRRecognizer(vc)
    case "tesseract":
        return NewTesseractRecognizer(vc)
    case "mock":
        return NewMockOCRRecognizer(), nil
    default:
        return NewPaddleOCRRecognizer(vc) // 默认
    }
}
```

Wire 注入时，所有依赖 OCR 的组件只需依赖 `OCRRecognizer` 接口，不感知具体实现：

```go
// Agent 工具只依赖接口
func NewOCRTool(recognizer OCRRecognizer) tool.InvokableTool {
    return utils.InferTool("recognize_invoice", "...",
        func(ctx context.Context, input *OCRInput) (*InvoiceResult, error) {
            return recognizer.Recognize(ctx, input.ImageData, input.MimeType)
        })
}
```

#### 4.3.4 PaddleOCR 微服务（默认实现）

**接口协议**:
- 主协议: gRPC（protobuf 定义，类型安全）
- 降级: HTTP REST `POST /recognize`（multipart/form-data）
- 超时: 30s

**降级策略**:
- OCR 服务不可用 -> Agent 跳过 OCR 工具，询问用户手动输入
- OCR 结果置信度 < 60% -> 返回结果 + 标记"需人工确认"，Agent 提示核对
- 服务彻底不可达 -> 若配置了降级驱动（tesseract/mock），自动切换

### 4.4 图片预处理（OCR 前处理）

| 候选方案 | 类型 | 维护状态 | 备注 |
|---------|------|:--:|------|
| **prism** | 纯 Go | ✅ 2026 | API 兼容 imaging，已修复 CVE-2023-36308 |
| bimg | CGO + libvips | ✅ | 极快，但需安装 libvips 系统库 |
| disintegration/imaging | 纯 Go | ❌ 2020 | 已归档，有未修复 CVE |

**选定**: **prism** (`github.com/agentine/prism`)

**理由**:
- 纯 Go，零系统依赖，Docker 镜像体积可控
- 支持自动方向修正（手机拍照常见问题）、对比度增强、灰度化
- 100% API 兼容 imaging，迁移成本为零

**预处理流程**:
```
原图 → AutoOrientation → Fit(2000px) → Contrast(+15%) → Sharpen(1.0) → Grayscale → JPEG(95%)
```

---

## 五、新选型：前端技术栈

### 5.1 前端框架

| 候选方案 | 类型 | 生态 | 备注 |
|---------|------|:--:|------|
| **React 18** | SPA 框架 | 最丰富 | 生态最大，Ant Design 配套 |
| Vue 3 | SPA 框架 | 丰富 | 学习曲线低，但组件库不如 React |
| HTMX + 模板 | MPA | 有限 | 极简，但交互能力受限 |

**选定**: **React 18 + TypeScript**

### 5.2 UI 组件库

| 候选方案 | 设计语言 | 中文支持 | 图表支持 |
|---------|------|:--:|:--:|
| **Ant Design 5** | 企业级 | ⭐⭐⭐⭐⭐ | 无内置，需配合图表库 |
| Arco Design | 企业级 | ⭐⭐⭐⭐⭐ | 内置简单图表 |
| Material UI | Google | ⭐⭐⭐ | 需额外配置中文 |

**选定**: **Ant Design 5** (`antd` v5)

**理由**: 中文企业级 UI 的事实标准，Table/Form/Modal/Steps 等组件开箱即用。

### 5.3 图表库

| 候选方案 | 图表类型 | 定制能力 | 备注 |
|---------|------|:--:|------|
| **ECharts** | 全类型 | 极高 | Apache 项目，中文文档完善 |
| Recharts | React 原生 | 中 | JSX 声明式，简单易用 |
| AntV/G2 | 全类型 | 高 | Ant Design 同门，定制灵活 |

**选定**: **ECharts** (`echarts-for-react`)

**理由**: 预算看板需要环形图和柱状图，ECharts 的配置式 API 比 Recharts 的 JSX 更适合复杂图表。

### 5.4 构建工具

| 候选方案 | 类型 | 速度 | 备注 |
|---------|------|:--:|------|
| **Vite** | 构建工具 | 极快 | 2026 年主流选择 |
| Create React App | 构建工具 | 慢 | 已不推荐 |
| Next.js | 全栈框架 | 快 | SSR 功能本项目不需要 |

**选定**: **Vite 6 + React 插件**

### 5.5 SSE 客户端

不引入第三方 SSE 库（如 `eventsource` polyfill）。浏览器原生 `EventSource` API 已足够，结合 React `useEffect` 封装成自定义 hook：

```typescript
// 自定义 useSSE hook，无需外部依赖
function useSSE(url: string) {
  useEffect(() => {
    const es = new EventSource(url);
    es.addEventListener('message', handler);
    es.addEventListener('tool_call', handler);
    es.addEventListener('done', () => es.close());
    return () => es.close();
  }, [url]);
}
```

### 5.6 HTTP 客户端

**选定**: 浏览器原生 `fetch` API（不引入 axios）。2026 年所有主流浏览器已完整支持 fetch，且 Streaming Response 处理 SSE 降级方案时更灵活。

---

## 六、新选型：Go 辅助依赖

### 6.1 HTTP 客户端（Go 端调用 OCR 服务）

| 候选方案 | 维护状态 | 特点 |
|---------|:--:|------|
| **net/http** | ✅ 标准库 | 零依赖，Go 1.23 已足够好用 |
| resty | ✅ 活跃 | 链式 API，但增加依赖 |

**选定**: **`net/http`（标准库）**

**理由**: Go 1.23 的 `net/http` 已支持 context、超时、重定向控制，OCR 客户端只需简单的 POST 请求，无需引入额外依赖。

### 6.2 UUID 生成

| 候选方案 | 维护状态 | 特点 |
|---------|:--:|------|
| **google/uuid** | ✅ 活跃 | 官方实现，v5 支持 UUIDv7（时间有序） |
| gofrs/uuid | ✅ 活跃 | 社区实现 |

**选定**: **`github.com/google/uuid` v5**。用于生成报销单号中的 UUID 部分和文件存储的随机文件名。

### 6.3 金额处理

**选定**: **`int64`（分为单位）**，不使用 `float64` 或 `decimal` 库。

**理由**: 涉及金额的计算（汇总、预算扣减）需要避免浮点精度问题。以"分"为最小单位，`int64` 可表示最大 ¥92,233,720,368,547,758.07，远超业务需要。前端展示时除以 100 转换。

> 注: GORM 模型中存储时使用 `int64`，API 序列化/前端展示时转为 `float64`（元）。

### 6.4 文件上传校验

**选定**: 标准库 `mime/multipart` + 手工 MIME 白名单校验。

无需引入 `github.com/h2non/filetype` 等第三方库——通过读取文件头魔数（magic bytes）校验真实类型即可：
- `\xFF\xD8\xFF` = JPEG
- `\x89PNG` = PNG
- `%PDF` = PDF

---

## 七、依赖总览

### 7.1 go.mod 依赖清单

```
module github.com/CycleZero/Reimbee

go 1.23.0

// ========== gin-template 已有 ==========
require (
    github.com/fatih/color v1.18.0
    github.com/gin-contrib/pprof v1.5.3
    github.com/gin-gonic/gin v1.10.0
    github.com/golang-jwt/jwt/v5 v5.2.1
    github.com/google/wire v0.6.0
    github.com/redis/go-redis/v9 v9.7.0
    github.com/shengyanli1982/law v0.1.19
    github.com/spf13/viper v1.19.0
    go.uber.org/zap v1.27.0
    gopkg.in/natefinch/lumberjack.v2 v2.2.1
    gorm.io/driver/mysql v1.5.7
    gorm.io/gorm v1.25.12
)

// ========== 新增：Agent + LLM ==========
require (
    github.com/cloudwego/eino v0.9.x                    // Agent 框架
    github.com/cloudwego/eino-ext/components/model/openai // OpenAI 兼容 ChatModel
)

// ========== 新增：PDF 生成 ==========
require (
    github.com/gpdf-dev/gpdf v0.x.x                    // PDF 生成
)

// ========== 新增：邮件 ==========
require (
    github.com/wneessen/go-mail v0.7.3                 // SMTP 邮件
)

// ========== 新增：OCR（gRPC 客户端） ==========
require (
    google.golang.org/grpc v1.64                       // gRPC 调用 OCR 微服务
    google.golang.org/protobuf v1.34                   // protobuf 序列化
)

// ========== 新增：工具库 ==========
require (
    github.com/google/uuid v1.6.0                      // UUID 生成
    github.com/agentine/prism v0.x.x                   // 图片预处理（OCR 前，可选）
)
```

### 7.2 Python OCR 微服务依赖（独立 go.mod）

```
# ocr-service/requirements.txt
paddleocr>=2.8
grpcio>=1.60
grpcio-tools>=1.60
flask>=3.0       # HTTP 降级备选
pillow>=10.0     # 图片预处理（Python 端冗余）
```

### 7.3 React 前端依赖

```json
{
  "dependencies": {
    "react": "^18.3",
    "react-dom": "^18.3",
    "antd": "^5.20",
    "@ant-design/icons": "^5.4",
    "echarts": "^5.5",
    "echarts-for-react": "^3.0"
  },
  "devDependencies": {
    "typescript": "^5.5",
    "vite": "^6.0",
    "@vitejs/plugin-react": "^4.3"
  }
}
```

---

## 八、技术栈全景图

```
┌─────────────────────────────────────────────────────────────┐
│                      前端 (React 18)                         │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌────────────┐ │
│  │ Antd 5   │  │ ECharts  │  │ EventSrce│  │  Vite 6    │ │
│  │ (UI库)   │  │ (图表)    │  │ (SSE)    │  │ (构建)     │ │
│  └──────────┘  └──────────┘  └──────────┘  └────────────┘ │
└─────────────────────────┬───────────────────────────────────┘
                          │ REST + SSE
┌─────────────────────────┴───────────────────────────────────┐
│                    Go Backend (Gin v1.10)                    │
│                                                              │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌────────────┐ │
│  │ Eino ADK │  │  GORM    │  │ Google   │  │ Viper+Zap  │ │
│  │ (Agent)  │  │ (MySQL)  │  │ Wire(DI) │  │ (配置+日志) │ │
│  └────┬─────┘  └────┬─────┘  └──────────┘  └────────────┘ │
│       │             │                                        │
│  ┌────┴─────┐  ┌────┴─────┐  ┌──────────┐  ┌────────────┐ │
│  │ OpenAI   │  │  MySQL   │  │ go-redis │  │ gpdf(PDF)  │ │
│  │ 兼容 API  │  │   8.0     │  │ (Session)│  │ go-mail(信)│ │
│  └──────────┘  └──────────┘  └──────────┘  └────────────┘ │
│                                                              │
│  ┌──────────────────────────────────────────────────────┐   │
│  │ OCR Client (net/http) → gRPC/HTTP → OCR Service      │   │
│  └──────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
                          │
┌─────────────────────────┴───────────────────────────────────┐
│              OCR 微服务 (Python + PaddleOCR)                  │
│  ┌──────────┐  ┌──────────┐  ┌────────────┐                │
│  │ gRPC     │  │ PaddleOCR│  │ Pillow     │                │
│  │ (接口)    │  │ (识别引擎)│  │ (图片预处理)│                │
│  └──────────┘  └──────────┘  └────────────┘                │
└─────────────────────────────────────────────────────────────┘
```

---

## 九、选型决策记录

| # | 决策 | 方案 | 备选 | 决策日期 |
|:--:|------|------|------|:--:|
| D1 | Agent 框架 | Eino ADK | LangChainGo | 2026-07-03 |
| D2 | LLM 提供商 | OpenAI 兼容 API | 火山方舟 ARK | 2026-07-03 |
| D3 | PDF 生成 | gpdf | maroto v2 / wkhtmltopdf | 2026-07-03 |
| D4 | 邮件发送 | go-mail | 无（不可降级） | 2026-07-03 |
| D5 | OCR 架构 | Go interface + 策略模式，默认 PaddleOCR | Tesseract 本地 / 火山 OCR API / Mock | 2026-07-03 |
| D5.1 | OCR 接口抽象 | `OCRRecognizer` interface + 配置驱动切换 | — | 2026-07-03 |
| D6 | 图片预处理 | prism | bimg | 2026-07-03 |
| D7 | 前端框架 | React 18 + TypeScript | Vue 3 | 2026-07-03 |
| D8 | UI 组件库 | Ant Design 5 | Arco Design | 2026-07-03 |
| D9 | 图表库 | ECharts | Recharts | 2026-07-03 |
| D10 | 金额精度 | int64 (分) | float64 / shopspring/decimal | 2026-07-03 |
| D11 | Go HTTP 客户端 | net/http (标准库) | resty | 2026-07-03 |
| D12 | UUID | google/uuid v5 | gofrs/uuid | 2026-07-03 |

---

*文档结束*
