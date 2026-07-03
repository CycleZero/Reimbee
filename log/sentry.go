package log

//
//import (
//	"errors"
//	"runtime"
//	"strings"
//
//	psentry "micro-kit/sentry"
//
//	"github.com/getsentry/sentry-go"
//	"go.uber.org/zap/zapcore"
//)
//
//// NewSentryCore 返回把 WARN/ERROR 级别 zap log 上报到 Sentry Issue 的 zapcore.Core
////
//// 调用方必须先调用 psentry.Init / psentry.InitFromEtcd 初始化 hub，
//// 否则返回 nil。返回 nil 时调用方应跳过该 core，不要塞进 NewTee。
////
//// 注意：官方 sentry-go/zap@v0.46.2 走的是 Sentry Logs API（需开 EnableLogs，
//// 数据落 Logs UI 而非 Issue stream），不符合我们想要的 issue 监控习惯，
//// 因此自实现一个直接调 CaptureMessage / CaptureException 的 core。
//func NewSentryCore() zapcore.Core {
//	if !psentry.Enabled() {
//		return nil
//	}
//	return &sentryIssueCore{
//		LevelEnabler: zapcore.WarnLevel,
//	}
//}
//
//// sentryIssueCore 把 WARN+ERROR 级别的 zap log 推到 Sentry Issue
////
//// 不缓冲、不批量，每条 Entry 同步调用 hub。Sentry SDK 自身内部有 transport
//// 异步缓冲，调用本身耗时可忽略。
//type sentryIssueCore struct {
//	zapcore.LevelEnabler
//	fields []zapcore.Field
//}
//
//// With 实现 zapcore.Core；累积 With 字段后续在 Write 时合并到 sentry scope tags
//func (c *sentryIssueCore) With(fields []zapcore.Field) zapcore.Core {
//	clone := *c
//	clone.fields = append(clone.fields[:len(clone.fields):len(clone.fields)], fields...)
//	return &clone
//}
//
//// Check 实现 zapcore.Core；高于 WARN 的进入 Write
//func (c *sentryIssueCore) Check(entry zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
//	if c.Enabled(entry.Level) {
//		return ce.AddCore(entry, c)
//	}
//	return ce
//}
//
//// Write 实现 zapcore.Core；每条 Entry 上报为 Sentry Issue
//func (c *sentryIssueCore) Write(entry zapcore.Entry, fields []zapcore.Field) error {
//	hub := sentry.CurrentHub().Clone()
//	hub.WithScope(func(scope *sentry.Scope) {
//		// 等级映射
//		switch entry.Level {
//		case zapcore.WarnLevel:
//			scope.SetLevel(sentry.LevelWarning)
//		case zapcore.ErrorLevel:
//			scope.SetLevel(sentry.LevelError)
//		case zapcore.DPanicLevel, zapcore.PanicLevel, zapcore.FatalLevel:
//			scope.SetLevel(sentry.LevelFatal)
//		}
//
//		// caller 信息（filename:line）
//		if entry.Caller.Defined {
//			scope.SetTag("file", trimCallerPath(entry.Caller.File))
//			scope.SetContext("caller", sentry.Context{
//				"line": entry.Caller.Line,
//				"func": entry.Caller.Function,
//			})
//		}
//
//		// 合并 With 字段 + 当条字段，作为 sentry tag/extra
//		all := append(c.fields[:len(c.fields):len(c.fields)], fields...)
//		injectFieldsToScope(scope, all)
//
//		// 优先用字段里的 error 类型上报为 exception（带 stacktrace）
//		if err := extractErrorField(all); err != nil {
//			hub.CaptureException(wrappedErr(entry.Message, err))
//		} else {
//			hub.CaptureMessage(entry.Message)
//		}
//	})
//	return nil
//}
//
//// Sync 实现 zapcore.Core；不缓冲，无需 sync
//func (c *sentryIssueCore) Sync() error { return nil }
//
//// injectFieldsToScope 把 zap fields 转成 sentry scope tags + fields context
////
//// 由于 sentry.Scope 没有 SetExtra（只有 SetContext + map），统一把非简单类型
//// 字段聚合到一个 "fields" Context 里。
//func injectFieldsToScope(scope *sentry.Scope, fields []zapcore.Field) {
//	extras := make(sentry.Context, len(fields))
//	for _, f := range fields {
//		switch f.Type {
//		case zapcore.StringType:
//			scope.SetTag(f.Key, f.String)
//		case zapcore.BoolType:
//			if f.Integer == 1 {
//				scope.SetTag(f.Key, "true")
//			} else {
//				scope.SetTag(f.Key, "false")
//			}
//		case zapcore.ErrorType:
//			// error 通过 extractErrorField 单独捕获，这里跳过避免重复 tag
//			continue
//		default:
//			// 其余复杂类型聚合到 fields context
//			if f.Interface != nil {
//				extras[f.Key] = f.Interface
//			} else if f.String != "" {
//				extras[f.Key] = f.String
//			} else {
//				extras[f.Key] = f.Integer
//			}
//		}
//	}
//	if len(extras) > 0 {
//		scope.SetContext("fields", extras)
//	}
//}
//
//// extractErrorField 从 fields 中提取 zap.Error 字段对应的 error 值
//func extractErrorField(fields []zapcore.Field) error {
//	for _, f := range fields {
//		if f.Type == zapcore.ErrorType {
//			if err, ok := f.Interface.(error); ok {
//				return err
//			}
//		}
//	}
//	return nil
//}
//
//// wrappedErr 把 logger.Error 的 message 拼到 err 前面，方便 sentry issue 标题阅读
//func wrappedErr(msg string, err error) error {
//	if msg == "" {
//		return err
//	}
//	return errors.New(msg + ": " + err.Error())
//}
//
//// trimCallerPath 取 caller 文件路径的短名，剥掉 GOPATH 前缀
//func trimCallerPath(p string) string {
//	// 优先取 vcyuan-backend-app/ 之后的相对路径
//	const proj = "vcyuan-backend-app/"
//	if i := strings.LastIndex(p, proj); i >= 0 {
//		return p[i+len(proj):]
//	}
//	// 退而求其次取最后两段
//	parts := strings.Split(p, "/")
//	if len(parts) > 2 {
//		return strings.Join(parts[len(parts)-2:], "/")
//	}
//	return p
//}
//
//// 占位：保留 runtime 包以便未来扩展（栈帧筛选等）
//var _ = runtime.Caller
