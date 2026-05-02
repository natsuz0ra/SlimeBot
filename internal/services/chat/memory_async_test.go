package chat

import (
	"slimebot/internal/domain"
	"testing"
)

func TestBuildMemoryPayloadFromJob_StripsLegacyMemoryBlock(t *testing.T) {
	job := domain.MemoryWriteJob{MessageContent: "before <memory>{\"name\":\"n\",\"description\":\"d\",\"type\":\"project\",\"content\":\"c\"}</memory> after"}
	payload, reason := buildMemoryPayloadFromJob(job)
	if reason == "" {
		if payload == "" {
			t.Fatal("expected extracted payload from surrounding text")
		}
		return
	}
	if reason != "no_effective_increment" {
		t.Fatalf("unexpected reason: %s", reason)
	}
}

func TestBuildMemoryPayloadFromJob_RuleExtraction(t *testing.T) {
	job := domain.MemoryWriteJob{MessageContent: "本次决定：后续发布采用 canary + health check，作为当前项目默认流程。"}
	payload, reason := buildMemoryPayloadFromJob(job)
	if reason != "" {
		t.Fatalf("unexpected reason: %s", reason)
	}
	if payload == "" {
		t.Fatal("expected non-empty payload")
	}
}
