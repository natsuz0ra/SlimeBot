package memory

import (
	"errors"
	"slimebot/internal/domain"
)

type Target = domain.MemoryTarget

const (
	TargetMemory = domain.MemoryTargetAssistant
	TargetUser   = domain.MemoryTargetUser
)

const entrySeparator = "\n§\n"

var (
	ErrInvalidTarget        = domain.ErrInvalidMemoryTarget
	ErrEmptyEntry           = errors.New("memory entry is empty")
	ErrDuplicateEntry       = errors.New("memory entry already exists")
	ErrEntryNotFound        = errors.New("memory entry not found")
	ErrAmbiguousMatch       = errors.New("memory entry match is ambiguous")
	ErrMemoryLimitExceeded  = errors.New("memory character limit exceeded")
	ErrUnsafeMemory         = errors.New("memory entry failed safety scan")
	ErrMemoryDisabled       = errors.New("memory is disabled")
	ErrUserProfileDisabled  = errors.New("user profile memory is disabled")
	ErrInvalidMemorySetting = errors.New("invalid memory setting")
)

type Config struct {
	Enabled            bool
	UserProfileEnabled bool
	MemoryCharLimit    int
	UserCharLimit      int
	NudgeInterval      int
}

type TargetState = domain.MemoryTargetState
type Snapshot = domain.MemorySnapshot

func DefaultConfig() Config {
	return Config{
		Enabled:            true,
		UserProfileEnabled: true,
		MemoryCharLimit:    2200,
		UserCharLimit:      1375,
		NudgeInterval:      10,
	}
}

func NormalizeTarget(value string) (Target, error) {
	return domain.NormalizeMemoryTarget(value)
}

func targetLimit(cfg Config, target Target) int {
	if target == TargetUser {
		return cfg.UserCharLimit
	}
	return cfg.MemoryCharLimit
}

func targetEnabled(cfg Config, target Target) bool {
	if !cfg.Enabled {
		return false
	}
	if target == TargetUser {
		return cfg.UserProfileEnabled
	}
	return true
}
