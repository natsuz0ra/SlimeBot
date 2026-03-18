package services

import (
	"os"
	"path/filepath"
	"testing"
)

func TestChatUploadService_SaveConsumeCleanup(t *testing.T) {
	root := t.TempDir()
	svc := NewChatUploadService(root)

	// multipart.FileHeader.Open 依赖内部字段，直接通过 SaveFiles 不便构造，这里只验证 Consume/Cleanup 核心链路。
	// 用手动注入模拟已保存的附件。
	path := filepath.Join(root, "a.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write fixture failed: %v", err)
	}
	item := UploadedAttachment{
		ID:        "att_1",
		SessionID: "s1",
		Name:      "a.txt",
		Ext:       "TXT",
		SizeBytes: 5,
		MimeType:  "text/plain",
		IconType:  "text",
		Path:      path,
	}
	svc.items[item.ID] = item

	consumed, err := svc.Consume("s1", []string{"att_1"})
	if err != nil {
		t.Fatalf("consume failed: %v", err)
	}
	if len(consumed) != 1 {
		t.Fatalf("expected 1 consumed attachment, got %d", len(consumed))
	}
	svc.Cleanup(consumed)
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatalf("expected file cleaned up, stat err=%v", statErr)
	}
}

func TestChatUploadService_ConsumeSessionMismatch(t *testing.T) {
	svc := NewChatUploadService(t.TempDir())
	svc.items["att_2"] = UploadedAttachment{
		ID:        "att_2",
		SessionID: "s1",
		Name:      "b.txt",
		Ext:       "TXT",
	}
	_, err := svc.Consume("s2", []string{"att_2"})
	if err == nil {
		t.Fatal("expected session mismatch error")
	}
}
