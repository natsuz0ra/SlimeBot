package repositories

import (
	"context"
	"fmt"
	"slimebot/internal/domain"
	"strings"
	"testing"
	"time"
)

func TestChatSearchVisibleContentAndPagination(t *testing.T) {
	db := NewSQLiteDBTest(t, "chat_search")
	repo := New(db)
	ctx := context.Background()
	session, err := repo.CreateSession(ctx, "发布计划 keyword", "/tmp/project")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Millisecond)
	contents := []struct{ role, content string }{
		{"assistant", "<!-- TOOL_CALL:hidden-keyword -->无需匹配"},
		{"system", "keyword 内部提示"},
		{"user", "这是 keyword 消息"},
		{"assistant", "<!-- THINKING:private -->keyword 回复"},
		{"user", "查找 100% 和 a_b 以及 C:\\tmp，不把通配符当搜索模式"},
	}
	for i, item := range contents {
		_, err := repo.AddMessageWithInput(ctx, domain.AddMessageInput{SessionID: session.ID, Role: item.role, Content: item.content, CreatedAt: now.Add(time.Duration(i) * time.Second)})
		if err != nil {
			t.Fatal(err)
		}
	}
	hits, err := repo.SearchChats(ctx, "keyword", "all", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 3 || hits[0].MessageID != "" || hits[1].Role != "assistant" || hits[2].Role != "user" {
		t.Fatalf("unexpected hits: %+v", hits)
	}
	for _, hit := range hits {
		if strings.Contains(hit.Content, "<!--") || hit.CreatedAt.IsZero() || hit.WorkingDirectory != "/tmp/project" {
			t.Fatalf("invalid hit: %+v", hit)
		}
	}
	page, err := repo.SearchChats(ctx, "keyword", "messages", 1, 1)
	if err != nil || len(page) != 1 || page[0].Role != "user" {
		t.Fatalf("pagination: %+v %v", page, err)
	}
	for _, query := range []string{"100%", "a_b", `C:\tmp`, "通配符"} {
		hits, err := repo.SearchChats(ctx, query, "messages", 10, 0)
		if err != nil || len(hits) != 1 {
			t.Fatalf("literal query %q: %+v %v", query, hits, err)
		}
	}
	hits, err = repo.SearchChats(ctx, "private", "all", 10, 0)
	if err != nil || len(hits) != 0 {
		t.Fatalf("hidden marker: %+v %v", hits, err)
	}
	// More than one SQL batch of marker-only candidates must not consume visible result offsets.
	for i := 0; i < 105; i++ {
		db.Create(&domain.Message{ID: fmt.Sprintf("marker-%d", i), SessionID: session.ID, Role: "assistant", Content: "<!-- TOOL_CALL:keyword -->", CreatedAt: now.Add(time.Hour), Seq: int64(i + 10)})
	}
	hits, err = repo.SearchChats(ctx, "KEYWORD", "messages", 10, 0)
	if err != nil || len(hits) != 2 {
		t.Fatalf("batch/ASCII case: %+v %v", hits, err)
	}
	if err := repo.DeleteSession(ctx, session.ID); err != nil {
		t.Fatal(err)
	}
	hits, err = repo.SearchChats(ctx, "keyword", "all", 10, 0)
	if err != nil || len(hits) != 0 {
		t.Fatalf("deleted session: %+v %v", hits, err)
	}
}

func TestSearchHitCursorOpensBoundedHistoryAtSameTimestamp(t *testing.T) {
	repo := New(NewSQLiteDBTest(t, "search_history_cursor"))
	ctx := context.Background()
	session, err := repo.CreateSession(ctx, "历史定位")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Millisecond)
	for i := 1; i <= 60; i++ {
		content := "普通消息"
		if i == 20 {
			content = "唯一搜索结果"
		}
		if _, err := repo.AddMessageWithInput(ctx, domain.AddMessageInput{SessionID: session.ID, Role: "user", Content: content, CreatedAt: now}); err != nil {
			t.Fatal(err)
		}
	}
	hits, err := repo.SearchChats(ctx, "唯一搜索结果", "messages", 30, 0)
	if err != nil || len(hits) != 1 {
		t.Fatalf("search: %+v %v", hits, err)
	}
	hit := hits[0]
	beforeSeq := hit.Seq + 1
	previous, hasOlder, err := repo.ListSessionMessagesPage(ctx, session.ID, 10, &hit.CreatedAt, &beforeSeq, nil, nil)
	if err != nil || !hasOlder || len(previous) != 10 || previous[9].ID != hit.MessageID {
		t.Fatalf("older cursor: %+v %v %v", previous, hasOlder, err)
	}
	following, hasNewer, err := repo.ListSessionMessagesPage(ctx, session.ID, 10, nil, nil, &hit.CreatedAt, &hit.Seq)
	if err != nil || !hasNewer || len(following) != 10 || following[0].Seq != 21 || following[9].Seq != 30 {
		t.Fatalf("newer cursor: %+v %v %v", following, hasNewer, err)
	}
}
