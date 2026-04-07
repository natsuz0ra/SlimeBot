package cli

import (
	"strings"

	"slimebot/internal/domain"
)

type CommandMeta struct {
	Command     string
	Description string
}

const maxCommandHints = 5

var supportedCommands = []CommandMeta{
	{Command: "/new", Description: "Create a new chat (lazy session creation)"},
	{Command: "/session", Description: "Browse, switch, or delete sessions"},
	{Command: "/model", Description: "Switch default model"},
	{Command: "/skills", Description: "Browse and delete installed skills"},
	{Command: "/mcp", Description: "Manage MCP configs"},
	{Command: "/help", Description: "Show available commands"},
}

func ListSupportedCommands() []CommandMeta {
	out := make([]CommandMeta, len(supportedCommands))
	copy(out, supportedCommands)
	return out
}

// MatchCommandHints returns prefix-matched command metadata in stable command order.
func MatchCommandHints(input string) []CommandMeta {
	trimmed := strings.TrimSpace(input)
	if !strings.HasPrefix(trimmed, "/") {
		return nil
	}
	matched := make([]CommandMeta, 0, len(supportedCommands))
	for _, cmd := range supportedCommands {
		if strings.HasPrefix(cmd.Command, trimmed) {
			matched = append(matched, cmd)
			if len(matched) == maxCommandHints {
				break
			}
		}
	}
	return matched
}

// CompleteCommand performs tab completion and returns the first prefix match.
func CompleteCommand(input string) (string, bool) {
	matched := MatchCommandHints(input)
	if len(matched) == 0 {
		return "", false
	}
	return matched[0].Command, true
}

type sessionCreator interface {
	Create(name string) (*domain.Session, error)
}

// EnsureSessionID creates a new session only when current id is empty.
func EnsureSessionID(current string, creator sessionCreator) (string, error) {
	if strings.TrimSpace(current) != "" {
		return current, nil
	}
	created, err := creator.Create("New Chat")
	if err != nil {
		return "", err
	}
	if created == nil {
		return "", nil
	}
	return strings.TrimSpace(created.ID), nil
}
