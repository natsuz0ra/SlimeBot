package workspace

import (
	"bytes"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/pmezard/go-difflib/difflib"
)

type CheckpointDiff struct {
	Diff      string `json:"diff"`
	Binary    bool   `json:"binary"`
	Truncated bool   `json:"truncated"`
}

func (m *CheckpointManager) Diff(sessionID, id, path string) (CheckpointDiff, error) {
	record, err := m.record(sessionID, id)
	if err != nil {
		return CheckpointDiff{}, err
	}
	for _, f := range record.changes {
		if f.Path == path {
			return renderCheckpointDiff(f)
		}
	}
	return CheckpointDiff{}, ErrCheckpointMissing
}

func renderCheckpointDiff(f changedFile) (CheckpointDiff, error) {
	const byteLimit = 192 << 10
	const lineLimit = 4000
	var before, after []byte
	oldName, newName := "/dev/null", "/dev/null"
	if f.Before != nil {
		before, oldName = f.Before.Data, "a/"+f.Path
	}
	if f.After != nil {
		after, newName = f.After.Data, "b/"+f.Path
	}
	if !utf8.Valid(before) || !utf8.Valid(after) || bytes.IndexByte(before, 0) >= 0 || bytes.IndexByte(after, 0) >= 0 {
		return CheckpointDiff{Binary: true}, nil
	}
	// Bound diff CPU cost and the response size independently of saved file bytes.
	if len(before)+len(after) > byteLimit || bytes.Count(before, []byte{'\n'})+bytes.Count(after, []byte{'\n'}) > lineLimit {
		return CheckpointDiff{Truncated: true}, nil
	}
	var out strings.Builder
	if f.Before != nil && f.After != nil && f.Before.Mode != f.After.Mode {
		fmt.Fprintf(&out, "old mode %04o\nnew mode %04o\n", f.Before.Mode, f.After.Mode)
	}
	if bytes.Equal(before, after) && (f.Before == nil || f.After == nil) {
		fmt.Fprintf(&out, "--- %s\n+++ %s\n", oldName, newName)
	}
	err := difflib.WriteUnifiedDiff(&out, difflib.UnifiedDiff{
		A: diffLines(string(before)), B: diffLines(string(after)),
		FromFile: oldName, ToFile: newName, Context: 3,
	})
	if err != nil {
		return CheckpointDiff{}, err
	}
	text := out.String()
	truncated := len(text) > byteLimit
	if truncated {
		text = text[:strings.LastIndexByte(text[:byteLimit], '\n')+1]
	}
	return CheckpointDiff{Diff: text, Truncated: truncated}, nil
}

func diffLines(text string) []string {
	if text == "" {
		return nil
	}
	lines := strings.SplitAfter(text, "\n")
	if last := len(lines) - 1; lines[last] == "" {
		return lines[:last]
	} else {
		lines[last] += "\n\\ No newline at end of file\n"
	}
	return lines
}
