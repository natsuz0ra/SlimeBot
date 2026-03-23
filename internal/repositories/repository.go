package repositories

import (
	"context"
	"sync"

	"gorm.io/gorm"
)

type Repository struct {
	db      *gorm.DB
	ftsOnce sync.Once
	ftsOK   bool
}

// New 创建 Repository 实例
func New(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) dbWithContext(ctx context.Context) *gorm.DB {
	if ctx == nil {
		return r.db
	}
	return r.db.WithContext(ctx)
}
