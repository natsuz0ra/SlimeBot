package settings

import (
	"context"
	"testing"
)

func TestTerminalPolicyAndEnvironmentOverride(t *testing.T) {
	ctx := context.Background()
	t.Setenv("SLIMEBOT_TERMINAL_ENABLED", "")
	svc := NewSettingsService(&memorySettingsStore{values: map[string]string{}})
	if settings, err := svc.Terminal(ctx); err != nil || !settings.Enabled || settings.LockedByEnv {
		t.Fatalf("unexpected default %#v, %v", settings, err)
	}
	enabled, shell := true, "  pwsh.exe "
	if err := svc.UpdateTerminal(ctx, &enabled, &shell); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SLIMEBOT_TERMINAL_ENABLED", "FALSE")
	if settings, err := svc.Terminal(ctx); err != nil || settings.Enabled || !settings.LockedByEnv || settings.DefaultShell != "pwsh.exe" {
		t.Fatalf("unexpected policy %#v, %v", settings, err)
	}
}
