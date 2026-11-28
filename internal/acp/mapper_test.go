package acp

import (
	"testing"

	sdk "github.com/coder/acp-go-sdk"
)

func TestContentText(t *testing.T) {
	text := sdk.TextBlock("hello")
	if got := ContentText(text); got != "hello" {
		t.Fatalf("text block = %q", got)
	}
	link := sdk.ResourceLinkBlock("notes.md", "file:///notes.md")
	if got := ContentText(link); got != "notes.md (file:///notes.md)" {
		t.Fatalf("resource link = %q", got)
	}
	embedded := sdk.ResourceBlock(sdk.EmbeddedResourceResource{TextResourceContents: &sdk.TextResourceContents{
		Uri: "file:///main.go", Text: "package main",
	}})
	if got := ContentText(embedded); got != "file:///main.go\npackage main" {
		t.Fatalf("embedded text resource = %q", got)
	}
	blob := sdk.ResourceBlock(sdk.EmbeddedResourceResource{BlobResourceContents: &sdk.BlobResourceContents{
		Uri: "file:///logo.png", Blob: "AA==",
	}})
	if got := ContentText(blob); got != "file:///logo.png" {
		t.Fatalf("blob resource = %q", got)
	}
	if got := ContentText(sdk.ContentBlock{}); got != "" {
		t.Fatalf("empty block = %q", got)
	}
}

func TestToolKind(t *testing.T) {
	cases := []struct {
		name, command string
		want          sdk.ToolKind
	}{
		{"apply_patch", "", sdk.ToolKindEdit},
		{"", "write_file", sdk.ToolKindEdit},
		{"read_file", "", sdk.ToolKindRead},
		{"", "list_directory", sdk.ToolKindRead},
		{"grep", "", sdk.ToolKindSearch},
		{"", "search_web", sdk.ToolKindSearch},
		{"exec", "", sdk.ToolKindExecute},
		{"", "shell command", sdk.ToolKindExecute},
		{"ask_question", "", sdk.ToolKindOther},
	}
	for _, tc := range cases {
		if got := ToolKind(tc.name, tc.command); got != tc.want {
			t.Fatalf("ToolKind(%q, %q) = %q, want %q", tc.name, tc.command, got, tc.want)
		}
	}
}

func TestSelectPermission(t *testing.T) {
	options := []sdk.PermissionOption{
		{OptionId: "reject-always", Kind: sdk.PermissionOptionKindRejectAlways},
		{OptionId: "reject", Kind: sdk.PermissionOptionKindRejectOnce},
		{OptionId: "allow", Kind: sdk.PermissionOptionKindAllowOnce},
		{OptionId: "allow-always", Kind: sdk.PermissionOptionKindAllowAlways},
	}
	allow := SelectPermission(options, true)
	if allow.Outcome.Selected == nil || allow.Outcome.Selected.OptionId != "allow" {
		t.Fatalf("allow selection = %+v", allow.Outcome)
	}
	reject := SelectPermission(options, false)
	if reject.Outcome.Selected == nil || reject.Outcome.Selected.OptionId != "reject" {
		t.Fatalf("reject selection = %+v", reject.Outcome)
	}
	allowAlways := SelectPermission([]sdk.PermissionOption{{OptionId: "always", Kind: sdk.PermissionOptionKindAllowAlways}}, true)
	if allowAlways.Outcome.Selected == nil || allowAlways.Outcome.Selected.OptionId != "always" {
		t.Fatalf("allow-always fallback = %+v", allowAlways.Outcome)
	}
	rejectAlways := SelectPermission([]sdk.PermissionOption{{OptionId: "never", Kind: sdk.PermissionOptionKindRejectAlways}}, false)
	if rejectAlways.Outcome.Selected == nil || rejectAlways.Outcome.Selected.OptionId != "never" {
		t.Fatalf("reject-always fallback = %+v", rejectAlways.Outcome)
	}
	none := SelectPermission(nil, true)
	if none.Outcome.Cancelled == nil {
		t.Fatalf("expected cancelled outcome, got %+v", none.Outcome)
	}
	if cancelled := CancelledPermission(); cancelled.Outcome.Cancelled == nil {
		t.Fatalf("expected cancelled outcome")
	}
}
