package repositories

import (
	"context"
	"slimebot/internal/domain"
	"testing"
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

func totalUsage(items []domain.UsageSummary) int64 {
	var total int64
	for _, item := range items {
		total += item.TotalTokens
	}
	return total
}
