package repositories

import (
	"context"
	"errors"
	"fmt"
	"slimebot/internal/apperrors"
	"slimebot/internal/domain"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func escapeSQLiteLikePattern(s string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return replacer.Replace(s)
}

func (r *Repository) ListSessions(ctx context.Context, limit int, offset int, query string) ([]domain.Session, error) {
	var sessions []domain.Session
	q := r.dbWithContext(ctx).Where("kind = ?", domain.SessionKindChat).Order("updated_at desc")
	if trimmed := strings.TrimSpace(query); trimmed != "" {
		like := "%" + escapeSQLiteLikePattern(trimmed) + "%"
		q = q.Where("name LIKE ? ESCAPE '\\'", like)
	}
	if limit > 0 {
		q = q.Limit(limit).Offset(offset)
	}
	err := q.Find(&sessions).Error
	return sessions, err
}

func (r *Repository) GetSessionByID(ctx context.Context, id string) (*domain.Session, error) {
	var session domain.Session
	err := r.dbWithContext(ctx).First(&session, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("session %s: %w", id, apperrors.ErrNotFound)
	}
	return &session, err
}

func (r *Repository) CreateSession(ctx context.Context, name string, workingDirectory ...string) (*domain.Session, error) {
	session := &domain.Session{
		ID:   uuid.NewString(),
		Name: name,
	}
	if len(workingDirectory) > 0 {
		session.WorkingDirectory = workingDirectory[0]
	}
	err := r.dbWithContext(ctx).Create(session).Error
	return session, err
}

func (r *Repository) CreateSessionWithID(ctx context.Context, id, name string) (*domain.Session, error) {
	session := &domain.Session{
		ID:   id,
		Name: name,
	}
	err := r.dbWithContext(ctx).Create(session).Error
	return session, err
}

func (r *Repository) RenameSessionByUser(ctx context.Context, id, name string) error {
	return r.dbWithContext(ctx).Model(&domain.Session{}).
		Where("id = ?", id).
		Updates(map[string]any{"name": name, "is_title_locked": true, "updated_at": time.Now()}).
		Error
}

func (r *Repository) UpdateSessionTitle(ctx context.Context, id, name string) (bool, error) {
	result := r.dbWithContext(ctx).Model(&domain.Session{}).
		Where("id = ? AND is_title_locked = ? AND name <> ?", id, false, name).
		Updates(map[string]any{"name": name, "updated_at": time.Now()})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

func (r *Repository) DeleteSession(ctx context.Context, id string) error {
	return r.dbWithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var agents []domain.AgentDescriptor
		if err := tx.Where("root_id = ?", id).Find(&agents).Error; err != nil {
			return err
		}
		ids := []string{id}
		for _, a := range agents {
			ids = append(ids, a.SessionID)
		}
		for _, model := range []any{&domain.AgentDescriptor{}, &domain.AgentTurn{}, &domain.AgentTask{}, &domain.AgentArtifact{}, &domain.AgentApproval{}, &domain.AgentEvent{}} {
			if err := tx.Where("root_id = ?", id).Delete(model).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("target_id IN ?", ids).Delete(&domain.AgentInbox{}).Error; err != nil {
			return err
		}
		if err := tx.Where("session_id = ?", id).Delete(&domain.AgentRootRequest{}).Error; err != nil {
			return err
		}
		if err := tx.Where("session_id IN ?", ids).Delete(&domain.ThinkingRecord{}).Error; err != nil {
			return err
		}
		var teamRunIDs []string
		if err := tx.Model(&domain.TeamRun{}).Where("session_id IN ?", ids).Pluck("id", &teamRunIDs).Error; err != nil {
			return err
		}
		if len(teamRunIDs) > 0 {
			if err := tx.Where("team_run_id IN ?", teamRunIDs).Delete(&domain.TeamMemberRun{}).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("session_id IN ?", ids).Delete(&domain.TeamRun{}).Error; err != nil {
			return err
		}
		// Delete messages.
		if err := tx.Table("messages").Where("session_id IN ?", ids).Delete(nil).Error; err != nil {
			return err
		}
		// Delete tool call records.
		if err := tx.Table("tool_call_records").Where("session_id IN ?", ids).Delete(nil).Error; err != nil {
			return err
		}
		for _, model := range []any{&domain.ContextEntry{}, &domain.ContextCheckpoint{}, &domain.ContextHead{}} {
			if err := tx.Where("scope_id IN ?", ids).Delete(model).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("session_id IN ?", ids).Delete(&domain.SessionContextSummary{}).Error; err != nil {
			return err
		}
		// Delete the session row.
		return tx.Table("sessions").Where("id IN ?", ids).Delete(nil).Error
	})
}

// SearchChats scans SQL candidates in bounded batches so hidden timeline markers
// do not produce hits or change the pagination of visible results.
func (r *Repository) SearchChats(ctx context.Context, query, scope string, limit, offset int) ([]domain.ChatSearchHit, error) {
	like := "%" + escapeSQLiteLikePattern(query) + "%"
	sql := `SELECT s.id AS session_id, s.name AS session_name, s.working_directory, '' AS message_id,
 '' AS role, s.name AS content, s.updated_at AS created_at, 0 AS seq, 0 AS kind
 FROM sessions s WHERE s.deleted_at IS NULL AND s.kind = 'chat' AND s.name LIKE ? ESCAPE '\' AND ? <> 'messages'
 UNION ALL
 SELECT s.id, s.name, s.working_directory, m.id, m.role, m.content, m.created_at, m.seq, 1 AS kind
 FROM messages m JOIN sessions s ON s.id = m.session_id
 WHERE s.deleted_at IS NULL AND s.kind = 'chat' AND m.role IN ('user', 'assistant')
 AND m.content LIKE ? ESCAPE '\' AND ? <> 'titles'
 ORDER BY kind ASC, created_at DESC, session_id ASC, seq DESC, message_id ASC
 LIMIT ? OFFSET ?`
	results := make([]domain.ChatSearchHit, 0, limit)
	skipped := 0
	// ponytail: substring search scans SQLite rows; add FTS when archive size makes it measurably slow.
	for cursor := 0; ; cursor += 100 {
		var candidates []domain.ChatSearchHit
		if err := r.dbWithContext(ctx).Raw(sql, like, scope, like, scope, 100, cursor).Scan(&candidates).Error; err != nil {
			return nil, err
		}
		for _, hit := range candidates {
			if hit.MessageID != "" {
				hit.Content = domain.StripContentMarkers(hit.Content)
			}
			if !strings.Contains(strings.ToLower(hit.Content), strings.ToLower(query)) {
				continue
			}
			if skipped < offset {
				skipped++
				continue
			}
			results = append(results, hit)
			if len(results) == limit {
				return results, nil
			}
		}
		if len(candidates) < 100 {
			return results, nil
		}
	}
}

// CreateTaskRunContext persists an isolated execution context for the chat engine.
// Its messages belong to run history and are excluded from ordinary chat listings/search.
func (r *Repository) CreateTaskRunContext(ctx context.Context, name, workingDirectory string) (*domain.Session, error) {
	session := &domain.Session{ID: uuid.NewString(), Name: name, Kind: domain.SessionKindTaskRun, IsTitleLocked: true, WorkingDirectory: workingDirectory}
	err := r.dbWithContext(ctx).Create(session).Error
	return session, err
}
