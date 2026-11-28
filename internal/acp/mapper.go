package acp

import (
	"strings"

	sdk "github.com/coder/acp-go-sdk"
)

func ContentText(block sdk.ContentBlock) string {
	switch {
	case block.Text != nil:
		return block.Text.Text
	case block.ResourceLink != nil:
		return block.ResourceLink.Name + " (" + block.ResourceLink.Uri + ")"
	case block.Resource != nil && block.Resource.Resource.TextResourceContents != nil:
		resource := block.Resource.Resource.TextResourceContents
		return resource.Uri + "\n" + resource.Text
	case block.Resource != nil && block.Resource.Resource.BlobResourceContents != nil:
		return block.Resource.Resource.BlobResourceContents.Uri
	default:
		return ""
	}
}

func ToolKind(name, command string) sdk.ToolKind {
	value := strings.ToLower(command + " " + name)
	switch {
	case strings.Contains(value, "apply_patch"), strings.Contains(value, "edit"), strings.Contains(value, "write"):
		return sdk.ToolKindEdit
	case strings.Contains(value, "read"), strings.Contains(value, "list"):
		return sdk.ToolKindRead
	case strings.Contains(value, "search"), strings.Contains(value, "grep"):
		return sdk.ToolKindSearch
	case strings.Contains(value, "exec"), strings.Contains(value, "shell"):
		return sdk.ToolKindExecute
	default:
		return sdk.ToolKindOther
	}
}

func CancelledPermission() sdk.RequestPermissionResponse {
	return sdk.RequestPermissionResponse{Outcome: sdk.RequestPermissionOutcome{Cancelled: &sdk.RequestPermissionOutcomeCancelled{Outcome: "cancelled"}}}
}

func SelectPermission(options []sdk.PermissionOption, allow bool) sdk.RequestPermissionResponse {
	preferred, alternate := sdk.PermissionOptionKindRejectOnce, sdk.PermissionOptionKindRejectAlways
	if allow {
		preferred, alternate = sdk.PermissionOptionKindAllowOnce, sdk.PermissionOptionKindAllowAlways
	}
	for _, kind := range []sdk.PermissionOptionKind{preferred, alternate} {
		for _, option := range options {
			if option.Kind == kind {
				return sdk.RequestPermissionResponse{Outcome: sdk.RequestPermissionOutcome{
					Selected: &sdk.RequestPermissionOutcomeSelected{Outcome: "selected", OptionId: option.OptionId},
				}}
			}
		}
	}
	return CancelledPermission()
}
