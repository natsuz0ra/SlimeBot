package settings

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"slimebot/internal/apperrors"
	"slimebot/internal/constants"

	"github.com/google/uuid"
)

const promptTemplatesKey = "prompt_templates"

type PromptTemplate struct {
	ID          string `json:"id"`
	Slash       string `json:"slash"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Content     string `json:"content"`
}

var promptSlashPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)
var reservedPromptSlashes = map[string]bool{
	"terminal": true, "workspace": true, "compact": true, "checkpoints": true,
	"export": true, "plan": true, "clear-privacy": true, "help": true,
}

func defaultPromptTemplates(language string) []PromptTemplate {
	if language == "en-US" {
		return []PromptTemplate{
			{ID: "default-review", Slash: "review", Title: "Review changes", Content: "Review the changes in {{cursor}}. List bugs and missing tests with file references."},
			{ID: "default-explain", Slash: "explain", Title: "Explain code", Content: "Explain how {{cursor}} works and describe its inputs and outputs."},
			{ID: "default-fix", Slash: "fix", Title: "Fix an issue", Content: "Find the root cause of {{cursor}}, implement a fix, and verify it with targeted tests."},
			{ID: "default-test", Slash: "test", Title: "Add tests", Content: "Add focused tests for {{cursor}}, covering typical inputs and key edge cases."},
		}
	}
	return []PromptTemplate{
		{ID: "default-review", Slash: "review", Title: "审查变更", Content: "请审查 {{cursor}} 的改动，列出缺陷和缺失的测试，并标注文件位置。"},
		{ID: "default-explain", Slash: "explain", Title: "解释代码", Content: "请解释 {{cursor}} 的实现、输入和输出。"},
		{ID: "default-fix", Slash: "fix", Title: "修复问题", Content: "请定位 {{cursor}} 的根因，修复后运行相关测试。"},
		{ID: "default-test", Slash: "test", Title: "补充测试", Content: "请为 {{cursor}} 补充测试，覆盖典型输入和关键边界。"},
	}
}

func (s *SettingsService) PromptTemplates(ctx context.Context) ([]PromptTemplate, error) {
	raw, err := s.store.GetSetting(ctx, promptTemplatesKey)
	if err != nil {
		return nil, err
	}
	if raw == "" {
		language, err := s.store.GetSetting(ctx, constants.SettingLanguage)
		if err != nil {
			return nil, err
		}
		return defaultPromptTemplates(language), nil
	}
	items := make([]PromptTemplate, 0)
	err = json.Unmarshal([]byte(raw), &items)
	return items, err
}

func (s *SettingsService) UpdatePromptTemplates(ctx context.Context, items []PromptTemplate) ([]PromptTemplate, error) {
	if items == nil || len(items) > 100 {
		return nil, fmt.Errorf("%w: expected an array of up to 100 templates", apperrors.ErrInvalidInput)
	}
	slashes := make(map[string]bool, len(items))
	ids := make(map[string]bool, len(items))
	for i := range items {
		item := &items[i]
		item.ID = strings.TrimSpace(item.ID)
		item.Slash = strings.TrimSpace(item.Slash)
		item.Title = strings.TrimSpace(item.Title)
		item.Description = strings.TrimSpace(item.Description)
		if item.ID == "" {
			item.ID = uuid.NewString()
		}
		if !promptSlashPattern.MatchString(item.Slash) || reservedPromptSlashes[item.Slash] || slashes[item.Slash] {
			return nil, fmt.Errorf("%w: invalid, duplicate, or reserved slash command at row %d", apperrors.ErrInvalidInput, i+1)
		}
		if len(item.ID) > 128 || ids[item.ID] || len(item.Title) == 0 || len(item.Title) > 128 ||
			len(item.Description) > 512 || len(item.Content) > 16<<10 || strings.TrimSpace(item.Content) == "" ||
			strings.ContainsRune(item.Content, 0) || strings.Count(item.Content, "{{cursor}}") > 1 {
			return nil, fmt.Errorf("%w: invalid template fields at row %d", apperrors.ErrInvalidInput, i+1)
		}
		slashes[item.Slash], ids[item.ID] = true, true
	}
	raw, err := json.Marshal(items)
	if err != nil {
		return nil, err
	}
	if err := s.store.SetSetting(ctx, promptTemplatesKey, string(raw)); err != nil {
		return nil, err
	}
	return items, nil
}
