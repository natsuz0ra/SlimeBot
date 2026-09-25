package chat

import (
	"strings"
	"testing"

	llmsvc "slimebot/internal/services/llm"
)

func TestAppendToolOutcomesPlacesScreenshotsAfterToolResults(t *testing.T) {
	messages := appendToolOutcomes(nil, []parallelToolOutcome{
		{toolCallID: "call1", messageContent: "snapshot", imageURL: "data:image/jpeg;base64,abc"},
		{toolCallID: "call2", messageContent: "clicked"},
	})
	if len(messages) != 3 || messages[0].Role != "tool" || messages[1].Role != "tool" || messages[2].Role != "user" {
		t.Fatalf("unexpected message order: %#v", messages)
	}
	parts := messages[2].ContentParts
	if len(parts) != 2 || parts[1].Type != llmsvc.ChatMessageContentPartTypeImage || parts[1].ImageURL != "data:image/jpeg;base64,abc" {
		t.Fatalf("image was not forwarded: %#v", parts)
	}
	if !strings.Contains(parts[0].Text, "call1") {
		t.Fatalf("image lacks source tool call: %#v", parts)
	}
}
