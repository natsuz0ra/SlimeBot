package repositories

import (
	"context"
	"log/slog"
	"time"

	gormlogger "gorm.io/gorm/logger"
)

type gormSlogLogger struct {
	slowThreshold time.Duration
}

func newGormSlogLogger(slowThreshold time.Duration) gormlogger.Interface {
	return &gormSlogLogger{slowThreshold: slowThreshold}
}

func (l *gormSlogLogger) LogMode(level gormlogger.LogLevel) gormlogger.Interface {
	return l
}

func (l *gormSlogLogger) Info(ctx context.Context, msg string, data ...any) {}

func (l *gormSlogLogger) Warn(ctx context.Context, msg string, data ...any) {
	slog.Warn(msg, "data", data)
}

func (l *gormSlogLogger) Error(ctx context.Context, msg string, data ...any) {
	slog.Error(msg, "data", data)
}

func (l *gormSlogLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	elapsed := time.Since(begin)
	sql, rows := fc()
	if err != nil {
		slog.Warn("sql_error", "err", err, "sql", sql, "rows", rows, "ms", elapsed.Milliseconds())
		return
	}
	if elapsed >= l.slowThreshold {
		slog.Warn("sql_slow", "ms", elapsed.Milliseconds(), "rows", rows, "sql", sql)
	}
}
