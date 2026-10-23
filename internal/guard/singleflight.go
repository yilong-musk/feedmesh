package guard

import "sync"

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
func Do[T any](g *Group, key string, fn func() (T, error)) (T, error, bool) {
	if g == nil {
		v, err := fn()
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

	v, err := fn()

	g.mu.Lock()
	c.val = v
	c.err = err
	c.done = true
	delete(g.m, key)
	g.mu.Unlock()
	c.wg.Done()

	return v, err, false
}
