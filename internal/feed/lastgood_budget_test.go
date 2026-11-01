package feed

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yilong-musk/feedmesh/internal/cache"
	"github.com/yilong-musk/feedmesh/internal/logx"
	"github.com/yilong-musk/feedmesh/internal/smzdm"
	"github.com/yilong-musk/feedmesh/internal/weibo"
)

// ageStoredKey rewrites a stored entry so it is older than its freshness TTL
// while its on-disk expiry is still in the future.
func ageStoredKey(t *testing.T, dir, key string, age time.Duration) {
	t.Helper()
	p := filepath.Join(dir, cache.SafeKey(key)+".json")
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read %s: %v", p, err)
	}
	var e cache.Entry
	if err := json.Unmarshal(raw, &e); err != nil {
		t.Fatalf("decode %s: %v", p, err)
	}
	e.StoredAt = time.Now().Add(-age).UTC()
	e.ExpiresAt = time.Now().Add(6*time.Hour - age).UTC()
	out, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, out, 0o600); err != nil {
		t.Fatal(err)
	}
}

// A category page served from its last good entry must say that it is degraded
// and must spend the hour's read budget instead of buying the 600 KB page again
// on the next poll.
func TestSmzdmFenleiLastGoodIsFlaggedAndSpendsTheBudget(t *testing.T) {
	var down atomic.Bool
	var hits atomic.Int32
	body := buildFenleiPage(t, fenleiCards()...)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/fenlei/") {
			http.NotFound(w, r)
			return
		}
		hits.Add(1)
		if down.Load() {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	dir := t.TempDir()
	c, err := cache.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	client := smzdm.New(smzdm.Config{BaseURL: srv.URL, Timeout: 2 * time.Second, PageTimeout: 5 * time.Second})
	s := NewService(Clients{Smzdm: client}, c, logx.Discard(), DefaultTTLs())
	ctx := context.Background()

	if _, err := s.GetSmzdmFenlei(ctx, "tixuhuli", 20, false); err != nil {
		t.Fatalf("warm read: %v", err)
	}
	warmHits := hits.Load()
	ageStoredKey(t, dir, "smzdm:fenlei:tixuhuli", 2*time.Hour)
	down.Store(true)

	first, err := s.GetSmzdmFenlei(ctx, "tixuhuli", 20, false)
	if err != nil {
		t.Fatalf("the last good page must be served: %v", err)
	}
	if !first.Degraded {
		t.Error("degraded = false, want true for a page served from the last good entry")
	}
	if len(first.Items) == 0 {
		t.Fatal("want the cached cards")
	}
	if got := hits.Load() - warmHits; got != 1 {
		t.Fatalf("upstream reads on the first failing poll = %d, want 1", got)
	}

	second, err := s.GetSmzdmFenlei(ctx, "tixuhuli", 20, false)
	if err != nil {
		t.Fatalf("second poll inside the budget window: %v", err)
	}
	if !second.Degraded {
		t.Error("degraded = false on the second poll, want true")
	}
	if got := hits.Load() - warmHits; got != 1 {
		t.Fatalf("upstream reads after two failing polls = %d, want 1: a failed read must spend the hour's budget", got)
	}
}

// fakeWeibo serves the profile, timeline and hot-search endpoints with
// independent switches and read counters.
type fakeWeibo struct {
	profileDown  atomic.Bool
	timelineDown atomic.Bool
	hotDown      atomic.Bool
	profileHits  atomic.Int32
	timelineHits atomic.Int32
	hotHits      atomic.Int32
}

func (f *fakeWeibo) server(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/visitor":
			http.SetCookie(w, &http.Cookie{Name: "SUB", Value: "x", Path: "/"})
			_, _ = w.Write([]byte(`{"retcode":20000000}`))
		case "/hot":
			f.hotHits.Add(1)
			if f.hotDown.Load() {
				http.Error(w, "boom", http.StatusInternalServerError)
				return
			}
			_, _ = w.Write([]byte(`{"ok":1,"data":{"realtime":[` +
				`{"word":"topic one","num":100,"realpos":1},` +
				`{"word":"topic two","num":50,"realpos":2}]}}`))
		case "/api/container/getIndex":
			if r.URL.Query().Get("containerid") == "" {
				f.profileHits.Add(1)
				if f.profileDown.Load() {
					http.Error(w, "boom", http.StatusInternalServerError)
					return
				}
				_, _ = w.Write([]byte(`{"ok":1,"data":{"userInfo":{"id":1,"screen_name":"tester"}}}`))
				return
			}
			f.timelineHits.Add(1)
			if f.timelineDown.Load() {
				http.Error(w, "boom", http.StatusInternalServerError)
				return
			}
			_, _ = w.Write([]byte(`{"ok":1,"data":{"cards":[{"mblog":{` +
				`"id":"1001","created_at":"Fri Oct 23 14:30:04 +0800 2026","text":"<p>hello</p>"}}]}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func (f *fakeWeibo) serviceFor(t *testing.T, base, dir string) *Service {
	t.Helper()
	c, err := cache.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	client := weibo.New(weibo.Config{
		BaseURL: base, VisitorURL: base + "/visitor", HotURL: base + "/hot",
		Timeout: 2 * time.Second, Backoff: time.Millisecond, Retries: 1,
	})
	return NewService(Clients{Weibo: client}, c, logx.Discard(), DefaultTTLs())
}

// A feed whose title comes from a stale profile is degraded too: the reader
// has to be able to tell that part of the document is old.
func TestFeedIsDegradedWhenOnlyTheProfileFails(t *testing.T) {
	f := &fakeWeibo{}
	srv := f.server(t)
	dir := t.TempDir()
	s := f.serviceFor(t, srv.URL, dir)
	ctx := context.Background()

	if _, err := s.GetFeed(ctx, "1", 10, false); err != nil {
		t.Fatalf("warm read: %v", err)
	}
	ageStoredKey(t, dir, "profile:1", 2*time.Hour)
	f.profileDown.Store(true)

	got, err := s.GetFeed(ctx, "1", 10, false)
	if err != nil {
		t.Fatalf("a stale profile must not fail the feed: %v", err)
	}
	if !strings.Contains(got.Title(), "tester") {
		t.Fatalf("title = %q, want the cached profile's name", got.Title())
	}
	if !got.Degraded {
		t.Error("degraded = false, want true when the profile came from the cache after a failed read")
	}
}

// After a failed read the same key must not be sent upstream again on every
// poll: the last good entry is served while the failure cools down.
func TestFailedKeyBacksOffBeforeTheNextUpstreamRead(t *testing.T) {
	f := &fakeWeibo{}
	srv := f.server(t)
	dir := t.TempDir()
	s := f.serviceFor(t, srv.URL, dir)
	ctx := context.Background()

	clock := time.Now()
	s.now = func() time.Time { return clock }

	if _, err := s.GetFeed(ctx, "1", 10, false); err != nil {
		t.Fatalf("warm read: %v", err)
	}
	warm := f.timelineHits.Load()
	ageStoredKey(t, dir, "timeline:1", 6*time.Minute)
	f.timelineDown.Store(true)

	if _, err := s.GetFeed(ctx, "1", 10, false); err != nil {
		t.Fatalf("last good timeline: %v", err)
	}
	failedReads := f.timelineHits.Load() - warm
	if failedReads == 0 {
		t.Fatal("the failing read never reached upstream")
	}

	// Inside the cool-off: the stale entry is served without a new attempt.
	clock = clock.Add(10 * time.Second)
	if _, err := s.GetFeed(ctx, "1", 10, false); err != nil {
		t.Fatalf("second poll inside the cool-off: %v", err)
	}
	if got := f.timelineHits.Load() - warm; got != failedReads {
		t.Fatalf("upstream reads inside the cool-off = %d, want %d (no new attempt)", got, failedReads)
	}

	// Past the cool-off: the read is attempted again.
	clock = clock.Add(2 * time.Minute)
	if _, err := s.GetFeed(ctx, "1", 10, false); err != nil {
		t.Fatalf("poll after the cool-off: %v", err)
	}
	if got := f.timelineHits.Load() - warm; got <= failedReads {
		t.Fatalf("upstream reads after the cool-off = %d, want another attempt", got)
	}
}

// A hot-search ranking loses its value within minutes, so the shared six-hour
// window must not apply to it: an hour-old ranking is served, an older one is
// an error like it was before.
func TestHotSearchLastGoodWindowIsAnHour(t *testing.T) {
	f := &fakeWeibo{}
	srv := f.server(t)
	dir := t.TempDir()
	s := f.serviceFor(t, srv.URL, dir)
	ctx := context.Background()

	if _, err := s.GetHot(ctx, 2, false); err != nil {
		t.Fatalf("warm read: %v", err)
	}
	ageStoredKey(t, dir, "hot:top", 2*time.Hour)
	f.hotDown.Store(true)

	if got, err := s.GetHot(ctx, 2, false); err == nil {
		t.Fatalf("a two-hour-old ranking must not be served as realtime: degraded=%v items=%d", got.Degraded, len(got.Items))
	}

	ageStoredKey(t, dir, "hot:top", 20*time.Minute)
	if got, err := s.GetHot(ctx, 2, false); err != nil {
		t.Fatalf("a twenty-minute-old ranking must be served: %v", err)
	} else if !got.Degraded {
		t.Error("degraded = false, want true for a last good hot search")
	}
}
