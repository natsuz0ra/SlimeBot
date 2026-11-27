package privacy

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"slimebot/internal/domain"
	"slimebot/internal/services/llm"
)

const mappingSecret = "RegressionSyntheticMapping_123"

func mappingKey(fill byte) string {
	return base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{fill}, 32))
}

func setMappingPersistence(t *testing.T, s *Service, ctx context.Context, on bool) Config {
	t.Helper()
	c, err := s.GetConfig(ctx)
	if err != nil {
		t.Fatal(err)
	}
	c.MappingPersistence = on
	if err := s.Update(ctx, c); err != nil {
		t.Fatal(err)
	}
	return c
}

func createMapping(t *testing.T, s *Service, ctx context.Context, c Config) string {
	t.Helper()
	tx, err := s.begin(ctx, c, false)
	if err != nil {
		t.Fatal(err)
	}
	placeholder, err := tx.placeholder("SECRET", mappingSecret)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.finish(false); err != nil {
		t.Fatal(err)
	}
	return placeholder
}

func checkMapping(t *testing.T, s *Service, ctx context.Context, c Config, placeholder string, present bool) {
	t.Helper()
	tx, err := s.begin(ctx, c, false)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.finish(false)
	value, err := tx.restore(placeholder)
	if (present && (err != nil || value != mappingSecret)) || (!present && err == nil) {
		t.Fatalf("mapping presence %t, wanted %t", err == nil, present)
	}
}

func TestMappingEncryptedRestartWithMaskingDisabled(t *testing.T) {
	key := mappingKey(1)
	t.Setenv("SLIMEBOT_PRIVACY_MAPPING_KEY", key)
	s, store, ctx := setup(t)
	c := setMappingPersistence(t, s, ctx, true)
	placeholder := createMapping(t, s, ctx, c)
	c.Enabled = false
	if err := s.Update(ctx, c); err != nil {
		t.Fatal(err)
	}
	s.Close()
	restarted := NewService(store, s.dir)
	t.Cleanup(restarted.Close)
	snapshot, err := restarted.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), mappingSecret) || strings.Contains(string(encoded), key) || strings.Contains(store.raw, key) {
		t.Fatal("mapping material appeared in settings or snapshot")
	}
	if len(restarted.scopes) != 0 {
		t.Fatal("snapshot eagerly loaded plaintext maps")
	}
	files, err := os.ReadDir(filepath.Join(s.dir, mappingDirName))
	if err != nil || len(files) < 2 {
		t.Fatalf("missing encrypted mapping store: %v", err)
	}
	for _, file := range files {
		data, err := os.ReadFile(filepath.Join(s.dir, mappingDirName, file.Name()))
		if err != nil || bytes.Contains(data, []byte(mappingSecret)) || bytes.Contains(data, []byte(placeholder)) || bytes.Contains(data, []byte("session-one")) || bytes.Contains(data, []byte(key)) || bytes.Contains(data, bytes.Repeat([]byte{1}, 32)) {
			t.Fatal("mapping store is unreadable or contains plaintext")
		}
	}
	next := fakeProvider{run: func(_ context.Context, _ llm.ModelRuntimeConfig, m []llm.ChatMessage, _ []llm.ToolDef, _ llm.StreamCallbacks) (*llm.StreamResult, error) {
		if m[0].Content != "DB_PASSWORD=UnmaskedExample123" {
			t.Fatal("masking was unexpectedly enabled")
		}
		return &llm.StreamResult{AssistantMessage: llm.ChatMessage{Content: placeholder}}, nil
	}}
	result, err := restarted.Wrap(next).StreamChatWithTools(ctx, llm.ModelRuntimeConfig{}, []llm.ChatMessage{{Content: "DB_PASSWORD=UnmaskedExample123"}}, nil, llm.StreamCallbacks{})
	if err != nil || result.AssistantMessage.Content != mappingSecret {
		t.Fatalf("restore-only request failed: %v", err)
	}
}

func TestMappingTTLAndDestroyBlockStaleFinish(t *testing.T) {
	t.Setenv("SLIMEBOT_PRIVACY_MAPPING_KEY", mappingKey(2))
	for _, persistent := range []bool{false, true} {
		t.Run(map[bool]string{false: "memory", true: "encrypted"}[persistent], func(t *testing.T) {
			s, store, ctx := setup(t)
			now := time.Now()
			s.now = func() time.Time { return now }
			c := setMappingPersistence(t, s, ctx, persistent)
			c.MappingTTLMinutes = 1
			if err := s.Update(ctx, c); err != nil {
				t.Fatal(err)
			}
			p := createMapping(t, s, ctx, c)
			stale, err := s.begin(ctx, c, false)
			if err != nil {
				t.Fatal(err)
			}
			now = now.Add(time.Minute)
			if err := s.cleanupMappings(ctx); err != nil {
				t.Fatal(err)
			}
			stale.finish(false)
			checkMapping(t, s, ctx, c, p, false)
			p = createMapping(t, s, ctx, c)
			stale, err = s.begin(ctx, c, false)
			if err != nil {
				t.Fatal(err)
			}
			if err := s.DestroySession("session-one"); err != nil {
				t.Fatal(err)
			}
			checkMapping(t, s, ctx, c, p, false)
			stale.finish(false)
			checkMapping(t, s, ctx, c, p, false)
			s.Close()
			restarted := NewService(store, s.dir)
			t.Cleanup(restarted.Close)
			restarted.now = func() time.Time { return now }
			checkMapping(t, restarted, ctx, c, p, false)
		})
	}
}

func TestMappingExpiryOnRestartAndTTLUpdates(t *testing.T) {
	t.Setenv("SLIMEBOT_PRIVACY_MAPPING_KEY", mappingKey(3))
	s, store, ctx := setup(t)
	now := time.Now()
	s.now = func() time.Time { return now }
	c := setMappingPersistence(t, s, ctx, true)
	c.MappingTTLMinutes = 2
	if err := s.Update(ctx, c); err != nil {
		t.Fatal(err)
	}
	one := createMapping(t, s, ctx, c)
	other := llm.WithPrivacyScope(ctx, "session-two", "", "test")
	two := createMapping(t, s, other, c)
	s.Close()
	now = now.Add(90 * time.Second)
	restarted := NewService(store, s.dir)
	t.Cleanup(restarted.Close)
	restarted.now = func() time.Time { return now }
	checkMapping(t, restarted, ctx, c, one, true)
	now = now.Add(time.Minute)
	if err := restarted.cleanupMappings(ctx); err != nil {
		t.Fatal(err)
	}
	checkMapping(t, restarted, other, c, two, false)
	c.MappingTTLMinutes = 1
	if err := restarted.Update(ctx, c); err != nil {
		t.Fatal(err)
	}
	c.MappingTTLMinutes = 100
	if err := restarted.Update(ctx, c); err != nil {
		t.Fatal(err)
	}
	checkMapping(t, restarted, ctx, c, one, false)
}

func TestMappingWrongKeyPreservesStoreAndOffPurges(t *testing.T) {
	key := mappingKey(4)
	t.Setenv("SLIMEBOT_PRIVACY_MAPPING_KEY", key)
	s, store, ctx := setup(t)
	c := setMappingPersistence(t, s, ctx, true)
	p := createMapping(t, s, ctx, c)
	s.Close()
	for _, wrong := range []string{"", "invalid", mappingKey(5), key} {
		t.Setenv("SLIMEBOT_PRIVACY_MAPPING_KEY", wrong)
		restarted := NewService(store, s.dir)
		snap, err := restarted.Snapshot(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if wrong == key {
			checkMapping(t, restarted, ctx, c, p, true)
		} else {
			if snap.Mappings.Mode != "unavailable" {
				t.Fatal("encryption problem not visible")
			}
			if tx, err := restarted.begin(ctx, c, false); err == nil {
				tx.finish(false)
				t.Fatal("session with inaccessible mappings should fail before provider I/O")
			}
			other := llm.WithPrivacyScope(ctx, "new-session", "", "test")
			createMapping(t, restarted, other, c)
		}
		restarted.Close()
	}
	t.Setenv("SLIMEBOT_PRIVACY_MAPPING_KEY", "")
	locked := NewService(store, s.dir)
	t.Cleanup(locked.Close)
	c = setMappingPersistence(t, locked, ctx, false)
	if _, err := os.Stat(filepath.Join(s.dir, mappingDirName)); !os.IsNotExist(err) {
		t.Fatalf("turning persistence off did not purge disk: %v", err)
	}
	checkMapping(t, locked, ctx, c, p, false)
}

func TestMappingDestroyWithoutKeyAndRejectMissingKeyAtSetup(t *testing.T) {
	t.Setenv("SLIMEBOT_PRIVACY_MAPPING_KEY", "")
	s, store, ctx := setup(t)
	c, _ := s.GetConfig(ctx)
	if c.MappingPersistence || c.MappingTTLMinutes != defaultMappingTTLMinutes {
		t.Fatal("unexpected default mapping settings")
	}
	c.MappingPersistence = true
	if err := s.Update(ctx, c); !errors.Is(err, ErrMappingConfiguration) {
		t.Fatalf("enabling without a deployment key was not rejected: %v", err)
	}
	if actual, _ := s.GetConfig(ctx); actual.MappingPersistence {
		t.Fatal("failed setup changed settings")
	}
	s.Close()

	t.Setenv("SLIMEBOT_PRIVACY_MAPPING_KEY", mappingKey(6))
	enabled := NewService(store, s.dir)
	t.Cleanup(enabled.Close)
	c = setMappingPersistence(t, enabled, ctx, true)
	p := createMapping(t, enabled, ctx, c)
	enabled.Close()

	t.Setenv("SLIMEBOT_PRIVACY_MAPPING_KEY", "")
	locked := NewService(store, s.dir)
	t.Cleanup(locked.Close)
	if err := locked.DestroySession("session-one"); err != nil {
		t.Fatal(err)
	}
	locked.Close()

	t.Setenv("SLIMEBOT_PRIVACY_MAPPING_KEY", mappingKey(6))
	restarted := NewService(store, s.dir)
	t.Cleanup(restarted.Close)
	checkMapping(t, restarted, ctx, c, p, false)
}

func TestMappingFileCannotBeMovedToAnotherSession(t *testing.T) {
	t.Setenv("SLIMEBOT_PRIVACY_MAPPING_KEY", mappingKey(7))
	s, store, ctx := setup(t)
	c := setMappingPersistence(t, s, ctx, true)
	createMapping(t, s, ctx, c)
	s.Close()
	path := filepath.Join(s.dir, mappingDirName)
	from := filepath.Join(path, mappingName("session-one"))
	to := filepath.Join(path, mappingName("session-two"))
	if err := os.Rename(from, to); err != nil {
		t.Fatal(err)
	}
	restarted := NewService(store, s.dir)
	t.Cleanup(restarted.Close)
	other := llm.WithPrivacyScope(ctx, "session-two", "", "test")
	if tx, err := restarted.begin(other, c, false); err == nil || tx != nil {
		t.Fatal("renaming a mapping file let another session read it")
	}
	if _, err := os.Stat(to); err != nil {
		t.Fatal("authenticated data was erased after a failed load")
	}
}

func TestMappingLimits(t *testing.T) {
	s, _, ctx := setup(t)
	c, _ := s.GetConfig(ctx)
	for _, ttl := range []int{0, -1, maxMappingTTLMinutes + 1} {
		c.MappingTTLMinutes = ttl
		if err := s.Update(ctx, c); err == nil {
			t.Fatal("out of range TTL accepted")
		}
	}
	c.MappingTTLMinutes = defaultMappingTTLMinutes
	if tx, err := s.begin(llm.WithPrivacyScope(ctx, strings.Repeat("x", maxMappingSessionID+1), "", "test"), c, false); err == nil || tx != nil {
		t.Fatal("oversized session identifier accepted")
	}
	tx, err := s.begin(ctx, c, false)
	if err != nil {
		t.Fatal(err)
	}
	value := strings.Repeat("a", maxMappingValue)
	if _, err := tx.placeholder("SECRET", value+"b"); !errors.Is(err, errMappingLimit) {
		t.Fatal("oversized mapping accepted")
	}
	for i := range maxMappingBytes / maxMappingValue {
		if _, err := tx.placeholder("SECRET", string(rune(i))+value[1:]); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := tx.placeholder("SECRET", "over budget"); !errors.Is(err, errMappingLimit) {
		t.Fatal("oversized mapping table accepted")
	}
	if err := tx.finish(false); err != nil {
		t.Fatal(err)
	}
	p := createMapping(t, s, ctx, c)
	checkMapping(t, s, ctx, c, p, true)
}

func TestMappingWriteFailureAndBlockedRequestDoNotSend(t *testing.T) {
	t.Setenv("SLIMEBOT_PRIVACY_MAPPING_KEY", mappingKey(10))
	s, _, ctx := setup(t)
	c := setMappingPersistence(t, s, ctx, true)
	called := false
	next := fakeProvider{run: func(context.Context, llm.ModelRuntimeConfig, []llm.ChatMessage, []llm.ToolDef, llm.StreamCallbacks) (*llm.StreamResult, error) {
		called = true
		return &llm.StreamResult{}, nil
	}}
	wrapped := s.Wrap(next)
	m := []llm.ChatMessage{{Content: "DB_PASSWORD=BlockedSynthetic123"}}
	c.Actions["PASSWORD"] = Block
	if err := s.Update(ctx, c); err != nil {
		t.Fatal(err)
	}
	if _, err := wrapped.StreamChatWithTools(ctx, llm.ModelRuntimeConfig{}, m, nil, llm.StreamCallbacks{}); !errors.Is(err, ErrBlocked) || called {
		t.Fatalf("blocked request reached provider or returned unexpected error: %v", err)
	}
	dir := filepath.Join(s.dir, mappingDirName)
	if _, err := os.Stat(filepath.Join(dir, mappingName("session-one"))); !os.IsNotExist(err) {
		t.Fatal("blocked request retained an unused mapping")
	}
	c.Actions["PASSWORD"] = Mask
	if err := s.Update(ctx, c); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(dir, dir+"-moved"); err != nil {
		t.Fatal(err)
	}
	if _, err := wrapped.StreamChatWithTools(ctx, llm.ModelRuntimeConfig{}, m, nil, llm.StreamCallbacks{}); err == nil || called {
		t.Fatalf("request reached provider despite encrypted store write failure: %v", err)
	}
	if err := os.Rename(dir+"-moved", dir); err != nil {
		t.Fatal(err)
	}
	if err := s.cleanupMappings(ctx); err != nil {
		t.Fatal(err)
	}
	before, err := s.Snapshot(ctx)
	if err != nil || !before.Mappings.StorageError {
		t.Fatal("read-only cleanup hid storage write failure")
	}
	if _, err := wrapped.StreamChatWithTools(ctx, llm.ModelRuntimeConfig{}, m, nil, llm.StreamCallbacks{}); err != nil || !called {
		t.Fatalf("request did not recover after store was restored: %v", err)
	}
	after, err := s.Snapshot(ctx)
	if err != nil || after.Mappings.StorageError {
		t.Fatal("write recovery did not clear storage write failure")
	}
}

type sessionMappingStore struct {
	testStore
	present, failWrite bool
}

func (s *sessionMappingStore) GetSessionByID(_ context.Context, id string) (*domain.Session, error) {
	if s.present {
		return &domain.Session{ID: id}, nil
	}
	return nil, errors.New("missing")
}

func (s *sessionMappingStore) SetSetting(ctx context.Context, k, v string) error {
	if s.failWrite {
		return errors.New("settings write failed")
	}
	return s.testStore.SetSetting(ctx, k, v)
}

func TestMappingUnknownSessionCanMaskWithoutRetaining(t *testing.T) {
	t.Setenv("SLIMEBOT_PRIVACY_MAPPING_KEY", mappingKey(8))
	store := &sessionMappingStore{}
	s := NewService(store, t.TempDir())
	t.Cleanup(s.Close)
	ctx := llm.WithPrivacyScope(context.Background(), "session-one", "", "test")
	c := setMappingPersistence(t, s, ctx, true)
	store.present = true
	p := createMapping(t, s, ctx, c)
	checkMapping(t, s, ctx, c, p, true)
	store.present = false
	if err := s.DestroySession("session-one"); err != nil {
		t.Fatal(err)
	}
	p = createMapping(t, s, ctx, c)
	checkMapping(t, s, ctx, c, p, false)
	if _, err := os.Stat(filepath.Join(s.dir, mappingDirName, mappingName("session-one"))); !os.IsNotExist(err) {
		t.Fatalf("deleted session acquired a new disk mapping: %v", err)
	}
}

func TestMappingFailedSettingsWriteKeepsUnexpiredEntries(t *testing.T) {
	t.Setenv("SLIMEBOT_PRIVACY_MAPPING_KEY", mappingKey(9))
	store := &sessionMappingStore{present: true}
	s := NewService(store, t.TempDir())
	t.Cleanup(s.Close)
	ctx := llm.WithPrivacyScope(context.Background(), "session-one", "", "test")
	now := time.Now()
	s.now = func() time.Time { return now }
	c := setMappingPersistence(t, s, ctx, true)
	p := createMapping(t, s, ctx, c)
	now = now.Add(5 * time.Minute)
	shorter := c
	shorter.MappingTTLMinutes = 1
	store.failWrite = true
	if err := s.Update(ctx, shorter); err == nil {
		t.Fatal("failed settings write was reported as successful")
	}
	s.Close()
	restarted := NewService(store, s.dir)
	t.Cleanup(restarted.Close)
	restarted.now = func() time.Time { return now }
	checkMapping(t, restarted, ctx, c, p, true)
}

type cleanupBlockingStore struct{ started chan struct{} }

func (b cleanupBlockingStore) GetSetting(ctx context.Context, _ string) (string, error) {
	close(b.started)
	<-ctx.Done()
	return "", ctx.Err()
}
func (cleanupBlockingStore) SetSetting(context.Context, string, string) error { return nil }

func TestMappingCleanupCloseCancelsWorker(t *testing.T) {
	store := cleanupBlockingStore{make(chan struct{})}
	s := NewService(store, t.TempDir())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.StartMappingCleanup(ctx)
	s.StartMappingCleanup(ctx)
	select {
	case <-store.started:
	case <-time.After(2 * time.Second):
		t.Fatal("startup cleanup did not start")
	}
	done := make(chan struct{})
	go func() { s.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Close did not cancel and join startup cleanup")
	}
}
