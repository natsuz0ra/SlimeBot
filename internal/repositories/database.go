package repositories

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"slimebot/internal/domain"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupSessionMemoryFTS5(db *gorm.DB) error {
	if err := db.Exec(`CREATE VIRTUAL TABLE IF NOT EXISTS session_memories_fts USING fts5(
		summary,
		keywords_text,
		content='session_memories',
		content_rowid='rowid',
		tokenize='unicode61'
	)`).Error; err != nil {
		slog.Warn("session_memories_fts_unavailable", "err", err)
		return nil
	}
	triggers := []string{
		`CREATE TRIGGER IF NOT EXISTS session_memories_ai AFTER INSERT ON session_memories BEGIN
			INSERT INTO session_memories_fts(rowid, summary, keywords_text) VALUES (new.rowid, new.summary, new.keywords_text);
		END`,
		`CREATE TRIGGER IF NOT EXISTS session_memories_ad AFTER DELETE ON session_memories BEGIN
			INSERT INTO session_memories_fts(session_memories_fts, rowid, summary, keywords_text) VALUES('delete', old.rowid, old.summary, old.keywords_text);
		END`,
		`CREATE TRIGGER IF NOT EXISTS session_memories_au AFTER UPDATE ON session_memories BEGIN
			INSERT INTO session_memories_fts(session_memories_fts, rowid, summary, keywords_text) VALUES('delete', old.rowid, old.summary, old.keywords_text);
			INSERT INTO session_memories_fts(rowid, summary, keywords_text) VALUES (new.rowid, new.summary, new.keywords_text);
		END`,
	}
	for _, sql := range triggers {
		if err := db.Exec(sql).Error; err != nil {
			slog.Warn("session_memories_fts_trigger_failed", "err", err)
			return nil
		}
	}
	return nil
}

func NewSQLite(dbPath string) (*gorm.DB, error) {
	absPath, err := filepath.Abs(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve database path: %w", err)
	}

	db, err := gorm.Open(sqlite.Open(absPath), &gorm.Config{PrepareStmt: true})
	if err != nil {
		return nil, fmt.Errorf("failed to connect database: %w", err)
	}

	if err := db.AutoMigrate(
		&domain.Session{},
		&domain.Message{},
		&domain.SessionMemory{},
		&domain.ToolCallRecord{},
		&domain.AppSetting{},
		&domain.LLMConfig{},
		&domain.MCPConfig{},
		&domain.MessagePlatformConfig{},
		&domain.Skill{},
	); err != nil {
		return nil, fmt.Errorf("auto migration failed: %w", err)
	}
	_ = setupSessionMemoryFTS5(db)

	return db, nil
}
