package memory

import (
	"fmt"
	"strings"
	"time"
)

// MemoryType 记忆类型分类，参考 Claude Code 的四种记忆类型。
type MemoryType string

const (
	MemoryTypeUser      MemoryType = "user"      // 用户画像、角色、目标、偏好
	MemoryTypeFeedback  MemoryType = "feedback"  // 工作方式指导（应做/不应做）
	MemoryTypeProject   MemoryType = "project"   // 项目上下文、目标、进度
	MemoryTypeReference MemoryType = "reference" // 外部系统引用指针
)

var validMemoryTypes = map[MemoryType]bool{
	MemoryTypeUser:      true,
	MemoryTypeFeedback:  true,
	MemoryTypeProject:   true,
	MemoryTypeReference: true,
}

func ParseMemoryType(s string) (MemoryType, error) {
	t := MemoryType(strings.ToLower(strings.TrimSpace(s)))
	if !validMemoryTypes[t] {
		return "", fmt.Errorf("invalid memory type: %q (valid: user, feedback, project, reference)", s)
	}
	return t, nil
}

// MemoryEntry 表示一条记忆条目，对应文件系统中的一个 .md 文件。
type MemoryEntry struct {
	Name        string     `yaml:"name"`             // 记忆名称，同时作为文件名
	Description string     `yaml:"description"`      // 一行描述，用于 MEMORY.md 索引
	Type        MemoryType `yaml:"type"`             // 记忆类型
	SessionID   string     `yaml:"session_id"`       // 所属会话 ID，空表示全局记忆
	Created     time.Time  `yaml:"created"`          // 创建时间
	Updated     time.Time  `yaml:"updated"`          // 最后更新时间
	Content     string     `yaml:"-" json:"content"` // frontmatter 之后的正文内容
	FilePath    string     `yaml:"-" json:"-"`       // 文件绝对路径（运行时填充）
	slug        string     `yaml:"-" json:"-"`       // 缓存 slug，避免 time.Now() 重复调用
}

// Slug 返回可用于文件名的安全标识符。
// 结果被缓存，确保多次调用返回相同值。
func (e *MemoryEntry) Slug() string {
	if e.slug != "" {
		return e.slug
	}
	e.slug = Slugify(e.Name)
	if e.slug == "" {
		e.slug = fmt.Sprintf("memory_%d", time.Now().UnixNano())
	}
	return e.slug
}

// SetSlug 强制设置 slug（用于从文件名恢复缓存）。
func (e *MemoryEntry) SetSlug(s string) {
	e.slug = s
}

// Slugify 将名称转为文件名安全的纯 ASCII 标识符。
func Slugify(name string) string {
	s := strings.TrimSpace(name)
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, " ", "_")
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// FileName 返回带扩展名的文件名。
func (e *MemoryEntry) FileName() string {
	return e.Slug() + ".md"
}

// BleveDocument 是 bleve 索引的文档结构。
type BleveDocument struct {
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Type        string    `json:"type"`
	SessionID   string    `json:"session_id"`
	Content     string    `json:"content"`
	Updated     time.Time `json:"updated"`
}

// ToBleveDocument 转换为 bleve 索引文档。
func (e *MemoryEntry) ToBleveDocument() BleveDocument {
	return BleveDocument{
		Name:        e.Name,
		Description: e.Description,
		Type:        string(e.Type),
		SessionID:   e.SessionID,
		Content:     e.Content,
		Updated:     e.Updated,
	}
}
