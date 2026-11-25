package skill

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"slimebot/internal/constants"
	"slimebot/internal/domain"
)

// SkillRuntimeService manages skill directory injection, activation, and runtime deletion.
type SkillRuntimeService struct {
	store         domain.SkillStore
	skillsRootAbs string
	statePath     string
	sourcesMu     sync.RWMutex
	sources       []SkillSource
	catalogMu     sync.RWMutex
	cachedPrompt  string
	cachedSkills  []domain.Skill
	cacheUntil    time.Time
}

const catalogCacheTTL = 30 * time.Second

// NewSkillRuntimeService creates a skill runtime service.
func NewSkillRuntimeService(store domain.SkillStore, skillsRoot string) *SkillRuntimeService {
	return NewSkillRuntimeServiceWithOptions(store, skillsRoot, SkillRuntimeOptions{})
}

// NewSkillRuntimeServiceWithOptions creates a skill runtime service with external discovery sources.
func NewSkillRuntimeServiceWithOptions(store domain.SkillStore, skillsRoot string, opts SkillRuntimeOptions) *SkillRuntimeService {
	absRoot, _ := filepath.Abs(strings.TrimSpace(skillsRoot))
	stateDir := strings.TrimSpace(opts.StateDir)
	if stateDir == "" {
		stateDir = absRoot
	}
	stateDirAbs, _ := filepath.Abs(stateDir)
	return &SkillRuntimeService{
		store:         store,
		skillsRootAbs: absRoot,
		statePath:     filepath.Join(stateDirAbs, externalSkillStateFilename),
		sources:       cloneSources(opts.Sources),
	}
}

// SetSources replaces external skill discovery sources and clears the catalog cache.
func (s *SkillRuntimeService) SetSources(sources []SkillSource) {
	s.sourcesMu.Lock()
	s.sources = cloneSources(sources)
	s.sourcesMu.Unlock()
	s.clearCatalogCache()
}

// AddSources appends external skill discovery sources and clears the catalog cache.
func (s *SkillRuntimeService) AddSources(sources []SkillSource) {
	s.sourcesMu.Lock()
	s.sources = cloneSources(append(s.sources, sources...))
	s.sourcesMu.Unlock()
	s.clearCatalogCache()
}

// ListSkills returns installed and discovered skills, including disabled external skills for UI management.
func (s *SkillRuntimeService) ListSkills() ([]domain.Skill, error) {
	items, err := s.listAllSkills()
	if err != nil {
		return nil, err
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Name < items[j].Name
	})
	return items, nil
}

func (s *SkillRuntimeService) listAllSkills() ([]domain.Skill, error) {
	localItems, err := s.store.ListSkills()
	if err != nil {
		return nil, err
	}
	items := make([]domain.Skill, 0, len(localItems))
	for _, item := range localItems {
		items = append(items, markLocalSkill(item))
	}

	state, err := loadSkillState(s.statePath)
	if err != nil {
		return nil, err
	}
	s.sourcesMu.RLock()
	sources := cloneSources(s.sources)
	s.sourcesMu.RUnlock()
	for _, source := range sources {
		discovered, err := discoverSourceSkills(source, state)
		if err != nil {
			return nil, err
		}
		items = append(items, discovered...)
	}
	return items, nil
}

func (s *SkillRuntimeService) enabledSkills() ([]domain.Skill, error) {
	items, err := s.ListSkills()
	if err != nil {
		return nil, err
	}
	enabled := items[:0]
	for _, item := range items {
		if item.Enabled {
			enabled = append(enabled, item)
		}
	}
	return enabled, nil
}

// BuildCatalogPrompt builds the skill catalog text for model context.
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

	items, err := s.enabledSkills()
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
		b.WriteString("    <name>" + escapeXML(item.ID) + "</name>\n")
		b.WriteString("    <display_name>" + escapeXML(item.Name) + "</display_name>\n")
		b.WriteString("    <description>" + escapeXML(item.Description) + "</description>\n")
		b.WriteString("    <source>" + escapeXML(item.SourceLabel) + "</source>\n")
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

// ToolCacheKey returns a content-based key for skill tool definition caching.
func (s *SkillRuntimeService) ToolCacheKey() string {
	items, err := s.enabledSkills()
	if err != nil {
		return "skills:error"
	}
	parts := make([]string, 0, len(items))
	for _, item := range items {
		hash := "unreadable"
		if dir, resolveErr := s.resolveSkillDir(item); resolveErr == nil {
			if raw, readErr := os.ReadFile(filepath.Join(dir, "SKILL.md")); readErr == nil {
				sum := sha256.Sum256(raw)
				hash = hex.EncodeToString(sum[:])
			}
		}
		parts = append(parts, item.ID+":"+hash)
	}
	sort.Strings(parts)
	return strings.Join(parts, "|")
}

// ActivateSkill loads SKILL.md by name and marks the skill active for this session.
func (s *SkillRuntimeService) ActivateSkill(name string, activated map[string]struct{}) (string, bool, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", false, fmt.Errorf("skill name cannot be empty")
	}
	if _, ok := activated[name]; ok {
		return fmt.Sprintf("<skill_content name=\"%s\">\nThis skill is already activated in the current session.\n</skill_content>", escapeXML(name)), true, nil
	}

	item, err := s.findEnabledSkill(name)
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
	contentHash := sha256.Sum256(raw)
	b.WriteString("<skill_content name=\"" + escapeXML(item.ID) + "\" display_name=\"" + escapeXML(item.Name) + "\" content_hash=\"" + hex.EncodeToString(contentHash[:]) + "\">\n")
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

	activated[item.ID] = struct{}{}
	return b.String(), false, nil
}

// DeleteSkillByID removes the skill directory and clears runtime cache.
func (s *SkillRuntimeService) DeleteSkillByID(id string) error {
	item, err := s.findSkill(id)
	if err != nil {
		return err
	}
	if item != nil && item.ReadOnly {
		return fmt.Errorf("skill %s is read-only and cannot be deleted from SlimeBot", id)
	}
	if err := s.store.DeleteSkill(id); err != nil {
		return err
	}
	s.clearCatalogCache()
	return nil
}

// resolveSkillDir resolves the skill directory from stored paths and checks traversal safety.
func (s *SkillRuntimeService) resolveSkillDir(item domain.Skill) (string, error) {
	if item.ReadOnly {
		dir := strings.TrimSpace(item.AbsolutePath)
		if dir == "" {
			dir = filepath.FromSlash(strings.TrimSpace(item.RelativePath))
		}
		if dir == "" {
			return "", fmt.Errorf("invalid skill path")
		}
		info, err := os.Stat(dir)
		if err != nil {
			return "", err
		}
		if !info.IsDir() {
			return "", fmt.Errorf("skill path is not a directory")
		}
		return dir, nil
	}
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

// SetSkillEnabled updates the SlimeBot enabled flag for an external read-only skill.
func (s *SkillRuntimeService) SetSkillEnabled(id string, enabled bool) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("skill id cannot be empty")
	}
	item, err := s.findSkill(id)
	if err != nil {
		return err
	}
	if item == nil {
		return fmt.Errorf("skill not found: %s", id)
	}
	if !item.ReadOnly {
		return fmt.Errorf("local SlimeBot skills cannot be disabled")
	}
	state, err := loadSkillState(s.statePath)
	if err != nil {
		return err
	}
	state[item.ID] = enabled
	if err := saveSkillState(s.statePath, state); err != nil {
		return err
	}
	s.clearCatalogCache()
	return nil
}

func (s *SkillRuntimeService) findEnabledSkill(nameOrID string) (*domain.Skill, error) {
	items, err := s.enabledSkills()
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		if item.ID == nameOrID || (!item.ReadOnly && item.Name == nameOrID) {
			copy := item
			return &copy, nil
		}
	}
	return nil, nil
}

func (s *SkillRuntimeService) findSkill(id string) (*domain.Skill, error) {
	items, err := s.ListSkills()
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		if item.ID == id || (!item.ReadOnly && item.Name == id) {
			copy := item
			return &copy, nil
		}
	}
	return nil, nil
}

func (s *SkillRuntimeService) clearCatalogCache() {
	s.catalogMu.Lock()
	s.cachedPrompt = ""
	s.cachedSkills = nil
	s.cacheUntil = time.Time{}
	s.catalogMu.Unlock()
}

// InvalidateCatalog is the shared invalidation entry for installs and learning writes.
func (s *SkillRuntimeService) InvalidateCatalog() { s.clearCatalogCache() }

// stripFrontmatter parses and strips YAML frontmatter from SKILL.md.
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

// escapeXML escapes skill content for safe embedding in XML-shaped prompts.
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
