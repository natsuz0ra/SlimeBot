package repositories

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"slimebot/internal/domain"
	llm "slimebot/internal/services/llm"
	"time"
)

var ErrContextConflict = errors.New("上下文已变化，请重新准备请求")

func bumpContextHistory(tx *gorm.DB, scope string) error {
	return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "scope_id"}}, DoUpdates: clause.Assignments(map[string]any{"history_revision": gorm.Expr("history_revision + 1"), "updated_at": time.Now()})}).Create(&domain.ContextHead{ScopeID: scope, HistoryRevision: 1}).Error
}

// GetContextSnapshot is strictly read-only, including for sessions without a head.
func (r *Repository) GetContextSnapshot(ctx context.Context, scope string) (domain.ContextSnapshot, error) {
	var s domain.ContextSnapshot
	s.Head.ScopeID = scope
	err := r.dbWithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("scope_id = ?", scope).Take(&s.Head).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err := tx.Where("scope_id = ?", scope).Order("message_seq asc, id asc").Find(&s.Entries).Error; err != nil {
			return err
		}
		return tx.Where("scope_id = ? AND active = ? AND status = ?", scope, true, "committed").Order("created_at asc, id asc").Find(&s.Checkpoints).Error
	})
	return s, err
}

// SyncContextSeeds imports legacy UI history and the current, actual user input.
// Existing exact assistant steps are never reconstructed from the aggregated UI message.
func (r *Repository) SyncContextSeeds(ctx context.Context, scope string, seeds []domain.ContextSeed) error {
	return r.dbWithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var session domain.Session
		if err := tx.Where("id = ?", scope).Take(&session).Error; err != nil {
			return err
		}
		var history []domain.Message
		if err := tx.Where("session_id = ?", scope).Order("seq asc").Find(&history).Error; err != nil {
			return err
		}
		if len(history) != len(seeds) {
			return ErrContextConflict
		}
		for i, m := range history {
			if seeds[i].MessageID != m.ID || seeds[i].SourceHash != domain.ContextMessageHash(m) {
				return ErrContextConflict
			}
		}
		var all []domain.ContextEntry
		if err := tx.Select("id, source_message_id, fidelity").Where("scope_id = ?", scope).Find(&all).Error; err != nil {
			return err
		}
		existing := map[string][]domain.ContextEntry{}
		for _, e := range all {
			existing[e.SourceMessageID] = append(existing[e.SourceMessageID], e)
		}
		changed := false
		for _, seed := range seeds {
			old := existing[seed.MessageID]
			if len(old) > 0 {
				// The expanded current input replaces only a previously imported user entry.
				if seed.Fidelity == "exact" && len(seed.Messages) == 1 && seed.Messages[0].Role == "user" && len(old) == 1 && old[0].Fidelity == "legacy_replay" {
					b, err := json.Marshal(seed.Messages[0])
					if err != nil {
						return err
					}
					if err := tx.Model(&domain.ContextEntry{}).Where("id = ?", old[0].ID).Updates(map[string]any{"payload": string(b), "fidelity": "exact"}).Error; err != nil {
						return err
					}
					changed = true
				}
				continue
			}
			for _, m := range seed.Messages {
				b, err := json.Marshal(m)
				if err != nil {
					return err
				}
				e := domain.ContextEntry{ScopeID: scope, SourceMessageID: seed.MessageID, MessageSeq: seed.Seq, Fidelity: seed.Fidelity, Payload: string(b), ToolCallID: m.ToolCallID, SourceKind: contextSourceKind(m, seed.Messages[0].Role == "user")}
				if err := tx.Create(&e).Error; err != nil {
					return err
				}
				changed = true
			}
		}
		if changed {
			return bumpContextHistory(tx, scope)
		}
		return nil
	})
}

func (r *Repository) AppendContextEntries(ctx context.Context, scope, request, routeHash string, messageSeq, expected int64, messages []llm.ChatMessage) ([]domain.ContextEntry, int64, error) {
	var entries []domain.ContextEntry
	err := r.dbWithContext(ctx).Transaction(func(tx *gorm.DB) error {
		update := tx.Model(&domain.ContextHead{}).Where("scope_id = ? AND history_revision = ?", scope, expected).Updates(map[string]any{"history_revision": expected + 1, "updated_at": time.Now()})
		if update.Error != nil {
			return update.Error
		}
		if update.RowsAffected != 1 {
			return ErrContextConflict
		}
		for _, m := range messages {
			b, err := json.Marshal(m)
			if err != nil {
				return err
			}
			e := domain.ContextEntry{ScopeID: scope, RequestID: request, MessageSeq: messageSeq, Fidelity: "exact", Payload: string(b), ToolCallID: m.ToolCallID, SourceKind: contextSourceKind(m, false), SourceRouteHash: routeHash}
			if err := tx.Create(&e).Error; err != nil {
				return err
			}
			entries = append(entries, e)
		}
		return nil
	})
	return entries, expected + 1, err
}

func (r *Repository) BindContextRequest(ctx context.Context, scope, request, messageID string) error {
	return r.dbWithContext(ctx).Where("scope_id = ? AND request_id = ?", scope, request).Model(&domain.ContextEntry{}).Update("source_message_id", messageID).Error
}

func (r *Repository) CreateContextAttempt(ctx context.Context, c *domain.ContextCheckpoint) error {
	return r.dbWithContext(ctx).Create(c).Error
}
func (r *Repository) FinishContextAttempt(ctx context.Context, id, status, reason string) error {
	if len(reason) > 500 {
		reason = reason[:500]
	}
	return r.dbWithContext(ctx).Model(&domain.ContextCheckpoint{}).Where("id = ? AND status = ?", id, "pending").Updates(map[string]any{"status": status, "failure": reason, "updated_at": time.Now()}).Error
}

func (r *Repository) CommitContextCheckpoint(ctx context.Context, c *domain.ContextCheckpoint, replaced []string) error {
	return r.dbWithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&domain.ContextHead{}).Where("scope_id = ? AND history_revision = ? AND projection_revision = ?", c.ScopeID, c.HistoryRevision, c.ProjectionRevision).Updates(map[string]any{"projection_revision": c.ProjectionRevision + 1, "updated_at": time.Now()})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrContextConflict
		}
		if len(replaced) > 0 {
			if err := tx.Model(&domain.ContextCheckpoint{}).Where("scope_id = ? AND id IN ?", c.ScopeID, replaced).Updates(map[string]any{"active": false, "status": "superseded"}).Error; err != nil {
				return err
			}
		}
		result = tx.Model(&domain.ContextCheckpoint{}).Where("id = ? AND scope_id = ? AND status = ?", c.ID, c.ScopeID, "pending").Updates(map[string]any{"active": true, "status": "committed", "summary": c.Summary, "after_tokens": c.AfterTokens, "usage_json": c.UsageJSON, "updated_at": time.Now()})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return fmt.Errorf("checkpoint attempt unavailable: %w", ErrContextConflict)
		}
		return nil
	})
}

func invalidateContext(tx *gorm.DB, scope string, fromSeq int64) error {
	if err := tx.Where("scope_id = ? AND message_seq >= ?", scope, fromSeq).Delete(&domain.ContextEntry{}).Error; err != nil {
		return err
	}
	if err := tx.Model(&domain.ContextCheckpoint{}).Where("scope_id = ?", scope).Updates(map[string]any{"active": false, "status": "superseded"}).Error; err != nil {
		return err
	}
	if err := bumpContextHistory(tx, scope); err != nil {
		return err
	}
	return tx.Model(&domain.ContextHead{}).Where("scope_id = ?", scope).Update("projection_revision", gorm.Expr("projection_revision + 1")).Error
}

func (r *Repository) VerifyContextRevision(ctx context.Context, scope string, history, projection int64) error {
	var head domain.ContextHead
	if err := r.dbWithContext(ctx).Where("scope_id = ?", scope).Take(&head).Error; err != nil {
		return err
	}
	if head.HistoryRevision != history || head.ProjectionRevision != projection {
		return ErrContextConflict
	}
	return nil
}

func contextSourceKind(m llm.ChatMessage, direct bool) string {
	if m.Role == "user" {
		if direct {
			return "direct_user"
		}
		return "tool_artifact"
	}
	return m.Role
}

// Request metadata is written only on actual preparation, never by usage reads.
func (r *Repository) RecordContextRequest(ctx context.Context, scope string, history, projection int64, c llm.ModelRuntimeConfig, tools []llm.ToolDef) error {
	b, err := json.Marshal(tools)
	if err != nil {
		return err
	}
	result := r.dbWithContext(ctx).Model(&domain.ContextHead{}).Where("scope_id = ? AND history_revision = ? AND projection_revision = ?", scope, history, projection).Updates(map[string]any{"last_tools_json": string(b), "last_config_id": c.ConfigID, "last_model": c.Model, "last_window": c.ContextSize, "last_output_reserve": c.MaxOutputTokens})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrContextConflict
	}
	return nil
}
