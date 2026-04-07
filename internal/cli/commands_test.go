package cli

import (
	"testing"

	"slimebot/internal/domain"
)

func TestMatchCommandHints(t *testing.T) {
	got := MatchCommandHints("/m")
	if len(got) != 2 || got[0].Command != "/model" || got[1].Command != "/mcp" {
		t.Fatalf("unexpected hints: %#v", got)
	}
}

func TestMatchCommandHintsSlashShowsTopFive(t *testing.T) {
	got := MatchCommandHints("/")
	if len(got) != 5 {
		t.Fatalf("expected 5 hints, got=%d (%#v)", len(got), got)
	}
}

func TestMatchCommandHintsHaveDescriptions(t *testing.T) {
	got := MatchCommandHints("/")
	if len(got) == 0 {
		t.Fatal("expected hints")
	}
	for _, item := range got {
		if item.Command == "" || item.Description == "" {
			t.Fatalf("expected command and description, got=%#v", item)
		}
	}
}

func TestCompleteCommand(t *testing.T) {
	full, ok := CompleteCommand("/se")
	if !ok {
		t.Fatal("expected completion")
	}
	if full != "/session" {
		t.Fatalf("unexpected completion: %s", full)
	}

	_, ok = CompleteCommand("/x")
	if ok {
		t.Fatal("expected no completion")
	}
}

type fakeSessionCreator struct {
	created int
	id      string
}

func (f *fakeSessionCreator) Create(_ string) (*domain.Session, error) {
	f.created++
	return &domain.Session{ID: f.id}, nil
}

func TestEnsureSessionIDLazyCreate(t *testing.T) {
	creator := &fakeSessionCreator{id: "s-1"}

	id, err := EnsureSessionID("", creator)
	if err != nil {
		t.Fatalf("ensure session failed: %v", err)
	}
	if id != "s-1" || creator.created != 1 {
		t.Fatalf("unexpected create result: id=%s created=%d", id, creator.created)
	}

	id, err = EnsureSessionID("existing", creator)
	if err != nil {
		t.Fatalf("ensure existing failed: %v", err)
	}
	if id != "existing" || creator.created != 1 {
		t.Fatalf("unexpected existing result: id=%s created=%d", id, creator.created)
	}
}
