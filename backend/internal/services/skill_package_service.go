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

func (s *SkillPackageService) SkillsRootAbs() string {
	return s.skillsRootAbs
}

func (s *SkillPackageService) InstallFromZip(filename string, data []byte) (*models.Skill, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("上传文件为空")
	}
	if len(data) > consts.MaxSkillZipBytes {
		return nil, fmt.Errorf("zip 文件过大，最大允许 %d MB", consts.MaxSkillZipBytes/(1024*1024))
	}
	if !strings.HasSuffix(strings.ToLower(strings.TrimSpace(filename)), ".zip") {
		return nil, fmt.Errorf("仅支持上传 .zip 文件")
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
		return nil, fmt.Errorf("skill %s 已存在，请先删除后重试", meta.Name)
	}

	if err := os.MkdirAll(s.skillsRootAbs, os.ModePerm); err != nil {
		return nil, fmt.Errorf("创建 skills 目录失败: %w", err)
	}

	tmpDir, err := os.MkdirTemp(s.skillsRootAbs, ".upload-*")
	if err != nil {
		return nil, fmt.Errorf("创建临时目录失败: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	if err := s.extractZip(data, tmpDir, files); err != nil {
		return nil, err
	}

	srcSkillDir := filepath.Join(tmpDir, meta.Name)
	if !isWithinRoot(tmpDir, srcSkillDir) {
		return nil, fmt.Errorf("技能目录路径非法")
	}
	if st, statErr := os.Stat(srcSkillDir); statErr != nil || !st.IsDir() {
		return nil, fmt.Errorf("解压后的 skill 目录不存在")
	}

	destSkillDir := filepath.Join(s.skillsRootAbs, meta.Name)
	if !isWithinRoot(s.skillsRootAbs, destSkillDir) {
		return nil, fmt.Errorf("目标路径非法")
	}
	if _, statErr := os.Stat(destSkillDir); statErr == nil {
		return nil, fmt.Errorf("skill %s 已存在，请先删除后重试", meta.Name)
	}

	if err := os.Rename(srcSkillDir, destSkillDir); err != nil {
		return nil, fmt.Errorf("移动 skill 目录失败: %w", err)
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

func (s *SkillPackageService) validateZipAndCollect(data []byte) (*parsedSkillMetadata, map[string]*zip.File, error) {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, nil, fmt.Errorf("zip 解析失败: %w", err)
	}
	if len(reader.File) == 0 {
		return nil, nil, fmt.Errorf("zip 内容为空")
	}
	if len(reader.File) > consts.MaxSkillFileCount {
		return nil, nil, fmt.Errorf("zip 中文件过多，最多允许 %d 个", consts.MaxSkillFileCount)
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
			return nil, nil, fmt.Errorf("zip 结构非法：存在无效路径 %s", file.Name)
		}
		topLevels[parts[0]] = struct{}{}
		collected[cleanName] = file

		if !file.FileInfo().IsDir() {
			if file.UncompressedSize64 > consts.MaxSkillSingleFileSize {
				return nil, nil, fmt.Errorf("文件 %s 超过单文件大小限制 %d MB", cleanName, consts.MaxSkillSingleFileSize/(1024*1024))
			}
			total += file.UncompressedSize64
		}
	}

	if len(topLevels) != 1 {
		return nil, nil, fmt.Errorf("zip 必须且只能包含一个顶层目录")
	}

	var topDir string
	for name := range topLevels {
		topDir = name
	}

	if !isValidSkillName(topDir) {
		return nil, nil, fmt.Errorf("顶层目录名不符合规范（仅小写字母/数字/连字符，1-64，不能以连字符开头或结尾，不能连续连字符）")
	}
	if total > consts.MaxSkillExtractedBytes {
		return nil, nil, fmt.Errorf("解压后总大小超过限制 %d MB", consts.MaxSkillExtractedBytes/(1024*1024))
	}

	skillFilePath := topDir + "/SKILL.md"
	skillFile, ok := collected[skillFilePath]
	if !ok || skillFile.FileInfo().IsDir() {
		return nil, nil, fmt.Errorf("顶层目录内必须包含 SKILL.md")
	}

	meta, err := readSkillMetadata(skillFile, topDir)
	if err != nil {
		return nil, nil, err
	}
	return meta, collected, nil
}

func (s *SkillPackageService) extractZip(data []byte, targetRoot string, files map[string]*zip.File) error {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return fmt.Errorf("zip 解析失败: %w", err)
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
			return fmt.Errorf("检测到非法解压路径: %s", cleanName)
		}

		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(destPath, os.ModePerm); err != nil {
				return fmt.Errorf("创建目录失败: %w", err)
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(destPath), os.ModePerm); err != nil {
			return fmt.Errorf("创建父目录失败: %w", err)
		}

		src, err := file.Open()
		if err != nil {
			return fmt.Errorf("读取 zip 条目失败: %w", err)
		}
		dst, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, file.Mode().Perm())
		if err != nil {
			_ = src.Close()
			return fmt.Errorf("写入文件失败: %w", err)
		}
		if _, err = io.Copy(dst, src); err != nil {
			_ = dst.Close()
			_ = src.Close()
			return fmt.Errorf("解压文件失败: %w", err)
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
		return "", fmt.Errorf("zip 中存在绝对路径: %s", raw)
	}
	if strings.Contains(normalized, ":") {
		return "", fmt.Errorf("zip 中存在盘符路径: %s", raw)
	}

	cleaned := path.Clean(normalized)
	if cleaned == "." {
		return "", nil
	}
	if strings.HasPrefix(cleaned, "../") || cleaned == ".." {
		return "", fmt.Errorf("zip 中存在越界路径: %s", raw)
	}
	return cleaned, nil
}

func readSkillMetadata(file *zip.File, expectedName string) (*parsedSkillMetadata, error) {
	rc, err := file.Open()
	if err != nil {
		return nil, fmt.Errorf("读取 SKILL.md 失败: %w", err)
	}
	defer rc.Close()

	raw, err := io.ReadAll(io.LimitReader(rc, consts.MaxSkillSingleFileSize))
	if err != nil {
		return nil, fmt.Errorf("读取 SKILL.md 失败: %w", err)
	}
	name, description, err := parseSkillFrontmatter(string(raw))
	if err != nil {
		return nil, err
	}
	if name != expectedName {
		return nil, fmt.Errorf("SKILL.md 中 name(%s) 与顶层目录(%s) 不一致", name, expectedName)
	}
	return &parsedSkillMetadata{Name: name, Description: description}, nil
}

func parseSkillFrontmatter(content string) (string, string, error) {
	text := strings.TrimPrefix(content, "\uFEFF")
	if !strings.HasPrefix(text, "---") {
		return "", "", fmt.Errorf("SKILL.md 缺少 YAML frontmatter")
	}
	parts := strings.SplitN(text, "---", 3)
	if len(parts) < 3 {
		return "", "", fmt.Errorf("SKILL.md frontmatter 格式错误")
	}
	frontmatter := strings.TrimSpace(parts[1])
	if frontmatter == "" {
		return "", "", fmt.Errorf("SKILL.md frontmatter 不能为空")
	}

	var fm skillFrontmatter
	if err := yaml.Unmarshal([]byte(frontmatter), &fm); err != nil {
		return "", "", fmt.Errorf("SKILL.md frontmatter YAML 解析失败: %w", err)
	}

	name := strings.TrimSpace(fm.Name)
	description := strings.TrimSpace(fm.Description)
	if !isValidSkillName(name) {
		return "", "", fmt.Errorf("SKILL.md name 不符合规范")
	}
	if description == "" {
		return "", "", fmt.Errorf("SKILL.md description 不能为空")
	}
	if len([]rune(description)) > 1024 {
		return "", "", fmt.Errorf("SKILL.md description 超过 1024 字符限制")
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
