package sandbox

import (
	"io"
	"strings"
	"testing"
)

func TestCappedOutputKeepsPrefixWithoutBlockingProducer(t *testing.T) {
	b := cappedOutput{limit: 8}
	for _, s := range []string{"abcd", "efghijkl", "more"} {
		n, err := b.Write([]byte(s))
		if err != nil || n != len(s) {
			t.Fatal("capture backpressured the child process")
		}
	}
	if b.String() != "abcdefgh" || !b.truncated || b.Len() > 8 {
		t.Fatal("output limit not enforced")
	}
	// os/exec copies from a pipe. Do not accidentally expose bytes.Buffer's
	// ReaderFrom optimization, which would bypass the bounded Write method.
	copyTarget := cappedOutput{limit: 8}
	_, err := io.Copy(&copyTarget, struct{ io.Reader }{strings.NewReader(strings.Repeat("x", 100))})
	if err != nil || copyTarget.Len() != 8 || !copyTarget.truncated {
		t.Fatal("pipe copy bypassed capture limit")
	}
	plain := cappedOutput{}
	_, _ = plain.Write([]byte(strings.Repeat("x", 32)))
	if plain.Len() != 32 || plain.truncated {
		t.Fatal("unlimited callers changed behavior")
	}
}
