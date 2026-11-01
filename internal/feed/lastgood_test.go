package feed_test

import (
	"context"
	"encoding/json"
	"fmt"
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
	"github.com/yilong-musk/feedmesh/internal/v2ex"
	"github.com/yilong-musk/feedmesh/internal/weibo"
)

// ageCacheEntry rewrites a stored entry so it looks older than its freshness
// TTL while its on-disk expiry (the stale window) is still in the future.
func ageCacheEntry(t *testing.T, dir, key string, age time.Duration) {
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

// weiboUpstream is a minimal weibo stand-in: profile, timeline and long text,
// with a switch to simulate an upstream outage.
type weiboUpstream struct {
	down atomic.Bool
}

func (u *weiboUpstream) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if u.down.Load() {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		switch r.URL.Path {
		case "/visitor":
			http.SetCookie(w, &http.Cookie{Name: "SUB", Value: "x", Path: "/"})
			fmt.Fprint(w, `{"retcode":20000000}`)
		case "/api/container/getIndex":
			if r.URL.Query().Get("containerid") == "" {
				fmt.Fprint(w, `{"ok":1,"data":{"userInfo":{"id":1,"screen_name":"tester"}}}`)
				return
			}
			fmt.Fprint(w, `{"ok":1,"data":{"cards":[{"mblog":{
				"id":"1001","created_at":"Fri Oct 23 14:30:04 +0800 2026",
				"text":"<p>hello timeline</p>"}}]}}`)
		default:
			http.NotFound(w, r)
		}
	}
}

func TestGetFeedServesLastGoodTimelineWhenUpstreamFails(t *testing.T) {
	up := &weiboUpstream{}
	srv := httptest.NewServer(up.handler())
	defer srv.Close()

	dir := t.TempDir()
	c, err := cache.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	client := weibo.New(weibo.Config{
		BaseURL: srv.URL, VisitorURL: srv.URL + "/visitor",
		Timeout: 2 * time.Second, Backoff: time.Millisecond,
	})
	svc := feed.NewService(feed.Clients{Weibo: client}, c, nil, feed.DefaultTTLs())
	ctx := context.Background()

	warm, err := svc.GetFeed(ctx, "1", 10, false)
	if err != nil {
		t.Fatalf("warm read: %v", err)
	}
	if warm.Degraded {
		t.Fatal("degraded = true on a healthy upstream")
	}

	// Past the 5-minute freshness window, still inside the stale window.
	ageCacheEntry(t, dir, "timeline:1", 6*time.Minute)
	up.down.Store(true)

	f, err := svc.GetFeed(ctx, "1", 10, false)
	if err != nil {
		t.Fatalf("the last good timeline must be served: %v", err)
	}
	if !f.Degraded {
		t.Fatal("degraded = false, want true for a last-good timeline")
	}
	if len(f.Items) != 1 {
		t.Fatalf("items = %d, want the single cached post", len(f.Items))
	}
}

func TestGetFeedUsesStaleProfileWhenOnlyTheProfileFails(t *testing.T) {
	up := &weiboUpstream{}
	srv := httptest.NewServer(up.handler())
	defer srv.Close()

	dir := t.TempDir()
	c, err := cache.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	client := weibo.New(weibo.Config{
		BaseURL: srv.URL, VisitorURL: srv.URL + "/visitor",
		Timeout: 2 * time.Second, Backoff: time.Millisecond,
	})
	svc := feed.NewService(feed.Clients{Weibo: client}, c, nil, feed.DefaultTTLs())
	ctx := context.Background()

	if _, err := svc.GetFeed(ctx, "1", 10, false); err != nil {
		t.Fatalf("warm read: %v", err)
	}
	// Profile TTL is an hour: age it past that, keep the timeline fresh.
	ageCacheEntry(t, dir, "profile:1", 2*time.Hour)
	up.down.Store(true)

	f, err := svc.GetFeed(ctx, "1", 10, false)
	if err != nil {
		t.Fatalf("a stale profile must not fail the feed: %v", err)
	}
	if !strings.Contains(f.Title(), "tester") {
		t.Fatalf("title = %q, want the cached profile's name", f.Title())
	}
}

func TestGetFeedColdFailureStillReturnsAnError(t *testing.T) {
	up := &weiboUpstream{}
	up.down.Store(true)
	srv := httptest.NewServer(up.handler())
	defer srv.Close()

	c, err := cache.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	client := weibo.New(weibo.Config{
		BaseURL: srv.URL, VisitorURL: srv.URL + "/visitor",
		Timeout: time.Second, Backoff: time.Millisecond, Retries: 1,
	})
	svc := feed.NewService(feed.Clients{Weibo: client}, c, nil, feed.DefaultTTLs())

	if _, err := svc.GetFeed(context.Background(), "1", 10, false); err == nil {
		t.Fatal("want an error when nothing is cached")
	}
}

// v2exListUpstream serves one Atom entry so the list path can be exercised
// with a switch for outages.
func v2exListUpstream(down *atomic.Bool) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if down.Load() {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
<title>V2EX</title>
<entry>
<title>[分享创造] 测试帖</title>
<link rel="alternate" href="https://www.v2ex.com/t/777#reply2" />
<published>2026-10-23T10:00:00Z</published>
<author><name>alice</name></author>
<content type="html"><![CDATA[<p>正文</p>]]></content>
</entry>
</feed>`)
	}))
}

func TestV2exServesLastGoodListWhenUpstreamFails(t *testing.T) {
	var down atomic.Bool
	srv := v2exListUpstream(&down)
	defer srv.Close()

	dir := t.TempDir()
	c, err := cache.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	client := v2ex.New(v2ex.Config{BaseURL: srv.URL, Timeout: 2 * time.Second})
	svc := feed.NewService(feed.Clients{V2ex: client}, c, nil, feed.DefaultTTLs())
	ctx := context.Background()

	if _, err := svc.GetV2ex(ctx, "latest", 5, false); err != nil {
		t.Fatalf("warm read: %v", err)
	}
	ageCacheEntry(t, dir, "v2ex:latest", 20*time.Minute)
	down.Store(true)

	f, err := svc.GetV2ex(ctx, "latest", 5, false)
	if err != nil {
		t.Fatalf("the last good list must be served: %v", err)
	}
	if !f.Degraded {
		t.Fatal("degraded = false, want true for a last-good list")
	}
	if len(f.Items) == 0 {
		t.Fatal("want the cached entries")
	}
}
