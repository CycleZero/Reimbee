package log

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fatih/color"
	"github.com/shengyanli1982/law"
	"go.uber.org/zap"
	"go.uber.org/zap/buffer"
	"go.uber.org/zap/zapcore"
)

var ColorResetStr = "\x1b[0m"
var LenColorResetStr = len(ColorResetStr)

var (
	Red    = color.New(color.FgHiRed).SprintFunc()
	Blue   = color.New(color.FgHiBlue).SprintFunc()
	Yellow = color.New(color.FgHiYellow).SprintFunc()
	Green  = color.New(color.FgHiGreen).SprintFunc()
)

// UseLawAsyncWriter 文件日志异步引擎选择：
//
//	false (默认): chan + 自实现, 零外部依赖, 锁竞争低
//	true:         law.WriteAsyncer, MPSC 队列, 外部依赖 shengyanli1982/law
var UseLawAsyncWriter = true

// UseLawConsoleWriter 控制台日志异步引擎选择：
//
//	false (默认): bufio + LockedWriteSyncer, 简单高效
//	true:         law.WriteAsyncer, 与控制台异步写入统一引擎
var UseLawConsoleWriter = true

type Logger struct {
	*zap.Logger
}

var globalLogger *Logger

func NewLogger(
	mode int,
	level string,
	logDir string,
	appName string,
) (*Logger, error) {
	logPath := GetLogPath(logDir, appName)

	// 创建日志目录
	if logPath != "" {
		dir := filepath.Dir(logPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("创建日志目录失败：%w", err)
		}
	}

	// 解析日志级别
	zLevel, err := zapcore.ParseLevel(level)
	if err != nil {
		return nil, err
	}

	// 编码器配置（带颜色）
	colorEncoderConfig := zapcore.EncoderConfig{
		TimeKey:  "T",
		LevelKey: "L",
		NameKey:  "N",
		//CallerKey:        "C",
		MessageKey:       "M",
		StacktraceKey:    "S",
		LineEnding:       zapcore.DefaultLineEnding,
		EncodeLevel:      customLevelColorEncoder,
		EncodeTime:       customTimeEncoder,
		EncodeDuration:   zapcore.StringDurationEncoder,
		EncodeCaller:     zapcore.ShortCallerEncoder,
		ConsoleSeparator: " ",
	}

	// 无颜色的编码器配置（用于文件）
	plainEncoderConfig := zapcore.EncoderConfig{
		TimeKey:          "T",
		LevelKey:         "L",
		NameKey:          "N",
		CallerKey:        "C",
		MessageKey:       "M",
		StacktraceKey:    "S",
		LineEnding:       zapcore.DefaultLineEnding,
		EncodeLevel:      zapcore.CapitalLevelEncoder,
		EncodeTime:       zapcore.TimeEncoderOfLayout("2006-01-02 15:04:05.000"),
		EncodeDuration:   zapcore.StringDurationEncoder,
		EncodeCaller:     zapcore.ShortCallerEncoder,
		ConsoleSeparator: " ",
	}

	//TODO
	isDev := mode == 0

	// 控制台写入器
	var consoleEncoder zapcore.Encoder
	var fileEncoder zapcore.Encoder

	if isDev {
		consoleEncoder = zapcore.NewConsoleEncoder(colorEncoderConfig)
		color.NoColor = false
		fileEncoder = zapcore.NewConsoleEncoder(plainEncoderConfig)
	} else {
		consoleEncoder = &CustomEncoder{zapcore.NewJSONEncoder(plainEncoderConfig)}
		fileEncoder = zapcore.NewJSONEncoder(plainEncoderConfig)
	}

	// 控制台输出：同步直写 color.Output，简单可靠
	//consoleWriterSyncer := zapcore.AddSync(color.Output)
	var consoleWriterSyncer zapcore.WriteSyncer
	if UseLawConsoleWriter {
		consoleWriterSyncer = zapcore.AddSync(NewLawAsyncWriter(color.Output))
	} else {
		consoleWriterSyncer = NewBufferedWriteSyncer(color.Output, 64*1024)
	}
	var fileWriteSyncer zapcore.WriteSyncer
	if logPath != "" {
		if UseLawAsyncWriter {
			fileWriteSyncer = zapcore.AddSync(NewLawAsyncWriter(NewFileWriter(logPath)))
		} else {
			fileWriteSyncer = NewAsyncWriteSyncer(NewFileWriter(logPath), 256*1024)
		}
		//fileWriteSyncer = zapcore.AddSync(NewFileWriter(logPath))
	}

	var cores []zapcore.Core
	cores = append(cores, zapcore.NewCore(consoleEncoder, consoleWriterSyncer, zLevel))
	if logPath != "" && fileWriteSyncer != nil {
		cores = append(cores, zapcore.NewCore(fileEncoder, fileWriteSyncer, zLevel))
	}
	// 若 Sentry SDK 已初始化（main.go 在 NewLogger 之前调过 InitSentry），
	// 自动 attach Sentry core 把 WARN+ERROR 上报上去；未初始化时 NewSentryCore 返回 nil
	//if sentryCore := NewSentryCore(); sentryCore != nil {
	//	cores = append(cores, sentryCore)
	//}

	core := zapcore.NewTee(cores...)
	return &Logger{zap.New(core)}, nil
}

type CustomEncoder struct {
	zapcore.Encoder
}

func (c *CustomEncoder) EncodeEntry(entry zapcore.Entry, fields []zap.Field) (*buffer.Buffer, error) {
	buf, err := c.Encoder.EncodeEntry(entry, fields)
	if err != nil {
		return buf, err
	}

	switch entry.Level {
	case zapcore.WarnLevel:
		e := buf.String()[:strings.LastIndex(buf.String(), ColorResetStr)+LenColorResetStr]
		t := buf.String()[strings.LastIndex(buf.String(), ColorResetStr)+LenColorResetStr:]
		buf.Reset()
		buf.WriteString(e + Yellow(t))
	case zapcore.ErrorLevel:
		e := buf.String()[:strings.LastIndex(buf.String(), ColorResetStr)+LenColorResetStr]
		t := buf.String()[strings.LastIndex(buf.String(), ColorResetStr)+LenColorResetStr:]
		buf.Reset()
		buf.WriteString(e + Red(t))
	}
	return buf, nil
}
func customLevelColorEncoder(level zapcore.Level, enc zapcore.PrimitiveArrayEncoder) {
	var colorize func(a ...interface{}) string

	switch level {
	case zapcore.DebugLevel:
		colorize = Blue
	case zapcore.InfoLevel:
		colorize = Green
	case zapcore.WarnLevel:
		colorize = Yellow
	case zapcore.ErrorLevel:
		colorize = Red
	default:
		colorize = color.New(color.FgHiWhite).SprintFunc()
	}

	enc.AppendString(colorize(level.CapitalString()))
}

// 自定义时间编码器，添加颜色
func customTimeEncoder(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
	enc.AppendString(color.New(color.FgCyan).SprintFunc()(t.Format("2006-01-02 15:04:05.000")))
}

//func init() {
//	var err error
//	globalLogger, err = zap.NewDevelopment()
//	if err != nil {
//		// 处理错误，例如使用默认配置或panic
//		panic("failed to initialize logger: " + err.Error())
//	}
//}

func GetLogger() *Logger {
	if globalLogger == nil {
		panic("致命错误:创建logger失败,触发panic")
	}
	return globalLogger
}

func SugaredLogger() *zap.SugaredLogger {
	return GetLogger().Sugar()
}

func SetGlobalLogger(l *Logger) {
	globalLogger = l
}

// InitLogger 根据配置初始化全局日志器，必须在 GetLogger 之前调用
func InitLogger(mode string, level string, logDir string) error {
	var modeInt int
	if mode == "prod" {
		modeInt = 1
	}
	l, err := NewLogger(modeInt, level, logDir, "prompter")
	if err != nil {
		return err
	}
	SetGlobalLogger(l)
	return nil
}

// ============================================================
// 高性能日志 Writer
// ============================================================

// bufferedWriteSyncer 缓冲同步写入器：包装 bufio.Writer，减少 write syscall 次数
type bufferedWriteSyncer struct {
	w  *bufio.Writer
	mu sync.Mutex
}

func NewBufferedWriteSyncer(w io.Writer, size int) *bufferedWriteSyncer {
	return &bufferedWriteSyncer{w: bufio.NewWriterSize(w, size)}
}

func (s *bufferedWriteSyncer) Write(p []byte) (int, error) {
	s.mu.Lock()
	n, err := s.w.Write(p)
	s.mu.Unlock()
	return n, err
}

func (s *bufferedWriteSyncer) Sync() error {
	s.mu.Lock()
	err := s.w.Flush()
	s.mu.Unlock()
	return err
}

// asyncWriteSyncer 异步写入器：chan 解耦日志生产与磁盘 I/O
// Go chan 内部无全局锁，与 law.MPSCQueue(mutex) 不同，高并发下保持高性能
type asyncWriteSyncer struct {
	ch      chan []byte
	bufPool sync.Pool
	wg      sync.WaitGroup
	w       io.WriteCloser
	bufW    *bufio.Writer
}

func NewAsyncWriteSyncer(w io.WriteCloser, bufSize int) *asyncWriteSyncer {
	s := &asyncWriteSyncer{
		ch:   make(chan []byte, 4096), // 4K 缓冲，远超瞬时日志峰值
		w:    w,
		bufW: bufio.NewWriterSize(w, bufSize),
	}
	s.bufPool.New = func() any { return make([]byte, 0, 4096) }
	s.wg.Add(1)
	go s.drain()
	return s
}

func (s *asyncWriteSyncer) drain() {
	defer s.wg.Done()
	for data := range s.ch {
		s.bufW.Write(data)
		s.bufPool.Put(data[:0])
	}
	s.bufW.Flush()
	s.w.Close()
}

func (s *asyncWriteSyncer) Write(p []byte) (int, error) {
	buf := s.bufPool.Get().([]byte)
	buf = append(buf[:0], p...)
	s.ch <- buf // 阻塞写入，保证日志完整性；Go chan 内部无全局锁，高并发下高效
	return len(p), nil
}

func (s *asyncWriteSyncer) Sync() error { return nil }

func (s *asyncWriteSyncer) Close() {
	close(s.ch)
	s.wg.Wait()
}

// ============================================================
// law 异步写入器 (可选, UseLawAsyncWriter=true 时生效)
// ============================================================

func NewLawAsyncWriter(w io.Writer) *law.WriteAsyncer {
	conf := law.NewConfig()
	conf.WithBufferSize(1024 * 1024 * 10)
	return law.NewWriteAsyncer(w, conf)
}

//
//// ============================================================
//// Kratos 适配
//// ============================================================
//
//// NewKratosLogger 创建全局 logger 并返回带标准字段的 Kratos logger。
////
//// 封装各服务 main.go 中重复的"创建 zap logger → 设为全局 → 包装为 Kratos logger
//// → 注入标准字段"四步流程。
////
//// 用法（main.go）：
////
////	logger := locallog.NewKratosLogger(
////	    int(bc.Log.Mode.Number()),
////	    bc.Log.Level.String(),
////	    bc.Log.Path,
////	    conf.ServiceName,
////	    id,
////	    Name,
////	    Version,
////	)
////	logger.Log(log.LevelInfo, "init logger success")
//func NewKratosLogger(
//	mode int,
//	level string,
//	logPath string,
//	appName string,
//	serviceID string,
//	serviceName string,
//	serviceVersion string,
//) log.Logger {
//	l, err := NewLogger(mode, level, logPath, appName)
//	if err != nil {
//		panic(err)
//	}
//	SetGlobalLogger(l)
//	return log.With(kratoszap.NewLogger(l.Logger),
//		"ts", log.DefaultTimestamp,
//		"caller", log.DefaultCaller,
//		"service.id", serviceID,
//		"service.name", serviceName,
//		"service.version", serviceVersion,
//		"trace.id", tracing.TraceID(),
//		"span.id", tracing.SpanID(),
//		"uid", meta.UidValuer(),
//		"username", meta.UserNameValuer(),
//	)
//}
//
//func GetKratosLogger() log.Logger {
//	l := kratoszap.NewLogger(GetLogger().Logger)
//	return l
//}
//
//func GetKratosLogHelper() *log.Helper {
//	h := log.NewHelper(GetKratosLogger(),
//		log.WithSprint(Sprint),
//	)
//	return h
//}
