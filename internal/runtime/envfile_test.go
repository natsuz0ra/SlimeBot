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
	jwtSecret := findEnvValue(content, "JWT_SECRET")
	if strings.TrimSpace(jwtSecret) == "" {
		t.Fatal("missing key JWT_SECRET should be appended with non-empty value")
	}
	if jwtSecret == "YOUR_JWT_SECRET" || jwtSecret == "CHANGE_ME" {
		t.Fatal("JWT_SECRET placeholder should be replaced by generated secret")
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

	jwtFirst := findEnvValue(string(first), "JWT_SECRET")
	jwtSecond := findEnvValue(string(second), "JWT_SECRET")
	if jwtFirst == "" || jwtSecond == "" {
		t.Fatal("JWT_SECRET should exist after ensureAndLoadEnv")
	}
	if jwtFirst != jwtSecond {
		t.Fatal("JWT_SECRET should stay stable on repeated ensureAndLoadEnv runs")
	}
}

func findEnvValue(content string, key string) string {
	prefix := strings.TrimSpace(key) + "="
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), prefix) {
			return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), prefix))
		}
	}
	return ""
}
