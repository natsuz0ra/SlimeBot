package services

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/goccy/go-yaml"
	"slimebot/backend/internal/consts"
	"slimebot/backend/internal/models"
	"slimebot/backend/internal/repositories"
)

type SkillPackageService struct {
	repo          *repositories.Repository
	skillsRoot    string
	skillsRootAbs string
}

type parsedSkillMetadata struct {
	Name        string
	Description string
}

type skillFrontmatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

func NewSkillPackageService(repo *repositories.Repository, skillsRoot string) *SkillPackageService {
	absRoot, _ := filepath.Abs(strings.TrimSpace(skillsRoot))
	return &SkillPackageService{
		repo:          repo,
		skillsRoot:    strings.TrimSpace(skillsRoot),
		skillsRootAbs: absRoot,
	}
}

// InstallFromZip 按“校验 -> 解压 -> 移动 -> 入库”流程安装技能包。
// 注意文件系统与数据库不是原子事务：若入库失败会尝试回滚目标目录。
func (s *SkillPackageService) InstallFromZip(filename string, data []byte) (*models.Skill, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("uploaded file is empty")
	}
	if len(data) > consts.MaxSkillZipBytes {
		return nil, fmt.Errorf("zip file is too large, maximum allowed is %d MB", consts.MaxSkillZipBytes/(1024*1024))
	}
	if !strings.HasSuffix(strings.ToLower(strings.TrimSpace(filename)), ".zip") {
		return nil, fmt.Errorf("only .zip files are supported")
	}

	meta, files, err := s.validateZipAndCollect(data)
	if err != nil {
		return nil, err
	}

	exists, err := s.repo.GetSkillByName(meta.Name)
	if err != nil {
		return nil, err
	}
	if exists != nil {
		return nil, fmt.Errorf("skill %s already exists, delete it before retrying", meta.Name)
	}

	if err := os.MkdirAll(s.skillsRootAbs, os.ModePerm); err != nil {
		return nil, fmt.Errorf("failed to create skills directory: %w", err)
	}

	tmpDir, err := os.MkdirTemp(s.skillsRootAbs, ".upload-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	if err := s.extractZip(data, tmpDir, files); err != nil {
		return nil, err
	}

	srcSkillDir := filepath.Join(tmpDir, meta.Name)
	if !isWithinRoot(tmpDir, srcSkillDir) {
		return nil, fmt.Errorf("invalid skill directory path")
	}
	if st, statErr := os.Stat(srcSkillDir); statErr != nil || !st.IsDir() {
		return nil, fmt.Errorf("extracted skill directory does not exist")
	}

	destSkillDir := filepath.Join(s.skillsRootAbs, meta.Name)
	if !isWithinRoot(s.skillsRootAbs, destSkillDir) {
		return nil, fmt.Errorf("invalid target path")
	}
	if _, statErr := os.Stat(destSkillDir); statErr == nil {
		return nil, fmt.Errorf("skill %s already exists, delete it before retrying", meta.Name)
	}

	if err := os.Rename(srcSkillDir, destSkillDir); err != nil {
		return nil, fmt.Errorf("failed to move skill directory: %w", err)
	}

	relativePath := filepath.ToSlash(filepath.Join(filepath.Base(s.skillsRootAbs), meta.Name))
	item, err := s.repo.CreateSkill(models.Skill{
		Name:         meta.Name,
		RelativePath: relativePath,
		Description:  meta.Description,
		UploadedAt:   time.Now(),
	})
	if err != nil {
		_ = os.RemoveAll(destSkillDir)
		return nil, err
	}
	return item, nil
}

// validateZipAndCollect 校验 zip 结构与资源限制，并提取 SKILL.md 元数据。
func (s *SkillPackageService) validateZipAndCollect(data []byte) (*parsedSkillMetadata, map[string]*zip.File, error) {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse zip: %w", err)
	}
	if len(reader.File) == 0 {
		return nil, nil, fmt.Errorf("zip content is empty")
	}
	if len(reader.File) > consts.MaxSkillFileCount {
		return nil, nil, fmt.Errorf("too many files in zip, maximum allowed is %d", consts.MaxSkillFileCount)
	}

	topLevels := make(map[string]struct{})
	collected := make(map[string]*zip.File, len(reader.File))
	var total uint64
	for _, file := range reader.File {
		cleanName, cleanErr := sanitizeZipPath(file.Name)
		if cleanErr != nil {
			return nil, nil, cleanErr
		}
		if cleanName == "" {
			continue
		}

		parts := strings.Split(cleanName, "/")
		if len(parts) == 0 || strings.TrimSpace(parts[0]) == "" {
			return nil, nil, fmt.Errorf("invalid zip structure: invalid path %s", file.Name)
		}
		topLevels[parts[0]] = struct{}{}
		collected[cleanName] = file

		if !file.FileInfo().IsDir() {
			if file.UncompressedSize64 > consts.MaxSkillSingleFileSize {
				return nil, nil, fmt.Errorf("file %s exceeds single-file size limit %d MB", cleanName, consts.MaxSkillSingleFileSize/(1024*1024))
			}
			total += file.UncompressedSize64
		}
	}

	if len(topLevels) != 1 {
		return nil, nil, fmt.Errorf("zip must contain exactly one top-level directory")
	}

	var topDir string
	for name := range topLevels {
		topDir = name
	}

	if !isValidSkillName(topDir) {
		return nil, nil, fmt.Errorf("invalid top-level directory name (lowercase letters/numbers/hyphens only, length 1-64, no leading/trailing/consecutive hyphens)")
	}
	if total > consts.MaxSkillExtractedBytes {
		return nil, nil, fmt.Errorf("total extracted size exceeds limit %d MB", consts.MaxSkillExtractedBytes/(1024*1024))
	}

	skillFilePath := topDir + "/SKILL.md"
	skillFile, ok := collected[skillFilePath]
	if !ok || skillFile.FileInfo().IsDir() {
		return nil, nil, fmt.Errorf("top-level directory must contain SKILL.md")
	}

	meta, err := readSkillMetadata(skillFile, topDir)
	if err != nil {
		return nil, nil, err
	}
	return meta, collected, nil
}

// extractZip 仅提取通过 validateZipAndCollect 白名单筛选后的条目，避免路径逃逸。
func (s *SkillPackageService) extractZip(data []byte, targetRoot string, files map[string]*zip.File) error {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return fmt.Errorf("failed to parse zip: %w", err)
	}

	for _, file := range reader.File {
		cleanName, cleanErr := sanitizeZipPath(file.Name)
		if cleanErr != nil {
			return cleanErr
		}
		if cleanName == "" {
			continue
		}
		if _, ok := files[cleanName]; !ok {
			continue
		}

		destPath := filepath.Join(targetRoot, filepath.FromSlash(cleanName))
		if !isWithinRoot(targetRoot, destPath) {
			return fmt.Errorf("detected invalid extraction path: %s", cleanName)
		}

		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(destPath, os.ModePerm); err != nil {
				return fmt.Errorf("failed to create directory: %w", err)
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(destPath), os.ModePerm); err != nil {
			return fmt.Errorf("failed to create parent directory: %w", err)
		}

		src, err := file.Open()
		if err != nil {
			return fmt.Errorf("failed to read zip entry: %w", err)
		}
		dst, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, file.Mode().Perm())
		if err != nil {
			_ = src.Close()
			return fmt.Errorf("failed to write file: %w", err)
		}
		if _, err = io.Copy(dst, src); err != nil {
			_ = dst.Close()
			_ = src.Close()
			return fmt.Errorf("failed to extract file: %w", err)
		}
		_ = dst.Close()
		_ = src.Close()
	}
	return nil
}

func sanitizeZipPath(raw string) (string, error) {
	normalized := strings.ReplaceAll(strings.TrimSpace(raw), "\\", "/")
	normalized = strings.TrimPrefix(normalized, "./")
	if normalized == "" {
		return "", nil
	}
	if strings.HasPrefix(normalized, "/") {
		return "", fmt.Errorf("zip contains an absolute path: %s", raw)
	}
	if strings.Contains(normalized, ":") {
		return "", fmt.Errorf("zip contains a drive path: %s", raw)
	}

	cleaned := path.Clean(normalized)
	if cleaned == "." {
		return "", nil
	}
	if strings.HasPrefix(cleaned, "../") || cleaned == ".." {
		return "", fmt.Errorf("zip contains an out-of-root path: %s", raw)
	}
	return cleaned, nil
}

func readSkillMetadata(file *zip.File, expectedName string) (*parsedSkillMetadata, error) {
	rc, err := file.Open()
	if err != nil {
		return nil, fmt.Errorf("failed to read SKILL.md: %w", err)
	}
	defer rc.Close()

	raw, err := io.ReadAll(io.LimitReader(rc, consts.MaxSkillSingleFileSize))
	if err != nil {
		return nil, fmt.Errorf("failed to read SKILL.md: %w", err)
	}
	name, description, err := parseSkillFrontmatter(string(raw))
	if err != nil {
		return nil, err
	}
	if name != expectedName {
		return nil, fmt.Errorf("SKILL.md name (%s) does not match top-level directory (%s)", name, expectedName)
	}
	return &parsedSkillMetadata{Name: name, Description: description}, nil
}

func parseSkillFrontmatter(content string) (string, string, error) {
	text := strings.TrimPrefix(content, "\uFEFF")
	if !strings.HasPrefix(text, "---") {
		return "", "", fmt.Errorf("SKILL.md is missing YAML frontmatter")
	}
	parts := strings.SplitN(text, "---", 3)
	if len(parts) < 3 {
		return "", "", fmt.Errorf("invalid SKILL.md frontmatter format")
	}
	frontmatter := strings.TrimSpace(parts[1])
	if frontmatter == "" {
		return "", "", fmt.Errorf("SKILL.md frontmatter cannot be empty")
	}

	var fm skillFrontmatter
	if err := yaml.Unmarshal([]byte(frontmatter), &fm); err != nil {
		return "", "", fmt.Errorf("failed to parse SKILL.md frontmatter YAML: %w", err)
	}

	name := strings.TrimSpace(fm.Name)
	description := strings.TrimSpace(fm.Description)
	if !isValidSkillName(name) {
		return "", "", fmt.Errorf("invalid SKILL.md name")
	}
	if description == "" {
		return "", "", fmt.Errorf("SKILL.md description cannot be empty")
	}
	if len([]rune(description)) > 1024 {
		return "", "", fmt.Errorf("SKILL.md description exceeds 1024 characters")
	}
	return name, description, nil
}

func isValidSkillName(name string) bool {
	if len(name) == 0 || len(name) > 64 {
		return false
	}
	if strings.HasPrefix(name, "-") || strings.HasSuffix(name, "-") || strings.Contains(name, "--") {
		return false
	}
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			continue
		}
		return false
	}
	return true
}

func isWithinRoot(root, target string) bool {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(absRoot, absTarget)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != "..")
}

func listSkillResourceFiles(skillDir string) ([]string, error) {
	results := make([]string, 0, 64)
	err := filepath.WalkDir(skillDir, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if p == skillDir {
			return nil
		}
		rel, err := filepath.Rel(skillDir, p)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if rel == "SKILL.md" {
			return nil
		}
		results = append(results, rel)
		if len(results) >= consts.MaxSkillResourcesShown {
			return fs.SkipAll
		}
		return nil
	})
	if err != nil && err != fs.SkipAll {
		return nil, err
	}
	return results, nil
}
