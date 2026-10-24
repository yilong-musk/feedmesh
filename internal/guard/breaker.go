// Package guard holds the two request-shaping primitives the upstream client
// needs: a circuit breaker that stops hammering weibo after a block, and a
// single-flight group that collapses concurrent identical fetches.
package guard

import (
	"sync"
	"time"
)

// State is a point-in-time view of the breaker, rendered in /healthz.
type State struct {
	Open  bool       `json:"open"`
	Until *time.Time `json:"until"`
	Trips int        `json:"trips"`
}

// Breaker is a cooldown circuit breaker.
//
// It opens when the upstream answers with a block status (418/403) and stays
// open for a fixed cooldown. Once the cooldown elapses it half-opens: exactly
// one caller is admitted as a probe. A successful probe closes the breaker; a
// repeated block reopens it and releases the probe for the next cooling window.
type Breaker struct {
	mu       sync.Mutex
	cooldown time.Duration
	now      func() time.Time
	until    time.Time
	probing  bool
	trips    int
}

// NewBreaker returns a closed breaker with the given cooldown.
func NewBreaker(cooldown time.Duration) *Breaker {
	if cooldown <= 0 {
		cooldown = 10 * time.Minute
	}
	return &Breaker{cooldown: cooldown, now: time.Now}
}

// SetClock replaces the clock; tests use it to travel through the cooldown.
func (b *Breaker) SetClock(now func() time.Time) {
	if b == nil || now == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.now = now
}

// Allow is a compatibility wrapper around AllowRequest. A true result after a
// cooldown has been admitted as the half-open probe and must be released with
// Success, Trip, or AbortProbe; request paths should call AllowRequest directly
// so they know whether they own the probe.
func (b *Breaker) Allow() bool {
	allowed, _ := b.AllowRequest()
	return allowed
}

// AllowRequest reports whether a request may proceed and whether it was
// admitted as the single half-open probe. Callers that receive probe=true must
// eventually call Success, Trip, or AbortProbe to release the probe.
func (b *Breaker) AllowRequest() (allowed, probe bool) {
	if b == nil {
		return true, false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.until.IsZero() {
		return true, false
	}
	if b.now().Before(b.until) {
		return false, false
	}
	if b.probing {
		return false, false
	}
	b.probing = true
	return true, true
}

// Success closes the breaker after the half-open probe succeeds. It is a no-op
// when the breaker did not admit a probe, so an ordinary in-flight request
// cannot close a breaker that another request just tripped.
func (b *Breaker) Success() {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.probing {
		return
	}
	b.probing = false
	b.until = time.Time{}
}

// AbortProbe releases a half-open probe without closing the breaker. The next
// caller may become the probe after the current cooldown has elapsed.
func (b *Breaker) AbortProbe() {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.probing = false
}

// Trip opens the breaker and returns the instant it will close again.
func (b *Breaker) Trip() time.Time {
	if b == nil {
		return time.Time{}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.probing = false
	b.until = b.now().Add(b.cooldown)
	b.trips++
	return b.until
}

// Reset closes the breaker immediately, e.g. after a successful probe.
func (b *Breaker) Reset() {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.probing = false
	b.until = time.Time{}
}

// State snapshots the breaker for health reporting.
func (b *Breaker) State() State {
	if b == nil {
		return State{}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	st := State{Trips: b.trips}
	if !b.until.IsZero() && b.now().Before(b.until) {
		until := b.until
		st.Open = true
		st.Until = &until
	}
	return st
}
