package contextsvc

import (
	"context"
	"testing"

	"slimebot/internal/domain"
	"slimebot/internal/repositories"
	llm "slimebot/internal/services/llm"
)

func reloadSeeds(t *testing.T, repo *repositories.Repository, scope string) []domain.ContextSeed {
	t.Helper()
	history, err := repo.ListAllSessionMessages(context.Background(), scope, -1)
	if err != nil {
		t.Fatal(err)
	}
	seeds := make([]domain.ContextSeed, 0, len(history))
	for _, message := range history {
		seeds = append(seeds, domain.ContextSeed{
			MessageID: message.ID, Seq: message.Seq, SourceHash: domain.ContextMessageHash(message),
			Fidelity: "legacy_replay", Messages: []llm.ChatMessage{{Role: message.Role, Content: message.Content}},
		})
	}
	seeds[len(seeds)-1].Fidelity = "exact"
	return seeds
}

func TestManualReasonPersistsAndCalibrationReloads(t *testing.T) {
	provider := &summaryProvider{}
	repo, run, scope, _ := persistentRun(t, provider)
	prepared, err := run.PrepareWithReason(context.Background(), config(), nil, true, "manual")
	if err != nil || !prepared.Compacted || prepared.Reason != "manual" {
		t.Fatalf("manual compaction failed: prepared=%+v err=%v", prepared, err)
	}
	snapshot, err := repo.GetContextSnapshot(context.Background(), scope)
	if err != nil || len(snapshot.Checkpoints) != 1 || snapshot.Checkpoints[0].Reason != "manual" {
		t.Fatalf("manual reason was not persisted: %#v err=%v", snapshot.Checkpoints, err)
	}

	run.Calibrate(llm.TokenUsage{InputTokens: 200}, llm.ProviderOpenAI, 100)
	reloaded, err := New(repo, nil).Begin(context.Background(), scope, "next-request", nil, reloadSeeds(t, repo, scope))
	if err != nil {
		t.Fatal(err)
	}
	defer reloaded.Close()
	if _, err := reloaded.Prepare(context.Background(), config(), nil, false); err != nil {
		t.Fatal(err)
	}
	if reloaded.calibration < 2 || reloaded.calibrationSample != 1 {
		t.Fatalf("calibration did not survive reload: ratio=%v samples=%d", reloaded.calibration, reloaded.calibrationSample)
	}
}

func TestCompactionCooldownSurvivesReloadAndManualBypassesIt(t *testing.T) {
	provider := &summaryProvider{bad: "not json"}
	repo, run, scope, _ := persistentRun(t, provider)
	modelConfig := config()
	modelConfig.ContextSize = 32768
	if _, err := run.PrepareWithReason(context.Background(), modelConfig, nil, true, "idle"); err == nil {
		t.Fatal("invalid summary unexpectedly succeeded")
	}
	if run.cooldownUntil.IsZero() {
		t.Fatal("failure did not set cooldown")
	}
	persisted, snapshotErr := repo.GetContextSnapshot(context.Background(), scope)
	if snapshotErr != nil || persisted.Head.CompactionCooldownUntil == nil {
		t.Fatalf("cooldown was not persisted: head=%+v err=%v", persisted.Head, snapshotErr)
	}
	run.Close()

	reloaded, err := New(repo, llm.NewFactory(provider)).Begin(context.Background(), scope, "next-request", nil, reloadSeeds(t, repo, scope))
	if err != nil {
		t.Fatal(err)
	}
	defer reloaded.Close()
	calls := provider.calls
	if _, err := reloaded.PrepareWithReason(context.Background(), modelConfig, nil, true, "idle"); err != nil {
		t.Fatalf("cooldown should defer idle work without a hard overflow: %v", err)
	}
	if provider.calls != calls {
		t.Fatal("idle compaction ignored persisted cooldown")
	}

	provider.bad = ""
	prepared, err := reloaded.PrepareWithReason(context.Background(), modelConfig, nil, true, "manual")
	if err != nil || !prepared.Compacted || !reloaded.cooldownUntil.IsZero() {
		t.Fatalf("manual compaction did not bypass and clear cooldown: %+v err=%v", prepared, err)
	}
	snapshot, err := repo.GetContextSnapshot(context.Background(), scope)
	if err != nil || snapshot.Head.CompactionFailures != 0 || snapshot.Head.CompactionCooldownUntil != nil {
		t.Fatalf("successful compaction did not clear persisted failure state: %#v err=%v", snapshot.Head, err)
	}
}
