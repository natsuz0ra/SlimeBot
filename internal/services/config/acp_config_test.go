package config

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"slimebot/internal/apperrors"
	"slimebot/internal/domain"
)

type acpConfigStoreStub struct {
	items   map[string]domain.ACPConfig
	updated int
}

func newACPConfigStoreStub() *acpConfigStoreStub {
	return &acpConfigStoreStub{items: make(map[string]domain.ACPConfig)}
}

func (s *acpConfigStoreStub) ListACPConfigs(context.Context) ([]domain.ACPConfig, error) {
	ids := make([]string, 0, len(s.items))
	for id := range s.items {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	items := make([]domain.ACPConfig, 0, len(ids))
	for _, id := range ids {
		items = append(items, s.items[id])
	}
	return items, nil
}

func (s *acpConfigStoreStub) GetACPConfig(_ context.Context, id string) (domain.ACPConfig, error) {
	item, ok := s.items[id]
	if !ok {
		return domain.ACPConfig{}, apperrors.ErrNotFound
	}
	return item, nil
}

func (s *acpConfigStoreStub) CreateACPConfig(_ context.Context, value *domain.ACPConfig) error {
	now := time.Now().UTC()
	value.CreatedAt, value.UpdatedAt = now, now
	s.items[value.ID] = *value
	return nil
}

func (s *acpConfigStoreStub) UpdateACPConfig(_ context.Context, value *domain.ACPConfig) error {
	current, ok := s.items[value.ID]
	if !ok {
		return apperrors.ErrNotFound
	}
	if !current.UpdatedAt.Equal(value.UpdatedAt) {
		return apperrors.ErrInvalidInput
	}
	s.updated++
	value.UpdatedAt = current.UpdatedAt.Add(time.Second)
	s.items[value.ID] = *value
	return nil
}

func (s *acpConfigStoreStub) DeleteACPConfig(_ context.Context, id string) error {
	if _, ok := s.items[id]; !ok {
		return apperrors.ErrNotFound
	}
	delete(s.items, id)
	return nil
}

func acpPtr[T any](value T) *T { return &value }

func decodeJSON(raw string, target any) error { return json.Unmarshal([]byte(raw), target) }

func TestACPConfigServiceCreateMasksSecrets(t *testing.T) {
	store := newACPConfigStoreStub()
	service := NewACPConfigService(store)
	ctx := context.Background()
	name, command := "Zed Agent", "npx"
	args := []string{"-y", "@zed-industries/claude-code-acp"}
	created, err := service.Create(ctx, ACPConfigInput{
		Name: &name, Command: &command, Args: &args,
		Env: map[string]string{"ZED_TOKEN": "top-secret", "PLAIN": "value"},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.EnvJSON != "" || len(created.EnvKeys) != 2 {
		t.Fatalf("created config leaked environment values: %+v", created)
	}
	if !slices.Equal(created.EnvKeys, []string{"PLAIN", "ZED_TOKEN"}) {
		t.Fatalf("env keys = %v", created.EnvKeys)
	}
	if !slices.Equal(created.Args, args) {
		t.Fatalf("args = %v", created.Args)
	}
	if created.TimeoutSec != 300 || !created.IsEnabled {
		t.Fatalf("defaults = timeout %d enabled %v", created.TimeoutSec, created.IsEnabled)
	}
	stored := store.items[created.ID]
	if !strings.Contains(stored.EnvJSON, "top-secret") {
		t.Fatalf("stored env should keep the secret: %q", stored.EnvJSON)
	}

	listed, err := service.List(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(listed) != 1 || listed[0].EnvJSON != "" || len(listed[0].EnvKeys) != 2 {
		t.Fatalf("list leaked secrets: %+v", listed)
	}
}

func TestACPConfigServiceRejectsInvalidInput(t *testing.T) {
	service := NewACPConfigService(newACPConfigStoreStub())
	ctx := context.Background()
	empty, command, timeout := "", "npx", 0
	short := "a"
	tooLong := strings.Repeat("a", 129)
	badCwd := "relative/not/absolute"
	cases := map[string]ACPConfigInput{
		"empty name":        {Name: &empty, Command: &command},
		"empty command":     {Name: &short, Command: &empty},
		"newline command":   {Name: &short, Command: acpPtr("bad\ncommand")},
		"timeout zero":      {Name: &short, Command: &command, TimeoutSec: &timeout},
		"invalid env key":   {Name: &short, Command: &command, Env: map[string]string{"BAD KEY": "x"}},
		"invalid cwd":       {Name: &short, Command: &command, DefaultCwd: &badCwd},
		"too long name":     {Name: &tooLong, Command: &command},
		"invalid arg value": {Name: &short, Command: &command, Args: &[]string{"bad\x00arg"}},
	}
	for label, input := range cases {
		if _, err := service.Create(ctx, input); !errors.Is(err, apperrors.ErrInvalidInput) {
			t.Fatalf("%s: expected invalid input, got %v", label, err)
		}
	}
}

func TestACPConfigServiceUpdateRetainsUntouchedSecrets(t *testing.T) {
	store := newACPConfigStoreStub()
	service := NewACPConfigService(store)
	ctx := context.Background()
	name, command := "Agent", "agent"
	created, err := service.Create(ctx, ACPConfigInput{
		Name: &name, Command: &command,
		Env: map[string]string{"KEEP": "keep-secret", "CHANGE": "old-secret", "DROP": "drop-secret"},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	disabled := false
	updated, err := service.Update(ctx, created.ID, ACPConfigInput{
		IsEnabled: &disabled,
		Env:       map[string]string{"CHANGE": "new-secret"},
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.IsEnabled {
		t.Fatal("expected disabled config")
	}
	if !slices.Equal(updated.EnvKeys, []string{"CHANGE", "DROP", "KEEP"}) {
		t.Fatalf("env keys after update = %v", updated.EnvKeys)
	}
	var stored map[string]string
	if err := decodeJSON(store.items[created.ID].EnvJSON, &stored); err != nil {
		t.Fatalf("decode stored env: %v", err)
	}
	if stored["KEEP"] != "keep-secret" || stored["CHANGE"] != "new-secret" || stored["DROP"] != "drop-secret" {
		t.Fatalf("stored env = %v", stored)
	}

	updated, err = service.Update(ctx, created.ID, ACPConfigInput{RemoveEnvKeys: []string{"DROP", "KEEP"}})
	if err != nil {
		t.Fatalf("remove env: %v", err)
	}
	if !slices.Equal(updated.EnvKeys, []string{"CHANGE"}) {
		t.Fatalf("env keys after removal = %v", updated.EnvKeys)
	}

	// Removing an unknown key and updating only the flag keeps the other secrets.
	enabled := true
	updated, err = service.Update(ctx, created.ID, ACPConfigInput{IsEnabled: &enabled, RemoveEnvKeys: []string{"MISSING"}})
	if err != nil {
		t.Fatalf("re-enable: %v", err)
	}
	if !updated.IsEnabled || !slices.Equal(updated.EnvKeys, []string{"CHANGE"}) {
		t.Fatalf("unexpected config after re-enable: %+v", updated)
	}
	if got := store.items[created.ID].EnvJSON; !strings.Contains(got, "new-secret") {
		t.Fatalf("stored env lost value: %q", got)
	}
}

func TestACPProcessValuesHandlesEmptyJSON(t *testing.T) {
	args, env, err := ACPProcessValues(domain.ACPConfig{})
	if err != nil {
		t.Fatalf("empty config: %v", err)
	}
	if len(args) != 0 || len(env) != 0 || args == nil || env == nil {
		t.Fatalf("args=%v env=%v", args, env)
	}
	if _, _, err := ACPProcessValues(domain.ACPConfig{ArgsJSON: "not-json"}); err == nil {
		t.Fatal("expected invalid args json error")
	}
}
