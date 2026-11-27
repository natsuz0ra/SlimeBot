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
	Config   Config        `json:"config"`
	Model    ModelStatus   `json:"model"`
	Records  []Record      `json:"records"`
	Mappings MappingStatus `json:"mappings"`
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

	mappingDisk     mappingDisk
	mappingOptions  mappingOptions
	mappingLeases   map[string]*mappingLease
	mappingEpoch    uint64
	mappingErr      error
	mappingWriteErr bool
	closed          bool
	now             func() time.Time

	cleanupMu      sync.Mutex
	cleanupStopped bool
	cleanupCancel  context.CancelFunc
	cleanupDone    chan struct{}

	// Detector may be replaced in focused tests without loading model weights.
	detect func(context.Context, string, float64) ([]span, error)
}

func NewService(store domain.SettingsStore, dir string) *Service {
	s := &Service{
		store: store, dir: dir, scopes: map[string]scopeState{}, records: []Record{}, ruleCache: map[[32]byte][]span{},
		mappingDisk: newMappingDisk(dir), mappingLeases: map[string]*mappingLease{}, now: time.Now,
	}
	s.model = NewLocalModel(filepath.Join(dir, "model"))
	s.detect = s.model.Detect
	return s
}

var errInvalidSavedConfig = errors.New("invalid saved privacy settings")

func (s *Service) GetConfig(ctx context.Context) (Config, error) {
	c := DefaultConfig()
	if s.store == nil {
		return c, errors.New("privacy settings unavailable")
	}
	raw, err := s.store.GetSetting(ctx, settingKey)
	if err != nil {
		return c, errors.New("cannot read privacy settings")
	}
	if raw != "" && json.Unmarshal([]byte(raw), &c) != nil {
		return c, errInvalidSavedConfig
	}
	if c.Validate() != nil {
		return c, errInvalidSavedConfig
	}
	return c.normalized(), nil
}

func (s *Service) Update(ctx context.Context, c Config) error {
	return s.UpdateWithMappingSettings(ctx, c, &c.MappingPersistence, &c.MappingTTLMinutes)
}

// UpdateWithMappingSettings preserves either lifecycle option omitted by a
// legacy client. In particular, only an explicit opt-out purges stored mappings.
func (s *Service) UpdateWithMappingSettings(ctx context.Context, c Config, persistent *bool, ttl *int) error {
	c = c.normalized()
	err := func() error {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.closed {
			return errPrivacyClosed
		}
		old, err := s.GetConfig(ctx)
		if errors.Is(err, errInvalidSavedConfig) {
			// Allow an administrator to repair invalid saved settings without
			// treating an existing encrypted store as implicitly opted out.
			old = DefaultConfig()
			if s.mappingOptions.ttl != 0 {
				old.MappingPersistence = s.mappingOptions.encrypted
				old.MappingTTLMinutes = int(s.mappingOptions.ttl / time.Minute)
			} else {
				old.MappingPersistence, err = s.mappingDisk.directoryExists()
				if err != nil {
					return err
				}
				if old.MappingPersistence {
					old.MappingTTLMinutes = maxMappingTTLMinutes
				}
			}
		} else if err != nil {
			return err
		}
		c.MappingPersistence = old.MappingPersistence
		c.MappingTTLMinutes = old.MappingTTLMinutes
		if persistent != nil {
			c.MappingPersistence = *persistent
		}
		if ttl != nil {
			c.MappingTTLMinutes = *ttl
		}
		if err := c.Validate(); err != nil {
			return err
		}
		data, err := json.Marshal(c)
		if err != nil {
			return err
		}
		if err = s.prepareMappingSettingsLocked(old, c); err != nil {
			return err
		}
		if err = s.store.SetSetting(ctx, settingKey, string(data)); err != nil {
			return errors.New("cannot save privacy settings")
		}
		return s.applyMappingSettingsLocked(c)
	}()
	if err == nil && !c.ModelEnabled {
		s.model.Stop()
	}
	return err
}

func (s *Service) Snapshot(ctx context.Context) (Snapshot, error) {
	s.mu.Lock()
	c, err := s.GetConfig(ctx)
	if err != nil {
		s.mu.Unlock()
		return Snapshot{}, err
	}
	if !s.closed {
		s.initMappingsLocked(c)
	}
	records := append([]Record{}, s.records...)
	status := s.mappingStatusLocked()
	s.mu.Unlock()
	return Snapshot{Config: c, Model: s.model.Status(), Records: records, Mappings: status}, nil
}

func (s *Service) ClearRecords()      { s.mu.Lock(); s.records = []Record{}; s.mu.Unlock() }
func (s *Service) Model() *LocalModel { return s.model }
func (s *Service) Close() {
	// Cancel before waiting for mu: startup cleanup may be reading settings
	// under mu with the worker's context.
	s.cleanupMu.Lock()
	s.cleanupStopped = true
	if s.cleanupCancel != nil {
		s.cleanupCancel()
	}
	done := s.cleanupDone
	s.cleanupMu.Unlock()

	s.mu.Lock()
	s.closed = true
	clear(s.scopes)
	clear(s.mappingLeases)
	clear(s.ruleCache)
	s.key = nil
	s.mappingDisk.aead = nil
	s.mu.Unlock()
	if done != nil {
		<-done
	}
	s.model.Cancel()
	s.model.Stop()
}

func (s *Service) begin(ctx context.Context, c Config, preview bool) (*transaction, error) {
	return s.beginRequest(ctx, c, preview, false)
}

func (s *Service) beginRequest(ctx context.Context, c Config, preview, restoreOnly bool) (*transaction, error) {
	scope := llm.PrivacyScopeFromContext(ctx)
	retained := !preview && scope.SessionID != ""
	if !retained {
		if restoreOnly {
			return nil, nil
		}
		scope.SessionID = randomID()
	} else if len(scope.SessionID) > maxMappingSessionID {
		return nil, errMappingLimit
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, errPrivacyClosed
	}
	var state scopeState
	var lease *mappingLease
	if retained {
		s.initMappingsLocked(c)
		var err error
		state, lease, err = s.takeMappingLocked(scope.SessionID, restoreOnly)
		if err != nil || (restoreOnly && lease == nil) {
			return nil, err
		}
	}
	if err := s.loadPlaceholderKeyLocked(); err != nil {
		if lease != nil {
			lease.active--
			if lease.active == 0 {
				delete(s.mappingLeases, scope.SessionID)
			}
		}
		return nil, err
	}
	values := make(map[string]entry, len(state.values))
	t := &transaction{service: s, config: c, key: s.key, scope: scope, values: values, index: map[string][]entry{}, record: Record{ID: randomID(), Time: time.Now(), SessionID: scope.SessionID, Source: scope.Source, Kinds: map[string]int{}}, preview: preview, mappingContext: ctx, mappingLease: lease, mappingUsed: state.used, mappingEpoch: s.mappingEpoch}
	encodedConfig, _ := json.Marshal(c)
	t.configHash = sha256.Sum256(encodedConfig)
	for p, v := range state.values {
		values[p] = v
		t.indexValue(v)
	}
	return t, nil
}

func (s *Service) loadPlaceholderKeyLocked() error {
	if s.key == nil {
		if err := os.MkdirAll(s.dir, 0o700); err != nil {
			return errors.New("cannot create privacy state directory")
		}
		path := filepath.Join(s.dir, "placeholder.key")
		key, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			key = make([]byte, 32)
			if _, err = rand.Read(key); err != nil {
				return errors.New("cannot create privacy key")
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
			return errors.New("cannot load privacy key")
		}
		s.key = key
	}
	return nil
}

func (t *transaction) finish(failed bool) error {
	if t.preview {
		return nil
	}
	s := t.service
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || t.finished {
		return nil
	}
	t.finished = true
	var err error
	if !failed && !t.mappingCommitted {
		err = t.commitMappingLocked(s.now())
	}
	if t.mappingLease != nil {
		id := t.scope.SessionID
		if s.mappingLeases[id] != t.mappingLease {
			return err
		}
		t.mappingLease.active--
		if t.mappingLease.active == 0 {
			delete(s.mappingLeases, id)
		}
	}
	if t.suppressRecord {
		return err
	}
	t.record.Failed = failed || err != nil
	if !t.scanComplete {
		t.scanDuration = time.Since(t.record.Time)
	}
	t.record.DurationMS = (t.scanDuration + t.restoreDuration).Milliseconds()
	s.records = append([]Record{t.record}, s.records...)
	if len(s.records) > 200 {
		s.records = s.records[:200]
	}
	return err
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
		previewConfig := config.normalized()
		// A preview never uses live mappings, regardless of the client draft.
		previewConfig.MappingPersistence = c.MappingPersistence
		previewConfig.MappingTTLMinutes = c.MappingTTLMinutes
		c = previewConfig
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
