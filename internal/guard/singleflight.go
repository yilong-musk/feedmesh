package guard

import (
	"fmt"
	"sync"
)

type call struct {
	wg   sync.WaitGroup
	val  any
	err  error
	done bool
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
func Do[T any](g *Group, key string, fn func() (T, error)) (v T, err error, shared bool) {
	if g == nil {
		v, err = fn()
		return v, err, false
	}

	g.mu.Lock()
	if existing, ok := g.m[key]; ok {
		g.mu.Unlock()
		existing.wg.Wait()
		v, _ := existing.val.(T)
		return v, existing.err, true
	}
	c := &call{}
	c.wg.Add(1)
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
		c.done = true
		delete(g.m, key)
		g.mu.Unlock()
		c.wg.Done()
	}()

	v, err = fn()
	return v, err, false
}
