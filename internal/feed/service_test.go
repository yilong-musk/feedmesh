package feed_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yilong-musk/weibo-rss/internal/cache"
	"github.com/yilong-musk/weibo-rss/internal/feed"
	"github.com/yilong-musk/weibo-rss/internal/weibo"
)

// domainUpstream serves just enough of weibo for vanity-domain resolution and
// counts how often the domain page is actually requested.
func domainUpstream(t *testing.T, hits *atomic.Int32) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/visitor":
			http.SetCookie(w, &http.Cookie{Name: "SUB", Value: "_2Aktestcookie", Path: "/"})
			fmt.Fprint(w, `window.visitor_gray_callback && visitor_gray_callback({"retcode":20000000})`)
		case "/rmrb":
			hits.Add(1)
			fmt.Fprint(w, `<html><body><a href="//m.weibo.cn/u/2803301701">人民日报</a></body></html>`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestResolveUIDRefreshBypassesDomainCache(t *testing.T) {
	var hits atomic.Int32
	srv := domainUpstream(t, &hits)

	client := weibo.New(weibo.Config{
		BaseURL:    srv.URL,
		VisitorURL: srv.URL + "/visitor",
		Timeout:    2 * time.Second,
		Backoff:    5 * time.Millisecond,
	})
	c, err := cache.New(t.TempDir())
	if err != nil {
		t.Fatalf("cache.New: %v", err)
	}
	svc := feed.NewService(client, c, nil, feed.TTLs{})
	ctx := context.Background()

	uid, err := svc.ResolveUID(ctx, "rmrb", false)
	if err != nil {
		t.Fatalf("first resolve: %v", err)
	}
	if uid != "2803301701" {
		t.Fatalf("uid = %q, want 2803301701", uid)
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("upstream hits = %d after first resolve, want 1", got)
	}

	if _, err := svc.ResolveUID(ctx, "rmrb", false); err != nil {
		t.Fatalf("cached resolve: %v", err)
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("upstream hits = %d, want 1 (cache should answer)", got)
	}

	// --refresh must reach upstream, not silently reuse domain:<name>.
	if _, err := svc.ResolveUID(ctx, "rmrb", true); err != nil {
		t.Fatalf("refresh resolve: %v", err)
	}
	if got := hits.Load(); got != 2 {
		t.Fatalf("upstream hits = %d after refresh, want 2", got)
	}
}
