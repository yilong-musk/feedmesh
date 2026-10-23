package guard

import (
	"sync"
	"testing"
	"time"
)

type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func newFakeClock() *fakeClock {
	return &fakeClock{t: time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)}
}

func (f *fakeClock) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.t
}

func (f *fakeClock) Advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.t = f.t.Add(d)
}

func TestBreakerClosedByDefault(t *testing.T) {
	b := NewBreaker(10 * time.Minute)
	if !b.Allow() {
		t.Fatal("fresh breaker must allow requests")
	}
	st := b.State()
	if st.Open || st.Until != nil {
		t.Fatalf("fresh breaker state = %+v, want closed with nil until", st)
	}
}

func TestBreakerTripBlocksForCooldown(t *testing.T) {
	clock := newFakeClock()
	b := NewBreaker(10 * time.Minute)
	b.SetClock(clock.Now)

	until := b.Trip()
	if !until.After(clock.Now()) {
		t.Fatalf("trip deadline %v is not in the future", until)
	}
	if b.Allow() {
		t.Fatal("breaker must block while cooling down")
	}
	st := b.State()
	if !st.Open || st.Until == nil {
		t.Fatalf("state = %+v, want open with a deadline", st)
	}
	if st.Trips != 1 {
		t.Fatalf("trips = %d, want 1", st.Trips)
	}

	// Still inside the cooldown.
	clock.Advance(9 * time.Minute)
	if b.Allow() {
		t.Fatal("breaker must still block one minute before the deadline")
	}

	// Cooldown elapsed: one probe is allowed and the breaker closes.
	clock.Advance(2 * time.Minute)
	if !b.Allow() {
		t.Fatal("breaker must allow a probe after the cooldown")
	}
	if st := b.State(); st.Open || st.Until != nil {
		t.Fatalf("after cooldown state = %+v, want closed", st)
	}
}

func TestBreakerReopensOnRepeatedBlock(t *testing.T) {
	clock := newFakeClock()
	b := NewBreaker(time.Minute)
	b.SetClock(clock.Now)

	b.Trip()
	clock.Advance(2 * time.Minute)
	if !b.Allow() {
		t.Fatal("expected the breaker to close after cooldown")
	}
	b.Trip()
	if b.Allow() {
		t.Fatal("a repeated block must reopen the breaker")
	}
	if st := b.State(); st.Trips != 2 {
		t.Fatalf("trips = %d, want 2", st.Trips)
	}
}

func TestBreakerReset(t *testing.T) {
	b := NewBreaker(time.Hour)
	b.Trip()
	if b.Allow() {
		t.Fatal("expected open")
	}
	b.Reset()
	if !b.Allow() {
		t.Fatal("reset must close the breaker")
	}
}

func TestNilBreakerAllows(t *testing.T) {
	var b *Breaker
	if !b.Allow() {
		t.Fatal("nil breaker must allow")
	}
	if st := b.State(); st.Open {
		t.Fatal("nil breaker state must be closed")
	}
	if !b.Trip().IsZero() {
		t.Fatal("nil breaker trip must return zero time")
	}
}
