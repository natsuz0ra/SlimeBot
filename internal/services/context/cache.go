package contextsvc

import (
	"errors"
	"fmt"
	"time"
)

type summaryRejection struct{ reason string }

func (e *summaryRejection) Error() string { return e.reason }
func reject(reason string) error          { return &summaryRejection{reason: reason} }

type cacheEntry struct {
	summary, rejection string
	expires            time.Time
	touched            uint64
	size               int
}

func (s *Service) cached(key string) (string, error, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for k, v := range s.cache {
		if !now.Before(v.expires) {
			delete(s.cache, k)
			s.cacheBytes -= v.size
		}
	}
	v, ok := s.cache[key]
	if !ok {
		return "", nil, false
	}
	s.tick++
	v.touched = s.tick
	s.cache[key] = v
	if v.rejection != "" {
		return "", reject(v.rejection), true
	}
	return v.summary, nil, true
}
func (s *Service) cacheResult(key, summary string, err error) {
	var rejection string
	if err != nil {
		var e *summaryRejection
		if !errors.As(err, &e) {
			return
		}
		rejection = e.reason
	}
	size := len(key) + len(summary) + len(rejection)
	if size > 256*1024 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cache == nil {
		s.cache = map[string]cacheEntry{}
	}
	if old, ok := s.cache[key]; ok {
		s.cacheBytes -= old.size
		delete(s.cache, key)
	}
	for len(s.cache) >= 16 || s.cacheBytes+size > 256*1024 {
		oldest := ""
		tick := ^uint64(0)
		for k, v := range s.cache {
			if v.touched < tick {
				oldest = k
				tick = v.touched
			}
		}
		if oldest == "" {
			break
		}
		s.cacheBytes -= s.cache[oldest].size
		delete(s.cache, oldest)
	}
	ttl := 5 * time.Minute
	if rejection != "" {
		ttl = 30 * time.Second
	}
	s.tick++
	s.cache[key] = cacheEntry{summary: summary, rejection: rejection, expires: time.Now().Add(ttl), touched: s.tick, size: size}
	s.cacheBytes += size
}

func cacheKey(scope, inputHash string) string { return fmt.Sprintf("%s:%s", scope, inputHash) }
