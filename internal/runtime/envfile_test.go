package runtime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testTemplate = "" +
	"SERVER_PORT=8080\n" +
	"JWT_SECRET=CHANGE_ME\n" +
	"EMBEDDING_ORT_LIB_PATH=\n"

func TestEnsureAndLoadEnv_CreatesMissingFile(t *testing.T) {
	envPath := filepath.Join(t.TempDir(), ".env")

	if err := ensureAndLoadEnv(envPath, testTemplate); err != nil {
		t.Fatalf("ensureAndLoadEnv failed: %v", err)
	}

	raw, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("read env failed: %v", err)
	}
	if string(raw) != testTemplate {
		t.Fatalf("unexpected env content:\n%s", string(raw))
	}
}

func TestEnsureAndLoadEnv_AppendsOnlyMissingKeys(t *testing.T) {
	envPath := filepath.Join(t.TempDir(), ".env")
	origin := "" +
		"SERVER_PORT=9090\n" +
		"# keep comments\n"
	if err := os.WriteFile(envPath, []byte(origin), 0o644); err != nil {
		t.Fatalf("write env failed: %v", err)
	}

	if err := ensureAndLoadEnv(envPath, testTemplate); err != nil {
		t.Fatalf("ensureAndLoadEnv failed: %v", err)
	}

	raw, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("read env failed: %v", err)
	}
	content := string(raw)
	if !strings.Contains(content, "SERVER_PORT=9090") {
		t.Fatal("existing key should be preserved")
	}
	if !strings.Contains(content, "JWT_SECRET=CHANGE_ME") {
		t.Fatal("missing key JWT_SECRET should be appended")
	}
	if !strings.Contains(content, "EMBEDDING_ORT_LIB_PATH=") {
		t.Fatal("missing key EMBEDDING_ORT_LIB_PATH should be appended")
	}
}

func TestEnsureAndLoadEnv_IsIdempotent(t *testing.T) {
	envPath := filepath.Join(t.TempDir(), ".env")
	if err := ensureAndLoadEnv(envPath, testTemplate); err != nil {
		t.Fatalf("first ensureAndLoadEnv failed: %v", err)
	}
	first, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("read env failed: %v", err)
	}
	if err := ensureAndLoadEnv(envPath, testTemplate); err != nil {
		t.Fatalf("second ensureAndLoadEnv failed: %v", err)
	}
	second, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("read env failed: %v", err)
	}
	if string(first) != string(second) {
		t.Fatalf("env file changed after idempotent run:\n--- first ---\n%s\n--- second ---\n%s", string(first), string(second))
	}
}
