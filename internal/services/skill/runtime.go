package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"slimebot/internal/constants"
	"slimebot/internal/domain"
	llmsvc "slimebot/internal/services/llm"
)

// SkillRuntimeService 负责技能目录注入、激活与运行期删除。
type SkillRuntimeService struct {
	store         domain.SkillStore
	skillsRootAbs string
	catalogMu     sync.RWMutex
	cachedPrompt  string
	cachedSkills  []domain.Skill
	cacheUntil    time.Time
}

const catalogCacheTTL = 30 * time.Second

// NewSkillRuntimeService 创建技能运行时服务。
func NewSkillRuntimeService(store domain.SkillStore, skillsRoot string) *SkillRuntimeService {
	absRoot, _ := filepath.Abs(strings.TrimSpace(skillsRoot))
	return &SkillRuntimeService{
		store:         store,
		skillsRootAbs: absRoot,
	}
}

// ListSkills 返回当前已安装技能列表。
func (s *SkillRuntimeService) ListSkills() ([]domain.Skill, error) {
	items, err := s.store.ListSkills()
	if err != nil {
		return nil, err
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Name < items[j].Name
	})
	return items, nil
}

// BuildCatalogPrompt 生成可注入模型上下文的技能目录描述。
func (s *SkillRuntimeService) BuildCatalogPrompt() (string, []domain.Skill, error) {
	s.catalogMu.RLock()
	if time.Now().Before(s.cacheUntil) {
		prompt := s.cachedPrompt
		items := make([]domain.Skill, len(s.cachedSkills))
		copy(items, s.cachedSkills)
		s.catalogMu.RUnlock()
		return prompt, items, nil
	}
	s.catalogMu.RUnlock()

	items, err := s.ListSkills()
	if err != nil {
		return "", nil, err
	}
	if len(items) == 0 {
		s.catalogMu.Lock()
		s.cachedPrompt = ""
		s.cachedSkills = nil
		s.cacheUntil = time.Now().Add(catalogCacheTTL)
		s.catalogMu.Unlock()
		return "", items, nil
	}

	var b strings.Builder
	b.WriteString("## available_skills\n")
	b.WriteString("The following skills provide specialized capabilities. When a task matches the description, call `activate_skill` by name to load full instructions before execution.\n")
	b.WriteString("If a skill references relative paths (for example, scripts/ or references/), they are relative to the skill directory.\n\n")
	b.WriteString("<available_skills>\n")
	for _, item := range items {
		b.WriteString("  <skill>\n")
		b.WriteString("    <name>" + escapeXML(item.Name) + "</name>\n")
		b.WriteString("    <description>" + escapeXML(item.Description) + "</description>\n")
		b.WriteString("    <location>" + escapeXML(item.RelativePath) + "/SKILL.md</location>\n")
		b.WriteString("  </skill>\n")
	}
	b.WriteString("</available_skills>\n")
	prompt := b.String()

	s.catalogMu.Lock()
	s.cachedPrompt = prompt
	s.cachedSkills = make([]domain.Skill, len(items))
	copy(s.cachedSkills, items)
	s.cacheUntil = time.Now().Add(catalogCacheTTL)
	s.catalogMu.Unlock()

	return prompt, items, nil
}

// BuildActivateSkillToolDef 构造 activate_skill 工具定义，供模型触发技能加载。
func (s *SkillRuntimeService) BuildActivateSkillToolDef(skills []domain.Skill) *llmsvc.ToolDef {
	if len(skills) == 0 {
		return nil
	}
	enumValues := make([]any, 0, len(skills))
	for _, item := range skills {
		enumValues = append(enumValues, item.Name)
	}

	return &llmsvc.ToolDef{
		Name:        "activate_skill",
		Description: "Load a skill guide by name. Call only when the task matches the skill description.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": "Skill name to activate.",
					"enum":        enumValues,
				},
			},
			"required": []string{"name"},
		},
	}
}

// ActivateSkill 按名称加载 SKILL.md 内容并标记本轮会话已激活。
func (s *SkillRuntimeService) ActivateSkill(name string, activated map[string]struct{}) (string, bool, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", false, fmt.Errorf("skill name cannot be empty")
	}
	if _, ok := activated[name]; ok {
		return fmt.Sprintf("<skill_content name=\"%s\">\nThis skill is already activated in the current session.\n</skill_content>", escapeXML(name)), true, nil
	}

	item, err := s.store.GetSkillByName(name)
	if err != nil {
		return "", false, err
	}
	if item == nil {
		return "", false, fmt.Errorf("skill not found: %s", name)
	}

	skillDir, err := s.resolveSkillDir(*item)
	if err != nil {
		return "", false, err
	}
	raw, err := os.ReadFile(filepath.Join(skillDir, "SKILL.md"))
	if err != nil {
		return "", false, fmt.Errorf("failed to read SKILL.md: %w", err)
	}

	body, err := stripFrontmatter(string(raw))
	if err != nil {
		return "", false, err
	}
	files, err := listSkillResourceFiles(skillDir)
	if err != nil {
		return "", false, fmt.Errorf("failed to read skill resources: %w", err)
	}

	var b strings.Builder
	b.WriteString("<skill_content name=\"" + escapeXML(item.Name) + "\">\n")
	b.WriteString(body)
	b.WriteString("\n\nSkill directory: " + filepath.ToSlash(skillDir) + "\n")
	b.WriteString("Relative paths in this skill are relative to the skill directory.\n")
	if len(files) > 0 {
		b.WriteString("\n<skill_resources>\n")
		for _, f := range files {
			b.WriteString("  <file>" + escapeXML(f) + "</file>\n")
		}
		if len(files) >= constants.MaxSkillResourcesShown {
			b.WriteString("  <note>Resource list truncated</note>\n")
		}
		b.WriteString("</skill_resources>\n")
	}
	b.WriteString("</skill_content>")

	activated[name] = struct{}{}
	return b.String(), false, nil
}

// DeleteSkillByID 删除技能目录并清空运行时缓存。
func (s *SkillRuntimeService) DeleteSkillByID(id string) error {
	if err := s.store.DeleteSkill(id); err != nil {
		return err
	}
	s.catalogMu.Lock()
	s.cachedPrompt = ""
	s.cachedSkills = nil
	s.cacheUntil = time.Time{}
	s.catalogMu.Unlock()
	return nil
}

// resolveSkillDir 根据存储路径解析技能目录并校验越界风险。
func (s *SkillRuntimeService) resolveSkillDir(item domain.Skill) (string, error) {
	base := filepath.Clean(s.skillsRootAbs)
	candidate := filepath.Join(base, item.Name)
	if rel := strings.TrimSpace(item.RelativePath); rel != "" {
		rel = filepath.FromSlash(rel)
		if strings.Contains(rel, "..") {
			return "", fmt.Errorf("invalid skill path")
		}
		candidate = filepath.Join(filepath.Dir(base), rel)
	}
	if !isWithinRoot(filepath.Dir(base), candidate) {
		return "", fmt.Errorf("skill path is out of root")
	}
	return candidate, nil
}

// stripFrontmatter 解析并移除 SKILL.md 的 YAML frontmatter。
func stripFrontmatter(content string) (string, error) {
	text := strings.TrimPrefix(content, "\uFEFF")
	if !strings.HasPrefix(text, "---") {
		return "", fmt.Errorf("SKILL.md is missing frontmatter")
	}
	parts := strings.SplitN(text, "---", 3)
	if len(parts) < 3 {
		return "", fmt.Errorf("invalid SKILL.md frontmatter format")
	}
	body := strings.TrimSpace(parts[2])
	if body == "" {
		return "", fmt.Errorf("SKILL.md body is empty")
	}
	return body, nil
}

// escapeXML 对技能内容做 XML 转义，避免嵌入提示词时破坏结构。
func escapeXML(s string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		"\"", "&quot;",
		"'", "&apos;",
	)
	return replacer.Replace(s)
}
