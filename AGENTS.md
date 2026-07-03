# Reimbee — 项目规约

## 语言规范

- **注释**: 所有代码注释使用中文
- **日志**: 所有 zap 日志消息使用中文
- **错误**: 所有 error 返回的消息使用中文
- **变量/函数/类型名**: 使用英文（Go 语言规范）

## 架构规范

- 项目基于 `CycleZero/gin-template` DDD-lite 架构
- 分层: `model → repo → biz → service → router`
- 依赖注入: Google Wire
- 每个领域模块提供独立的 `ProviderSet`

## 金额规范

- 数据库存储: `int64`（分为单位）
- API 传输/前端展示: `float64`（元为单位）
- 在 repo 层存储分，在 service 层做转换
