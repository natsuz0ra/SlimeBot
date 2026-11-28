package skill

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"slimebot/internal/domain"
	sbruntime "slimebot/internal/runtime"
)

const externalSkillStateFilename = ".external-skill-state.json"

// SkillSource describes a directory owned by an external agent.
type SkillSource struct {
	Provider string
	Label    string
	Scope    string
	Root     string
	ReadOnly bool
}

// SkillRuntimeOptions configures external skill discovery.
type SkillRuntimeOptions struct {
	StateDir string
	Sources  []SkillSource
}

type skillEnabledState struct {
	Enabled map[string]bool `json:"enabled"`
}

// DefaultGlobalSkillSources returns the built-in global agent skill directories.
func DefaultGlobalSkillSources(extraHermesRoots []string) []SkillSource {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return nil
	}
	sources := []SkillSource{
		{Provider: "codex", Label: "Codex", Scope: "global", Root: filepath.Join(home, ".codex", "skills"), ReadOnly: true},
		{Provider: "claude", Label: "Claude Code", Scope: "global", Root: filepath.Join(home, ".claude", "skills"), ReadOnly: true},
		{Provider: "hermes", Label: "Hermes Agent", Scope: "global", Root: filepath.Join(home, ".hermes", "skills"), ReadOnly: true},
	}
	for _, root := range extraHermesRoots {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		sources = append(sources, SkillSource{
			Provider: "hermes",
			Label:    "Hermes Agent",
			Scope:    "global",
			Root:     sbruntime.ExpandHome(root),
			ReadOnly: true,
		})
	}
	return sources
}

// ProjectSkillSources returns project-relative skill directories for CLI mode.
func ProjectSkillSources(workingDir string) []SkillSource {
	workingDir = strings.TrimSpace(workingDir)
	if workingDir == "" {
		return nil
	}
	return []SkillSource{
		{Provider: "codex", Label: "Codex", Scope: "project", Root: filepath.Join(workingDir, ".codex", "skills"), ReadOnly: true},
		{Provider: "claude", Label: "Claude Code", Scope: "project", Root: filepath.Join(workingDir, ".claude", "skills"), ReadOnly: true},
		{Provider: "hermes", Label: "Hermes Agent", Scope: "project", Root: filepath.Join(workingDir, ".hermes", "skills"), ReadOnly: true},
		{Provider: "agent", Label: "Agent Skills", Scope: "project", Root: filepath.Join(workingDir, ".agents", "skills"), ReadOnly: true},
	}
}

func (s SkillSource) normalized() SkillSource {
	s.Provider = normalizeIDPart(s.Provider)
	s.Scope = normalizeIDPart(s.Scope)
	s.Label = strings.TrimSpace(s.Label)
	s.Root = sbruntime.ExpandHome(strings.TrimSpace(s.Root))
	if s.Label == "" {
		s.Label = s.Provider
	}
	if s.Scope == "" {
		s.Scope = "global"
	}
	return s
}

func discoverSourceSkills(source SkillSource, state map[string]bool) ([]domain.Skill, error) {
	source = source.normalized()
	if source.Provider == "" || source.Root == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(source.Root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	items := make([]domain.Skill, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(source.Root, entry.Name())
		raw, err := os.ReadFile(filepath.Join(dir, "SKILL.md"))
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		name, description, err := parseSkillFrontmatter(string(raw))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", filepath.Join(dir, "SKILL.md"), err)
		}
		id := externalSkillID(source.Provider, source.Scope, name)
		enabled, ok := state[id]
		if !ok {
			enabled = true
		}
		info, err := os.Stat(filepath.Join(dir, "SKILL.md"))
		if err != nil {
			return nil, err
		}
		items = append(items, domain.Skill{
			ID:           id,
			Name:         name,
			RelativePath: filepath.ToSlash(dir),
			Description:  description,
			Source:       source.Scope,
			SourceLabel:  source.Label,
			Provider:     source.Provider,
			ReadOnly:     source.ReadOnly,
			Enabled:      enabled,
			AbsolutePath: dir,
			UploadedAt:   info.ModTime(),
			CreatedAt:    info.ModTime(),
			UpdatedAt:    info.ModTime(),
		})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].ID < items[j].ID
	})
	return items, nil
}

func externalSkillID(provider, scope, name string) string {
	return normalizeIDPart(provider) + ":" + normalizeIDPart(scope) + ":" + name
}

func normalizeIDPart(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastHyphen := false
	for _, r := range value {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if ok {
			b.WriteRune(r)
			lastHyphen = false
			continue
		}
		if !lastHyphen {
			b.WriteByte('-')
			lastHyphen = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func loadSkillState(path string) (map[string]bool, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]bool{}, nil
		}
		return nil, err
	}
	var state skillEnabledState
	if err := json.Unmarshal(raw, &state); err != nil {
		return nil, err
	}
	if state.Enabled == nil {
		state.Enabled = map[string]bool{}
	}
	return state.Enabled, nil
}

func saveSkillState(path string, enabled map[string]bool) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(skillEnabledState{Enabled: enabled}, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return os.WriteFile(path, raw, 0o644)
}

func markLocalSkill(item domain.Skill) domain.Skill {
	item.Source = "local"
	item.SourceLabel = "SlimeBot"
	item.Provider = "slimebot"
	item.ReadOnly = false
	item.Enabled = true
	return item
}

func cloneSources(sources []SkillSource) []SkillSource {
	out := make([]SkillSource, 0, len(sources))
	seen := make(map[string]struct{}, len(sources))
	for _, source := range sources {
		source = source.normalized()
		if source.Provider == "" || source.Root == "" {
			continue
		}
		key := source.Provider + "\x00" + source.Scope + "\x00" + source.Root
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, source)
	}
	return out
}
