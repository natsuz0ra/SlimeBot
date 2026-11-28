package repositories

import (
	"fmt"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func NewSQLiteDBTest(t testing.TB, namespace string) *gorm.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:%s_%d?mode=memory&cache=shared", namespace, time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: newGormSlogLogger(200 * time.Millisecond)})
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	pool.SetMaxOpenConns(1)
	if err := RunMigrations(db); err != nil {
		t.Fatalf("run migrations failed: %v", err)
	}
	return db
}
