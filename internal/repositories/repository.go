package repositories

import "gorm.io/gorm"

// Repository 仓储聚合根，提供对数据库的访问入口
type Repository struct {
	db *gorm.DB
}

// New 创建 Repository 实例
func New(db *gorm.DB) *Repository {
	return &Repository{db: db}
}
