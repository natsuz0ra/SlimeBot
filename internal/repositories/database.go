package repositories

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func NewSQLite(dbPath string) (*gorm.DB, error) {
	absPath, err := filepath.Abs(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve database path: %w", err)
	}

	db, err := gorm.Open(sqlite.Open(absPath), &gorm.Config{
		PrepareStmt: true,
		Logger:      newGormSlogLogger(200 * time.Millisecond),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect database: %w", err)
	}
	pool, err := db.DB()
	if err != nil {
		return nil, err
	}
	pool.SetMaxOpenConns(1)

	if err := RunMigrations(db); err != nil {
		return nil, err
	}
	// Pending checkpoints are crash-recovery state, so this runs on every
	// startup rather than being gated by the schema migration ledger.
	if err := recoverPendingContextCheckpoints(db); err != nil {
		return nil, fmt.Errorf("checkpoint recovery failed: %w", err)
	}

	return db, nil
}
