package xlog

import (
	"fmt"
	"io"
	"path"
	"reflect"
	"runtime/debug"
	"sync"
	"sync/atomic"

	"scheduler-platform/internal/xlog/sysx"
)

const callerDepth = 4

var (
	timeFormat        = "2006-01-02T15:04:05.000Z07:00"
	encoding   uint32 = jsonEncodingType
	// maxContentLength is used to truncate the log content, 0 for not truncating.
	maxContentLength uint32
	// use uint32 for atomic operations
	disableStat uint32
	logLevel    uint32
	options     logOptions
	writer      = new(atomicWriter)
	setupOnce   sync.Once
)

type (
	// LogField is a key-value pair that will be added to the log entry.
	LogField struct {
		Key   string
		Value any
	}

	// LogOption defines the method to customize the logging.
	LogOption func(options *logOptions)

	logEntry map[string]any

	logOptions struct {
		gzipEnabled           bool
		logStackCooldownMills int
		keepDays              int
		maxBackups            int
		maxSize               int
		rotationRule          string
	}
)

// Debug writes v into access log.
func Debug(v ...any) {
	if shallLog(DebugLevel) {
		writeDebug(fmt.Sprint(v...))
	}
}

// Debugf writes v with format into access log.
func Debugf(format string, v ...any) {
	if shallLog(DebugLevel) {
		writeDebug(fmt.Sprintf(format, v...))
	}
}

// DisableStat disables the stat logs.
func DisableStat() {
	atomic.StoreUint32(&disableStat, 1)
}

// Error writes v into error log.
func Error(v ...any) {
	if shallLog(ErrorLevel) {
		writeError(fmt.Sprint(v...))
	}
}

// Errorf writes v with format into error log.
func Errorf(format string, v ...any) {
	if shallLog(ErrorLevel) {
		writeError(fmt.Errorf(format, v...).Error())
	}
}

// ErrorStack writes v along with call stack into error log.
func ErrorStack(v ...any) {
	if shallLog(ErrorLevel) {
		// there is newline in stack string
		writeStack(fmt.Sprint(v...))
	}
}

// Field returns a LogField for the given key and value.
func Field(key string, value any) LogField {
	return LogField{
		Key:   key,
		Value: value,
	}
}

// Info writes v into access log.
func Info(v ...any) {
	if shallLog(InfoLevel) {
		writeInfo(fmt.Sprint(v...))
	}
}

// Infof writes v with format into access log.
func Infof(format string, v ...any) {
	if shallLog(InfoLevel) {
		writeInfo(fmt.Sprintf(format, v...))
	}
}

// SetLevel sets the logging level. It can be used to suppress some logs.
func SetLevel(level uint32) {
	atomic.StoreUint32(&logLevel, level)
}

// SetWriter sets the logging writer. It can be used to customize the logging.
func SetWriter(w Writer) {
	if atomic.LoadUint32(&logLevel) != disableLevel {
		writer.Store(w)
	}
}

// SetUp sets up the logx.
// If already set up, return nil.
// We allow SetUp to be called multiple times, because, for example,
// we need to allow different service frameworks to initialize logx respectively.
func SetUp(c LogConf) (err error) {
	// Ignore the later SetUp calls.
	// Because multiple services in one process might call SetUp respectively.
	// Need to wait for the first caller to complete the execution.
	setupOnce.Do(func() {
		setupLogLevel(c.Level)
		setupFieldKeys(c.FieldKeys)

		if !c.Stat {
			DisableStat()
		}

		if len(c.TimeFormat) > 0 {
			timeFormat = c.TimeFormat
		}

		if len(c.FileTimeFormat) > 0 {
			fileTimeFormat = c.FileTimeFormat
		}

		atomic.StoreUint32(&maxContentLength, c.MaxContentLength)

		switch c.Encoding {
		case plainEncoding:
			atomic.StoreUint32(&encoding, plainEncodingType)
		default:
			atomic.StoreUint32(&encoding, jsonEncodingType)
		}

		switch c.Mode {
		case fileMode:
			err = setupWithFiles(c)
		case volumeMode:
			err = setupWithVolume(c)
		default:
			setupWithConsole()
		}
	})

	return
}

// WithCooldownMillis customizes logging on writing call stack interval.
func WithCooldownMillis(millis int) LogOption {
	return func(opts *logOptions) {
		opts.logStackCooldownMills = millis
	}
}

// WithKeepDays customizes logging to keep logs with days.
func WithKeepDays(days int) LogOption {
	return func(opts *logOptions) {
		opts.keepDays = days
	}
}

// WithGzip customizes logging to automatically gzip the log files.
func WithGzip() LogOption {
	return func(opts *logOptions) {
		opts.gzipEnabled = true
	}
}

// WithMaxBackups customizes how many log files backups will be kept.
func WithMaxBackups(count int) LogOption {
	return func(opts *logOptions) {
		opts.maxBackups = count
	}
}

// WithMaxSize customizes how much space the writing log file can take up.
func WithMaxSize(size int) LogOption {
	return func(opts *logOptions) {
		opts.maxSize = size
	}
}

// WithRotation customizes which log rotation rule to use.
func WithRotation(r string) LogOption {
	return func(opts *logOptions) {
		opts.rotationRule = r
	}
}

func addCaller(fields ...LogField) []LogField {
	return append(fields, Field(callerKey, getCaller(callerDepth)))
}

func createOutput(path string) (io.WriteCloser, error) {
	if len(path) == 0 {
		return nil, ErrLogPathNotSet
	}

	var rule RotateRule
	switch options.rotationRule {
	case sizeRotationRule:
		rule = NewSizeLimitRotateRule(path, backupFileDelimiter, options.keepDays, options.maxSize,
			options.maxBackups, options.gzipEnabled)
	default:
		rule = DefaultRotateRule(path, backupFileDelimiter, options.keepDays, options.gzipEnabled)
	}

	return NewLogger(path, rule, options.gzipEnabled)
}

func encodeError(err error) (ret string) {
	return encodeWithRecover(err, func() string {
		return err.Error()
	})
}

func encodeStringer(v fmt.Stringer) (ret string) {
	return encodeWithRecover(v, func() string {
		return v.String()
	})
}

func encodeWithRecover(arg any, fn func() string) (ret string) {
	defer func() {
		if err := recover(); err != nil {
			if v := reflect.ValueOf(arg); v.Kind() == reflect.Ptr && v.IsNil() {
				ret = nilAngleString
			} else {
				ret = fmt.Sprintf("panic: %v", err)
			}
		}
	}()

	return fn()
}

func getWriter() Writer {
	w := writer.Load()
	if w == nil {
		w = writer.StoreIfNil(newConsoleWriter())
	}

	return w
}

func handleOptions(opts []LogOption) {
	for _, opt := range opts {
		opt(&options)
	}
}

func setupFieldKeys(c fieldKeyConf) {
	if len(c.CallerKey) > 0 {
		callerKey = c.CallerKey
	}
	if len(c.ContentKey) > 0 {
		contentKey = c.ContentKey
	}
	if len(c.DurationKey) > 0 {
		durationKey = c.DurationKey
	}
	if len(c.LevelKey) > 0 {
		levelKey = c.LevelKey
	}
	if len(c.SpanKey) > 0 {
		spanKey = c.SpanKey
	}
	if len(c.TimestampKey) > 0 {
		timestampKey = c.TimestampKey
	}
	if len(c.TraceKey) > 0 {
		traceKey = c.TraceKey
	}
	if len(c.TruncatedKey) > 0 {
		truncatedKey = c.TruncatedKey
	}
}

func setupLogLevel(level string) {
	switch level {
	case levelDebug:
		SetLevel(DebugLevel)
	case levelInfo:
		SetLevel(InfoLevel)
	case levelError:
		SetLevel(ErrorLevel)
	case levelSevere:
		SetLevel(SevereLevel)
	}
}

func setupWithConsole() {
	SetWriter(newConsoleWriter())
}

func setupWithFiles(c LogConf) error {
	w, err := newFileWriter(c)
	if err != nil {
		return err
	}

	SetWriter(w)
	return nil
}

func setupWithVolume(c LogConf) error {
	if len(c.ServiceName) == 0 {
		return ErrLogServiceNameNotSet
	}

	c.Path = path.Join(c.Path, c.ServiceName, sysx.Hostname())
	return setupWithFiles(c)
}

func shallLog(level uint32) bool {
	return atomic.LoadUint32(&logLevel) <= level
}

// writeDebug writes v into debug log.
// Not checking shallLog here is for performance consideration.
// If we check shallLog here, the fmt.Sprint might be called even if the log level is not enabled.
// The caller should check shallLog before calling this function.
func writeDebug(val any, fields ...LogField) {
	getWriter().Debug(val, mergeGlobalFields(addCaller(fields...))...)
}

// writeError writes v into the error log.
// Not checking shallLog here is for performance consideration.
// If we check shallLog here, the fmt.Sprint might be called even if the log level is not enabled.
// The caller should check shallLog before calling this function.
func writeError(val any, fields ...LogField) {
	getWriter().Error(val, mergeGlobalFields(addCaller(fields...))...)
}

// writeInfo writes v into info log.
// Not checking shallLog here is for performance consideration.
// If we check shallLog here, the fmt.Sprint might be called even if the log level is not enabled.
// The caller should check shallLog before calling this function.
func writeInfo(val any, fields ...LogField) {
	getWriter().Info(val, mergeGlobalFields(addCaller(fields...))...)
}

// writeStack writes v into stack log.
// Not checking shallLog here is for performance consideration.
// If we check shallLog here, the fmt.Sprint might be called even if the log level is not enabled.
// The caller should check shallLog before calling this function.
func writeStack(msg string) {
	getWriter().Stack(fmt.Sprintf("%s\n%s", msg, string(debug.Stack())))
}
