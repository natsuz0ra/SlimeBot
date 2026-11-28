package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"
)

// ContextEntry stores the actual message seen by the model, independently of UI aggregation.
type ContextEntry struct {
	ToolCallID      string `gorm:"size:128"`
	SourceRouteHash string `gorm:"size:64"`
	SourceKind      string `gorm:"size:24"`
	ID              int64  `gorm:"primaryKey;autoIncrement"`
	ScopeID         string `gorm:"size:64;not null;index:idx_context_entries_scope_order,priority:1"`
	MessageSeq      int64  `gorm:"not null;index:idx_context_entries_scope_order,priority:2"`
	SourceMessageID string `gorm:"size:36;index"`
	RequestID       string `gorm:"size:64;index"`
	Fidelity        string `gorm:"size:24;not null"`
	Payload         string `gorm:"type:text;not null"`
	CreatedAt       time.Time
}

func (e ContextEntry) Message() (ChatMessage, error) {
	var m ChatMessage
	err := json.Unmarshal([]byte(e.Payload), &m)
	return m, err
}

type ContextHead struct {
	LastToolsJSON           string `gorm:"type:text"`
	LastConfigID            string `gorm:"size:36"`
	LastModel               string `gorm:"size:256"`
	LastWindow              int
	LastOutputReserve       int
	ScopeID                 string `gorm:"primaryKey;size:64"`
	HistoryRevision         int64  `gorm:"not null;default:0"`
	ProjectionRevision      int64  `gorm:"not null;default:0"`
	CompactionFailures      int    `gorm:"not null;default:0"`
	CompactionCooldownUntil *time.Time
	UpdatedAt               time.Time
}

// ContextCalibration persists the conservative local-to-provider token ratio.
// It is keyed only by provider and model; endpoint credentials are never stored.
type ContextCalibration struct {
	ID        string  `gorm:"primaryKey;size:64"`
	Provider  string  `gorm:"size:32;not null;index:idx_context_calibration_route,unique,priority:1"`
	Model     string  `gorm:"size:256;not null;index:idx_context_calibration_route,unique,priority:2"`
	Ratio     float64 `gorm:"not null;default:1"`
	Samples   int64   `gorm:"not null;default:0"`
	UpdatedAt time.Time
}

type ContextCheckpoint struct {
	ParentIDs          string `gorm:"type:text"`
	ID                 string `gorm:"primaryKey;size:36"`
	ScopeID            string `gorm:"size:64;not null;index"`
	RequestID          string `gorm:"size:64"`
	SourceIDs          string `gorm:"type:text;not null"`
	InputHash          string `gorm:"size:64;not null"`
	Summary            string `gorm:"type:text"`
	Status             string `gorm:"size:24;not null;index"`
	Active             bool   `gorm:"not null;default:false"`
	HistoryRevision    int64
	ProjectionRevision int64
	BeforeTokens       int
	AfterTokens        int
	Model              string `gorm:"size:256"`
	Reason             string `gorm:"size:64"`
	Failure            string `gorm:"size:512"`
	UsageJSON          string `gorm:"type:text"`
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type ContextSnapshot struct {
	Head        ContextHead
	Entries     []ContextEntry
	Checkpoints []ContextCheckpoint
	LastAttempt *ContextCheckpoint `gorm:"-"`
}
type ContextSeed struct {
	MessageID  string
	Seq        int64
	SourceHash string
	Fidelity   string
	Messages   []ChatMessage
}

func ContextMessageHash(m Message) string {
	b, _ := json.Marshal([]any{m.ID, m.Seq, m.Role, m.Content, m.AttachmentsJSON})
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
