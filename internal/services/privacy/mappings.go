package privacy

import (
	"context"
	"errors"
	"fmt"
	"time"

	"slimebot/internal/domain"
)

const (
	maxMappingScopes       = 128
	maxActiveScopes        = 256
	maxMappingEntries      = 8192
	maxMappingBytes        = 4 << 20
	maxMappingValue        = 64 << 10
	maxMappingSessionID    = 1024
	minPlaceholderBytes    = len("{{SB_X_}}") + 32
	maxPlaceholderBytes    = minPlaceholderBytes + 23
	mappingCleanupInterval = time.Minute
)

// ErrMappingConfiguration indicates an encrypted store is not ready for a
// requested settings change. No deployment key material is included.
var ErrMappingConfiguration = errors.New("privacy mapping configuration")

var (
	errMappingLimit  = errors.New("privacy mapping capacity exceeded; nothing was sent")
	errPrivacyClosed = errors.New("privacy service is closed")
)

type MappingStatus struct {
	Mode         string `json:"mode"`
	KeyStatus    string `json:"keyStatus"`
	StorageError bool   `json:"storageError,omitempty"`
}

type mappingOptions struct {
	encrypted bool
	ttl       time.Duration
}

func optionsFor(c Config) mappingOptions {
	return mappingOptions{c.MappingPersistence, time.Duration(c.MappingTTLMinutes) * time.Minute}
}

type mappingLease struct{ active int }

func mappingExpired(used, now time.Time, ttl time.Duration) bool {
	return !used.Add(ttl).After(now)
}

// All mapping state, including file I/O, is serialized by s.mu. Transactions
// work on local copies, so scanning or streaming a response never holds it.
func (s *Service) initMappingsLocked(c Config) {
	if s.mappingOptions.ttl != 0 {
		return
	}
	s.mappingOptions = optionsFor(c)
	s.mappingErr = s.sweepMappingsLocked(s.now(), s.mappingOptions.ttl)
}

func (s *Service) mappingStatusLocked() MappingStatus {
	status := MappingStatus{Mode: "memory", KeyStatus: "ready"}
	status.StorageError = s.mappingWriteErr
	for _, err := range []error{s.mappingDisk.keyErr, s.mappingErr} {
		switch {
		case errors.Is(err, errMappingKeyMissing):
			status.KeyStatus = "missing"
		case errors.Is(err, errMappingKeyInvalid):
			status.KeyStatus = "invalid"
		case errors.Is(err, errMappingKeyMismatch):
			status.KeyStatus = "mismatch"
		case err != nil:
			status.StorageError = true
		}
	}
	if status.StorageError || (s.mappingOptions.encrypted && status.KeyStatus != "ready") {
		status.Mode = "unavailable"
	} else if s.mappingOptions.encrypted {
		status.Mode = "encrypted"
	}
	return status
}

func (s *Service) expireScopesLocked(now time.Time, ttl time.Duration) {
	for id, state := range s.scopes {
		if mappingExpired(state.used, now, ttl) {
			delete(s.scopes, id)
			// An in-flight copy may still restore its live response, but its
			// finish must not renew this expired snapshot.
			delete(s.mappingLeases, id)
		}
	}
}

func (s *Service) sweepMappingsLocked(now time.Time, ttl time.Duration) error {
	if !s.mappingOptions.encrypted {
		return s.mappingDisk.purge()
	}
	return s.mappingDisk.sweep(now, ttl, false)
}

func (s *Service) cleanupMappings(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	if s.mappingOptions.ttl == 0 {
		c, err := s.GetConfig(ctx)
		if err != nil {
			return err
		}
		s.initMappingsLocked(c)
		return s.mappingErr
	}
	now := s.now()
	s.expireScopesLocked(now, s.mappingOptions.ttl)
	s.mappingErr = s.sweepMappingsLocked(now, s.mappingOptions.ttl)
	return s.mappingErr
}

// StartMappingCleanup sweeps at startup and every minute until ctx is
// canceled or Close is called. It is idempotent; no worker starts in NewService.
func (s *Service) StartMappingCleanup(ctx context.Context) {
	s.cleanupMu.Lock()
	defer s.cleanupMu.Unlock()
	if s.cleanupStopped || s.cleanupDone != nil || ctx.Err() != nil {
		return
	}
	ctx, s.cleanupCancel = context.WithCancel(ctx)
	done := make(chan struct{})
	s.cleanupDone = done
	go func() {
		defer close(done)
		ticker := time.NewTicker(mappingCleanupInterval)
		defer ticker.Stop()
		for ctx.Err() == nil {
			_ = s.cleanupMappings(ctx) // Readiness is exposed by Snapshot.
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

// DestroySession removes retained mappings and invalidates in-flight commits.
// Disk deletion does not need the encryption key, including during a key outage.
func (s *Service) DestroySession(sessionID string) error {
	if sessionID == "" || len(sessionID) > maxMappingSessionID {
		return errors.New("invalid privacy mapping session identifier")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.scopes, sessionID)
	delete(s.mappingLeases, sessionID)
	if err := s.mappingDisk.delete(mappingName(sessionID)); err != nil {
		s.mappingErr = err
		return err
	}
	return nil
}

func (s *Service) cacheScopeLocked(id string, state scopeState) {
	s.scopes[id] = state
	if len(s.scopes) <= maxMappingScopes {
		return
	}
	old := ""
	for candidate, v := range s.scopes {
		if candidate != id && (old == "" || v.used.Before(s.scopes[old].used)) {
			old = candidate
		}
	}
	delete(s.scopes, old)
	// When capacity evicts a snapshot, a request already using it can still
	// finish. Its lease stays valid unless the snapshot expires or is destroyed.
}

func (s *Service) takeMappingLocked(id string, restoreOnly bool) (scopeState, *mappingLease, error) {
	now := s.now()
	s.expireScopesLocked(now, s.mappingOptions.ttl)
	state := s.scopes[id]
	if len(state.values) == 0 && s.mappingOptions.encrypted {
		loaded, err := s.mappingDisk.load(mappingName(id), now, s.mappingOptions.ttl)
		if err != nil {
			s.mappingErr = err
			return scopeState{}, nil, err
		}
		state = loaded
		if len(state.values) != 0 {
			s.cacheScopeLocked(id, state)
		}
	}
	if restoreOnly && len(state.values) == 0 {
		return scopeState{}, nil, nil
	}
	lease := s.mappingLeases[id]
	if lease == nil {
		if len(s.mappingLeases) >= maxActiveScopes {
			return scopeState{}, nil, errMappingLimit
		}
		lease = &mappingLease{}
		s.mappingLeases[id] = lease
	}
	lease.active++
	return state, lease, nil
}

// commitMapping runs after scanning but before any provider I/O, so a
// failed write never silently emits new placeholders that were not retained.
func (t *transaction) commitMapping() error {
	s := t.service
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errPrivacyClosed
	}
	return t.commitMappingLocked(s.now())
}

func (t *transaction) commitMappingLocked(now time.Time) error {
	s := t.service
	id := t.scope.SessionID
	t.mappingCommitted = true
	if t.mappingLease == nil || len(t.values) == 0 {
		return nil
	}
	s.expireScopesLocked(now, s.mappingOptions.ttl)
	if s.mappingLeases[id] != t.mappingLease || (!t.mappingUsed.IsZero() && mappingExpired(t.mappingUsed, now, s.mappingOptions.ttl)) {
		return errors.New("privacy mappings expired or were removed during request preparation")
	}
	// The production settings repository can also verify session existence.
	// Standalone services may omit that capability. Unknown or deleted sessions
	// may mask within the current request, but must not acquire retained state.
	if sessions, ok := s.store.(interface {
		GetSessionByID(context.Context, string) (*domain.Session, error)
	}); ok {
		session, err := sessions.GetSessionByID(t.mappingContext, id)
		if err != nil || session == nil {
			return nil
		}
	}
	values := make(map[string]entry, min(len(t.values), maxMappingEntries))
	valueBytes := 0
	// A full retained table must not block new requests. Keep new values
	// first, then fill the remaining budget with existing entries.
	for _, table := range []map[string]entry{t.newValues, s.scopes[id].values, t.values} {
		for p, v := range table {
			if len(values) >= maxMappingEntries {
				break
			}
			if _, ok := values[p]; ok || valueBytes+len(v.Value) > maxMappingBytes {
				continue
			}
			values[p] = v
			valueBytes += len(v.Value)
		}
	}
	state := scopeState{values: values, used: now}
	if s.mappingOptions.encrypted && t.mappingEpoch == s.mappingEpoch {
		if err := s.mappingDisk.save(mappingName(id), state, now, s.mappingOptions.ttl); err != nil {
			if errors.Is(err, errMappingKeyMissing) || errors.Is(err, errMappingKeyInvalid) || errors.Is(err, errMappingKeyMismatch) {
				// A new session can still mask in memory during a key outage.
				s.mappingErr = err
			} else {
				s.mappingWriteErr = true
				return err
			}
		} else {
			// Unexpected write failures fail the request before provider I/O;
			// keep the warning until a real write succeeds, not just a read.
			s.mappingWriteErr = false
		}
	}
	s.cacheScopeLocked(id, state)
	return nil
}

// Receiving a new TTL is never permission to revive data that already
// expired under the old TTL. Only an explicit transition to memory removes
// inaccessible encrypted data without the deployment key.
func (s *Service) prepareMappingSettingsLocked(old, next Config) error {
	s.initMappingsLocked(old)
	opts := optionsFor(next)
	previous := s.mappingOptions
	if opts.encrypted && !previous.encrypted {
		if err := s.mappingDisk.purge(); err != nil {
			return err
		}
		if err := s.mappingDisk.ensureReady(); err != nil {
			return fmt.Errorf("%w: %v", ErrMappingConfiguration, err)
		}
	}
	if opts.ttl != previous.ttl && previous.encrypted && opts.encrypted {
		// Before the settings write, only purge records that are already
		// expired under the saved policy. This also verifies key readiness.
		s.mappingErr = s.mappingDisk.sweep(s.now(), previous.ttl, false)
		if s.mappingErr != nil {
			return fmt.Errorf("%w: %v", ErrMappingConfiguration, s.mappingErr)
		}
	}
	return nil
}

func (s *Service) applyMappingSettingsLocked(c Config) error {
	opts := optionsFor(c)
	previous := s.mappingOptions
	if opts.encrypted != previous.encrypted {
		s.mappingEpoch++
		s.mappingErr = nil
		s.mappingWriteErr = false
	}
	s.mappingOptions = opts
	now := s.now()
	s.expireScopesLocked(now, min(previous.ttl, opts.ttl))
	if !opts.encrypted || opts.ttl < previous.ttl {
		// Settings are committed. If cleanup hits an I/O error, report it
		// and retry under the committed policy on the next cleanup tick.
		s.mappingErr = s.sweepMappingsLocked(now, opts.ttl)
		return s.mappingErr
	}
	return nil
}
