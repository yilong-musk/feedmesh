package feed_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/yilong-musk/feedmesh/internal/cache"
	"github.com/yilong-musk/feedmesh/internal/feed"
	"github.com/yilong-musk/feedmesh/internal/logx"
	"github.com/yilong-musk/feedmesh/internal/weibo"
	"github.com/yilong-musk/feedmesh/internal/xnitter"
)

const xTimeline = `<?xml version="1.0" encoding="UTF-8"?>
<rss xmlns:dc="http://purl.org/dc/elements/1.1/" version="2.0"><channel>
<title>Elon Musk / @elonmusk</title><link>https://n.test/elonmusk</link><ttl>40</ttl>
<item><title>Hello from X</title><dc:creator>@elonmusk</dc:creator>
<description><![CDATA[<p>Hello <a href="https://n.test/bot">@Bot</a></p><img src="https://pbs.twimg.com/media/a.jpg"/>]]></description>
<pubDate>Fri, 23 Oct 2026 05:50:22 GMT</pubDate><guid>111</guid>
<link>https://n.test/elonmusk/status/111#m</link></item>
<item><title>RT by @elonmusk: breaking</title><dc:creator>@cb_doge</dc:creator>
<description><![CDATA[<p>breaking</p>]]></description>
<pubDate>Thu, 22 Oct 2026 17:15:55 GMT</pubDate><guid>222</guid>
<link>https://n.test/cb_doge/status/222#m</link></item>
<item><title>third</title><dc:creator>@elonmusk</dc:creator>
<description><![CDATA[<p>third</p>]]></description>
<pubDate>Thu, 22 Oct 2026 10:00:00 GMT</pubDate><guid>333</guid>
<link>https://n.test/elonmusk/status/333#m</link></item>
</channel></rss>`

func xUpstream(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/elonmusk/rss" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/rss+xml")
		// Mirror links must match the origin that served them.
		_, _ = w.Write([]byte(strings.ReplaceAll(xTimeline, "https://n.test", "http://"+r.Host)))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func xService(t *testing.T, upstream *httptest.Server) *feed.Service {
	t.Helper()
	c, err := cache.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return feed.NewService(feed.Clients{
		X: xnitter.New(xnitter.Config{
			Instances: []string{upstream.URL},
			StatusURL: upstream.URL + "/status",
			Timeout:   2 * time.Second,
		}),
	}, c, logx.Discard(), feed.DefaultTTLs())
}

func TestGetX(t *testing.T) {
	up := xUpstream(t)
	svc := xService(t, up)
	f, err := svc.GetX(context.Background(), "@elonmusk", 3, false)
	if err != nil {
		t.Fatalf("GetX: %v", err)
	}
	if f.Title() != "elonmusk / X" || f.Link() != "https://x.com/elonmusk" {
		t.Fatalf("feed meta = %q / %q", f.Title(), f.Link())
	}
	if f.TTL != 40 {
		t.Errorf("TTL = %d, want 40", f.TTL)
	}
	if len(f.Items) != 3 {
		t.Fatalf("items = %d", len(f.Items))
	}
	first := f.Items[0]
	if first.URL != "https://x.com/elonmusk/status/111" {
		t.Errorf("url = %q", first.URL)
	}
	if first.Title != "Hello from X" {
		t.Errorf("title = %q", first.Title)
	}
	if len(first.Images) != 1 || first.Images[0] != "https://pbs.twimg.com/media/a.jpg" {
		t.Errorf("images = %v", first.Images)
	}
	if strings.Contains(first.HTML, strings.TrimPrefix(up.URL, "http://")) {
		t.Errorf("item HTML leaked the mirror: %s", first.HTML)
	}
	if !strings.HasPrefix(f.Items[1].Title, "🔁 ") {
		t.Errorf("retweet title = %q", f.Items[1].Title)
	}
	if f.Items[2].Images == nil {
		t.Error("an image-less item must expose an empty slice, not nil")
	}
	if got := first.CreatedAt.UTC().Format(time.RFC3339); got != "2026-10-23T05:50:22Z" {
		t.Errorf("created_at = %s", got)
	}
}

func TestGetXCachesAndRefreshes(t *testing.T) {
	svc := xService(t, xUpstream(t))
	ctx := context.Background()
	if _, err := svc.GetX(ctx, "elonmusk", 3, false); err != nil {
		t.Fatalf("first call: %v", err)
	}
	cached, err := svc.GetX(ctx, "elonmusk", 3, false)
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if !cached.Cached {
		t.Error("second call should be served from cache")
	}
	fresh, err := svc.GetX(ctx, "elonmusk", 3, true)
	if err != nil {
		t.Fatalf("refresh call: %v", err)
	}
	if fresh.Cached {
		t.Error("refresh=1 should bypass the cache")
	}
}

func TestGetXRespectsLimit(t *testing.T) {
	svc := xService(t, xUpstream(t))
	f, err := svc.GetX(context.Background(), "elonmusk", 2, false)
	if err != nil {
		t.Fatalf("GetX: %v", err)
	}
	if len(f.Items) != 2 {
		t.Fatalf("limit=2 returned %d items", len(f.Items))
	}
}

func TestGetXRejectsBadHandle(t *testing.T) {
	svc := xService(t, xUpstream(t))
	_, err := svc.GetX(context.Background(), "not a handle", 3, false)
	if err == nil {
		t.Fatal("expected an error")
	}
	var we *weibo.Error
	if !errors.As(err, &we) || we.Code != weibo.CodeBadRequest {
		t.Fatalf("error = %v, want CodeBadRequest", err)
	}
}

func TestGetXSurfacesUpstreamFailure(t *testing.T) {
	broken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusBadGateway)
	}))
	t.Cleanup(broken.Close)
	svc := xService(t, broken)
	_, err := svc.GetX(context.Background(), "elonmusk", 3, false)
	if err == nil {
		t.Fatal("expected an upstream error")
	}
	var we *weibo.Error
	if !errors.As(err, &we) || we.Code != weibo.CodeUpstreamUnavailable {
		t.Fatalf("error = %v, want CodeUpstreamUnavailable", err)
	}
}
