// Package upstream holds the small pieces every provider client needs when it
// talks to an upstream: bounded body reads and a context-aware request slot.
package upstream

import (
	"context"
	"errors"
	"fmt"
	"io"
)

// ErrBodyTooLarge reports a response body above the caller's limit. Reading
// only the first limit bytes used to hand a truncated document to the parser,
// which then failed with a misleading "not JSON" or "parse" error.
var ErrBodyTooLarge = errors.New("upstream response body exceeds the size limit")

// ReadBody reads r up to limit bytes. A longer body returns the first limit
// bytes together with ErrBodyTooLarge.
func ReadBody(r io.Reader, limit int64) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return b, err
	}
	if int64(len(b)) > limit {
		return b[:limit], fmt.Errorf("%w (%d bytes)", ErrBodyTooLarge, limit)
	}
	return b, nil
}

// drainLimit is how much of an unwanted body is read so the connection can go
// back to the pool; anything longer is cheaper to drop with the connection.
const drainLimit = 64 << 10

// Drain discards the rest of an unwanted body (up to 64 KB) so the keep-alive
// connection can be reused. The caller still closes the body.
func Drain(r io.Reader) {
	_, _ = io.Copy(io.Discard, io.LimitReader(r, drainLimit))
}

// Slot is a counting semaphore whose waiters leave when their context ends.
// A sync.Mutex held across network I/O makes a queued caller wait past its own
// deadline; a Slot does not.
type Slot chan struct{}

// NewSlot returns a slot that admits n holders at a time.
func NewSlot(n int) Slot {
	if n < 1 {
		n = 1
	}
	return make(Slot, n)
}

// Acquire blocks until the caller holds the slot or ctx ends.
func (s Slot) Acquire(ctx context.Context) error {
	select {
	case s <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Release gives the slot back. It must follow a successful Acquire.
func (s Slot) Release() { <-s }
