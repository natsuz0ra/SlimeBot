package repositories

import (
	"context"
	"slimebot/internal/domain"
	"testing"
	"time"
)

func TestUsageSummaryGroupsBySessionModelAndProject(t *testing.T) {
	db := NewSQLiteDBTest(t, "usage_summary")
	if err := MigrateUsage(db); err != nil {
		t.Fatal(err)
	}
	repo := New(db)
	ctx := context.Background()
	records := []domain.UsageRecord{
		{ID: "one", RootSessionID: "root", SessionID: "root", ModelConfigID: "config", Model: "gpt", WorkingDirectory: "/work/a", InputTokens: 100, OutputTokens: 20, CacheReadInputTokens: 40, TotalTokens: 120},
		{ID: "two", RootSessionID: "root", SessionID: "child", ModelConfigID: "config", Model: "claude", WorkingDirectory: "/work/a", InputTokens: 10, OutputTokens: 5, CacheCreationInputTokens: 3, CacheReadInputTokens: 8, TotalTokens: 18},
		{ID: "three", RootSessionID: "other", SessionID: "other", Model: "gpt", WorkingDirectory: "/work/b", InputTokens: 1, OutputTokens: 2, TotalTokens: 3},
	}
	for i := range records {
		if err := repo.RecordUsage(ctx, &records[i]); err != nil {
			t.Fatal(err)
		}
	}
	for _, test := range []struct {
		group string
		count int
		total int64
	}{
		{domain.UsageGroupBySession, 2, 141}, {domain.UsageGroupByModel, 3, 141}, {domain.UsageGroupByProject, 2, 141},
	} {
		items, err := repo.SummarizeUsage(ctx, domain.UsageQuery{GroupBy: test.group})
		if err != nil || len(items) != test.count || totalUsage(items) != test.total {
			t.Fatalf("%s: %+v %v", test.group, items, err)
		}
	}
	items, err := repo.SummarizeUsage(ctx, domain.UsageQuery{GroupBy: domain.UsageGroupByProject, RootSessionID: "root"})
	if err != nil || len(items) != 1 || items[0].RequestCount != 2 || items[0].CacheReadInputTokens != 48 || items[0].TotalTokens != 138 {
		t.Fatalf("project summary: %+v %v", items, err)
	}
	output, err := repo.SessionOutputTokens(ctx, "root")
	if err != nil || output != 25 {
		t.Fatalf("output = %d %v", output, err)
	}
	if _, err := repo.SummarizeUsage(ctx, domain.UsageQuery{GroupBy: "day"}); err == nil {
		t.Fatal("invalid groupBy accepted")
	}
}

func TestUsageSummaryAttachesSessionMetadataAndFiltersByTimeRange(t *testing.T) {
	db := NewSQLiteDBTest(t, "usage_summary_meta_time")
	repo := New(db)
	ctx := context.Background()

	deletedAt := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	sessions := []domain.Session{
		{ID: "sess-chat", Kind: domain.SessionKindChat, Name: "排查登录问题"},
		{ID: "sess-task", Kind: domain.SessionKindTaskRun, Name: "每日巡检"},
		{ID: "sess-deleted", Kind: domain.SessionKindChat, Name: "已归档对话", DeletedAt: &deletedAt},
	}
	for i := range sessions {
		if err := db.Create(&sessions[i]).Error; err != nil {
			t.Fatal(err)
		}
	}

	t1 := time.Date(2026, 11, 20, 8, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC)
	t3 := time.Date(2026, 11, 26, 15, 0, 0, 0, time.UTC)
	records := []domain.UsageRecord{
		{ID: "r1", RootSessionID: "sess-chat", SessionID: "sess-chat", Model: "gpt-5", InputTokens: 100, OutputTokens: 50, TotalTokens: 150, CreatedAt: t1},
		{ID: "r2", RootSessionID: "sess-task", SessionID: "sess-task", Model: "gpt-5", InputTokens: 200, OutputTokens: 80, TotalTokens: 280, CreatedAt: t2},
		{ID: "r3", RootSessionID: "sess-deleted", SessionID: "sess-deleted", Model: "gpt-5", InputTokens: 40, OutputTokens: 20, TotalTokens: 60, CreatedAt: t3},
		{ID: "r4", RootSessionID: "sess-missing", SessionID: "sess-missing", Model: "gpt-5", InputTokens: 10, OutputTokens: 10, TotalTokens: 20, CreatedAt: t3},
	}
	for i := range records {
		if err := repo.RecordUsage(ctx, &records[i]); err != nil {
			t.Fatal(err)
		}
	}

	allItems, err := repo.SummarizeUsage(ctx, domain.UsageQuery{GroupBy: domain.UsageGroupBySession})
	if err != nil || len(allItems) != 4 {
		t.Fatalf("allItems = %+v %v", allItems, err)
	}
	byID := map[string]domain.UsageSummary{}
	for _, item := range allItems {
		byID[item.RootSessionID] = item
	}
	if got := byID["sess-chat"]; got.SessionName != "排查登录问题" || got.SessionKind != domain.SessionKindChat || !got.SessionExists {
		t.Fatalf("sess-chat metadata mismatch: %+v", got)
	}
	if got := byID["sess-task"]; got.SessionName != "每日巡检" || got.SessionKind != domain.SessionKindTaskRun || !got.SessionExists {
		t.Fatalf("sess-task metadata mismatch: %+v", got)
	}
	if got := byID["sess-deleted"]; got.SessionName != "已归档对话" || got.SessionExists {
		t.Fatalf("sess-deleted metadata mismatch: %+v", got)
	}
	if got := byID["sess-missing"]; got.SessionName != "" || got.SessionExists {
		t.Fatalf("sess-missing metadata mismatch: %+v", got)
	}

	// Half-open interval [t2, t3): includes r2 only, excludes r1 (< t2) and r3/r4 (== t3).
	ranged, err := repo.SummarizeUsage(ctx, domain.UsageQuery{
		GroupBy: domain.UsageGroupBySession,
		Since:   &t2,
		Until:   &t3,
	})
	if err != nil || len(ranged) != 1 || ranged[0].RootSessionID != "sess-task" || ranged[0].TotalTokens != 280 {
		t.Fatalf("ranged = %+v %v", ranged, err)
	}
}

func totalUsage(items []domain.UsageSummary) int64 {
	var total int64
	for _, item := range items {
		total += item.TotalTokens
	}
	return total
}
