package guard

import (
	"context"
	"fmt"
	"sync"
)

type call struct {
	done chan struct{}
	val  any
	err  error
}

// Group collapses concurrent calls sharing a key into a single execution.
// The zero value is not usable; call NewGroup.
type Group struct {
	mu sync.Mutex
	m  map[string]*call
}

// NewGroup returns an empty single-flight group.
func NewGroup() *Group {
	return &Group{m: make(map[string]*call)}
}

// Do runs fn once per key while a call for that key is in flight. Waiters
// receive the same value and error. The reported shared flag is true when the
// caller joined an existing call instead of doing the work itself.
//
// A waiter stops waiting when its own ctx ends and returns ctx.Err(); the
// in-flight call keeps running for the caller that started it and for the
// remaining waiters. fn is responsible for its own deadline: it must not
// depend on the cancellation of whichever caller happened to start it,
// otherwise one caller that goes away fails everybody who joined.
func Do[T any](ctx context.Context, g *Group, key string, fn func() (T, error)) (v T, err error, shared bool) {
	if g == nil {
		v, err = fn()
		return v, err, false
	}

	g.mu.Lock()
	if existing, ok := g.m[key]; ok {
		g.mu.Unlock()
		select {
		case <-existing.done:
		case <-ctx.Done():
			var zero T
			return zero, ctx.Err(), true
		}
		v, _ := existing.val.(T)
		return v, existing.err, true
	}
	c := &call{done: make(chan struct{})}
	g.m[key] = c
	g.mu.Unlock()

	// A panic in fn must still remove the in-flight call; otherwise every
	// waiter would block forever. Waiters receive a normal error instead.
	defer func() {
		if r := recover(); r != nil {
			var zero T
			v = zero
			err = fmt.Errorf("singleflight panic: %v", r)
		}
		g.mu.Lock()
		c.val = v
		c.err = err
		delete(g.m, key)
		g.mu.Unlock()
		close(c.done)
	}()

	v, err = fn()
	return v, err, false
}
