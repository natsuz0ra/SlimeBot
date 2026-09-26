package chat

import (
	"context"
	"testing"

	"slimebot/internal/constants"
)

type sandboxSettingsStore struct {
	values map[string]string
}

func (s sandboxSettingsStore) GetSetting(_ context.Context, key string) (string, error) {
	return s.values[key], nil
}

func (s sandboxSettingsStore) SetSetting(_ context.Context, key, value string) error {
	s.values[key] = value
	return nil
}

func TestResolveSandboxPolicyUsesCLISandboxSettingsForCLIContext(t *testing.T) {
	svc := &ChatService{settingsStore: sandboxSettingsStore{values: map[string]string{
		constants.SettingSandboxMode:              "danger-full-access",
		constants.SettingSandboxNetworkEnabled:    "true",
		constants.SettingCLISandboxMode:           "workspace-write",
		constants.SettingCLISandboxNetworkEnabled: "false",
	}}}
	ctx := constants.WithClientSurface(context.Background(), constants.ClientSurfaceCLI)

	policy, err := svc.resolveSandboxPolicy(ctx)
	if err != nil {
		t.Fatalf("resolve sandbox policy: %v", err)
	}
	if got := string(policy.Mode()); got != "workspace-write" {
		t.Fatalf("policy mode = %q, want workspace-write", got)
	}
	if policy.Network().Enabled {
		t.Fatal("CLI sandbox network should use CLI network setting")
	}
}

func TestResolveSandboxPolicyUsesWebSandboxSettingsByDefault(t *testing.T) {
	svc := &ChatService{settingsStore: sandboxSettingsStore{values: map[string]string{
		constants.SettingSandboxMode:              "danger-full-access",
		constants.SettingSandboxNetworkEnabled:    "true",
		constants.SettingCLISandboxMode:           "workspace-write",
		constants.SettingCLISandboxNetworkEnabled: "false",
	}}}

	policy, err := svc.resolveSandboxPolicy(context.Background())
	if err != nil {
		t.Fatalf("resolve sandbox policy: %v", err)
	}
	if got := string(policy.Mode()); got != "danger-full-access" {
		t.Fatalf("policy mode = %q, want danger-full-access", got)
	}
	if !policy.Network().Enabled {
		t.Fatal("web/default sandbox network should use web network setting")
	}
}

func TestResolveSandboxPolicyUsesWebSessionDirectory(t *testing.T) {
	svc := &ChatService{}
	directory := t.TempDir()
	ctx := constants.WithClientSurface(context.Background(), constants.ClientSurfaceWeb)
	policy, err := svc.resolveSandboxPolicy(ctx, directory)
	if err != nil {
		t.Fatal(err)
	}
	if policy.CWD() != directory {
		t.Fatalf("cwd=%q, want %q", policy.CWD(), directory)
	}
	cliCtx := constants.WithClientSurface(context.Background(), constants.ClientSurfaceCLI)
	cliPolicy, err := svc.resolveSandboxPolicy(cliCtx, directory)
	if err != nil {
		t.Fatal(err)
	}
	if cliPolicy.CWD() == directory {
		t.Fatal("CLI cwd changed by web session directory")
	}
}
