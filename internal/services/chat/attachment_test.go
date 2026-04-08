package chat

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	llmsvc "slimebot/internal/services/llm"
)

func TestBuildUserMessageContentParts_LargeDocumentFallsBackToMetadata(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "long.txt")
	raw := strings.Repeat("abcd", 80*1024) // > 200KB
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatalf("write file failed: %v", err)
	}

	parts, fallbacks := buildUserMessageContentParts("", []UploadedAttachment{
		{
			Name:      "long.txt",
			Ext:       "TXT",
			MimeType:  "text/plain",
			Category:  attachmentCategoryDocument,
			SizeBytes: int64(len(raw)),
			Path:      path,
		},
	})
	if len(fallbacks) != 1 {
		t.Fatalf("expected fallback metadata for oversized document, got: %v", fallbacks)
	}
	if len(parts) != 0 {
		t.Fatalf("expected no inline parts for oversized document, got %d", len(parts))
	}
}

func TestBuildUserMessageContentParts_SmallDocumentKeepsInlineFileData(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "small.txt")
	raw := strings.Repeat("abcd", 1024)
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatalf("write file failed: %v", err)
	}

	parts, fallbacks := buildUserMessageContentParts("", []UploadedAttachment{
		{
			Name:      "small.txt",
			Ext:       "TXT",
			MimeType:  "text/plain",
			Category:  attachmentCategoryDocument,
			SizeBytes: int64(len(raw)),
			Path:      path,
		},
	})
	if len(fallbacks) != 0 {
		t.Fatalf("expected no fallback metadata, got: %v", fallbacks)
	}
	if len(parts) != 1 {
		t.Fatalf("expected 1 part, got %d", len(parts))
	}
	if parts[0].Type != llmsvc.ChatMessageContentPartTypeFile {
		t.Fatalf("expected file part, got %q", parts[0].Type)
	}
	expect := base64.StdEncoding.EncodeToString([]byte(raw))
	if parts[0].FileDataBase64 != expect {
		t.Fatalf("expected full base64 file data preserved")
	}
}

func TestClassifyAttachmentCategory_DefaultToDocument(t *testing.T) {
	got := classifyAttachmentCategory("application/x-custom-binary", "bin")
	if got != attachmentCategoryDocument {
		t.Fatalf("expected default category=document, got=%q", got)
	}
}
