package chat

import (
	"slimebot/internal/domain"
	"testing"
)

func TestBuildMemoryPayloadFromJob_UsesLegacyMemory(t *testing.T) {
	job := domain.MemoryWriteJob{MessageContent: "before <memory>{\"name\":\"n\",\"description\":\"d\",\"type\":\"project\",\"content\":\"c\"}</memory> after"}
	payload, reason := buildMemoryPayloadFromJob(job)
	if reason != "" {
		t.Fatalf("unexpected reason: %s", reason)
	}
	if payload != "{\"name\":\"n\",\"description\":\"d\",\"type\":\"project\",\"content\":\"c\"}" {
		t.Fatalf("unexpected payload: %s", payload)
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
