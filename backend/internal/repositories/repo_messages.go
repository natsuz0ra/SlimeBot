package repositories

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"slimebot/backend/internal/models"
)

type AddMessageInput struct {
	SessionID         string
	Role              string
	Content           string
	IsInterrupted     bool
	IsStopPlaceholder bool
	Attachments       []models.MessageAttachment
}

// encodeMessageAttachments 将附件元信息序列化为 JSON；失败时回退为 [] 保持可读写。
func encodeMessageAttachments(items []models.MessageAttachment) string {
	if len(items) == 0 {
		return "[]"
	}
	data, err := json.Marshal(items)
	if err != nil {
		return "[]"
	}
	return string(data)
}

// decodeMessageAttachments 从 JSON 恢复附件元信息；解析失败时回退为空数组。
func decodeMessageAttachments(raw string) []models.MessageAttachment {
	if raw == "" {
		return []models.MessageAttachment{}
	}
	var items []models.MessageAttachment
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return []models.MessageAttachment{}
	}
	return items
}

// normalizeMessages 为查询结果补齐 Attachments 运行时字段，统一返回结构。
func normalizeMessages(items []models.Message) {
	for idx := range items {
		items[idx].Attachments = decodeMessageAttachments(items[idx].AttachmentsJSON)
	}
}

func (r *Repository) ListSessionMessages(sessionID string) ([]models.Message, error) {
	var messages []models.Message
	err := r.db.Where("session_id = ?", sessionID).Order("created_at asc").Find(&messages).Error
	normalizeMessages(messages)
	return messages, err
}

func (r *Repository) ListSessionMessagesPage(sessionID string, limit int, before *time.Time, after *time.Time) ([]models.Message, bool, error) {
	if limit <= 0 {
		limit = 10
	}

	base := r.db.Where("session_id = ?", sessionID)
	var (
		messages []models.Message
		err      error
		hasMore  bool
	)

	switch {
	case after != nil:
		err = base.
			Where("created_at > ?", after.UTC()).
			Order("created_at asc, id asc").
			Limit(limit).
			Find(&messages).
			Error
		if err != nil {
			return nil, false, err
		}
		if len(messages) == 0 {
			return messages, false, nil
		}
		var count int64
		latest := messages[len(messages)-1].CreatedAt
		err = base.Where("created_at > ?", latest).Count(&count).Error
		if err != nil {
			return nil, false, err
		}
		hasMore = count > 0
	default:
		query := base
		if before != nil {
			query = query.Where("created_at < ?", before.UTC())
		}
		err = query.
			Order("created_at desc, id desc").
			Limit(limit).
			Find(&messages).
			Error
		if err != nil {
			return nil, false, err
		}
		if len(messages) == 0 {
			return messages, false, nil
		}
		for left, right := 0, len(messages)-1; left < right; left, right = left+1, right-1 {
			messages[left], messages[right] = messages[right], messages[left]
		}
		var count int64
		oldest := messages[0].CreatedAt
		err = base.Where("created_at < ?", oldest).Count(&count).Error
		if err != nil {
			return nil, false, err
		}
		hasMore = count > 0
	}

	normalizeMessages(messages)
	return messages, hasMore, nil
}

func (r *Repository) ListRecentSessionMessages(sessionID string, limit int) ([]models.Message, error) {
	if limit <= 0 {
		return []models.Message{}, nil
	}

	var messages []models.Message
	err := r.db.
		Where("session_id = ?", sessionID).
		Order("created_at desc").
		Limit(limit).
		Find(&messages).
		Error
	if err != nil {
		return nil, err
	}

	for left, right := 0, len(messages)-1; left < right; left, right = left+1, right-1 {
		messages[left], messages[right] = messages[right], messages[left]
	}
	normalizeMessages(messages)
	return messages, nil
}

func (r *Repository) AddMessage(sessionID, role, content string) (*models.Message, error) {
	return r.AddMessageWithInput(AddMessageInput{
		SessionID: sessionID,
		Role:      role,
		Content:   content,
	})
}

// AddMessageWithInput 支持扩展字段落库（中断标记/附件元信息）。
// 与会话 updated_at 更新放在同一事务中，保证消息与会话时间线一致。
func (r *Repository) AddMessageWithInput(input AddMessageInput) (*models.Message, error) {
	message := &models.Message{
		ID:                uuid.NewString(),
		SessionID:         input.SessionID,
		Role:              input.Role,
		Content:           input.Content,
		IsInterrupted:     input.IsInterrupted,
		IsStopPlaceholder: input.IsStopPlaceholder,
		AttachmentsJSON:   encodeMessageAttachments(input.Attachments),
		Attachments:       input.Attachments,
	}
	err := r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(message).Error; err != nil {
			return err
		}
		return tx.Model(&models.Session{}).
			Where("id = ?", input.SessionID).
			Update("updated_at", time.Now()).
			Error
	})
	return message, err
}
