package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"slimebot/internal/domain"
)

type FileSystemSkillStore struct {
	skillsRoot    string
	skillsRootAbs string
}

func NewFileSystemSkillStore(skillsRoot string) *FileSystemSkillStore {
	absRoot, _ := filepath.Abs(strings.TrimSpace(skillsRoot))
	return &FileSystemSkillStore{
		skillsRoot:    strings.TrimSpace(skillsRoot),
		skillsRootAbs: absRoot,
	}
}

func (s *FileSystemSkillStore) ListSkills() ([]domain.Skill, error) {
	entries, err := os.ReadDir(s.skillsRootAbs)
	if err != nil {
		if os.IsNotExist(err) {
			return []domain.Skill{}, nil
		}
		return nil, err
	}

	items := make([]domain.Skill, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		item, err := s.readSkill(entry.Name())
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		if item != nil {
			items = append(items, *item)
		}
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].UploadedAt.Equal(items[j].UploadedAt) {
			return items[i].Name < items[j].Name
		}
		return items[i].UploadedAt.After(items[j].UploadedAt)
	})
	return items, nil
}

func (s *FileSystemSkillStore) GetSkillByName(name string) (*domain.Skill, error) {
	return s.readSkill(strings.TrimSpace(name))
}

func (s *FileSystemSkillStore) GetSkillByID(id string) (*domain.Skill, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, nil
	}
	if strings.Contains(id, "/") {
		id = filepath.Base(filepath.FromSlash(id))
	}
	return s.readSkill(id)
}

func (s *FileSystemSkillStore) CreateSkill(item domain.Skill) (*domain.Skill, error) {
	if strings.TrimSpace(item.Name) == "" {
		return nil, fmt.Errorf("skill name is required")
	}
	if item.UploadedAt.IsZero() {
		item.UploadedAt = time.Now()
	}
	if strings.TrimSpace(item.ID) == "" {
		item.ID = item.Name
	}
	if strings.TrimSpace(item.RelativePath) == "" {
		item.RelativePath = s.relativePath(item.Name)
	}
	return &item, nil
}

func (s *FileSystemSkillStore) DeleteSkill(id string) error {
	item, err := s.GetSkillByID(id)
	if err != nil || item == nil {
		return err
	}
	return os.RemoveAll(filepath.Join(s.skillsRootAbs, item.Name))
}

func (s *FileSystemSkillStore) readSkill(name string) (*domain.Skill, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, nil
	}
	skillDir := filepath.Join(s.skillsRootAbs, name)
	info, err := os.Stat(skillDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if !info.IsDir() {
		return nil, nil
	}

	skillFile := filepath.Join(skillDir, "SKILL.md")
	raw, err := os.ReadFile(skillFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	metaName, description, err := parseSkillFrontmatter(string(raw))
	if err != nil {
		return nil, err
	}
	if metaName != name {
		return nil, fmt.Errorf("skill directory %s does not match SKILL.md name %s", name, metaName)
	}

	fileInfo, err := os.Stat(skillFile)
	if err != nil {
		return nil, err
	}
	return &domain.Skill{
		ID:           name,
		Name:         name,
		RelativePath: s.relativePath(name),
		Description:  description,
		UploadedAt:   fileInfo.ModTime(),
		CreatedAt:    fileInfo.ModTime(),
		UpdatedAt:    fileInfo.ModTime(),
	}, nil
}

func (s *FileSystemSkillStore) relativePath(name string) string {
	return filepath.ToSlash(filepath.Join(filepath.Base(s.skillsRootAbs), name))
}
