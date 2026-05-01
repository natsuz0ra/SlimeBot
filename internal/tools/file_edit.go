package tools

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

type fileEditTool struct{}

type fileEditOperation struct {
	OldString          string                  `json:"old_string"`
	NewString          string                  `json:"new_string"`
	ReplaceAll         bool                    `json:"replace_all,omitempty"`
	LineSelector       *fileEditLineSelector   `json:"line_selector,omitempty"`
	OccurrenceSelector *fileEditNthSelector    `json:"occurrence_selector,omitempty"`
	AnchorContext      *fileEditAnchorSelector `json:"anchor_context,omitempty"`
	StrictMode         string                  `json:"strict_mode,omitempty"`
}

type fileEditRequest struct {
	FilePath   string              `json:"file_path"`
	Operations []fileEditOperation `json:"operations"`
}

type fileEditLineSelector struct {
	StartLine int `json:"start_line"`
	EndLine   int `json:"end_line"`
}

type fileEditNthSelector struct {
	Nth int `json:"nth"`
}

type fileEditAnchorSelector struct {
	Before string `json:"before"`
	Target string `json:"target"`
	After  string `json:"after"`
}

func init() {
	Register(&fileEditTool{})
}

func (f *fileEditTool) Name() string { return "file_edit" }

func (f *fileEditTool) Description() string {
	return "Edit UTF-8 text files by replacing exact strings, with stale-write protection."
}

func (f *fileEditTool) Commands() []Command {
	return []Command{{
		Name:        "edit",
		Description: "Supports single operation and batch edits. Existing files must be fully read with file_read first.",
		Params: []CommandParam{
			{Name: "file_path", Required: false, Description: "Single-edit mode file path.", Example: "/path/to/file.go"},
			{Name: "old_string", Required: false, Description: "Single-edit mode old text.", Example: "old text"},
			{Name: "new_string", Required: false, Description: "Single-edit mode new text.", Example: "new text"},
			{Name: "replace_all", Required: false, Description: "Single-edit mode replace all matches.", Example: "false"},
			{Name: "line_selector", Required: false, Description: "Precise line range selector: {start_line,end_line}.", Schema: map[string]any{"type": "object"}},
			{Name: "occurrence_selector", Required: false, Description: "Precise nth occurrence selector: {nth}.", Schema: map[string]any{"type": "object"}},
			{Name: "anchor_context", Required: false, Description: "Precise anchor selector: {before,target,after}.", Schema: map[string]any{"type": "object"}},
			{Name: "strict_mode", Required: false, Description: "Matching mode: exact|normalized (default exact).", Example: "exact"},
			{Name: "edits", Required: false, Description: "Batch mode edits: [{file_path,operations:[{old_string,new_string,replace_all}]}].", Schema: map[string]any{
				"type": "array",
				"items": map[string]any{
					"type":     "object",
					"required": []string{"file_path", "operations"},
				},
			}},
		},
	}}
}

func (f *fileEditTool) Execute(ctx context.Context, command string, params map[string]any) (*ExecuteResult, error) {
	switch command {
	case "edit":
		return f.edit(ctx, params)
	default:
		return nil, fmt.Errorf("file_edit tool does not support command: %s", command)
	}
}

func (f *fileEditTool) edit(ctx context.Context, params map[string]any) (*ExecuteResult, error) {
	requests, err := parseFileEditRequests(params)
	if err != nil {
		return nil, err
	}

	var out strings.Builder
	success := 0
	failed := 0
	metas := make([]FileToolMetadata, 0, len(requests))
	for i, req := range requests {
		meta, msg, editErr := f.editOne(ctx, req)
		if i > 0 {
			out.WriteString("\n")
		}
		if editErr != nil {
			if len(requests) == 1 {
				return nil, editErr
			}
			failed++
			out.WriteString(fmt.Sprintf("Edit %d failed: %v", i+1, editErr))
			continue
		}
		success++
		metas = append(metas, meta)
		out.WriteString(msg)
	}
	out.WriteString(fmt.Sprintf("\nSummary: succeeded=%d failed=%d total=%d", success, failed, len(requests)))
	if len(requests) == 1 && len(metas) == 1 {
		return &ExecuteResult{Output: out.String(), Metadata: metas[0]}, nil
	}
	return &ExecuteResult{Output: out.String(), Metadata: metas}, nil
}

func parseFileEditRequests(params map[string]any) ([]fileEditRequest, error) {
	var requests []fileEditRequest
	if ok, err := decodeParamInto(params, "edits", &requests); err != nil {
		return nil, fmt.Errorf("invalid edits: %w", err)
	} else if ok {
		if len(requests) == 0 {
			return nil, fmt.Errorf("edits must contain at least one item")
		}
		for i := range requests {
			requests[i].FilePath = strings.TrimSpace(requests[i].FilePath)
			if requests[i].FilePath == "" {
				return nil, fmt.Errorf("edits[%d].file_path is required", i)
			}
			if len(requests[i].Operations) == 0 {
				return nil, fmt.Errorf("edits[%d].operations must contain at least one operation", i)
			}
		}
		return requests, nil
	}

	replaceAll, _, err := paramBool(params, "replace_all")
	if err != nil {
		return nil, err
	}
	filePath := paramStringTrim(params, "file_path")
	oldString := paramString(params, "old_string")
	newString := paramString(params, "new_string")
	strictMode := strings.TrimSpace(paramString(params, "strict_mode"))
	var lineSelector *fileEditLineSelector
	if ok, err := decodeParamInto(params, "line_selector", &lineSelector); err != nil {
		return nil, fmt.Errorf("invalid line_selector: %w", err)
	} else if ok && lineSelector == nil {
		return nil, fmt.Errorf("line_selector cannot be null")
	}
	var occurrenceSelector *fileEditNthSelector
	if ok, err := decodeParamInto(params, "occurrence_selector", &occurrenceSelector); err != nil {
		return nil, fmt.Errorf("invalid occurrence_selector: %w", err)
	} else if ok && occurrenceSelector == nil {
		return nil, fmt.Errorf("occurrence_selector cannot be null")
	}
	var anchorContext *fileEditAnchorSelector
	if ok, err := decodeParamInto(params, "anchor_context", &anchorContext); err != nil {
		return nil, fmt.Errorf("invalid anchor_context: %w", err)
	} else if ok && anchorContext == nil {
		return nil, fmt.Errorf("anchor_context cannot be null")
	}
	if filePath == "" {
		return nil, fmt.Errorf("file_path is required when edits is not provided")
	}
	return []fileEditRequest{{
		FilePath: filePath,
		Operations: []fileEditOperation{{
			OldString:          oldString,
			NewString:          newString,
			ReplaceAll:         replaceAll,
			LineSelector:       lineSelector,
			OccurrenceSelector: occurrenceSelector,
			AnchorContext:      anchorContext,
			StrictMode:         strictMode,
		}},
	}}, nil
}

func (f *fileEditTool) editOne(ctx context.Context, req fileEditRequest) (FileToolMetadata, string, error) {
	path, err := resolveFilePath(req.FilePath)
	if err != nil {
		return FileToolMetadata{}, "", err
	}
	info, statErr := os.Stat(path)
	if statErr != nil && !os.IsNotExist(statErr) {
		return FileToolMetadata{}, "", fmt.Errorf("failed to stat file: %w", statErr)
	}
	if statErr == nil && info.IsDir() {
		return FileToolMetadata{}, "", fmt.Errorf("file_path is a directory, not a file: %s", path)
	}

	var original string
	var existed bool
	if os.IsNotExist(statErr) {
		original = ""
	} else {
		existed = true
		if info.Size() > fileWriteMaxSizeBytes {
			return FileToolMetadata{}, "", fmt.Errorf("file is too large to edit (%d bytes, max %d)", info.Size(), fileWriteMaxSizeBytes)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return FileToolMetadata{}, "", fmt.Errorf("failed to read file: %w", err)
		}
		original, err = validateTextBytes(data)
		if err != nil {
			return FileToolMetadata{}, "", err
		}
		if err := requireFreshFullRead(ctx, path, original, info); err != nil {
			return FileToolMetadata{}, "", err
		}
	}

	updated := original
	totalReplacements := 0
	for idx, op := range req.Operations {
		next, count, applyErr := applyOperationEdit(updated, op)
		if applyErr != nil {
			return FileToolMetadata{}, "", fmt.Errorf("operation %d failed: %w", idx+1, applyErr)
		}
		totalReplacements += count
		updated = next
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return FileToolMetadata{}, "", fmt.Errorf("failed to create parent directory: %w", err)
	}
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		return FileToolMetadata{}, "", fmt.Errorf("failed to write file: %w", err)
	}
	newInfo, _ := os.Stat(path)
	if state := readFileStateFromContext(ctx); state != nil {
		state.set(path, ReadFileEntry{Content: updated, MTimeUnix: fileMTimeUnix(newInfo), Offset: 1, Limit: 0, Partial: false})
	}

	action := "Update"
	verb := "updated"
	if !existed {
		action = "Create"
		verb = "created"
	}
	meta := buildFileToolMetadata(path, action, fileToolSummary(action, path), original, updated)
	msg := fmt.Sprintf("File %s successfully: %s\nReplacements: %d", verb, path, totalReplacements)
	return meta, msg, nil
}

func requireFreshFullRead(ctx context.Context, path string, current string, info os.FileInfo) error {
	state := readFileStateFromContext(ctx)
	if state == nil {
		return fmt.Errorf("file has not been read yet. Read it first with file_read before writing to it")
	}
	entry, ok := state.get(path)
	if !ok || entry.Partial {
		return fmt.Errorf("file has not been fully read yet. Read the full file with file_read before writing to it")
	}
	if current != entry.Content {
		return fmt.Errorf("file has been modified since it was read. Read it again before attempting to write")
	}
	if fileMTimeUnix(info) > entry.MTimeUnix && current != entry.Content {
		return fmt.Errorf("file has been modified since it was read. Read it again before attempting to write")
	}
	return nil
}

func applyTextEdit(original, oldString, newString string, replaceAll bool) (string, int, error) {
	if oldString == "" {
		if original != "" {
			return "", 0, fmt.Errorf("old_string is empty but file is not empty")
		}
		if newString == "" {
			return "", 0, fmt.Errorf("new_string is empty; no changes to make")
		}
		return newString, 1, nil
	}
	count := strings.Count(original, oldString)
	if count == 0 {
		return "", 0, fmt.Errorf("string to replace was not found in file")
	}
	if count > 1 && !replaceAll {
		return "", 0, fmt.Errorf("found %d matches of old_string; provide more context or set replace_all=true", count)
	}
	if replaceAll {
		return strings.ReplaceAll(original, oldString, newString), count, nil
	}
	return strings.Replace(original, oldString, newString, 1), 1, nil
}

var readPrefixPattern = regexp.MustCompile(`^\s*\d+\s*(\t|->\s?)`)

func applyOperationEdit(original string, op fileEditOperation) (string, int, error) {
	strictMode := strings.ToLower(strings.TrimSpace(op.StrictMode))
	if strictMode == "" {
		strictMode = "exact"
	}
	if strictMode != "exact" && strictMode != "normalized" {
		return "", 0, fmt.Errorf("invalid strict_mode=%q, must be exact|normalized", op.StrictMode)
	}
	selectorCount := 0
	if op.LineSelector != nil {
		selectorCount++
	}
	if op.OccurrenceSelector != nil {
		selectorCount++
	}
	if op.AnchorContext != nil {
		selectorCount++
	}
	if selectorCount > 1 {
		return "", 0, structuredEditError("AMBIGUOUS_SELECTOR", 0, "only one of line_selector/occurrence_selector/anchor_context can be provided")
	}

	oldString := op.OldString
	newString := op.NewString
	if strictMode == "normalized" {
		oldString = normalizeMatchText(oldString)
		newString = normalizeMatchText(newString)
	}
	actualNew := normalizeReplacementLineEndings(original, newString)

	if op.LineSelector != nil {
		return applyLineSelector(original, actualNew, op.LineSelector)
	}
	if op.OccurrenceSelector != nil {
		return applyOccurrenceSelector(original, oldString, actualNew, op.OccurrenceSelector)
	}
	if op.AnchorContext != nil {
		return applyAnchorSelector(original, oldString, actualNew, op.AnchorContext, strictMode)
	}

	if oldString == newString {
		return "", 0, fmt.Errorf("old_string and new_string are exactly the same")
	}
	next, count, err := applyTextEdit(original, oldString, actualNew, op.ReplaceAll)
	if err == nil {
		return next, count, nil
	}
	if strings.Contains(err.Error(), "found") && strings.Contains(err.Error(), "matches") {
		return "", 0, structuredEditError("MULTI_MATCH", countMatches(original, oldString), "set occurrence_selector.nth or provide anchor_context.before/after")
	}
	if strings.Contains(err.Error(), "not found") {
		return "", 0, structuredEditError("NOT_FOUND", 0, "check old_string, or use strict_mode=normalized if content includes line-number prefixes")
	}
	return "", 0, err
}

func applyLineSelector(original, newString string, selector *fileEditLineSelector) (string, int, error) {
	if selector.StartLine <= 0 || selector.EndLine <= 0 || selector.StartLine > selector.EndLine {
		return "", 0, structuredEditError("OUT_OF_RANGE", 0, "line_selector requires positive start_line/end_line and start_line<=end_line")
	}
	lines := strings.Split(strings.ReplaceAll(original, "\r\n", "\n"), "\n")
	hadTrailingNewline := strings.HasSuffix(original, "\n")
	if hadTrailingNewline && len(lines) > 0 {
		lines = lines[:len(lines)-1]
	}
	if selector.EndLine > len(lines) {
		return "", 0, structuredEditError("OUT_OF_RANGE", len(lines), "line_selector exceeds total lines; read file and adjust start/end line")
	}
	startIdx := selector.StartLine - 1
	endIdx := selector.EndLine
	newLines := strings.Split(strings.ReplaceAll(newString, "\r\n", "\n"), "\n")
	if strings.HasSuffix(newString, "\n") && len(newLines) > 0 {
		newLines = newLines[:len(newLines)-1]
	}
	updatedLines := append(append([]string{}, lines[:startIdx]...), append(newLines, lines[endIdx:]...)...)
	updated := strings.Join(updatedLines, "\n")
	if hadTrailingNewline || strings.HasSuffix(newString, "\n") {
		updated += "\n"
	}
	return updated, 1, nil
}

func applyOccurrenceSelector(original, oldString, newString string, selector *fileEditNthSelector) (string, int, error) {
	if selector.Nth <= 0 {
		return "", 0, structuredEditError("OUT_OF_RANGE", 0, "occurrence_selector.nth must be >= 1")
	}
	if oldString == "" {
		return "", 0, structuredEditError("INVALID_SELECTOR", 0, "old_string is required when using occurrence_selector")
	}
	start := 0
	index := -1
	matches := 0
	for {
		pos := strings.Index(original[start:], oldString)
		if pos < 0 {
			break
		}
		abs := start + pos
		matches++
		if matches == selector.Nth {
			index = abs
			break
		}
		start = abs + len(oldString)
	}
	if matches == 0 || index < 0 {
		return "", 0, structuredEditError("OUT_OF_RANGE", matches, "occurrence_selector.nth exceeds available matches")
	}
	updated := original[:index] + newString + original[index+len(oldString):]
	return updated, 1, nil
}

func applyAnchorSelector(original, oldString, newString string, selector *fileEditAnchorSelector, strictMode string) (string, int, error) {
	before := selector.Before
	target := selector.Target
	after := selector.After
	if strictMode == "normalized" {
		before = normalizeMatchText(before)
		target = normalizeMatchText(target)
		after = normalizeMatchText(after)
	}
	if target == "" {
		target = oldString
	}
	if target == "" {
		return "", 0, structuredEditError("INVALID_SELECTOR", 0, "anchor_context.target (or old_string) is required")
	}
	if oldString != "" && target != oldString {
		return "", 0, structuredEditError("INVALID_SELECTOR", 0, "anchor_context.target conflicts with old_string")
	}
	anchor := before + target + after
	if anchor == "" {
		return "", 0, structuredEditError("INVALID_SELECTOR", 0, "anchor_context cannot be empty")
	}
	total := strings.Count(original, anchor)
	if total == 0 {
		return "", 0, structuredEditError("NOT_FOUND", 0, "anchor_context not found; check before/target/after")
	}
	if total > 1 {
		return "", 0, structuredEditError("ANCHOR_NOT_UNIQUE", total, "add more before/after context to make anchor unique")
	}
	pos := strings.Index(original, anchor)
	if pos < 0 {
		return "", 0, structuredEditError("NOT_FOUND", 0, "anchor_context not found")
	}
	targetStart := pos + len(before)
	targetEnd := targetStart + len(target)
	updated := original[:targetStart] + newString + original[targetEnd:]
	return updated, 1, nil
}

func normalizeMatchText(s string) string {
	if s == "" {
		return s
	}
	lines := strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	for i := range lines {
		lines[i] = readPrefixPattern.ReplaceAllString(lines[i], "")
	}
	return strings.Join(lines, "\n")
}

func countMatches(original, oldString string) int {
	if oldString == "" {
		return 0
	}
	return strings.Count(original, oldString)
}

func structuredEditError(reason string, candidateCount int, suggested string) error {
	parts := []string{"edit selection failed"}
	parts = append(parts, "reason_code="+reason)
	if candidateCount > 0 {
		parts = append(parts, "candidate_count="+strconv.Itoa(candidateCount))
	}
	if strings.TrimSpace(suggested) != "" {
		parts = append(parts, "suggested_next_action="+suggested)
	}
	return errors.New(strings.Join(parts, "; "))
}
