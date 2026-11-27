package repositories

import (
	"context"
	"testing"

	"slimebot/internal/domain"
)

func TestSaveContextCalibrationKeepsConservativeRatioAndCountsStaleUpdates(t *testing.T) {
	repo := New(NewSQLiteDBTest(t, "context_calibration_merge"))
	ctx := context.Background()
	if err := repo.SaveContextCalibration(ctx, &domain.ContextCalibration{Provider: "openai", Model: "model", Ratio: 2, Samples: 1}); err != nil {
		t.Fatal(err)
	}
	// Simulate another session saving from an older, less conservative snapshot.
	if err := repo.SaveContextCalibration(ctx, &domain.ContextCalibration{Provider: "openai", Model: "model", Ratio: 1.2, Samples: 1}); err != nil {
		t.Fatal(err)
	}
	calibration, found, err := repo.LoadContextCalibration(ctx, "openai", "model")
	if err != nil || !found {
		t.Fatalf("calibration missing: found=%v err=%v", found, err)
	}
	if calibration.Ratio != 2 || calibration.Samples != 2 {
		t.Fatalf("stale update replaced calibration: ratio=%v samples=%d", calibration.Ratio, calibration.Samples)
	}
}
