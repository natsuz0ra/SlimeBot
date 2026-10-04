package repositories

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"slimebot/internal/domain"
	"time"
)

func (r *Repository) CreateAgent(ctx context.Context, a *domain.AgentDescriptor, input *domain.AgentInbox) error {
	return r.dbWithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&domain.AgentDescriptor{}).Where("root_id = ?", a.RootID).Count(&count).Error; err != nil {
			return err
		}
		if count >= 512 {
			return domain.ErrAgentCapacity
		}
		if err := tx.Model(&domain.AgentDescriptor{}).Where("request_id = ?", a.RequestID).Count(&count).Error; err != nil {
			return err
		}
		if count >= 16 {
			return domain.ErrAgentCapacity
		}
		session := domain.Session{ID: a.SessionID, Kind: domain.SessionKindSubagent, Name: a.Title, WorkingDirectory: a.Workspace, IsTitleLocked: true}
		if err := tx.Create(&session).Error; err != nil {
			return err
		}
		if err := tx.Create(a).Error; err != nil {
			return err
		}
		if a.ContextMode == "fork" {
			if err := forkAgentContext(tx, a.ParentID, a.SessionID); err != nil {
				return err
			}
		}
		return acceptAgentInbox(tx, input)
	})
}

func forkAgentContext(tx *gorm.DB, parent, child string) error {
	var last domain.Message
	if err := tx.Where("session_id = ? AND role = ? AND is_interrupted = ?", parent, "assistant", false).Order("seq desc").Take(&last).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	var source []domain.ContextEntry
	if err := tx.Where("scope_id = ? AND message_seq <= ? AND source_message_id <> ?", parent, last.Seq, "").Order("message_seq,id").Find(&source).Error; err != nil {
		return err
	}
	pending := map[string]bool{}
	end := 0
	for i, e := range source {
		msg, err := e.Message()
		if err != nil {
			return err
		}
		if msg.Role == "assistant" {
			for _, call := range msg.ToolCalls {
				pending[call.ID] = true
			}
		}
		if msg.Role == "tool" {
			delete(pending, msg.ToolCallID)
		}
		if len(pending) == 0 {
			end = i + 1
		}
	}
	for _, e := range source[:end] {
		e.ID = 0
		e.ScopeID = child
		e.RequestID = ""
		e.SourceMessageID = ""
		e.MessageSeq = 0
		message, err := e.Message()
		if err != nil {
			return err
		}
		message.SourceInboxID = ""
		if e.SourceKind == "direct_user" || e.SourceKind == "human_input" || e.SourceKind == "delegated_task" {
			e.SourceKind = "fork_background"
			message.SourceKind = "fork_background"
		}
		payload, err := json.Marshal(message)
		if err != nil {
			return err
		}
		e.Payload = string(payload)
		if err := tx.Create(&e).Error; err != nil {
			return err
		}
	}
	if end > 0 {
		return bumpContextHistory(tx, child)
	}
	return nil
}
func (r *Repository) GetAgent(ctx context.Context, id string) (*domain.AgentDescriptor, error) {
	var a domain.AgentDescriptor
	err := r.dbWithContext(ctx).Where("session_id = ?", id).Take(&a).Error
	return &a, err
}
func (r *Repository) ListAgents(ctx context.Context, root string) ([]domain.AgentDescriptor, error) {
	var a []domain.AgentDescriptor
	err := r.dbWithContext(ctx).Where("root_id = ?", root).Order("created_at, session_id").Limit(512).Find(&a).Error
	return a, err
}
func acceptAgentInbox(tx *gorm.DB, m *domain.AgentInbox) error {
	var old domain.AgentInbox
	err := tx.Where("target_id = ? AND client_id = ?", m.TargetID, m.ClientID).Take(&old).Error
	if err == nil {
		*m = old
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if m.RequestID != "" {
		var root domain.AgentRootRequest
		if err := tx.Where("id = ?", m.RequestID).Take(&root).Error; err != nil {
			return err
		}
		if root.Status != "running" && root.Status != "waiting" {
			return domain.ErrAgentConflict
		}
		if m.Source == "agent_message" && root.Messages >= 128 {
			return domain.ErrAgentBudget
		}
		if m.Source == "agent_message" {
			if err := tx.Model(&root).Where("messages = ?", root.Messages).Update("messages", root.Messages+1).Error; err != nil {
				return err
			}
		}
	}
	var count int64
	if err := tx.Model(&domain.AgentInbox{}).Where("target_id = ? AND status = ?", m.TargetID, "pending").Count(&count).Error; err != nil {
		return err
	}
	limit := int64(16)
	if m.Source == "agent_result" {
		limit = 128
	}
	if count >= limit {
		return domain.ErrAgentInboxFull
	}
	var seq int64
	if err := tx.Model(&domain.AgentInbox{}).Where("target_id = ?", m.TargetID).Select("COALESCE(MAX(seq),0)").Scan(&seq).Error; err != nil {
		return err
	}
	m.Seq = seq + 1
	m.Status = "pending"
	if m.ID == "" {
		m.ID = uuid.NewString()
	}
	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now()
	}
	return tx.Create(m).Error
}
func (r *Repository) AcceptAgentMessage(ctx context.Context, m *domain.AgentInbox) error {
	return r.dbWithContext(ctx).Transaction(func(tx *gorm.DB) error { return acceptAgentInbox(tx, m) })
}
func (r *Repository) ClaimAgentMessage(ctx context.Context, id, request, epoch string) (*domain.AgentInbox, *domain.AgentTurn, error) {
	var m domain.AgentInbox
	t := domain.AgentTurn{ID: uuid.NewString(), SessionID: id, Status: "running", Revision: 1, Epoch: epoch}
	err := r.dbWithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var live int64
		if err := tx.Model(&domain.AgentTurn{}).Where("session_id = ? AND status IN ?", id, []string{"running", "waiting", "stopping"}).Count(&live).Error; err != nil {
			return err
		}
		if live > 0 {
			return domain.ErrAgentConflict
		}
		q := tx.Where("target_id = ? AND status = ?", id, "pending")
		if request != "" {
			q = q.Where("request_id = ?", request)
		}
		if err := q.Order("seq").Take(&m).Error; err != nil {
			return err
		}
		var a domain.AgentDescriptor
		if err := tx.Where("session_id = ?", id).Take(&a).Error; err != nil {
			return err
		}
		t.RootID = a.RootID
		t.RequestID = m.RequestID
		t.MessageID = m.ID
		result := tx.Model(&domain.AgentInbox{}).Where("id = ? AND status = ?", m.ID, "pending").Updates(map[string]any{"status": "claimed", "turn_id": t.ID})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return domain.ErrAgentConflict
		}
		m.Status = "claimed"
		m.TurnID = t.ID
		return tx.Create(&t).Error
	})
	return &m, &t, err
}
func (r *Repository) ListAgentInbox(ctx context.Context, id, status string) ([]domain.AgentInbox, error) {
	var m []domain.AgentInbox
	q := r.dbWithContext(ctx).Where("target_id = ?", id)
	if status != "" {
		q = q.Where("status = ?", status)
	}
	err := q.Order("seq").Limit(128).Find(&m).Error
	return m, err
}
func (r *Repository) ClaimAgentRelays(ctx context.Context, id, request string, steerOnly bool) ([]domain.AgentInbox, error) {
	var messages []domain.AgentInbox
	err := r.dbWithContext(ctx).Transaction(func(tx *gorm.DB) error {
		q := tx.Where("target_id = ? AND request_id = ? AND status = ?", id, request, "pending")
		if steerOnly {
			q = q.Where("delivery = ?", "steer")
		}
		if err := q.Order("seq").Find(&messages).Error; err != nil {
			return err
		}
		for i := range messages {
			result := tx.Model(&domain.AgentInbox{}).Where("id = ? AND status = ?", messages[i].ID, "pending").Update("status", "claimed")
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return domain.ErrAgentConflict
			}
			messages[i].Status = "claimed"
		}
		return nil
	})
	return messages, err
}
func updateAgentTurn(tx *gorm.DB, t *domain.AgentTurn) error {
	result := tx.Model(&domain.AgentTurn{}).Where("id = ? AND revision = ? AND epoch = ? AND status IN ?", t.ID, t.Revision, t.Epoch, []string{"running", "waiting", "stopping"}).Updates(map[string]any{"status": t.Status, "stop_reason": t.StopReason, "answer": t.Answer, "thinking": t.Thinking, "error": t.Error, "input_tokens": t.InputTokens, "output_tokens": t.OutputTokens, "activity": t.Activity, "finished_at": t.FinishedAt, "revision": t.Revision + 1})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return domain.ErrAgentConflict
	}
	t.Revision++
	return nil
}
func (r *Repository) UpdateAgentTurn(ctx context.Context, t *domain.AgentTurn) error {
	return updateAgentTurn(r.dbWithContext(ctx), t)
}
func (r *Repository) CompleteAgentTurn(ctx context.Context, t *domain.AgentTurn, notice *domain.AgentInbox) error {
	original := t.Revision
	err := r.dbWithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := updateAgentTurn(tx, t); err != nil {
			return err
		}
		if err := tx.Model(&domain.AgentInbox{}).Where("id = ?", t.MessageID).Update("status", "completed").Error; err != nil {
			return err
		}
		if notice != nil {
			var root domain.AgentRootRequest
			if err := tx.Where("id = ?", notice.RequestID).Take(&root).Error; err != nil {
				return err
			}
			if root.Status == "running" || root.Status == "waiting" {
				if err := acceptAgentInbox(tx, notice); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Revision = original
	}
	return err
}
func (r *Repository) ListAgentRoots(ctx context.Context, session string) ([]domain.AgentRootRequest, error) {
	var roots []domain.AgentRootRequest
	err := r.dbWithContext(ctx).Where("session_id = ?", session).Order("created_at desc").Limit(32).Find(&roots).Error
	return roots, err
}
func (r *Repository) ListAgentTurns(ctx context.Context, id string) ([]domain.AgentTurn, error) {
	var t []domain.AgentTurn
	err := r.dbWithContext(ctx).Where("session_id = ?", id).Order("created_at desc").Limit(100).Find(&t).Error
	return t, err
}
func (r *Repository) GetAgentTurn(ctx context.Context, agentID, turnID string) (*domain.AgentTurn, error) {
	var turn domain.AgentTurn
	err := r.dbWithContext(ctx).Where("session_id = ? AND id = ?", agentID, turnID).Take(&turn).Error
	return &turn, err
}
func (r *Repository) PutAgentRoot(ctx context.Context, root *domain.AgentRootRequest) error {
	return r.dbWithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(root).Error
}
func (r *Repository) GetAgentRoot(ctx context.Context, id string) (*domain.AgentRootRequest, error) {
	var root domain.AgentRootRequest
	err := r.dbWithContext(ctx).Where("id = ?", id).Take(&root).Error
	return &root, err
}
func (r *Repository) UpdateAgentRoot(ctx context.Context, id, status string) error {
	return r.dbWithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&domain.AgentRootRequest{}).Where("id = ?", id).Update("status", status).Error; err != nil {
			return err
		}
		if status == "canceled" || status == "completed" || status == "interrupted" || status == "failed" {
			if err := tx.Model(&domain.AgentApproval{}).Where("request_id = ? AND status = ?", id, "pending").Update("status", "expired").Error; err != nil {
				return err
			}
			return tx.Model(&domain.AgentInbox{}).Where("request_id = ? AND (status = ? OR (status = ? AND turn_id = ?))", id, "pending", "claimed", "").Update("status", "discarded").Error
		}
		return nil
	})
}
func (r *Repository) ReserveAgentBudget(ctx context.Context, id string, n int) error {
	if n < 0 {
		return domain.ErrAgentBudget
	}
	result := r.dbWithContext(ctx).Model(&domain.AgentRootRequest{}).Where("id = ? AND status IN ? AND consumed + reserved + ? <= budget", id, []string{"running", "waiting"}, n).Update("reserved", gorm.Expr("reserved + ?", n))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return domain.ErrAgentBudget
	}
	return nil
}
func (r *Repository) SettleAgentBudget(ctx context.Context, id string, reserved, used int) error {
	if reserved < 0 || used < 0 {
		return domain.ErrAgentBudget
	}
	result := r.dbWithContext(ctx).Model(&domain.AgentRootRequest{}).Where("id = ? AND reserved >= ?", id, reserved).Updates(map[string]any{"reserved": gorm.Expr("reserved - ?", reserved), "consumed": gorm.Expr("consumed + ?", used)})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return domain.ErrAgentConflict
	}
	return nil
}
func (r *Repository) AppendAgentEvent(ctx context.Context, e *domain.AgentEvent) error {
	return r.dbWithContext(ctx).Create(e).Error
}
func (r *Repository) ListAgentEvents(ctx context.Context, root string, after int64) ([]domain.AgentEvent, error) {
	var e []domain.AgentEvent
	err := r.dbWithContext(ctx).Where("root_id = ? AND id > ?", root, after).Order("id").Limit(256).Find(&e).Error
	return e, err
}
func (r *Repository) PutAgentTask(ctx context.Context, t *domain.AgentTask, revision int64) error {
	if revision == 0 {
		t.Revision = 1
		return r.dbWithContext(ctx).Create(t).Error
	}
	result := r.dbWithContext(ctx).Model(&domain.AgentTask{}).Where("id = ? AND revision = ?", t.ID, revision).Select("title", "description", "acceptance", "owner_id", "status", "dependencies", "write_scopes", "artifact_id", "result", "revision", "updated_at").Updates(t)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return domain.ErrAgentConflict
	}
	return nil
}
func (r *Repository) ListAgentTasks(ctx context.Context, root string) ([]domain.AgentTask, error) {
	var t []domain.AgentTask
	err := r.dbWithContext(ctx).Where("root_id = ?", root).Order("created_at,id").Limit(256).Find(&t).Error
	return t, err
}
func (r *Repository) PutAgentArtifact(ctx context.Context, a *domain.AgentArtifact) error {
	return r.dbWithContext(ctx).Save(a).Error
}
func (r *Repository) ListAgentArtifacts(ctx context.Context, root string) ([]domain.AgentArtifact, error) {
	var a []domain.AgentArtifact
	err := r.dbWithContext(ctx).Where("root_id = ?", root).Order("created_at,id").Limit(256).Find(&a).Error
	return a, err
}
func (r *Repository) RecoverAgentRuntime(ctx context.Context) error {
	return r.dbWithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		if err := tx.Model(&domain.AgentTurn{}).Where("status IN ?", []string{"running", "waiting", "stopping"}).Updates(map[string]any{"status": "interrupted", "stop_reason": "interrupted", "finished_at": now, "revision": gorm.Expr("revision + 1")}).Error; err != nil {
			return err
		}
		if err := tx.Model(&domain.AgentTask{}).Where("status IN ?", []string{"in_progress", "awaiting_integration"}).Updates(map[string]any{"status": "needs_attention", "revision": gorm.Expr("revision + 1")}).Error; err != nil {
			return err
		}
		if err := tx.Model(&domain.AgentApproval{}).Where("status = ?", "pending").Update("status", "expired").Error; err != nil {
			return err
		}
		if err := tx.Model(&domain.AgentInbox{}).Where("status IN ?", []string{"pending", "claimed"}).Update("status", "interrupted").Error; err != nil {
			return err
		}
		return tx.Model(&domain.AgentRootRequest{}).Where("status IN ?", []string{"running", "waiting"}).Updates(map[string]any{"status": "interrupted", "reserved": 0}).Error
	})
}

func (r *Repository) PutAgentApproval(ctx context.Context, a *domain.AgentApproval) error {
	return r.dbWithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(a).Error
}
func (r *Repository) ListAgentApprovals(ctx context.Context, root string) ([]domain.AgentApproval, error) {
	var a []domain.AgentApproval
	err := r.dbWithContext(ctx).Where("root_id = ?", root).Order("created_at desc").Limit(128).Find(&a).Error
	return a, err
}
func (r *Repository) ResolveAgentApproval(ctx context.Context, id, status string) error {
	if status != "approved" && status != "rejected" && status != "expired" {
		return errors.New("invalid approval status")
	}
	result := r.dbWithContext(ctx).Model(&domain.AgentApproval{}).Where("id = ? AND status = ? AND request_id IN (?)", id, "pending", r.dbWithContext(ctx).Model(&domain.AgentRootRequest{}).Select("id").Where("status IN ?", []string{"running", "waiting"})).Update("status", status)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return domain.ErrAgentConflict
	}
	return nil
}

func (r *Repository) PauseAgentInbox(ctx context.Context, ids []string) error {
	return r.dbWithContext(ctx).Model(&domain.AgentInbox{}).Where("target_id IN ? AND status = ?", ids, "pending").Update("status", "parked").Error
}

func (r *Repository) RecordAgentActivity(ctx context.Context, turn, activity string) error {
	return r.dbWithContext(ctx).Model(&domain.AgentTurn{}).Where("id = ? AND status IN ?", turn, []string{"running", "waiting"}).Update("activity", activity).Error
}

func (r *Repository) GetAgentApproval(ctx context.Context, id string) (*domain.AgentApproval, error) {
	var a domain.AgentApproval
	err := r.dbWithContext(ctx).Where("id = ?", id).Take(&a).Error
	return &a, err
}

// RemovePendingAgentTask rolls back only a newly created, unclaimed spawn task.
func (r *Repository) RemovePendingAgentTask(ctx context.Context, id, root string) error {
	return r.dbWithContext(ctx).Where("id = ? AND root_id = ? AND status = ? AND owner_id = ? AND revision = ?", id, root, "pending", "", 1).Delete(&domain.AgentTask{}).Error
}
