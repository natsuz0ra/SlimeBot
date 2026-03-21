package mcp

import "testing"

func TestParseAndValidateConfig_Stdio(t *testing.T) {
	cfg, err := ParseAndValidateConfig(`{"command":"python","args":["-m","demo"]}`)
	if err != nil {
		t.Fatalf("expected nil err, got %v", err)
	}
	if cfg.Transport != "stdio" {
		t.Fatalf("expected stdio transport, got %s", cfg.Transport)
	}
	if cfg.Command != "python" {
		t.Fatalf("unexpected command: %s", cfg.Command)
	}
}

func TestParseAndValidateConfig_HTTP(t *testing.T) {
	cfg, err := ParseAndValidateConfig(`{"transport":"streamable_http","url":"https://example.com/mcp"}`)
	if err != nil {
		t.Fatalf("expected nil err, got %v", err)
	}
	if cfg.URL == "" {
		t.Fatalf("expected url not empty")
	}
}
