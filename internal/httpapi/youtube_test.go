package httpapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yilong-musk/feedmesh/internal/cache"
	"github.com/yilong-musk/feedmesh/internal/feed"
	"github.com/yilong-musk/feedmesh/internal/httpapi"
	"github.com/yilong-musk/feedmesh/internal/logx"
	"github.com/yilong-musk/feedmesh/internal/youtube"
)

const ytChannelID = "UC_x5XG1OV2P6uZZ5FSM9Ttw"

// youtubeFeedXML is a one-entry upload feed in the production shape.
const youtubeFeedXML = `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns:yt="http://www.youtube.com/xml/schemas/2015" xmlns:media="http://search.yahoo.com/mrss/" xmlns="http://www.w3.org/2005/Atom">
 <title>Test Channel</title>
 <entry>
  <id>yt:video:vid00000001</id>
  <yt:videoId>vid00000001</yt:videoId>
  <yt:channelId>` + ytChannelID + `</yt:channelId>
  <title>First upload</title>
  <link rel="alternate" href="https://www.youtube.com/watch?v=vid00000001"/>
  <published>2026-10-21T23:00:08+00:00</published>
  <media:group>
   <media:description>First description body.</media:description>
   <media:thumbnail url="https://i4.ytimg.com/vi/vid00000001/hqdefault.jpg" width="480" height="360"/>
   <media:community><media:statistics views="9862"/></media:community>
  </media:group>
 </entry>
</feed>`

const youtubeChannelPage = `<html><script>{"metadata":{"channelMetadataRenderer":{"externalId":"` + ytChannelID + `","title":"Test Channel"}}}</script></html>`

func TestYouTubeRSSRoute(t *testing.T) {
	h := newTestServer(t)

	for _, path := range []string{"/rss/youtube/@testchannel", "/rss/youtube/@testchannel.xml"} {
		rec := get(t, h, path)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d body=%s", path, rec.Code, rec.Body.String())
		}
		if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/rss+xml") {
			t.Fatalf("%s content-type = %q", path, ct)
		}
		if got := rec.Header().Get("Cache-Control"); got != "public, max-age=600" {
			t.Fatalf("%s cache-control = %q", path, got)
		}
		if got := rec.Header().Get("X-Feedmesh-Degraded"); got != "" {
			t.Fatalf("%s degraded header = %q", path, got)
		}
		body := rec.Body.String()
		for _, want := range []string{
			"<title>Test Channel</title>",
			"First upload",
			"https://www.youtube.com/watch?v=vid00000001",
			"https://i4.ytimg.com/vi/vid00000001/hqdefault.jpg",
			"<ttl>10</ttl>",
			"Watch on YouTube",
		} {
			if !strings.Contains(body, want) {
				t.Fatalf("%s missing %q:\n%s", path, want, body)
			}
		}
	}
}

func TestYouTubeAPIRoute(t *testing.T) {
	h := newTestServer(t)

	rec := get(t, h, "/api/v1/youtube?ref=@testchannel&limit=1")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var out struct {
		Ref       string `json:"ref"`
		ChannelID string `json:"channel_id"`
		Title     string `json:"title"`
		Cached    bool   `json:"cached"`
		Degraded  bool   `json:"degraded"`
		Items     []struct {
			ID     string   `json:"id"`
			Title  string   `json:"title"`
			URL    string   `json:"url"`
			Views  int64    `json:"views"`
			Images []string `json:"images"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v\n%s", err, rec.Body.String())
	}
	if out.Ref != "@testchannel" || out.ChannelID != ytChannelID || out.Title != "Test Channel" {
		t.Fatalf("meta = %+v", out)
	}
	if out.Degraded {
		t.Fatal("degraded = true on a healthy upstream")
	}
	if len(out.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(out.Items))
	}
	if out.Items[0].Views != 9862 || out.Items[0].URL == "" {
		t.Fatalf("item = %+v", out.Items[0])
	}

	// The second read is served from the cache.
	rec = get(t, h, "/api/v1/youtube?ref=@testchannel&limit=1")
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !out.Cached {
		t.Fatal("cached = false on the second read")
	}
}

func TestYouTubeAPIUnknownChannel(t *testing.T) {
	h := newTestServer(t)

	rec := get(t, h, "/api/v1/youtube?ref=missing")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "USER_NOT_FOUND") {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestYouTubeRSSDegradedHeader(t *testing.T) {
	// A dedicated server so the upstream can start failing after a warm read.
	var down atomic.Bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/@testchannel":
			_, _ = w.Write([]byte(youtubeChannelPage))
		case "/feeds/videos.xml":
			if down.Load() {
				http.NotFound(w, r)
				return
			}
			_, _ = w.Write([]byte(youtubeFeedXML))
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	c, err := cache.New(t.TempDir())
	if err != nil {
		t.Fatalf("cache: %v", err)
	}
	svc := feed.NewService(feed.Clients{
		Youtube: youtube.New(youtube.Config{BaseURL: upstream.URL, Timeout: 2 * time.Second, RetryDelay: time.Millisecond}),
	}, c, logx.Discard(), feed.DefaultTTLs())
	h := httpapi.New(httpapi.Config{Version: "test", Service: svc, Cache: c, Log: logx.Discard()}).Handler()

	if rec := get(t, h, "/rss/youtube/@testchannel"); rec.Code != http.StatusOK {
		t.Fatalf("warm status = %d", rec.Code)
	}
	down.Store(true)

	rec := get(t, h, "/rss/youtube/@testchannel?refresh=1")
	if rec.Code != http.StatusOK {
		t.Fatalf("degraded status = %d body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("X-Feedmesh-Degraded"); got != "1" {
		t.Fatalf("degraded header = %q, want 1", got)
	}
	if got := rec.Header().Get("Cache-Control"); !strings.Contains(got, "max-age=60") {
		t.Fatalf("degraded Cache-Control = %q, want max-age=60", got)
	}
	if !strings.Contains(rec.Body.String(), "First upload") {
		t.Fatalf("degraded body lost the last good item:\n%s", rec.Body.String())
	}
}
