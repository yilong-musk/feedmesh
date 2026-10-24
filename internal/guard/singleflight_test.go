package guard

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestGroupCollapsesConcurrentCalls(t *testing.T) {
	g := NewGroup()
	var runs atomic.Int32
	release := make(chan struct{})

	work := func() (string, error) {
		runs.Add(1)
		<-release
		return "value", nil
	}

	const callers = 8
	var wg sync.WaitGroup
	results := make([]string, callers)
	shared := make([]bool, callers)
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			v, err, s := Do(g, "key", work)
			if err != nil {
				t.Errorf("caller %d: %v", i, err)
			}
			results[i], shared[i] = v, s
		}(i)
	}

	// Give the goroutines a moment to pile onto the same key.
	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()

	if runs.Load() != 1 {
		t.Fatalf("work ran %d times, want 1", runs.Load())
	}
	for i, v := range results {
		if v != "value" {
			t.Fatalf("caller %d got %q", i, v)
		}
	}
	joiners := 0
	for _, s := range shared {
		if s {
			joiners++
		}
	}
	if joiners != callers-1 {
		t.Fatalf("joiners = %d, want %d", joiners, callers-1)
	}
}

func TestGroupPropagatesErrors(t *testing.T) {
	g := NewGroup()
	want := errors.New("boom")
	v, err, _ := Do(g, "key", func() (int, error) { return 0, want })
	if !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
	if v != 0 {
		t.Fatalf("v = %d, want zero value", v)
	}
}

func TestGroupRecoversPanicAndClearsKey(t *testing.T) {
	g := NewGroup()
	if _, err, _ := Do(g, "key", func() (int, error) {
		panic("boom")
	}); err == nil {
		t.Fatal("expected a panic-derived error")
	}
	v, err, _ := Do(g, "key", func() (int, error) { return 7, nil })
	if err != nil || v != 7 {
		t.Fatalf("call after panic = %d, %v", v, err)
	}
}

func TestGroupSequentialCallsRunAgain(t *testing.T) {
	g := NewGroup()
	var runs atomic.Int32
	for i := 0; i < 3; i++ {
		if _, err, _ := Do(g, "key", func() (int, error) {
			runs.Add(1)
			return 1, nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	if runs.Load() != 3 {
		t.Fatalf("runs = %d, want 3 (no caching across sequential calls)", runs.Load())
	}
}
