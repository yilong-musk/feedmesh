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
// open for a fixed cooldown. Once the cooldown elapses it closes again, giving
// the upstream exactly one chance per cooldown window: a repeated block simply
// reopens it.
type Breaker struct {
	mu       sync.Mutex
	cooldown time.Duration
	now      func() time.Time
	until    time.Time
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

// Allow reports whether a request may proceed. When the cooldown has elapsed
// the breaker closes itself and the request is allowed through.
func (b *Breaker) Allow() bool {
	if b == nil {
		return true
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.until.IsZero() {
		return true
	}
	if b.now().Before(b.until) {
		return false
	}
	b.until = time.Time{}
	return true
}

// Trip opens the breaker and returns the instant it will close again.
func (b *Breaker) Trip() time.Time {
	if b == nil {
		return time.Time{}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
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
