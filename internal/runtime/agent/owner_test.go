package agent

import (
	"path/filepath"
	"testing"
)

func TestOwnerExcludesConcurrentRecoveryAndReleasesLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime.lock")
	owner, err := AcquireOwner(path)
	if err != nil {
		t.Fatal(err)
	}
	if other, err := AcquireOwner(path); err == nil {
		other.Close()
		t.Fatal("second owner acquired the database")
	}
	if err := owner.Close(); err != nil {
		t.Fatal(err)
	}
	next, err := AcquireOwner(path)
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close()
}
