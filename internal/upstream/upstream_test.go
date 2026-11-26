package upstream

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestReadBody(t *testing.T) {
	b, err := ReadBody(strings.NewReader("12345"), 5)
	if err != nil || string(b) != "12345" {
		t.Fatalf("at the limit = %q, %v", b, err)
	}
	b, err = ReadBody(strings.NewReader("123456"), 5)
	if !errors.Is(err, ErrBodyTooLarge) || string(b) != "12345" {
		t.Fatalf("over the limit = %q, %v; want the first 5 bytes and ErrBodyTooLarge", b, err)
	}
}

func TestDrainStopsAtItsLimit(t *testing.T) {
	r := strings.NewReader(strings.Repeat("x", drainLimit+10))
	Drain(r)
	if r.Len() != 10 {
		t.Fatalf("left %d bytes, want 10", r.Len())
	}
}

func TestSlotWaiterLeavesOnItsDeadline(t *testing.T) {
	s := NewSlot(1)
	if err := s.Acquire(context.Background()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := s.Acquire(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("second Acquire = %v, want the deadline", err)
	}
	s.Release()
	if err := s.Acquire(context.Background()); err != nil {
		t.Fatalf("Acquire after Release = %v", err)
	}
}
