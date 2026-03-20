package chat

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildUserMessageContentParts_DocumentKeepsFullFileData(t *testing.T) {
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
	if len(fallbacks) != 0 {
		t.Fatalf("expected no fallback metadata, got: %v", fallbacks)
	}
	if len(parts) != 1 {
		t.Fatalf("expected 1 part, got %d", len(parts))
	}
	if parts[0].Type != ChatMessageContentPartTypeFile {
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
