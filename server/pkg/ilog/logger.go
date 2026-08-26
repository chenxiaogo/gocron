// Package ilog 基于 go.uber.org/zap 的统一日志封装（参考 go-proxy）。
// 提供全局便捷方法、链路日志入口 Trace/FromGin，以及 TraceZapFields 用于
// 从 context 提取 trace_id / span_id 作为 zap.Field，使业务日志、DB 日志与
// HTTP 访问日志同链路。
package ilog

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/trace"

	"scheduler-platform/pkg/tracex"
)

// Option 日志配置选项。
type Option struct {
	AppName      string
	Level        string
	WriteConsole bool
	WriteFile    bool
	LogPath      string
	FileName     string
	MaxSize      int
	MaxBackups   int
	MaxAge       int
	Compress     bool
}

type OptionFunc func(*Option)

func WithAppName(name string) OptionFunc {
	return func(o *Option) { o.AppName = name }
}
func WithLogLevel(level string) OptionFunc {
	return func(o *Option) { o.Level = level }
}
func WithWriteToConsole(b bool) OptionFunc {
	return func(o *Option) { o.WriteConsole = b }
}
func WithWriteToFile(b bool) OptionFunc {
	return func(o *Option) { o.WriteFile = b }
}
func WithLogPath(p string) OptionFunc {
	return func(o *Option) { o.LogPath = p }
}
func WithFileName(name string) OptionFunc {
	return func(o *Option) { o.FileName = name }
}
func WithMaxSize(n int) OptionFunc {
	return func(o *Option) { o.MaxSize = n }
}
func WithMaxBackups(n int) OptionFunc {
	return func(o *Option) { o.MaxBackups = n }
}
func WithMaxAge(n int) OptionFunc {
	return func(o *Option) { o.MaxAge = n }
}
func WithCompress(b bool) OptionFunc {
	return func(o *Option) { o.Compress = b }
}

var (
	globalLogger *zap.Logger
)

// NewLogger 初始化全局日志器（zap，支持 console / file 双输出）。
func NewLogger(funcs ...OptionFunc) error {
	opts := &Option{
		AppName:      "scheduler-platform",
		Level:        "info",
		WriteConsole: true,
		WriteFile:    false,
		LogPath:      "./logs",
		FileName:     "scheduler.log",
		MaxSize:      100,
		MaxBackups:   100,
		MaxAge:       30,
		Compress:     true,
	}
	for _, fn := range funcs {
		fn(opts)
	}

	level := parseLogLevel(opts.Level)
	var cores []zapcore.Core

	if opts.WriteConsole {
		cores = append(cores, newConsoleCore(level))
	}
	if opts.WriteFile && opts.LogPath != "" {
		if err := os.MkdirAll(opts.LogPath, 0755); err != nil {
			return err
		}
		core, err := newFileCore(opts, level)
		if err != nil {
			return err
		}
		cores = append(cores, core)
	}
	if len(cores) == 0 {
		cores = append(cores, newConsoleCore(level))
	}

	globalLogger = zap.New(zapcore.NewTee(cores...),
		zap.AddCaller(),
	).Named(opts.AppName)
	return nil
}

// GetLogger 返回全局 zap.Logger 实例（供底层/自定义使用）。
func GetLogger() *zap.Logger {
	if globalLogger == nil {
		// 兜底：未初始化时建一个 console logger，避免 nil 崩溃
		globalLogger = zap.New(newConsoleCore(zapcore.InfoLevel), zap.AddCaller())
	}
	return globalLogger
}

func newConsoleCore(level zapcore.Level) zapcore.Core {
	cfg := zapcore.EncoderConfig{
		TimeKey:        "time",
		LevelKey:       "level",
		CallerKey:      "caller",
		MessageKey:     "msg",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    coloredLevelEncoder,
		EncodeTime:     coloredTimeEncoder,
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeCaller:   coloredCallerEncoder,
	}
	encoder := zapcore.NewConsoleEncoder(cfg)
	return zapcore.NewCore(encoder, zapcore.Lock(os.Stdout), level)
}

// ANSI 颜色码
const (
	colorGray   = "\033[90m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorRed    = "\033[31m"
	colorBlue   = "\033[34m"
	colorCyan   = "\033[36m"
	colorBold   = "\033[1m"
	colorReset  = "\033[0m"
)

func coloredLevelEncoder(l zapcore.Level, enc zapcore.PrimitiveArrayEncoder) {
	var color string
	switch l {
	case zapcore.DebugLevel:
		color = colorCyan
	case zapcore.InfoLevel:
		color = colorGreen
	case zapcore.WarnLevel:
		color = colorYellow
	case zapcore.ErrorLevel, zapcore.DPanicLevel, zapcore.PanicLevel, zapcore.FatalLevel:
		color = colorRed
	default:
		color = colorReset
	}
	enc.AppendString(color + colorBold + l.CapitalString() + colorReset)
}

func coloredTimeEncoder(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
	enc.AppendString(colorGray + t.Format("2006-01-02 15:04:05.000") + colorReset)
}

func coloredCallerEncoder(caller zapcore.EntryCaller, enc zapcore.PrimitiveArrayEncoder) {
	enc.AppendString(colorGray + caller.TrimmedPath() + colorReset)
}

func newFileCore(opts *Option, level zapcore.Level) (zapcore.Core, error) {
	lumber := &lumberjack.Logger{
		Filename:   filepath.Join(opts.LogPath, opts.FileName),
		MaxSize:    opts.MaxSize,
		MaxBackups: opts.MaxBackups,
		MaxAge:     opts.MaxAge,
		Compress:   opts.Compress,
		LocalTime:  true,
	}
	cfg := zapcore.EncoderConfig{
		TimeKey:        "time",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		MessageKey:     "msg",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     customTimeEncoder,
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}
	encoder := zapcore.NewJSONEncoder(cfg)
	ws := zapcore.AddSync(lumber)
	return zapcore.NewCore(encoder, ws, level), nil
}

func customTimeEncoder(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
	enc.AppendString(t.Format("2006-01-02 15:04:05.000"))
}

func parseLogLevel(level string) zapcore.Level {
	switch strings.ToLower(level) {
	case "debug":
		return zapcore.DebugLevel
	case "warn", "warning":
		return zapcore.WarnLevel
	case "error":
		return zapcore.ErrorLevel
	case "panic":
		return zapcore.PanicLevel
	case "fatal":
		return zapcore.FatalLevel
	default:
		return zapcore.InfoLevel
	}
}

// TraceZapFields 从 context 提取链路 ID 作为 zap.Field。
// 与 go-proxy 示例一致：通过 OTel 的 trace.SpanFromContext 取 SpanContext，
// 有效则返回 trace_id / span_id 两个字段，否则返回 nil。
func TraceZapFields(ctx context.Context) []zap.Field {
	sc := trace.SpanFromContext(ctx).SpanContext()
	if !sc.IsValid() {
		return nil
	}
	return []zap.Field{
		zap.String("trace_id", sc.TraceID().String()),
		zap.String("span_id", sc.SpanID().String()),
	}
}

// Logger 统一日志接口（zap 封装）。
type Logger interface {
	Debug(args ...any)
	Info(args ...any)
	Warn(args ...any)
	Error(args ...any)
	Debugf(format string, args ...any)
	Infof(format string, args ...any)
	Warnf(format string, args ...any)
	Errorf(format string, args ...any)
}

// zapLogger 用 zap.SugaredLogger 实现 Logger 接口。
// traceID 用于在消息尾部追加链路标识（非 structured field，避免控制台输出 JSON）。
type zapLogger struct {
	s       *zap.SugaredLogger
	traceID string
}

func (l *zapLogger) Debug(args ...any)            { l.log("Debug", args...) }
func (l *zapLogger) Info(args ...any)             { l.log("Info", args...) }
func (l *zapLogger) Warn(args ...any)             { l.log("Warn", args...) }
func (l *zapLogger) Error(args ...any)            { l.log("Error", args...) }
func (l *zapLogger) Debugf(f string, args ...any) { l.logf("Debug", f, args...) }
func (l *zapLogger) Infof(f string, args ...any)  { l.logf("Info", f, args...) }
func (l *zapLogger) Warnf(f string, args ...any)  { l.logf("Warn", f, args...) }
func (l *zapLogger) Errorf(f string, args ...any) { l.logf("Error", f, args...) }

func (l *zapLogger) log(level string, args ...any) {
	if l.traceID != "" {
		args = append(args, "["+l.traceID+"]")
	}
	switch level {
	case "Debug":
		l.s.Debug(args...)
	case "Info":
		l.s.Info(args...)
	case "Warn":
		l.s.Warn(args...)
	case "Error":
		l.s.Error(args...)
	}
}

func (l *zapLogger) logf(level, format string, args ...any) {
	if l.traceID != "" {
		format += "  [" + l.traceID + "]"
	}
	switch level {
	case "Debug":
		l.s.Debugf(format, args...)
	case "Info":
		l.s.Infof(format, args...)
	case "Warn":
		l.s.Warnf(format, args...)
	case "Error":
		l.s.Errorf(format, args...)
	}
}

// Trace 返回绑定 context 链路 ID 的日志器（统一链路日志入口）。
// trace_id 追加在消息尾部，而非 structured field，避免控制台输出 JSON。
// context 来源：HTTP 处理用 FromGin(c) 或 tracex.FromRequest(c)；后台任务用 tracex.NewContext。
func Trace(ctx context.Context) Logger {
	return &zapLogger{
		s:       GetLogger().Sugar(),
		traceID: tracex.TraceIDShort(ctx),
	}
}

// FromGin 从 Gin 上下文返回绑定当前请求链路 ID 的日志器。
func FromGin(c *gin.Context) Logger {
	return Trace(tracex.FromRequest(c))
}

// WithContext 等价于 Trace，为兼容早期接入保留。
func WithContext(ctx context.Context) Logger {
	return Trace(ctx)
}

// 全局便捷方法（无 ctx，不携带链路字段）。
func Debug(args ...interface{})              { GetLogger().Sugar().Debug(args...) }
func Info(args ...interface{})               { GetLogger().Sugar().Info(args...) }
func Warn(args ...interface{})               { GetLogger().Sugar().Warn(args...) }
func Error(args ...interface{})              { GetLogger().Sugar().Error(args...) }
func Debugf(t string, args ...interface{})   { GetLogger().Sugar().Debugf(t, args...) }
func Infof(t string, args ...interface{})    { GetLogger().Sugar().Infof(t, args...) }
func Warnf(t string, args ...interface{})    { GetLogger().Sugar().Warnf(t, args...) }
func Errorf(t string, args ...interface{})   { GetLogger().Sugar().Errorf(t, args...) }

// Sync 刷新日志缓冲（zap 刷盘）。
func Sync() {
	_ = GetLogger().Sync()
}
