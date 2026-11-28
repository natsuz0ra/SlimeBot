package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"slimebot/internal/constants"
	"slimebot/internal/domain"
)

type skillWriter struct {
	runtime *SkillRuntimeService
	mu      sync.Mutex
	locks   map[string]*sync.Mutex
}

func newSkillWriter(runtime *SkillRuntimeService) *skillWriter {
	return &skillWriter{runtime: runtime, locks: map[string]*sync.Mutex{}}
}

func (w *skillWriter) lock(name string) *sync.Mutex {
	w.mu.Lock()
	defer w.mu.Unlock()
	lock := w.locks[name]
	if lock == nil {
		lock = &sync.Mutex{}
		w.locks[name] = lock
	}
	return lock
}

// write serializes read-before-write and generation for one local skill. The
// generator runs under the per-skill lock so it cannot overwrite a newer body.
func (w *skillWriter) write(name string, blockedTerms []string, generate func(current string) (string, error)) (*domain.Skill, error) {
	name = strings.TrimSpace(name)
	if !isValidSkillName(name) {
		return nil, fmt.Errorf("invalid skill name")
	}
	lock := w.lock(name)
	lock.Lock()
	defer lock.Unlock()

	root := filepath.Clean(w.runtime.skillsRootAbs)
	dir := filepath.Join(root, name)
	current := ""
	if info, err := os.Lstat(dir); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return nil, fmt.Errorf("skill directory is not a safe local directory")
		}
		markdownPath := filepath.Join(dir, "SKILL.md")
		fileInfo, err := os.Lstat(markdownPath)
		if err != nil {
			return nil, fmt.Errorf("read current SKILL.md: %w", err)
		}
		if fileInfo.Mode()&os.ModeSymlink != 0 || !fileInfo.Mode().IsRegular() {
			return nil, fmt.Errorf("SKILL.md is not a safe regular file")
		}
		raw, err := os.ReadFile(markdownPath)
		if err != nil {
			return nil, fmt.Errorf("read current SKILL.md: %w", err)
		}
		current = string(raw)
	} else if !os.IsNotExist(err) {
		return nil, err
	}

	markdown, err := generate(current)
	if err != nil {
		return nil, err
	}
	if err := validateLearnedMarkdown(markdown, name, current, blockedTerms); err != nil {
		return nil, err
	}
	created := current == ""
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	temporary, err := os.CreateTemp(dir, ".SKILL-*.tmp")
	if err != nil {
		return nil, err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	markdown = strings.TrimSpace(markdown) + "\n"
	if _, err = temporary.WriteString(markdown); err == nil {
		err = temporary.Sync()
	}
	if closeErr := temporary.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(temporaryPath, 0o644); err != nil {
		return nil, err
	}
	if err := os.Rename(temporaryPath, filepath.Join(dir, "SKILL.md")); err != nil {
		return nil, err
	}
	w.runtime.InvalidateCatalog()
	item, err := w.runtime.store.GetSkillByName(name)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, fmt.Errorf("written skill cannot be loaded")
	}
	if created {
		item.UploadedAt = item.UpdatedAt
	}
	return item, nil
}

var credentialPattern = regexp.MustCompile(`(?i)(api[_-]?key|secret|password|access[_-]?token)\s*[:=]\s*["']?[a-z0-9_./+\-=]{12,}`)
var bearerPattern = regexp.MustCompile(`(?i)bearer\s+[a-z0-9._\-]{16,}`)

func validateLearnedMarkdown(markdown, expectedName, current string, blockedTerms []string) error {
	trimmed := strings.TrimSpace(markdown)
	if trimmed == "" {
		return fmt.Errorf("generated SKILL.md is empty")
	}
	if len(trimmed) > constants.MaxSkillSingleFileSize {
		return fmt.Errorf("generated SKILL.md exceeds size limit")
	}
	if strings.HasPrefix(trimmed, "```") && strings.HasSuffix(trimmed, "```") {
		return fmt.Errorf("generated SKILL.md must not be wrapped in a code fence")
	}
	name, _, err := parseSkillFrontmatter(trimmed)
	if err != nil {
		return err
	}
	if name != expectedName {
		return fmt.Errorf("generated skill name %q does not match target %q", name, expectedName)
	}
	if _, err := stripFrontmatter(trimmed); err != nil {
		return err
	}
	if current != "" && strings.TrimSpace(current) == trimmed {
		return fmt.Errorf("generated skill has no substantive changes")
	}
	if strings.Contains(trimmed, "{{SB_") || credentialPattern.MatchString(trimmed) || bearerPattern.MatchString(trimmed) {
		return fmt.Errorf("generated skill contains credentials or privacy placeholders")
	}
	for _, term := range blockedTerms {
		term = strings.TrimSpace(term)
		if len(term) >= 8 && strings.Contains(trimmed, term) {
			return fmt.Errorf("generated skill contains a session-specific identifier")
		}
	}
	return nil
}
