package settings

import (
	"context"
	"errors"
	"strings"
	"testing"

	"slimebot/internal/apperrors"
)

func TestPromptTemplatesValidateAndPersistAtomically(t *testing.T) {
	ctx := context.Background()
	store := &memorySettingsStore{values: map[string]string{"language": "en-US"}}
	svc := NewSettingsService(store)
	defaults, err := svc.PromptTemplates(ctx)
	if err != nil || len(defaults) != 4 || defaults[0].Slash != "review" {
		t.Fatalf("defaults %+v, %v", defaults, err)
	}
	template := PromptTemplate{Slash: "my-review", Title: " Review ", Content: "Read {{cursor}} next"}
	saved, err := svc.UpdatePromptTemplates(ctx, []PromptTemplate{template})
	if err != nil || len(saved) != 1 || saved[0].ID == "" || saved[0].Title != "Review" {
		t.Fatalf("saved %+v, %v", saved, err)
	}
	for _, bad := range [][]PromptTemplate{
		{template, template},
		{{Slash: "help", Title: "Title", Content: "body"}},
		{{Slash: "UPPER", Title: "Title", Content: "body"}},
		{{Slash: "big", Title: "Title", Content: strings.Repeat("x", 16385)}},
		{{Slash: "two", Title: "Title", Content: "{{cursor}}{{cursor}}"}},
	} {
		if _, err := svc.UpdatePromptTemplates(ctx, bad); !errors.Is(err, apperrors.ErrInvalidInput) {
			t.Fatalf("invalid template accepted: %v", err)
		}
	}
	loaded, err := svc.PromptTemplates(ctx)
	if err != nil || len(loaded) != 1 || loaded[0] != saved[0] {
		t.Fatalf("failed validation changed saved templates: %+v, %v", loaded, err)
	}
}
