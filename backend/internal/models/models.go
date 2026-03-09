package models

import "time"

type Session struct {
	ID            string     `gorm:"primaryKey;size:36" json:"id"`
	Name          string     `gorm:"size:128;not null" json:"name"`
	IsTitleLocked bool       `gorm:"default:false;not null" json:"isTitleLocked"`
	ModelConfigID *string    `gorm:"size:36" json:"modelConfigId,omitempty"`
	CreatedAt     time.Time  `json:"createdAt"`
	UpdatedAt     time.Time  `gorm:"index" json:"updatedAt"`
	DeletedAt     *time.Time `gorm:"index" json:"-"`
}

type Message struct {
	ID        string    `gorm:"primaryKey;size:36" json:"id"`
	SessionID string    `gorm:"size:36;index;not null" json:"sessionId"`
	Role      string    `gorm:"size:16;index;not null" json:"role"`
	Content   string    `gorm:"type:text;not null" json:"content"`
	CreatedAt time.Time `gorm:"index" json:"createdAt"`
}

type AppSetting struct {
	Key       string    `gorm:"primaryKey;size:64" json:"key"`
	Value     string    `gorm:"type:text;not null" json:"value"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type LLMConfig struct {
	ID        string    `gorm:"primaryKey;size:36" json:"id"`
	Name      string    `gorm:"size:128;not null" json:"name"`
	BaseURL   string    `gorm:"size:512;not null" json:"baseUrl"`
	APIKey    string    `gorm:"size:512;not null" json:"apiKey"`
	Model     string    `gorm:"size:128;not null" json:"model"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type MCPConfig struct {
	ID         string    `gorm:"primaryKey;size:36" json:"id"`
	Name       string    `gorm:"size:128;not null" json:"name"`
	ServerURL  string    `gorm:"size:512;not null" json:"serverUrl"`
	AuthType   string    `gorm:"size:64" json:"authType"`
	AuthValue  string    `gorm:"size:512" json:"authValue"`
	IsEnabled  bool      `gorm:"default:true" json:"isEnabled"`
	CreatedAt  time.Time `json:"createdAt"`
	ModifiedAt time.Time `gorm:"column:updated_at" json:"updatedAt"`
}
