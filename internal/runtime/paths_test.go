package runtime

import "testing"

func TestExpandHome_EmptyStaysEmpty(t *testing.T) {
	if got := ExpandHome(""); got != "" {
		t.Fatalf("expected empty string, got=%q", got)
	}
}
