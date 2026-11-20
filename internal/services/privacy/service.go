package privacy

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"slimebot/internal/domain"
	"slimebot/internal/services/llm"
)

type Hit struct {
	Kind        string `json:"kind"`
	Source      string `json:"source"`
	Action      Action `json:"action"`
	Start       int    `json:"start"`
	End         int    `json:"end"`
	Placeholder string `json:"placeholder,omitempty"`
}

type Record struct {
	ID         string         `json:"id"`
	Time       time.Time      `json:"time"`
	SessionID  string         `json:"sessionId,omitempty"`
	Source     string         `json:"source"`
	Masked     int            `json:"masked"`
	Warned     int            `json:"warned"`
	Blocked    bool           `json:"blocked"`
	Restored   int            `json:"restored"`
	Kinds      map[string]int `json:"kinds"`
	DurationMS int64          `json:"durationMs"`
	Failed     bool           `json:"failed"`
}

type Snapshot struct {
	Config  Config      `json:"config"`
	Model   ModelStatus `json:"model"`
	Records []Record    `json:"records"`
}

type scopeState struct {
	values map[string]entry
	used   time.Time
}
type entry struct{ Value, Kind string }

type Service struct {
	store     domain.SettingsStore
	dir       string
	mu        sync.Mutex
	key       []byte
	scopes    map[string]scopeState
	records   []Record
	ruleCache map[[32]byte][]span
	model     *LocalModel
	// Detector may be replaced in focused tests without loading model weights.
	detect func(context.Context, string, float64) ([]span, error)
}

func NewService(store domain.SettingsStore, dir string) *Service {
	s := &Service{store: store, dir: dir, scopes: map[string]scopeState{}, records: []Record{}, ruleCache: map[[32]byte][]span{}}
	s.model = NewLocalModel(filepath.Join(dir, "model"))
	s.detect = s.model.Detect
	return s
}

func (s *Service) GetConfig(ctx context.Context) (Config, error) {
	c := DefaultConfig()
	if s.store == nil {
		return c, errors.New("privacy settings unavailable")
	}
	raw, err := s.store.GetSetting(ctx, settingKey)
	if err != nil {
		return c, errors.New("cannot read privacy settings")
	}
	if raw != "" {
		if json.Unmarshal([]byte(raw), &c) != nil {
			return c, errors.New("invalid saved privacy settings")
		}
	}
	if err = c.Validate(); err != nil {
		return c, err
	}
	return c.normalized(), nil
}

func (s *Service) Update(ctx context.Context, c Config) error {
	c = c.normalized()
	if err := c.Validate(); err != nil {
		return err
	}
	data, err := json.Marshal(c)
	if err != nil {
		return err
	}
	if err = s.store.SetSetting(ctx, settingKey, string(data)); err != nil {
		return errors.New("cannot save privacy settings")
	}
	if !c.ModelEnabled {
		s.model.Stop()
	}
	return nil
}

func (s *Service) Snapshot(ctx context.Context) (Snapshot, error) {
	c, err := s.GetConfig(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	s.mu.Lock()
	records := append([]Record{}, s.records...)
	s.mu.Unlock()
	return Snapshot{c, s.model.Status(), records}, nil
}

func (s *Service) ClearRecords()      { s.mu.Lock(); s.records = []Record{}; s.mu.Unlock() }
func (s *Service) Model() *LocalModel { return s.model }
func (s *Service) Close() {
	s.model.Cancel()
	s.model.Stop()
	s.mu.Lock()
	s.scopes = map[string]scopeState{}
	s.ruleCache = map[[32]byte][]span{}
	s.mu.Unlock()
}

func (s *Service) begin(ctx context.Context, c Config, preview bool) (*transaction, error) {
	scope := llm.PrivacyScopeFromContext(ctx)
	if scope.SessionID == "" {
		scope.SessionID = randomID()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.key == nil {
		if err := os.MkdirAll(s.dir, 0o700); err != nil {
			return nil, errors.New("cannot create privacy state directory")
		}
		path := filepath.Join(s.dir, "placeholder.key")
		key, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			key = make([]byte, 32)
			if _, err = rand.Read(key); err != nil {
				return nil, errors.New("cannot create privacy key")
			}
			f, openErr := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
			if os.IsExist(openErr) {
				key, err = os.ReadFile(path)
			} else if openErr != nil {
				err = openErr
			} else {
				_, err = f.Write(key)
				closeErr := f.Close()
				if err == nil {
					err = closeErr
				}
			}
		}
		if err != nil || len(key) != 32 {
			return nil, errors.New("cannot load privacy key")
		}
		s.key = key
	}
	values := map[string]entry{}
	if !preview {
		for p, v := range s.scopes[scope.SessionID].values {
			values[p] = v
		}
	}
	t := &transaction{service: s, config: c, key: s.key, scope: scope, values: values, index: map[string][]entry{}, record: Record{ID: randomID(), Time: time.Now(), SessionID: scope.SessionID, Source: scope.Source, Kinds: map[string]int{}}, preview: preview}
	encodedConfig, _ := json.Marshal(c)
	t.configHash = sha256.Sum256(encodedConfig)
	for _, v := range values {
		t.indexValue(v)
	}
	return t, nil
}

func (t *transaction) finish(failed bool) {
	if t.preview {
		return
	}
	s := t.service
	s.mu.Lock()
	defer s.mu.Unlock()
	// Each in-flight request has its own complete table. Eviction affects future
	// reuse only, never a request which is restoring a live response.
	state := s.scopes[t.scope.SessionID]
	next := map[string]entry{}
	bytes := 0
	for _, values := range []map[string]entry{t.values, state.values} {
		for p, v := range values {
			if _, ok := next[p]; ok {
				continue
			}
			if len(next) >= 8192 {
				break
			}
			if bytes+len(v.Value) > 4<<20 {
				continue
			}
			next[p] = v
			bytes += len(v.Value)
		}
	}
	state.values = next
	state.used = time.Now()
	s.scopes[t.scope.SessionID] = state
	if len(s.scopes) > 128 {
		oldest := ""
		for id, v := range s.scopes {
			if id != t.scope.SessionID && (oldest == "" || v.used.Before(s.scopes[oldest].used)) {
				oldest = id
			}
		}
		delete(s.scopes, oldest)
	}
	if t.suppressRecord {
		return
	}
	t.record.Failed = failed
	if !t.scanComplete {
		t.scanDuration = time.Since(t.record.Time)
	}
	t.record.DurationMS = (t.scanDuration + t.restoreDuration).Milliseconds()
	s.records = append([]Record{t.record}, s.records...)
	if len(s.records) > 200 {
		s.records = s.records[:200]
	}
}

type Preview struct {
	Text    string `json:"text"`
	Hits    []Hit  `json:"hits"`
	Blocked bool   `json:"blocked"`
}

func (s *Service) Preview(ctx context.Context, text string) (Preview, error) {
	return s.PreviewWithConfig(ctx, text, nil)
}

func (s *Service) PreviewWithConfig(ctx context.Context, text string, config *Config) (Preview, error) {
	if len(text) > 1<<20 {
		return Preview{}, errors.New("preview text is limited to 1 MB")
	}
	c, err := s.GetConfig(ctx)
	if err != nil {
		return Preview{}, err
	}
	if config != nil {
		c = config.normalized()
		if err = c.Validate(); err != nil {
			return Preview{}, err
		}
	}
	c.Enabled = true
	t, err := s.begin(ctx, c, true)
	if err != nil {
		return Preview{}, err
	}
	if c.ModelEnabled {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(c.ModelTimeoutSeconds)*time.Second)
		defer cancel()
	}
	out, err := t.maskText(ctx, text, "")
	if errors.Is(err, ErrBlocked) {
		err = nil
	}
	return Preview{out, t.hits, t.record.Blocked}, err
}

func randomID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return time.Now().UTC().Format("20060102150405.000000000")
	}
	return hex.EncodeToString(b[:])
}
