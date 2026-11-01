package httpapi_test

import (
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
	"github.com/yilong-musk/feedmesh/internal/feed"
	"github.com/yilong-musk/feedmesh/internal/guard"
	"github.com/yilong-musk/feedmesh/internal/httpapi"
	"github.com/yilong-musk/feedmesh/internal/logx"
	"github.com/yilong-musk/feedmesh/internal/weibo"
)

// ageStoredEntry rewrites a cache entry so it looks older than its freshness
// TTL while its on-disk expiry (the stale window) is still in the future.
func ageStoredEntry(t *testing.T, dir, key string, age time.Duration) {
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

// The timeline endpoint has to keep answering from the last good copy when the
// upstream fails, and it has to say so: the header explains the staleness and
// the short max-age lets the reader pick the fresh feed up on recovery.
func TestRSSDegradedHeaderOnLastGoodTimeline(t *testing.T) {
	var down atomic.Bool
	fake := fakeWeibo(t)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if down.Load() {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		fake(w, r)
	}))
	defer upstream.Close()

	dir := t.TempDir()
	c, err := cache.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	breaker := guard.NewBreaker(10 * time.Minute)
	client := weibo.New(weibo.Config{
		BaseURL:    upstream.URL,
		VisitorURL: upstream.URL + "/visitor",
		Timeout:    2 * time.Second,
		Backoff:    time.Millisecond,
		Breaker:    breaker,
		Log:        logx.Discard(),
	})
	svc := feed.NewService(feed.Clients{Weibo: client}, c, logx.Discard(), feed.DefaultTTLs())
	api := httpapi.New(httpapi.Config{
		Version: "test", Service: svc, Cache: c, Client: client, Breaker: breaker, Log: logx.Discard(),
	})
	h := api.Handler()

	warm := get(t, h, "/rss/user/2803301701")
	if warm.Code != http.StatusOK {
		t.Fatalf("warm status = %d, want 200", warm.Code)
	}
	if warm.Header().Get("X-Feedmesh-Degraded") != "" {
		t.Fatal("degraded header on a healthy read")
	}

	ageStoredEntry(t, dir, "timeline:2803301701", 6*time.Minute)
	down.Store(true)

	rec := get(t, h, "/rss/user/2803301701")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want the last good timeline", rec.Code)
	}
	if rec.Header().Get("X-Feedmesh-Degraded") != "1" {
		t.Fatal("missing X-Feedmesh-Degraded on a last-good timeline")
	}
	if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "max-age=60") {
		t.Fatalf("Cache-Control = %q, want max-age=60", cc)
	}
	if !strings.Contains(rec.Body.String(), "人民日报") {
		t.Fatal("body does not carry the cached timeline")
	}
}
