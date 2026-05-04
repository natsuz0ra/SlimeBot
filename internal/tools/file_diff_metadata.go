package tools

import (
	"path/filepath"
	"strings"
)

const fileDiffContextLines = 2

type FileDiffLine struct {
	Kind    string `json:"kind"`
	OldLine *int   `json:"oldLine,omitempty"`
	NewLine *int   `json:"newLine,omitempty"`
	Text    string `json:"text"`
}

type FileToolMetadata struct {
	FilePath  string         `json:"filePath"`
	Operation string         `json:"operation"`
	Summary   string         `json:"summary"`
	DiffLines []FileDiffLine `json:"diffLines"`
}

func buildFileToolMetadata(filePath, operation, summary, oldContent, newContent string) FileToolMetadata {
	return FileToolMetadata{
		FilePath:  filePath,
		Operation: operation,
		Summary:   summary,
		DiffLines: compactDiffLines(buildFullLineDiff(oldContent, newContent), fileDiffContextLines),
	}
}

func fileToolSummary(operation, filePath string) string {
	name := filepath.Base(filePath)
	if strings.TrimSpace(name) == "" || name == "." || name == string(filepath.Separator) {
		name = filePath
	}
	switch operation {
	case "Create":
		return "Created " + name
	case "Update":
		return "Updated " + name
	case "Write":
		return "Wrote " + name
	default:
		return operation + " " + name
	}
}

func splitDiffTextLines(content string) []string {
	if content == "" {
		return nil
	}
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	if strings.HasSuffix(content, "\n") && len(lines) > 0 {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func intPtr(v int) *int {
	return &v
}

func buildFullLineDiff(oldContent, newContent string) []FileDiffLine {
	oldLines := splitDiffTextLines(oldContent)
	newLines := splitDiffTextLines(newContent)
	table := make([][]int, len(oldLines)+1)
	for i := range table {
		table[i] = make([]int, len(newLines)+1)
	}
	for i := len(oldLines) - 1; i >= 0; i-- {
		for j := len(newLines) - 1; j >= 0; j-- {
			if oldLines[i] == newLines[j] {
				table[i][j] = table[i+1][j+1] + 1
			} else if table[i+1][j] >= table[i][j+1] {
				table[i][j] = table[i+1][j]
			} else {
				table[i][j] = table[i][j+1]
			}
		}
	}

	result := make([]FileDiffLine, 0, len(oldLines)+len(newLines))
	i, j := 0, 0
	for i < len(oldLines) && j < len(newLines) {
		if oldLines[i] == newLines[j] {
			result = append(result, FileDiffLine{Kind: "context", OldLine: intPtr(i + 1), NewLine: intPtr(j + 1), Text: oldLines[i]})
			i++
			j++
		} else if table[i+1][j] >= table[i][j+1] {
			result = append(result, FileDiffLine{Kind: "removed", OldLine: intPtr(i + 1), Text: oldLines[i]})
			i++
		} else {
			result = append(result, FileDiffLine{Kind: "added", NewLine: intPtr(j + 1), Text: newLines[j]})
			j++
		}
	}
	for i < len(oldLines) {
		result = append(result, FileDiffLine{Kind: "removed", OldLine: intPtr(i + 1), Text: oldLines[i]})
		i++
	}
	for j < len(newLines) {
		result = append(result, FileDiffLine{Kind: "added", NewLine: intPtr(j + 1), Text: newLines[j]})
		j++
	}
	return result
}

func compactDiffLines(lines []FileDiffLine, contextLines int) []FileDiffLine {
	if len(lines) == 0 {
		return nil
	}
	keep := make([]bool, len(lines))
	for i, line := range lines {
		if line.Kind == "context" {
			continue
		}
		start := i - contextLines
		if start < 0 {
			start = 0
		}
		end := i + contextLines
		if end >= len(lines) {
			end = len(lines) - 1
		}
		for j := start; j <= end; j++ {
			keep[j] = true
		}
	}
	result := make([]FileDiffLine, 0, len(lines))
	for i, line := range lines {
		if keep[i] {
			result = append(result, line)
		}
	}
	return result
}
