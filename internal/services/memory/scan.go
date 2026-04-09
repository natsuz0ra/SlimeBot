package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/goccy/go-yaml"
)

const (
	frontmatterDelimiter = "---"
	frontmatterMaxLines  = 30
	maxMemoryFiles       = 200
)

// memoryFrontmatter YAML frontmatter 结构。
type memoryFrontmatter struct {
	Name        string     `yaml:"name"`
	Description string     `yaml:"description"`
	Type        MemoryType `yaml:"type"`
	SessionID   string     `yaml:"session_id"`
	Created     time.Time  `yaml:"created"`
	Updated     time.Time  `yaml:"updated"`
}

// parseMemoryFile 解析单个记忆文件。
func parseMemoryFile(filePath string) (*MemoryEntry, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read file %s: %w", filePath, err)
	}

	fm, content, err := parseFrontmatter(string(data))
	if err != nil {
		return nil, fmt.Errorf("parse frontmatter %s: %w", filePath, err)
	}

	entry := &MemoryEntry{
		Name:        fm.Name,
		Description: fm.Description,
		Type:        fm.Type,
		SessionID:   fm.SessionID,
		Created:     fm.Created,
		Updated:     fm.Updated,
		Content:     strings.TrimSpace(content),
		FilePath:    filePath,
	}
	// 从文件名恢复 slug 缓存，避免 Slug() 重新计算产生不同值
	entry.SetSlug(strings.TrimSuffix(filepath.Base(filePath), ".md"))
	return entry, nil
}

// parseFrontmatter 从 markdown 内容中提取 YAML frontmatter 和正文。
func parseFrontmatter(raw string) (*memoryFrontmatter, string, error) {
	raw = strings.TrimSpace(raw)

	if !strings.HasPrefix(raw, frontmatterDelimiter) {
		return nil, "", fmt.Errorf("missing opening frontmatter delimiter")
	}

	// 找到闭合的 ---
	afterFirst := raw[len(frontmatterDelimiter):]
	closeIdx := strings.Index(afterFirst, "\n"+frontmatterDelimiter)
	if closeIdx < 0 {
		return nil, "", fmt.Errorf("missing closing frontmatter delimiter")
	}

	fmContent := strings.TrimSpace(afterFirst[:closeIdx])
	bodyStart := closeIdx + len("\n"+frontmatterDelimiter)
	body := ""
	if bodyStart < len(afterFirst) {
		body = afterFirst[bodyStart:]
	}

	var fm memoryFrontmatter
	if err := yaml.Unmarshal([]byte(fmContent), &fm); err != nil {
		return nil, "", fmt.Errorf("unmarshal yaml: %w", err)
	}

	// 验证类型
	if _, err := ParseMemoryType(string(fm.Type)); err != nil {
		return nil, "", err
	}

	return &fm, body, nil
}

// parseFrontmatterOnly 只解析 frontmatter，不读正文（用于快速扫描）。
func parseFrontmatterOnly(raw string) (*memoryFrontmatter, error) {
	raw = strings.TrimSpace(raw)
	if !strings.HasPrefix(raw, frontmatterDelimiter) {
		return nil, fmt.Errorf("missing opening frontmatter delimiter")
	}

	afterFirst := raw[len(frontmatterDelimiter):]
	closeIdx := strings.Index(afterFirst, "\n"+frontmatterDelimiter)
	if closeIdx < 0 {
		return nil, fmt.Errorf("missing closing frontmatter delimiter")
	}

	fmContent := strings.TrimSpace(afterFirst[:closeIdx])

	var fm memoryFrontmatter
	if err := yaml.Unmarshal([]byte(fmContent), &fm); err != nil {
		return nil, fmt.Errorf("unmarshal yaml: %w", err)
	}

	return &fm, nil
}

// scanMemoryDir 扫描目录，只解析 frontmatter（不含正文）。
func scanMemoryDir(baseDir string) ([]*MemoryEntry, error) {
	return scanDir(baseDir, false)
}

// scanMemoryDirFull 扫描目录，解析完整内容。
func scanMemoryDirFull(baseDir string) ([]*MemoryEntry, error) {
	return scanDir(baseDir, true)
}

func scanDir(baseDir string, fullContent bool) ([]*MemoryEntry, error) {
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read dir %s: %w", baseDir, err)
	}

	// 收集 .md 文件（排除 MEMORY.md）
	var mdFiles []os.DirEntry
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if name == entrypointFileName {
			continue
		}
		if !strings.HasSuffix(name, ".md") {
			continue
		}
		mdFiles = append(mdFiles, e)
	}

	// 按修改时间排序（最新在前）
	sort.Slice(mdFiles, func(i, j int) bool {
		fi, _ := mdFiles[i].Info()
		fj, _ := mdFiles[j].Info()
		return fi.ModTime().After(fj.ModTime())
	})

	// 限制数量
	if len(mdFiles) > maxMemoryFiles {
		mdFiles = mdFiles[:maxMemoryFiles]
	}

	var results []*MemoryEntry
	for _, f := range mdFiles {
		filePath := filepath.Join(baseDir, f.Name())

		data, err := os.ReadFile(filePath)
		if err != nil {
			continue
		}

		if fullContent {
			entry, parseErr := parseMemoryFile(filePath)
			if parseErr != nil {
				continue
			}
			results = append(results, entry)
		} else {
			// 只读前几行获取 frontmatter
			content := string(data)
			fm, parseErr := parseFrontmatterOnly(content)
			if parseErr != nil {
				continue
			}
			entry := &MemoryEntry{
				Name:        fm.Name,
				Description: fm.Description,
				Type:        fm.Type,
				SessionID:   fm.SessionID,
				Created:     fm.Created,
				Updated:     fm.Updated,
				FilePath:    filePath,
			}
			entry.SetSlug(strings.TrimSuffix(f.Name(), ".md"))
			results = append(results, entry)
		}
	}

	return results, nil
}
