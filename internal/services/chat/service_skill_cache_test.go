package chat

import "testing"

func TestChatServiceSessionSkillCache_MergeAndRead(t *testing.T) {
	svc := &ChatService{}

	initial := svc.getSessionActivatedSkills("s1")
	if len(initial) != 0 {
		t.Fatalf("expected empty cache, got %d", len(initial))
	}

	svc.mergeSessionActivatedSkills("s1", map[string]struct{}{
		"skill-a": {},
	})

	afterMerge := svc.getSessionActivatedSkills("s1")
	if len(afterMerge) != 1 {
		t.Fatalf("expected 1 skill in cache, got %d", len(afterMerge))
	}
	if _, ok := afterMerge["skill-a"]; !ok {
		t.Fatal("expected skill-a in cache")
	}
}

func TestChatServiceSessionSkillCache_ReturnsCopy(t *testing.T) {
	svc := &ChatService{}
	svc.mergeSessionActivatedSkills("s1", map[string]struct{}{
		"skill-a": {},
	})

	snapshot := svc.getSessionActivatedSkills("s1")
	delete(snapshot, "skill-a")
	snapshot["skill-b"] = struct{}{}

	again := svc.getSessionActivatedSkills("s1")
	if _, ok := again["skill-a"]; !ok {
		t.Fatal("expected cached skill-a to remain unchanged")
	}
	if _, ok := again["skill-b"]; ok {
		t.Fatal("did not expect skill-b to mutate cache")
	}
}
