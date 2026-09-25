package repositories

import (
	"context"
	"fmt"
	"testing"
	"time"

	"gorm.io/gorm"
)

func TestGormLoggerSkipsRecordNotFound(t *testing.T) {
	logger := newGormSlogLogger(time.Millisecond)
	logger.Trace(context.Background(), time.Now().Add(-time.Second), func() (string, int64) {
		t.Fatal("record not found should not format or log SQL")
		return "SELECT 1", 0
	}, fmt.Errorf("lookup: %w", gorm.ErrRecordNotFound))
}
