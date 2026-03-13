package services

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"corner/backend/internal/models"
	"corner/backend/internal/repositories"
)

type SkillRuntimeService struct {
	repo          *repositories.Repository
	skillsRootAbs string
}

func NewSkillRuntimeService(repo *repositories.Repository, skillsRoot string) *SkillRuntimeService {
	absRoot, _ := filepath.Abs(strings.TrimSpace(skillsRoot))
	return &SkillRuntimeService{
		repo:          repo,
		skillsRootAbs: absRoot,
	}
}

func (s *SkillRuntimeService) ListSkills() ([]models.Skill, error) {
	return s.repo.ListSkills()
}

func (s *SkillRuntimeService) BuildCatalogPrompt() (string, []models.Skill, error) {
	items, err := s.repo.ListSkills()
	if err != nil {
		return "", nil, err
	}
	if len(items) == 0 {
		return "", items, nil
	}

	var b strings.Builder
	b.WriteString("## available_skills\n")
	b.WriteString("以下 skills 提供专用能力。当任务与描述匹配时，请调用 `activate_skill` 工具按名称加载完整说明后再执行。\n")
	b.WriteString("若 skill 中引用相对路径（如 scripts/、references/），它们都相对于技能目录。\n\n")
	b.WriteString("<available_skills>\n")
	for _, item := range items {
		b.WriteString("  <skill>\n")
		b.WriteString("    <name>" + escapeXML(item.Name) + "</name>\n")
		b.WriteString("    <description>" + escapeXML(item.Description) + "</description>\n")
		b.WriteString("    <location>" + escapeXML(item.RelativePath) + "/SKILL.md</location>\n")
		b.WriteString("  </skill>\n")
	}
	b.WriteString("</available_skills>\n")
	return b.String(), items, nil
}

func (s *SkillRuntimeService) BuildActivateSkillToolDef(skills []models.Skill) *ToolDef {
	if len(skills) == 0 {
		return nil
	}
	enumValues := make([]any, 0, len(skills))
	for _, item := range skills {
		enumValues = append(enumValues, item.Name)
	}

	return &ToolDef{
		Name:        "activate_skill",
		Description: "按名称加载技能说明。仅在任务匹配技能描述时调用。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": "要激活的技能名称",
					"enum":        enumValues,
				},
			},
			"required": []string{"name"},
		},
	}
}

func (s *SkillRuntimeService) ActivateSkill(name string, activated map[string]struct{}) (string, bool, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", false, fmt.Errorf("skill 名称不能为空")
	}
	if _, ok := activated[name]; ok {
		return fmt.Sprintf("<skill_content name=\"%s\">\n该 skill 在当前会话已激活，无需重复加载。\n</skill_content>", escapeXML(name)), true, nil
	}

	item, err := s.repo.GetSkillByName(name)
	if err != nil {
		return "", false, err
	}
	if item == nil {
		return "", false, fmt.Errorf("skill 不存在: %s", name)
	}

	skillDir, err := s.resolveSkillDir(*item)
	if err != nil {
		return "", false, err
	}
	raw, err := os.ReadFile(filepath.Join(skillDir, "SKILL.md"))
	if err != nil {
		return "", false, fmt.Errorf("读取 SKILL.md 失败: %w", err)
	}

	body, err := stripFrontmatter(string(raw))
	if err != nil {
		return "", false, err
	}
	files, err := listSkillResourceFiles(skillDir)
	if err != nil {
		return "", false, fmt.Errorf("读取 skill 资源失败: %w", err)
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
		if len(files) >= maxSkillResourcesShown {
			b.WriteString("  <note>资源列表已截断</note>\n")
		}
		b.WriteString("</skill_resources>\n")
	}
	b.WriteString("</skill_content>")

	activated[name] = struct{}{}
	return b.String(), false, nil
}

func (s *SkillRuntimeService) DeleteSkillByID(id string) error {
	item, err := s.repo.GetSkillByID(id)
	if err != nil {
		return err
	}
	if item == nil {
		return nil
	}

	skillDir, err := s.resolveSkillDir(*item)
	if err != nil {
		return err
	}
	if _, statErr := os.Stat(skillDir); statErr == nil {
		if err := os.RemoveAll(skillDir); err != nil {
			return fmt.Errorf("删除技能目录失败: %w", err)
		}
	}
	if err := s.repo.DeleteSkill(id); err != nil {
		return err
	}
	return nil
}

func (s *SkillRuntimeService) resolveSkillDir(item models.Skill) (string, error) {
	base := filepath.Clean(s.skillsRootAbs)
	candidate := filepath.Join(base, item.Name)
	if rel := strings.TrimSpace(item.RelativePath); rel != "" {
		rel = filepath.FromSlash(rel)
		if strings.Contains(rel, "..") {
			return "", fmt.Errorf("技能路径非法")
		}
		candidate = filepath.Join(filepath.Dir(base), rel)
	}
	if !isWithinRoot(filepath.Dir(base), candidate) {
		return "", fmt.Errorf("技能路径越界")
	}
	return candidate, nil
}

func stripFrontmatter(content string) (string, error) {
	text := strings.TrimPrefix(content, "\uFEFF")
	if !strings.HasPrefix(text, "---") {
		return "", fmt.Errorf("SKILL.md 缺少 frontmatter")
	}
	parts := strings.SplitN(text, "---", 3)
	if len(parts) < 3 {
		return "", fmt.Errorf("SKILL.md frontmatter 格式错误")
	}
	body := strings.TrimSpace(parts[2])
	if body == "" {
		return "", fmt.Errorf("SKILL.md 正文为空")
	}
	return body, nil
}

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
