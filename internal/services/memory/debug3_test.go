package memory

import (
	"context"
	"testing"
	"time"
)

func TestDebugUpdatedTimes(t *testing.T) {
	dir := t.TempDir()
	svc, err := NewMemoryService(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Shutdown(context.Background())

	oldPayload := `{"name":"Old Plan","description":"deployment uses canary","type":"project","content":"deployment uses canary and health checks"}`
	newPayload := `{"name":"Latest Plan","description":"deployment uses canary","type":"project","content":"deployment uses canary and health checks"}`

	svc.EnqueueTurnMemory("session-1", "", oldPayload)
	time.Sleep(20 * time.Millisecond)
	svc.EnqueueTurnMemory("session-1", "", newPayload)

	entries, _ := svc.store.Scan()
	for _, e := range entries {
		t.Logf("slug=%s name=%s updated_unix_nano=%d updated_unix=%d", e.Slug(), e.Name, e.Updated.UnixNano(), e.Updated.Unix())
	}

	// Check searchAllEntries result
	all, _ := svc.searchAllEntries("deployment canary health checks", 5)
	for i, e := range all {
		t.Logf("searchAll[%d] name=%s updated_unix_nano=%d", i, e.Name, e.Updated.UnixNano())
	}
}
