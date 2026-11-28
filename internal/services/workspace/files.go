package workspace

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const MaxPreviewBytes = 512 << 10

var (
	ErrInvalidPath = errors.New("path is outside the workspace or inaccessible")
	ErrNotFile     = errors.New("path is not a regular file")
)

type TreeEntry struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	IsDirectory bool   `json:"isDirectory"`
	IsSymlink   bool   `json:"isSymlink,omitempty"`
}

type FilePreview struct {
	Path      string `json:"path"`
	Content   string `json:"content,omitempty"`
	Language  string `json:"language,omitempty"`
	Size      int64  `json:"size"`
	Binary    bool   `json:"binary"`
	Truncated bool   `json:"truncated"`
}

func canonicalRoot(directory string) (string, error) {
	if !filepath.IsAbs(directory) {
		return "", ErrInvalidPath
	}
	root, err := filepath.EvalSymlinks(directory)
	if err != nil {
		return "", err
	}
	return root, nil
}

func validRelativePath(path string) bool {
	if path == "" || path == "." {
		return true
	}
	if strings.ContainsAny(path, "\\:\x00") || strings.HasPrefix(path, "/") || !filepath.IsLocal(filepath.FromSlash(path)) {
		return false
	}
	return !slices.Contains(strings.Split(path, "/"), "..")
}

// resolveSafePath returns a root-relative physical path. Missing leaf components
// are allowed for Git deletions, but broken and escaping links are always rejected.
func resolveSafePath(root, relative string, allowMissing bool) (string, error) {
	if !validRelativePath(relative) {
		return "", ErrInvalidPath
	}
	path := filepath.Join(root, filepath.FromSlash(relative))
	var missing []string
	for {
		resolved, err := filepath.EvalSymlinks(path)
		if err == nil {
			base, err := filepath.Rel(root, resolved)
			if err != nil || !filepath.IsLocal(base) {
				return "", ErrInvalidPath
			}
			slices.Reverse(missing)
			return filepath.Join(base, filepath.Join(missing...)), nil
		}
		if !allowMissing || !errors.Is(err, os.ErrNotExist) || path == root {
			return "", err
		}
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			return "", ErrInvalidPath
		}
		missing = append(missing, filepath.Base(path))
		path = filepath.Dir(path)
	}
}

func OpenTree(directory, relative string, showHidden bool) ([]TreeEntry, error) {
	root, err := canonicalRoot(directory)
	if err != nil {
		return nil, err
	}
	target, err := resolveSafePath(root, relative, false)
	if err != nil {
		return nil, err
	}
	fs, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer fs.Close()
	// Read through the rooted handle so a concurrent link change cannot redirect the listing.
	folder, err := openReadOnly(fs, target)
	if err != nil {
		return nil, err
	}
	defer folder.Close()
	entries, err := folder.ReadDir(-1)
	if err != nil {
		return nil, err
	}
	items := make([]TreeEntry, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		path := filepath.ToSlash(filepath.Join(filepath.FromSlash(relative), name))
		if !validRelativePath(path) || (!showHidden && hiddenEntry(name)) {
			continue
		}
		item := TreeEntry{Name: name, Path: path, IsDirectory: entry.IsDir(), IsSymlink: entry.Type()&os.ModeSymlink != 0}
		if item.IsSymlink {
			real, err := resolveSafePath(root, path, false)
			if err != nil {
				continue
			}
			info, err := fs.Stat(real)
			if err != nil {
				continue
			}
			item.IsDirectory = info.IsDir()
		}
		items = append(items, item)
	}
	slices.SortFunc(items, func(a, b TreeEntry) int {
		if a.IsDirectory != b.IsDirectory {
			if a.IsDirectory {
				return -1
			}
			return 1
		}
		if order := strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)); order != 0 {
			return order
		}
		return strings.Compare(a.Name, b.Name)
	})
	return items, nil
}

func hiddenEntry(name string) bool {
	if strings.HasPrefix(name, ".") {
		return true
	}
	switch strings.ToLower(name) {
	case "node_modules", "dist", "target", "__pycache__":
		return true
	default:
		return false
	}
}

func ReadFile(directory, relative string) (FilePreview, error) {
	root, err := canonicalRoot(directory)
	if err != nil {
		return FilePreview{}, err
	}
	target, err := resolveSafePath(root, relative, false)
	if err != nil {
		return FilePreview{}, err
	}
	fs, err := os.OpenRoot(root)
	if err != nil {
		return FilePreview{}, err
	}
	defer fs.Close()
	info, err := fs.Stat(target)
	if err != nil {
		return FilePreview{}, err
	}
	if !info.Mode().IsRegular() {
		return FilePreview{}, ErrNotFile
	}
	f, err := openReadOnly(fs, target)
	if err != nil {
		return FilePreview{}, err
	}
	defer f.Close()
	info, err = f.Stat()
	if err != nil {
		return FilePreview{}, err
	}
	if !info.Mode().IsRegular() {
		return FilePreview{}, ErrNotFile
	}
	data, err := io.ReadAll(io.LimitReader(f, MaxPreviewBytes+1))
	if err != nil {
		return FilePreview{}, fmt.Errorf("read workspace file: %w", err)
	}
	result := FilePreview{Path: filepath.ToSlash(filepath.Clean(relative)), Size: info.Size()}
	result.Truncated = len(data) > MaxPreviewBytes
	result.Binary = bytes.Contains(data[:min(8192, len(data))], []byte{0})
	if !result.Binary {
		result.Content = string(data[:min(MaxPreviewBytes, len(data))])
		result.Language = fileLanguage(relative)
	}
	return result, nil
}

func fileLanguage(path string) string {
	switch strings.ToLower(filepath.Base(path)) {
	case "dockerfile":
		return "dockerfile"
	case "makefile":
		return "makefile"
	}
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	switch ext {
	case "go", "json", "diff", "html", "xml", "css", "scss", "sql", "java", "lua", "ruby", "perl", "kotlin":
		return ext
	case "vue", "svg":
		return "xml"
	case "ts", "tsx":
		return "typescript"
	case "js", "mjs", "cjs", "jsx":
		return "javascript"
	case "yml", "yaml":
		return "yaml"
	case "md", "markdown":
		return "markdown"
	case "sh", "bash", "zsh":
		return "bash"
	case "ps1", "psm1":
		return "powershell"
	case "cmd", "bat":
		return "dos"
	case "py":
		return "python"
	case "rs":
		return "rust"
	case "rb":
		return "ruby"
	case "c", "h":
		return "c"
	case "cpp", "cc", "hpp":
		return "cpp"
	case "toml", "ini", "cfg":
		return "ini"
	default:
		return ""
	}
}
