package feed

import (
	"strings"
	"time"
)

// staleWindow is how old a stored value may be when it is served because the
// upstream read failed. It is deliberately generous: a reader that gets a six
// hour old timeline keeps polling, while a reader that gets a 5xx treats the
// feed as broken. The Degraded flag and a short max-age make the staleness
// visible, and the next successful read replaces the entry.
const staleWindow = 6 * time.Hour

// hotStaleWindow caps the last good copy of the hot-search ranking. The list is
// labelled realtime and changes within minutes, so a six-hour-old ranking would
// misinform a reader more than an error would.
const hotStaleWindow = time.Hour

// failureBackoff is how long a key that just failed is answered from its last
// good entry before another upstream attempt is made. Without it a reader that
// keeps polling a degraded 200 drives one full retry chain per poll, and the
// retry chain is the expensive part (Weibo: a 9 s timeout and three attempts).
const failureBackoff = time.Minute

// staleWindowFor is the window a stored entry is kept, and may still be served
// in, after its freshness TTL ran out. A provider whose freshness TTL is longer
// than the window keeps its entry, and is served from it, for that TTL instead.
func staleWindowFor(key string, ttl time.Duration) time.Duration {
	window := staleWindow
	if strings.HasPrefix(key, "hot:") {
		window = hotStaleWindow
	}
	if ttl > window {
		return ttl
	}
	return window
}

// readFresh returns the stored value for key when it is younger than ttl.
func (s *Service) readFresh[T any](key string, ttl time.Duration) (T, bool) {
	var v T
	if s.cache == nil {
		return v, false
	}
	hit, storedAt, err := s.cache.Get(key, &v)
	if err != nil || !hit || storedAt.IsZero() {
		return v, false
	}
	if s.now().Sub(storedAt) > ttl {
		return v, false
	}
	return v, true
}

// readStale returns the stored value for key when it is within the stale
// window, whatever its freshness TTL was.
func (s *Service) readStale[T any](key string, ttl time.Duration) (T, bool) {
	var v T
	if s.cache == nil {
		return v, false
	}
	hit, storedAt, err := s.cache.Get(key, &v)
	if err != nil || !hit || storedAt.IsZero() {
		return v, false
	}
	if s.now().Sub(storedAt) > staleWindowFor(key, ttl) {
		return v, false
	}
	return v, true
}

// coolingDown reports whether key failed recently enough that another upstream
// attempt would only repeat the failure. Entries that aged out are dropped, so
// the map only holds keys that are failing right now.
func (s *Service) coolingDown(key string) bool {
	s.failMu.Lock()
	defer s.failMu.Unlock()
	at, ok := s.failedAt[key]
	if !ok {
		return false
	}
	if s.now().Sub(at) >= failureBackoff {
		delete(s.failedAt, key)
		return false
	}
	return true
}

// noteFailure starts the cool-off for key.
func (s *Service) noteFailure(key string) {
	s.failMu.Lock()
	defer s.failMu.Unlock()
	if s.failedAt == nil {
		s.failedAt = make(map[string]time.Time)
	}
	s.failedAt[key] = s.now()
}

// noteSuccess ends the cool-off for key.
func (s *Service) noteSuccess(key string) {
	s.failMu.Lock()
	defer s.failMu.Unlock()
	delete(s.failedAt, key)
}
