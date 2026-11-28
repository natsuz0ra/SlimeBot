package domain

import (
	"errors"
	"strings"
)

var ErrInvalidMemoryTarget = errors.New("invalid memory target")

type MemoryTarget string

const (
	MemoryTargetAssistant MemoryTarget = "memory"
	MemoryTargetUser      MemoryTarget = "user"
)

type MemoryTargetState struct {
	Target     MemoryTarget `json:"target"`
	Entries    []string     `json:"entries"`
	UsageChars int          `json:"usageChars"`
	CharLimit  int          `json:"charLimit"`
	EntryCount int          `json:"entryCount"`
	Enabled    bool         `json:"enabled"`
}

type MemorySnapshot struct {
	Memory                   MemoryTargetState `json:"memory"`
	User                     MemoryTargetState `json:"user"`
	MemoryEnabled            bool              `json:"memoryEnabled"`
	MemoryUserProfileEnabled bool              `json:"memoryUserProfileEnabled"`
	MemoryNudgeInterval      int               `json:"memoryNudgeInterval"`
	MemoryDirectory          string            `json:"memoryDirectory"`
}

func NormalizeMemoryTarget(value string) (MemoryTarget, error) {
	switch MemoryTarget(strings.ToLower(strings.TrimSpace(value))) {
	case MemoryTargetAssistant:
		return MemoryTargetAssistant, nil
	case MemoryTargetUser:
		return MemoryTargetUser, nil
	default:
		return "", ErrInvalidMemoryTarget
	}
}
