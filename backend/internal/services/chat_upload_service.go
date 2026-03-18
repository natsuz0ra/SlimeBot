package services

import (
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"slimebot/backend/internal/models"

	"github.com/google/uuid"
)

const (
	maxChatUploadFiles = 5
	maxChatUploadBytes = 10 * 1024 * 1024
)

type UploadedAttachment struct {
	ID        string
	SessionID string
	Name      string
	Ext       string
	SizeBytes int64
	MimeType  string
	Category  string
	IconType  string
	Path      string
}

// ToMessageAttachment 将运行时上传对象转换为可持久化的附件元信息。
func (a UploadedAttachment) ToMessageAttachment() models.MessageAttachment {
	return models.MessageAttachment{
		ID:        a.ID,
		Name:      a.Name,
		Ext:       a.Ext,
		SizeBytes: a.SizeBytes,
		MimeType:  a.MimeType,
		Category:  a.Category,
		IconType:  a.IconType,
	}
}

// ChatUploadService 管理聊天附件的临时生命周期：
// 1) SaveFiles 暂存并注册；
// 2) Consume 按会话消费；
// 3) Cleanup 在回合结束后删除临时文件。
type ChatUploadService struct {
	root string

	mu    sync.Mutex
	items map[string]UploadedAttachment
}

// NewChatUploadService 创建聊天附件临时存储服务。
func NewChatUploadService(root string) *ChatUploadService {
	return &ChatUploadService{
		root:  root,
		items: make(map[string]UploadedAttachment),
	}
}

// normalizeAttachmentName 归一化文件名，避免空名或路径注入。
func normalizeAttachmentName(name string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "unnamed"
	}
	base := filepath.Base(trimmed)
	if base == "." || base == string(filepath.Separator) || base == "" {
		return "unnamed"
	}
	return base
}

// attachmentIconType 根据扩展名/mime 推断前端展示图标类型。
func attachmentIconType(ext, mimeType string) string {
	e := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(ext), "."))
	m := strings.ToLower(strings.TrimSpace(mimeType))
	switch {
	case strings.HasPrefix(m, "image/"):
		return "image"
	case strings.HasPrefix(m, "audio/") || e == "mp3" || e == "wav" || e == "m4a" || e == "aac" || e == "ogg" || e == "flac":
		return "audio"
	case m == "application/pdf" || e == "pdf":
		return "pdf"
	case strings.Contains(m, "word") || e == "doc" || e == "docx":
		return "word"
	case strings.Contains(m, "sheet") || e == "xls" || e == "xlsx" || e == "csv":
		return "excel"
	case strings.Contains(m, "zip") || strings.Contains(m, "tar") || strings.Contains(m, "rar") || e == "7z":
		return "archive"
	case strings.HasPrefix(m, "text/") || e == "txt" || e == "md" || e == "json" || e == "yaml" || e == "yml":
		return "text"
	case e == "go" || e == "ts" || e == "tsx" || e == "js" || e == "jsx" || e == "py" || e == "java" || e == "sql" || e == "rs":
		return "code"
	default:
		return "file"
	}
}

// SaveFiles 校验并保存上传文件到临时目录，并返回可消费的附件引用。
// 目录按 session + 日期 + requestID 隔离，减少并发回合互相影响。
func (s *ChatUploadService) SaveFiles(sessionID string, files []*multipart.FileHeader) ([]UploadedAttachment, error) {
	if s == nil {
		return nil, fmt.Errorf("chat upload service is not initialized")
	}
	if strings.TrimSpace(sessionID) == "" {
		return nil, fmt.Errorf("session id is required")
	}
	if len(files) == 0 {
		return []UploadedAttachment{}, nil
	}
	if len(files) > maxChatUploadFiles {
		return nil, fmt.Errorf("at most %d files can be uploaded", maxChatUploadFiles)
	}

	requestDir := filepath.Join(s.root, sessionID, time.Now().UTC().Format("20060102"), uuid.NewString())
	if err := os.MkdirAll(requestDir, os.ModePerm); err != nil {
		return nil, fmt.Errorf("failed to create upload directory: %w", err)
	}

	saved := make([]UploadedAttachment, 0, len(files))
	for _, header := range files {
		if header == nil {
			continue
		}
		if header.Size <= 0 {
			return nil, fmt.Errorf("file %q is empty", header.Filename)
		}
		if header.Size > maxChatUploadBytes {
			return nil, fmt.Errorf("file %q exceeds 10MB size limit", header.Filename)
		}
		src, err := header.Open()
		if err != nil {
			return nil, fmt.Errorf("failed to open file %q: %w", header.Filename, err)
		}

		attachmentID := uuid.NewString()
		ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(header.Filename)), ".")
		dstPath := filepath.Join(requestDir, attachmentID+"_"+normalizeAttachmentName(header.Filename))
		dst, err := os.Create(dstPath)
		if err != nil {
			_ = src.Close()
			return nil, fmt.Errorf("failed to create temp file %q: %w", header.Filename, err)
		}

		written, copyErr := io.Copy(dst, src)
		closeErr := dst.Close()
		_ = src.Close()
		if copyErr != nil {
			return nil, fmt.Errorf("failed to save file %q: %w", header.Filename, copyErr)
		}
		if closeErr != nil {
			return nil, fmt.Errorf("failed to close temp file %q: %w", header.Filename, closeErr)
		}

		mimeType := detectStoredFileMime(dstPath, header.Header.Get("Content-Type"), ext)
		category := classifyAttachmentCategory(mimeType, ext)
		item := UploadedAttachment{
			ID:        attachmentID,
			SessionID: sessionID,
			Name:      normalizeAttachmentName(header.Filename),
			Ext:       strings.ToUpper(ext),
			SizeBytes: written,
			MimeType:  mimeType,
			Category:  category,
			IconType:  attachmentIconType(ext, mimeType),
			Path:      dstPath,
		}
		saved = append(saved, item)
	}

	// 统一注册到内存索引，后续由 Consume 一次性取走。
	s.mu.Lock()
	for _, item := range saved {
		s.items[item.ID] = item
	}
	s.mu.Unlock()
	return saved, nil
}

// Consume 按会话消费附件 ID，并从内存索引移除，避免重复复用。
func (s *ChatUploadService) Consume(sessionID string, ids []string) ([]UploadedAttachment, error) {
	if s == nil {
		return []UploadedAttachment{}, nil
	}
	if len(ids) == 0 {
		return []UploadedAttachment{}, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	items := make([]UploadedAttachment, 0, len(ids))
	for _, id := range ids {
		trimmed := strings.TrimSpace(id)
		if trimmed == "" {
			continue
		}
		item, ok := s.items[trimmed]
		if !ok {
			return nil, fmt.Errorf("attachment %s not found or expired", trimmed)
		}
		if item.SessionID != sessionID {
			return nil, fmt.Errorf("attachment %s does not belong to this session", trimmed)
		}
		delete(s.items, trimmed)
		items = append(items, item)
	}
	return items, nil
}

// Cleanup 删除临时文件并尝试清理空目录；该方法设计为幂等调用。
func (s *ChatUploadService) Cleanup(items []UploadedAttachment) {
	if len(items) == 0 {
		return
	}
	visitedDir := make(map[string]struct{})
	for _, item := range items {
		if strings.TrimSpace(item.Path) == "" {
			continue
		}
		_ = os.Remove(item.Path)
		dir := filepath.Dir(item.Path)
		if _, seen := visitedDir[dir]; seen {
			continue
		}
		visitedDir[dir] = struct{}{}
		_ = os.Remove(dir)
	}
}
