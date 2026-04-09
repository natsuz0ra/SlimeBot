package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// rebuildEntrypoint 重建 MEMORY.md 索引文件。
// 扫描目录下所有 .md 记忆文件，生成按更新时间排序的索引列表。
func rebuildEntrypoint(baseDir string, maxLines int) error {
	entries, err := scanMemoryDir(baseDir)
	if err != nil {
		return fmt.Errorf("scan for entrypoint: %w", err)
	}

	var b strings.Builder
	b.WriteString("# Memory Index\n\n")

	for _, entry := range entries {
		line := fmt.Sprintf("- [%s](%s) — %s\n", entry.Name, filepath.Base(entry.FilePath), entry.Description)
		b.WriteString(line)
	}

	content := b.String()

	// 截断到最大行数
	lines := strings.Split(content, "\n")
	if len(lines) > maxLines {
		lines = lines[:maxLines]
		content = strings.Join(lines, "\n")
	}

	entrypointPath := filepath.Join(baseDir, entrypointFileName)
	if err := os.WriteFile(entrypointPath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write entrypoint: %w", err)
	}

	return nil
}
