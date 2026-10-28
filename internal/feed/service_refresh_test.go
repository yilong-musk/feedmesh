package feed

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/yilong-musk/feedmesh/internal/cache"
)

func TestFetchCachedListRefreshReplacesWiderCache(t *testing.T) {
	c, err := cache.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	svc := NewService(Clients{}, c, nil, TTLs{})
	ctx := context.Background()

	items := func(version string, n int) []string {
		out := make([]string, n)
		for i := range out {
			out[i] = fmt.Sprintf("%s-%d", version, i)
		}
		return out
	}

	if _, hit, err := fetchCachedList(ctx, svc, "test:list", time.Hour, false, 10,
		func(n int) ([]string, error) { return items("v0", n), nil }); err != nil || hit {
		t.Fatalf("warm call = hit %v, err %v", hit, err)
	}

	// A smaller refresh must still replace the wider cached entry with fresh
	// upstream data (and keep the cache width rather than truncating it).
	freshEntry, hit, err := fetchCachedList(ctx, svc, "test:list", time.Hour, true, 3,
		func(n int) ([]string, error) {
			if n != 10 {
				t.Fatalf("refresh fetched width %d, want the previous width 10", n)
			}
			return items("v1", n), nil
		})
	if err != nil {
		t.Fatal(err)
	}
	if hit {
		t.Fatal("refresh reported a cache hit")
	}
	fresh := freshEntry.Items
	if len(fresh) != 10 || fresh[0] != "v1-0" {
		t.Fatalf("refresh returned %v", fresh)
	}

	var stored cachedList[string]
	if ok, _, err := c.Get("test:list", &stored); err != nil || !ok {
		t.Fatalf("stored entry = %v, %v", ok, err)
	}
	if stored.Want != 10 || len(stored.Items) != 10 || stored.Items[0] != "v1-0" {
		t.Fatalf("stored entry after refresh = %+v", stored)
	}

	// A later small normal request is served from the refreshed cache.
	cachedEntry, hit, err := fetchCachedList(ctx, svc, "test:list", time.Hour, false, 3,
		func(n int) ([]string, error) {
			t.Fatal("normal request unexpectedly went upstream")
			return nil, nil
		})
	if err != nil || !hit {
		t.Fatalf("cached call = hit %v, err %v", hit, err)
	}
	cached := cachedEntry.Items
	if len(cached) != 10 || cached[0] != "v1-0" {
		t.Fatalf("cached call returned %v", cached)
	}

	// If upstream shrinks, refresh must return the fresh shorter list instead
	// of silently resurrecting the old cached items.
	shrunkEntry, hit, err := fetchCachedList(ctx, svc, "test:list", time.Hour, true, 10,
		func(n int) ([]string, error) { return items("v2", 5), nil })
	if err != nil {
		t.Fatal(err)
	}
	if hit {
		t.Fatal("shrinking refresh reported a cache hit")
	}
	shrunk := shrunkEntry.Items
	if len(shrunk) != 5 || shrunk[0] != "v2-0" {
		t.Fatalf("shrinking refresh returned %v", shrunk)
	}
}

// A narrow refresh that starts while the cache is empty must not overwrite a
// wider entry written by a concurrent refresh. Before per-key serialisation,
// refresh skipped the wider-entry merge and the last writer won.
func TestConcurrentRefreshDoesNotNarrowCache(t *testing.T) {
	c, err := cache.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	svc := NewService(Clients{}, c, nil, TTLs{})
	ctx := context.Background()

	items := func(version string, n int) []string {
		out := make([]string, n)
		for i := range out {
			out[i] = fmt.Sprintf("%s-%d", version, i)
		}
		return out
	}

	type result struct {
		items []string
		err   error
	}
	narrowEntered := make(chan struct{})
	releaseNarrow := make(chan struct{})
	narrowDone := make(chan result, 1)
	wideDone := make(chan result, 1)

	go func() {
		entry, _, ferr := fetchCachedList(ctx, svc, "test:race", time.Hour, true, 5,
			func(n int) ([]string, error) {
				close(narrowEntered)
				<-releaseNarrow
				return items("narrow", n), nil
			})
		narrowDone <- result{entry.Items, ferr}
	}()
	<-narrowEntered

	go func() {
		entry, _, ferr := fetchCachedList(ctx, svc, "test:race", time.Hour, true, 20,
			func(n int) ([]string, error) { return items("wide", n), nil })
		wideDone <- result{entry.Items, ferr}
	}()

	// Give the wide refresh time to finish its write under the old code. With
	// per-key serialisation it is still waiting for the narrow refresh.
	var early *result
	select {
	case r := <-wideDone:
		early = &r
	case <-time.After(250 * time.Millisecond):
	}

	close(releaseNarrow)
	narrow := <-narrowDone
	var wide result
	if early != nil {
		wide = *early
	} else {
		wide = <-wideDone
	}
	if narrow.err != nil || wide.err != nil {
		t.Fatalf("refresh errors: narrow=%v wide=%v", narrow.err, wide.err)
	}

	var stored cachedList[string]
	if ok, _, err := c.Get("test:race", &stored); err != nil || !ok {
		t.Fatalf("stored entry = %v, %v", ok, err)
	}
	if stored.Want < 20 || len(stored.Items) < 20 {
		t.Fatalf("concurrent refresh narrowed the cache: want=%d items=%d", stored.Want, len(stored.Items))
	}
}
