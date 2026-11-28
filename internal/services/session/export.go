package session

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"slimebot/internal/apperrors"
	"slimebot/internal/constants"
	"slimebot/internal/domain"
)

type ExportOptions struct {
	Format          string
	IncludeTools    bool
	IncludeThinking bool
}

type ExportFile struct {
	Name      string
	MediaType string
	Data      []byte
}

type exportMessage struct {
	domain.Message
	ToolCalls []ToolCallHistory `json:"toolCalls,omitempty"`
	Thinking  []ThinkingHistory `json:"thinking,omitempty"`
}

type sessionExport struct {
	SchemaVersion int             `json:"schemaVersion"`
	ExportedAt    time.Time       `json:"exportedAt"`
	Session       domain.Session  `json:"session"`
	Messages      []exportMessage `json:"messages"`
}

var exportNameSeparators = regexp.MustCompile(`[^\pL\pN_-]+`)

func (s *SessionService) ReplaySession(ctx context.Context, id string) (domain.SessionExportSnapshot, error) {
	return s.store.ReadSessionExport(ctx, id, true, true)
}

func (s *SessionService) SetPinned(ctx context.Context, id string, pinned bool) (*domain.Session, error) {
	if constants.IsMessagePlatformSessionID(id) {
		return nil, apperrors.ErrInvalidInput
	}
	if err := s.store.SetSessionPinned(ctx, id, pinned); err != nil {
		return nil, err
	}
	return s.Get(ctx, id)
}

func (s *SessionService) Export(ctx context.Context, id string, opts ExportOptions) (ExportFile, error) {
	if opts.Format != "json" && opts.Format != "markdown" {
		return ExportFile{}, apperrors.ErrInvalidInput
	}
	snapshot, err := s.store.ReadSessionExport(ctx, id, opts.IncludeTools, opts.IncludeThinking)
	if err != nil {
		return ExportFile{}, err
	}
	ids := make(map[string]struct{}, len(snapshot.Messages))
	interrupted := make(map[string]struct{})
	for _, message := range snapshot.Messages {
		ids[message.ID] = struct{}{}
		if message.Role == "assistant" && message.IsInterrupted {
			interrupted[message.ID] = struct{}{}
		}
	}
	tools := buildToolCallHistory(snapshot.ToolCalls, ids, interrupted)
	thinking := buildThinkingHistory(snapshot.Thinking, ids, interrupted)
	export := sessionExport{
		SchemaVersion: 1, ExportedAt: time.Now().UTC(), Session: snapshot.Session,
		Messages: make([]exportMessage, 0, len(snapshot.Messages)),
	}
	for _, message := range snapshot.Messages {
		message.Content = domain.StripContentMarkers(message.Content)
		export.Messages = append(export.Messages, exportMessage{message, tools[message.ID], thinking[message.ID]})
	}
	stem := strings.Trim(exportNameSeparators.ReplaceAllString(snapshot.Session.Name, "-"), "-_")
	if stem == "" {
		stem = "chat"
	}
	filename := "slimebot-" + trimUTF8(stem, 80) + "-" + export.ExportedAt.Format("20060102")
	if opts.Format == "json" {
		data, err := json.MarshalIndent(export, "", "  ")
		return ExportFile{filename + ".json", "application/json", data}, err
	}
	return ExportFile{filename + ".md", "text/markdown; charset=utf-8", []byte(export.markdown())}, nil
}

func (export sessionExport) markdown() string {
	var result strings.Builder
	title := strings.Join(strings.Fields(export.Session.Name), " ")
	fmt.Fprintf(&result, "# %s\n\nExported %s\n", title, export.ExportedAt.Format(time.RFC3339))
	for _, message := range export.Messages {
		fmt.Fprintf(&result, "\n## %s · %s\n\n%s\n", message.Role, message.CreatedAt.Format(time.RFC3339), message.Content)
		for _, attachment := range message.Attachments {
			fmt.Fprintf(&result, "\n- Attachment: %s (%d bytes)\n", strings.Join(strings.Fields(attachment.Name), " "), attachment.SizeBytes)
		}
		for _, block := range message.Thinking {
			result.WriteString("\n<details><summary>Thinking</summary>\n\n")
			writeExportFence(&result, "text", block.Content)
			result.WriteString("\n</details>\n")
		}
		for _, tool := range message.ToolCalls {
			fmt.Fprintf(&result, "\n### %s (%s)\n\n", strings.Join(strings.Fields(tool.ToolName), " "), tool.Status)
			if len(tool.Params) > 0 {
				data, _ := json.MarshalIndent(tool.Params, "", "  ")
				writeExportFence(&result, "json", string(data))
			}
			output := strings.TrimSpace(strings.Join([]string{tool.Output, tool.Error}, "\n"))
			if output != "" {
				short := trimUTF8(output, 32<<10)
				if short != output {
					short += "\n[truncated]"
				}
				writeExportFence(&result, "text", short)
			}
		}
	}
	return result.String()
}

func trimUTF8(text string, maxBytes int) string {
	if len(text) <= maxBytes {
		return text
	}
	for maxBytes > 0 && !utf8.RuneStart(text[maxBytes]) {
		maxBytes--
	}
	return text[:maxBytes]
}

func writeExportFence(out *strings.Builder, lang, content string) {
	longest, current := 2, 0
	for _, r := range content {
		if r == '`' {
			current++
			longest = max(longest, current)
		} else {
			current = 0
		}
	}
	fence := strings.Repeat("`", longest+1)
	fmt.Fprintf(out, "%s%s\n%s\n%s\n", fence, lang, content, fence)
}
