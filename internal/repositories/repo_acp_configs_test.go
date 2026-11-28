package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"slimebot/internal/apperrors"
	"slimebot/internal/domain"
)

func TestACPConfigCRUD(t *testing.T) {
	repo := New(NewSQLiteDBTest(t, "acp_repo"))
	ctx := context.Background()

	first := &domain.ACPConfig{
		ID: "cfg-a", Name: "Zed", Command: "npx",
		ArgsJSON: `["-y","agent"]`, EnvJSON: `{"TOKEN":"secret"}`,
		TimeoutSec: 300, IsEnabled: true,
	}
	second := &domain.ACPConfig{
		ID: "cfg-b", Name: "Another", Command: "agent",
		ArgsJSON: `[]`, EnvJSON: `{}`, TimeoutSec: 120, IsEnabled: false,
	}
	for _, item := range []*domain.ACPConfig{first, second} {
		if err := repo.CreateACPConfig(ctx, item); err != nil {
			t.Fatalf("create %s: %v", item.ID, err)
		}
	}

	items, err := repo.ListACPConfigs(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(items) != 2 || items[0].ID != "cfg-b" || items[1].ID != "cfg-a" {
		t.Fatalf("expected name ordering, got %+v", items)
	}

	loaded, err := repo.GetACPConfig(ctx, "cfg-a")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if loaded.ArgsJSON != `["-y","agent"]` || loaded.EnvJSON != `{"TOKEN":"secret"}` {
		t.Fatalf("json round trip failed: %+v", loaded)
	}

	stale := loaded
	time.Sleep(2 * time.Millisecond)
	loaded.Name = "Zed Updated"
	loaded.IsEnabled = false
	loaded.TimeoutSec = 600
	if err := repo.UpdateACPConfig(ctx, &loaded); err != nil {
		t.Fatalf("update: %v", err)
	}
	if err := repo.UpdateACPConfig(ctx, &stale); !errors.Is(err, apperrors.ErrInvalidInput) {
		t.Fatalf("stale update error = %v", err)
	}

	reloaded, err := repo.GetACPConfig(ctx, "cfg-a")
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.Name != "Zed Updated" || reloaded.IsEnabled || reloaded.TimeoutSec != 600 || !reloaded.UpdatedAt.After(stale.UpdatedAt) {
		t.Fatalf("unexpected reloaded config: %+v", reloaded)
	}

	if err := repo.DeleteACPConfig(ctx, "cfg-a"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := repo.GetACPConfig(ctx, "cfg-a"); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("get after delete error = %v", err)
	}
	if err := repo.DeleteACPConfig(ctx, "cfg-a"); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("second delete error = %v", err)
	}
}

func TestACPConfigSessionBindingLifecycle(t *testing.T) {
	repo := New(NewSQLiteDBTest(t, "acp_binding"))
	ctx := context.Background()
	session, err := repo.CreateSession(ctx, "chat", t.TempDir())
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	config := &domain.ACPConfig{ID: "cfg-bind", Name: "Agent", Command: "agent", ArgsJSON: `[]`, EnvJSON: `{}`, TimeoutSec: 300, IsEnabled: false}
	if err := repo.CreateACPConfig(ctx, config); err != nil {
		t.Fatalf("create config: %v", err)
	}
	agentID := config.ID
	if err := repo.SetSessionACP(ctx, session.ID, &agentID, "remote-1"); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("binding a disabled agent should fail, got %v", err)
	}
	config.IsEnabled = true
	if err := repo.UpdateACPConfig(ctx, config); err != nil {
		t.Fatalf("enable config: %v", err)
	}
	if err := repo.SetSessionACP(ctx, session.ID, &agentID, "remote-1"); err != nil {
		t.Fatalf("bind: %v", err)
	}
	bound, err := repo.GetSessionByID(ctx, session.ID)
	if err != nil {
		t.Fatalf("get bound session: %v", err)
	}
	if bound.ACPAgentID == nil || *bound.ACPAgentID != config.ID || bound.ACPRemoteSessionID != "remote-1" {
		t.Fatalf("binding not persisted: %+v", bound)
	}

	if err := repo.DeleteACPConfig(ctx, config.ID); err != nil {
		t.Fatalf("delete config: %v", err)
	}
	unbound, err := repo.GetSessionByID(ctx, session.ID)
	if err != nil {
		t.Fatalf("get unbound session: %v", err)
	}
	if unbound.ACPAgentID != nil || unbound.ACPRemoteSessionID != "" {
		t.Fatalf("binding not cleared after delete: %+v", unbound)
	}

	if err := repo.SetSessionACP(ctx, "missing-session", &agentID, ""); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("missing session error = %v", err)
	}
}
