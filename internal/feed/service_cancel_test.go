package feed

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/yilong-musk/feedmesh/internal/cache"
)

func newCancelTestService(t *testing.T) *Service {
	t.Helper()
	c, err := cache.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return NewService(Clients{}, c, nil, TTLs{})
}

// A reader that disconnects while its upstream read is shared must not fail
// the readers that joined it, and must not put the key into the failure
// cool-off: nothing failed upstream.
func TestFetchCachedLeaderCancelDoesNotFailJoinedReaders(t *testing.T) {
	svc := newCancelTestService(t)
	const key = "test:shared"

	started := make(chan struct{})
	release := make(chan struct{})
	fn := func(ctx context.Context) (string, error) {
		close(started)
		select {
		case <-release:
			return "value", nil
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}

	leaderCtx, cancelLeader := context.WithCancel(context.Background())
	leaderDone := make(chan error, 1)
	go func() {
		_, _, _, err := fetchCached(leaderCtx, svc, key, time.Hour, false, fn)
		leaderDone <- err
	}()
	<-started

	type result struct {
		v   string
		err error
	}
	waiterDone := make(chan result, 1)
	go func() {
		v, _, _, err := fetchCached(context.Background(), svc, key, time.Hour, false,
			func(context.Context) (string, error) {
				t.Error("the joined reader must not start a second upstream read")
				return "", nil
			})
		waiterDone <- result{v, err}
	}()
	// Let the second reader join the flight, then drop the first one.
	time.Sleep(30 * time.Millisecond)
	cancelLeader()
	time.Sleep(30 * time.Millisecond)
	close(release)

	if err := <-leaderDone; err != nil {
		t.Fatalf("leader err = %v, want the shared read to finish", err)
	}
	if r := <-waiterDone; r.err != nil || r.v != "value" {
		t.Fatalf("joined reader = %q, %v; want value", r.v, r.err)
	}
	if svc.coolingDown(key) {
		t.Fatal("a cancelled caller put the key into the failure cool-off")
	}
}

func TestFetchCachedWaiterGivesUpOnItsOwnDeadline(t *testing.T) {
	svc := newCancelTestService(t)
	const key = "test:slow"

	started := make(chan struct{})
	release := make(chan struct{})
	defer close(release)
	go func() {
		_, _, _, _ = fetchCached(context.Background(), svc, key, time.Hour, false,
			func(context.Context) (string, error) {
				close(started)
				<-release
				return "value", nil
			})
	}()
	<-started

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	begin := time.Now()
	_, _, _, err := fetchCached(ctx, svc, key, time.Hour, false,
		func(context.Context) (string, error) { return "", nil })
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want the waiter's own deadline", err)
	}
	if waited := time.Since(begin); waited > time.Second {
		t.Fatalf("waiter blocked for %s after its deadline", waited)
	}
	if svc.coolingDown(key) {
		t.Fatal("a caller deadline put the key into the failure cool-off")
	}
}

func TestFetchCachedListQueuedReaderGivesUpOnItsOwnDeadline(t *testing.T) {
	svc := newCancelTestService(t)
	const key = "test:list-slow"

	started := make(chan struct{})
	release := make(chan struct{})
	defer close(release)
	go func() {
		_, _, _, _ = fetchCachedList(context.Background(), svc, key, time.Hour, false, 5,
			func(context.Context, int) ([]string, error) {
				close(started)
				<-release
				return []string{"a"}, nil
			})
	}()
	<-started

	// A wider request takes a different flight but queues on the same key
	// lock; it must still leave when its own request ends.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, _, _, err := fetchCachedList(ctx, svc, key, time.Hour, false, 10,
		func(context.Context, int) ([]string, error) { return nil, nil })
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want the queued reader's own deadline", err)
	}
	if svc.coolingDown(key) {
		t.Fatal("a caller deadline put the key into the failure cool-off")
	}
}

func TestDetachKeepsDeadlineButNotCancellation(t *testing.T) {
	parent, cancel := context.WithTimeout(context.Background(), time.Hour)
	want, _ := parent.Deadline()
	ctx, stop := detach(parent)
	defer stop()
	cancel()
	if err := ctx.Err(); err != nil {
		t.Fatalf("detached ctx err = %v after the parent was cancelled", err)
	}
	if got, ok := ctx.Deadline(); !ok || !got.Equal(want) {
		t.Fatalf("detached deadline = %v, %v; want %v", got, ok, want)
	}
}
