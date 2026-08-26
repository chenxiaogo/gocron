package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm/logger"

	"scheduler-platform/pkg/ilog"
)

// gormLogger 接入自研链路日志的自定义 GORM logger。
// 相比 gorm 默认 logger（输出到 stdout、无链路信息），它在 Trace 回调里
// 用 ilog.Trace(ctx) 输出，能从 ctx 中提取 trace/span 链路 ID，使数据库
// 慢查询 / 错误日志与业务日志、HTTP 访问日志同链路，便于排查关联。
// 默认级别为 Warn：仅记录慢查询与错误，不刷正常 SQL。
type gormLogger struct {
	level        logger.LogLevel
	slowThreshold time.Duration
}

// NewGormLogger 返回接入链路 ID 的 GORM logger。
func NewGormLogger() logger.Interface {
	return &gormLogger{
		level:         logger.Warn,
		slowThreshold: 200 * time.Millisecond,
	}
}

func (l *gormLogger) LogMode(level logger.LogLevel) logger.Interface {
	nl := *l
	nl.level = level
	return &nl
}

func (l *gormLogger) Info(ctx context.Context, msg string, data ...interface{}) {
	if l.level < logger.Info {
		return
	}
	ilog.Trace(ctx).Infof("%s", fmt.Sprintf(msg, data...))
}

func (l *gormLogger) Warn(ctx context.Context, msg string, data ...interface{}) {
	if l.level < logger.Warn {
		return
	}
	ilog.Trace(ctx).Warnf("%s", fmt.Sprintf(msg, data...))
}

func (l *gormLogger) Error(ctx context.Context, msg string, data ...interface{}) {
	if l.level < logger.Error {
		return
	}
	ilog.Trace(ctx).Errorf("%s", fmt.Sprintf(msg, data...))
}

// Trace 打印 SQL 日志：错误 / 慢查询 / Info 级正常 SQL。
// 统一走 ilog.Trace(ctx)，自动携带请求链路 ID。
func (l *gormLogger) Trace(ctx context.Context, begin time.Time, fc func() (sql string, rowsAffected int64), err error) {
	if l.level <= logger.Silent {
		return
	}
	elapsed := time.Since(begin)
	sql, rows := fc()
	rowStr := fmt.Sprintf("%d", rows)
	if rows == -1 {
		rowStr = "-"
	}
	ms := float64(elapsed.Nanoseconds()) / 1e6

	switch {
	case err != nil && l.level >= logger.Error && !errors.Is(err, logger.ErrRecordNotFound) && !errors.Is(err, context.Canceled):
		ilog.Trace(ctx).Errorf("[gorm] sql error, %.3fms [rows:%s] %s | err=%v", ms, rowStr, sql, err)
	case elapsed > l.slowThreshold && l.slowThreshold != 0 && l.level >= logger.Warn:
		ilog.Trace(ctx).Warnf("[gorm] SLOW SQL >= %v, %.3fms [rows:%s] %s", l.slowThreshold, ms, rowStr, sql)
	case l.level == logger.Info:
		ilog.Trace(ctx).Infof("[gorm] %.3fms [rows:%s] %s", ms, rowStr, sql)
	}
}
