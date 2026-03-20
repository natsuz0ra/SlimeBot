package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"slimebot/backend/internal/constants"
	"slimebot/backend/internal/domain"
	"slimebot/backend/internal/services/openai"
)

type SkillRuntimeService struct {
	store         domain.SkillStore
	skillsRootAbs string
}

func NewSkillRuntimeService(store domain.SkillStore, skillsRoot string) *SkillRuntimeService {
	absRoot, _ := filepath.Abs(strings.TrimSpace(skillsRoot))
	return &SkillRuntimeService{
		store:         store,
		skillsRootAbs: absRoot,
	}
}

func (s *SkillRuntimeService) ListSkills() ([]domain.Skill, error) {
	return s.store.ListSkills()
}

func (s *SkillRuntimeService) BuildCatalogPrompt() (string, []domain.Skill, error) {
	items, err := s.store.ListSkills()
	if err != nil {
		return "", nil, err
	}
	if len(items) == 0 {
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
	return b.String(), items, nil
}

func (s *SkillRuntimeService) BuildActivateSkillToolDef(skills []domain.Skill) *openai.ToolDef {
	if len(skills) == 0 {
		return nil
	}
	enumValues := make([]any, 0, len(skills))
	for _, item := range skills {
		enumValues = append(enumValues, item.Name)
	}

	return &openai.ToolDef{
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

func (s *SkillRuntimeService) DeleteSkillByID(id string) error {
	item, err := s.store.GetSkillByID(id)
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
			return fmt.Errorf("failed to delete skill directory: %w", err)
		}
	}
	if err := s.store.DeleteSkill(id); err != nil {
		return err
	}
	return nil
}

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
