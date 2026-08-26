package utils

import "go.uber.org/zap"

// ZapAdapter adapts *zap.Logger to the Logger interface.
type ZapAdapter struct {
	log *zap.Logger
}

func NewZapAdapter(log *zap.Logger) *ZapAdapter { return &ZapAdapter{log: log} }

func (l *ZapAdapter) Debug(msg string, fields ...interface{}) {
	l.log.Debug(msg, l.toFields(fields...)...)
}
func (l *ZapAdapter) Info(msg string, fields ...interface{}) {
	l.log.Info(msg, l.toFields(fields...)...)
}
func (l *ZapAdapter) Warn(msg string, fields ...interface{}) {
	l.log.Warn(msg, l.toFields(fields...)...)
}
func (l *ZapAdapter) Error(msg string, fields ...interface{}) {
	l.log.Error(msg, l.toFields(fields...)...)
}
func (l *ZapAdapter) Fatal(msg string, fields ...interface{}) {
	l.log.Fatal(msg, l.toFields(fields...)...)
}

func (l *ZapAdapter) toFields(fields ...interface{}) []zap.Field {
	zf := make([]zap.Field, 0, len(fields)/2)
	for i := 0; i+1 < len(fields); i += 2 {
		key, ok := fields[i].(string)
		if !ok {
			continue
		}
		switch v := fields[i+1].(type) {
		case string:
			zf = append(zf, zap.String(key, v))
		case int:
			zf = append(zf, zap.Int(key, v))
		case int64:
			zf = append(zf, zap.Int64(key, v))
		case uint:
			zf = append(zf, zap.Uint(key, v))
		case float64:
			zf = append(zf, zap.Float64(key, v))
		case bool:
			zf = append(zf, zap.Bool(key, v))
		case error:
			zf = append(zf, zap.NamedError(key, v))
		default:
			zf = append(zf, zap.Any(key, v))
		}
	}
	return zf
}