package utils

import (
	"log"
	"os"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Logger is the cache logger interface.
type Logger interface {
	Debug(msg string, fields ...interface{})
	Info(msg string, fields ...interface{})
	Warn(msg string, fields ...interface{})
	Error(msg string, fields ...interface{})
	Fatal(msg string, fields ...interface{})
}

// ZapLogger wraps zap.SugaredLogger.
type ZapLogger struct {
	logger *zap.SugaredLogger
}

func NewZapLogger(level string) *ZapLogger {
	var zapLevel zapcore.Level
	switch strings.ToLower(level) {
	case "debug":
		zapLevel = zap.DebugLevel
	case "info":
		zapLevel = zap.InfoLevel
	case "warn", "warning":
		zapLevel = zap.WarnLevel
	case "error":
		zapLevel = zap.ErrorLevel
	case "fatal":
		zapLevel = zap.FatalLevel
	default:
		zapLevel = zap.InfoLevel
	}

	config := zap.NewProductionConfig()
	config.Level = zap.NewAtomicLevelAt(zapLevel)
	config.EncoderConfig.TimeKey = "time"
	config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	config.OutputPaths = []string{"stdout"}
	config.ErrorOutputPaths = []string{"stderr"}

	if os.Getenv("APP_ENV") == "development" {
		config = zap.NewDevelopmentConfig()
		config.Level = zap.NewAtomicLevelAt(zapLevel)
	}

	logger, err := config.Build()
	if err != nil {
		log.Printf("Failed to create zap logger: %v", err)
		return &ZapLogger{logger: zap.NewNop().Sugar()}
	}
	return &ZapLogger{logger: logger.Sugar()}
}

func (l *ZapLogger) Debug(msg string, fields ...interface{}) { l.logger.Debugw(msg, fields...) }
func (l *ZapLogger) Info(msg string, fields ...interface{})  { l.logger.Infow(msg, fields...) }
func (l *ZapLogger) Warn(msg string, fields ...interface{})  { l.logger.Warnw(msg, fields...) }
func (l *ZapLogger) Error(msg string, fields ...interface{}) { l.logger.Errorw(msg, fields...) }
func (l *ZapLogger) Fatal(msg string, fields ...interface{}) { l.logger.Fatalw(msg, fields...) }
func (l *ZapLogger) Sync() error                             { return l.logger.Sync() }

// StdLogger is a standard library logger.
type StdLogger struct {
	logger *log.Logger
	level  string
}

func NewStdLogger(level string) *StdLogger {
	return &StdLogger{logger: log.New(os.Stdout, "", log.LstdFlags|log.Lshortfile), level: level}
}

func (l *StdLogger) Debug(msg string, fields ...interface{}) {
	if l.shouldLog("debug") {
		l.logger.Printf("[DEBUG] "+msg, fields...)
	}
}
func (l *StdLogger) Info(msg string, fields ...interface{}) {
	if l.shouldLog("info") {
		l.logger.Printf("[INFO] "+msg, fields...)
	}
}
func (l *StdLogger) Warn(msg string, fields ...interface{}) {
	if l.shouldLog("warn") {
		l.logger.Printf("[WARN] "+msg, fields...)
	}
}
func (l *StdLogger) Error(msg string, fields ...interface{}) {
	if l.shouldLog("error") {
		l.logger.Printf("[ERROR] "+msg, fields...)
	}
}
func (l *StdLogger) Fatal(msg string, fields ...interface{}) { l.logger.Fatalf("[FATAL] "+msg, fields...) }

func (l *StdLogger) shouldLog(level string) bool {
	levels := map[string]int{"debug": 0, "info": 1, "warn": 2, "error": 3, "fatal": 4}
	currentLevel := strings.ToLower(l.level)
	if _, ok := levels[currentLevel]; !ok {
		currentLevel = "info"
	}
	return levels[level] >= levels[currentLevel]
}

// NoopLogger is a no-op logger.
type NoopLogger struct{}

func NewNoopLogger() *NoopLogger { return &NoopLogger{} }

func (l *NoopLogger) Debug(msg string, fields ...interface{}) {}
func (l *NoopLogger) Info(msg string, fields ...interface{})  {}
func (l *NoopLogger) Warn(msg string, fields ...interface{})  {}
func (l *NoopLogger) Error(msg string, fields ...interface{}) {}
func (l *NoopLogger) Fatal(msg string, fields ...interface{}) {}

// LoggerFactory creates loggers.
type LoggerFactory struct{}

func (f *LoggerFactory) GetLogger(loggerType, level string) Logger {
	switch strings.ToLower(loggerType) {
	case "zap":
		return NewZapLogger(level)
	case "std":
		return NewStdLogger(level)
	case "noop":
		return NewNoopLogger()
	default:
		return NewStdLogger(level)
	}
}