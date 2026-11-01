package feed

import "time"

// staleWindow is how old a stored value may be when it is served because the
// upstream read failed. It is deliberately generous: a reader that gets a six
// hour old timeline keeps polling, while a reader that gets a 5xx treats the
// feed as broken. The Degraded flag and a short max-age make the staleness
// visible, and the next successful read replaces the entry.
const staleWindow = 6 * time.Hour

// storeTTL widens a provider's freshness TTL so the entry survives long enough
// to serve the last-good path. Freshness itself is decided by the read helpers
// from the stored time, never by the cache expiry.
func storeTTL(ttl time.Duration) time.Duration {
	if ttl > staleWindow {
		return ttl
	}
	return staleWindow
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
// window, whatever its freshness TTL was. Entries with a freshness TTL longer
// than the window may be served up to that TTL instead.
func (s *Service) readStale[T any](key string, ttl time.Duration) (T, bool) {
	var v T
	if s.cache == nil {
		return v, false
	}
	hit, storedAt, err := s.cache.Get(key, &v)
	if err != nil || !hit || storedAt.IsZero() {
		return v, false
	}
	window := staleWindow
	if ttl > window {
		window = ttl
	}
	if s.now().Sub(storedAt) > window {
		return v, false
	}
	return v, true
}
