package repositories

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
)

type contextSQLRecorder struct {
	*gormSlogLogger
	sql string
}

func (l *contextSQLRecorder) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	l.sql, _ = fc()
}

func TestGormLoggerDoesNotExpandContextPayloads(t *testing.T) {
	db := NewSQLiteDBTest(t, "context_sql_logging")
	logger := &contextSQLRecorder{gormSlogLogger: &gormSlogLogger{slowThreshold: time.Second}}
	secret := "private tool output and credential"
	if err := db.Session(&gorm.Session{Logger: logger}).Exec("INSERT INTO context_entries (scope_id, message_seq, fidelity, payload) VALUES (?, ?, ?, ?)", "scope", 1, "exact", secret).Error; err != nil {
		t.Fatal(err)
	}
	if strings.Contains(logger.sql, secret) || !strings.Contains(logger.sql, "?") {
		t.Fatalf("SQL diagnostics exposed bound values: %s", logger.sql)
	}
	var actual string
	if err := db.Raw("SELECT payload FROM context_entries WHERE scope_id = ?", "scope").Scan(&actual).Error; err != nil || actual != secret {
		t.Fatalf("redaction changed stored values: %q %v", actual, err)
	}
}

func TestGormLoggerSkipsRecordNotFound(t *testing.T) {
	logger := newGormSlogLogger(time.Millisecond)
	logger.Trace(context.Background(), time.Now().Add(-time.Second), func() (string, int64) {
		t.Fatal("record not found should not format or log SQL")
		return "SELECT 1", 0
	}, fmt.Errorf("lookup: %w", gorm.ErrRecordNotFound))
}
