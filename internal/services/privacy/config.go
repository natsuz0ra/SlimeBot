// Package privacy masks model request content locally and restores replies.
package privacy

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

const settingKey = "privacy_config"

const (
	defaultMappingTTLMinutes = 24 * 60
	maxMappingTTLMinutes     = 30 * 24 * 60
)

type Action string

const (
	Mask  Action = "mask"
	Warn  Action = "warn"
	Block Action = "block"
)

type Rule struct {
	Kind    string `json:"kind"`
	Prefix  string `json:"prefix,omitempty"`
	Pattern string `json:"pattern,omitempty"`
}

type Config struct {
	Enabled             bool              `json:"enabled"`
	Secrets             bool              `json:"secrets"`
	Personal            bool              `json:"personal"`
	Kinds               map[string]bool   `json:"kinds"`
	Words               []string          `json:"words"`
	IgnoreCase          bool              `json:"ignoreCase"`
	WholeWords          bool              `json:"wholeWords"`
	Rules               []Rule            `json:"rules"`
	Allow               []string          `json:"allow"`
	Actions             map[string]Action `json:"actions"`
	BypassProviderIDs   []string          `json:"bypassProviderIds"`
	ModelEnabled        bool              `json:"modelEnabled"`
	ModelThreshold      float64           `json:"modelThreshold"`
	ModelTimeoutSeconds int               `json:"modelTimeoutSeconds"`
	MappingPersistence  bool              `json:"mappingPersistence"`
	MappingTTLMinutes   int               `json:"mappingTtlMinutes"`
}

func DefaultConfig() Config {
	return Config{Secrets: true, Personal: true, Kinds: map[string]bool{}, Words: []string{}, Rules: []Rule{}, Allow: []string{}, Actions: map[string]Action{}, BypassProviderIDs: []string{}, ModelThreshold: 0.8, ModelTimeoutSeconds: 15, MappingTTLMinutes: defaultMappingTTLMinutes}
}

var kindPattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,23}$`)

func (c Config) Validate() error {
	if len(c.Kinds) > 64 || len(c.Actions) > 64 {
		return errors.New("too many privacy categories")
	}
	if c.ModelThreshold < 0.5 || c.ModelThreshold > 1 {
		return errors.New("modelThreshold must be between 0.5 and 1")
	}
	if c.ModelTimeoutSeconds < 1 || c.ModelTimeoutSeconds > 120 {
		return errors.New("modelTimeoutSeconds must be between 1 and 120")
	}
	if c.MappingTTLMinutes < 1 || c.MappingTTLMinutes > maxMappingTTLMinutes {
		return fmt.Errorf("mappingTtlMinutes must be between 1 and %d", maxMappingTTLMinutes)
	}
	if len(c.Rules) > 32 || len(c.Words) > 256 || len(c.Allow) > 256 || len(c.BypassProviderIDs) > 256 {
		return errors.New("too many privacy rules or values")
	}
	for _, values := range [][]string{c.Words, c.Allow, c.BypassProviderIDs} {
		for _, value := range values {
			if strings.TrimSpace(value) == "" || len(value) > 4096 {
				return errors.New("privacy values must contain 1 to 4096 bytes")
			}
		}
	}
	for kind, action := range c.Actions {
		if !kindPattern.MatchString(kind) || (action != Mask && action != Warn && action != Block) {
			return errors.New("invalid privacy action")
		}
	}
	for i, r := range c.Rules {
		if !kindPattern.MatchString(r.Kind) || (r.Prefix == "" && r.Pattern == "") || len(r.Prefix) > 128 || len(r.Pattern) > 1024 {
			return fmt.Errorf("invalid privacy rule %d", i+1)
		}
		if r.Pattern != "" {
			re, err := regexp.Compile(r.Pattern)
			if err != nil || re.MatchString("") {
				return fmt.Errorf("invalid or empty-matching expression in privacy rule %d", i+1)
			}
		}
	}
	return nil
}

func (c Config) normalized() Config {
	if c.Kinds == nil {
		c.Kinds = map[string]bool{}
	}
	if c.Actions == nil {
		c.Actions = map[string]Action{}
	}
	if c.Words == nil {
		c.Words = []string{}
	}
	if c.Allow == nil {
		c.Allow = []string{}
	}
	if c.Rules == nil {
		c.Rules = []Rule{}
	}
	if c.BypassProviderIDs == nil {
		c.BypassProviderIDs = []string{}
	}
	return c
}

func (c Config) action(kind string) Action {
	if a := c.Actions[kind]; a != "" {
		return a
	}
	return Mask
}
func (c Config) allows(value string) bool {
	for _, v := range c.Allow {
		if strings.EqualFold(value, v) {
			return true
		}
	}
	return false
}

func (c Config) covers(kind string) bool {
	if enabled, ok := c.Kinds[kind]; ok && !enabled {
		return false
	}
	switch kind {
	case "TERM":
		return true
	case "API_KEY", "PRIVATE_KEY", "TOKEN", "PASSWORD", "SECRET":
		return c.Secrets
	case "EMAIL", "PHONE", "ID_CARD", "BANK_CARD", "PERSON", "ADDRESS", "ACCOUNT":
		return c.Personal
	case "SSN", "PASSPORT", "IBAN", "BIRTHDAY", "MAC", "SERIAL", "USERNAME", "HOSTNAME", "IP", "BUCKET":
		return c.Personal && c.Kinds[kind]
	default:
		return c.Secrets
	}
}
