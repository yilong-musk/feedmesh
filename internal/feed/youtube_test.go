package feed_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yilong-musk/feedmesh/internal/cache"
	"github.com/yilong-musk/feedmesh/internal/feed"
	"github.com/yilong-musk/feedmesh/internal/logx"
	"github.com/yilong-musk/feedmesh/internal/weibo"
	"github.com/yilong-musk/feedmesh/internal/youtube"
)

const ytChannelID = "UC_x5XG1OV2P6uZZ5FSM9Ttw"

// ytFeedXML is a two-entry upload feed in the production shape: namespaces,
// media group, and the counters nested under media:community.
const ytFeedXML = `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns:yt="http://www.youtube.com/xml/schemas/2015" xmlns:media="http://search.yahoo.com/mrss/" xmlns="http://www.w3.org/2005/Atom">
 <link rel="self" href="http://www.youtube.com/feeds/videos.xml?channel_id=` + ytChannelID + `"/>
 <title>Test Channel</title>
 <entry>
  <id>yt:video:vid00000001</id>
  <yt:videoId>vid00000001</yt:videoId>
  <yt:channelId>` + ytChannelID + `</yt:channelId>
  <title>First upload</title>
  <link rel="alternate" href="https://www.youtube.com/watch?v=vid00000001"/>
  <published>2026-10-21T23:00:08+00:00</published>
  <updated>2026-10-24T22:10:34+00:00</updated>
  <media:group>
   <media:title>First upload</media:title>
   <media:description>First description body.</media:description>
   <media:thumbnail url="https://i4.ytimg.com/vi/vid00000001/hqdefault.jpg" width="480" height="360"/>
   <media:community>
    <media:starRating count="325" average="5.00" min="1" max="5"/>
    <media:statistics views="9862"/>
   </media:community>
  </media:group>
 </entry>
 <entry>
  <id>yt:video:vid00000002</id>
  <yt:videoId>vid00000002</yt:videoId>
  <yt:channelId>` + ytChannelID + `</yt:channelId>
  <title>Second upload</title>
  <link rel="alternate" href="https://www.youtube.com/shorts/vid00000002"/>
  <published>2026-10-20T10:00:00+00:00</published>
  <media:group>
   <media:description>Second description body.</media:description>
   <media:thumbnail url="https://i4.ytimg.com/vi/vid00000002/hqdefault.jpg" width="480" height="360"/>
   <media:community><media:statistics views="42"/></media:community>
  </media:group>
 </entry>
</feed>`

const ytChannelPage = `<html><script>{"metadata":{"channelMetadataRenderer":{"externalId":"` + ytChannelID + `","title":"Test Channel"}}}</script></html>`

// ytBrowseJSON is the shape of the browse reply the client falls back to when
// the Atom feed is in one of its outage windows.
const ytBrowseJSON = `{"contents":{"twoColumnBrowseResultsRenderer":{"tabs":[{"tabRenderer":{"title":"Videos","selected":true,"content":{"richGridRenderer":{"contents":[{"richItemRenderer":{"content":{"lockupViewModel":{"contentId":"dQw4w9WgXcQ","metadata":{"lockupMetadataViewModel":{"title":{"content":"Browse fallback upload"},"metadata":{"contentMetadataViewModel":{"metadataRows":[{"metadataParts":[{"text":{"content":"7.5K views"}},{"text":{"content":"2 days ago"}}]}]}}}},"contentImage":{"thumbnailViewModel":{"image":{"sources":[{"url":"https://i.ytimg.com/vi/dQw4w9WgXcQ/hq720.jpg","width":720}]}}}}}}}]}}}}]}},"metadata":{"channelMetadataRenderer":{"title":"Test Channel"}}}`

// ytUpstream is a controllable stand-in for youtube.com.
type ytUpstream struct {
	mu       sync.Mutex
	body     string
	feedHits int32
	pageHits int32
	down     atomic.Bool
	noPage   atomic.Bool
}

func (u *ytUpstream) setFeed(body string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.body = body
}

func (u *ytUpstream) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/@testchannel":
			atomic.AddInt32(&u.pageHits, 1)
			if u.noPage.Load() {
				http.NotFound(w, r)
				return
			}
			_, _ = w.Write([]byte(ytChannelPage))
		case r.URL.Path == "/feeds/videos.xml":
			atomic.AddInt32(&u.feedHits, 1)
			if u.down.Load() {
				// The production endpoint answers 404/5xx in short windows.
				http.NotFound(w, r)
				return
			}
			u.mu.Lock()
			body := u.body
			u.mu.Unlock()
			w.Header().Set("Content-Type", "application/atom+xml")
			_, _ = w.Write([]byte(body))
		default:
			http.NotFound(w, r)
		}
	}
}

func newYouTubeService(t *testing.T, base string) *feed.Service {
	t.Helper()
	return newYouTubeServiceAt(t, base, t.TempDir())
}

// newYouTubeServiceAt is newYouTubeService with a caller-owned cache directory,
// so a test can age the stored entry on disk.
func newYouTubeServiceAt(t *testing.T, base, dir string) *feed.Service {
	t.Helper()
	c, err := cache.New(dir)
	if err != nil {
		t.Fatalf("cache: %v", err)
	}
	yt := youtube.New(youtube.Config{BaseURL: base, Timeout: 2 * time.Second, RetryDelay: time.Millisecond})
	return feed.NewService(feed.Clients{Youtube: yt}, c, logx.Discard(), feed.DefaultTTLs())
}

// ageYouTubeFeedEntry rewrites the stored upload list so it looks fetched
// eleven minutes ago: past the ten-minute freshness window and past its cache
// expiry. Both fields move together, exactly as a real write ten minutes ago
// would leave them.
func ageYouTubeFeedEntry(t *testing.T, dir string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, "youtube_feed_*.json"))
	if err != nil || len(matches) != 1 {
		t.Fatalf("feed cache files = %v (err = %v), want exactly one", matches, err)
	}
	raw, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatalf("read entry: %v", err)
	}
	var entry cache.Entry
	if err := json.Unmarshal(raw, &entry); err != nil {
		t.Fatalf("decode entry: %v", err)
	}
	entry.ExpiresAt = time.Now().Add(-time.Minute)
	var value map[string]any
	if err := json.Unmarshal(entry.Value, &value); err != nil {
		t.Fatalf("decode value: %v", err)
	}
	value["fetched_at"] = time.Now().Add(-11 * time.Minute).UTC().Format(time.RFC3339Nano)
	if entry.Value, err = json.Marshal(value); err != nil {
		t.Fatalf("encode value: %v", err)
	}
	out, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("encode entry: %v", err)
	}
	if err := os.WriteFile(matches[0], out, 0o600); err != nil {
		t.Fatalf("write entry: %v", err)
	}
}

func TestGetYouTubeMapsItems(t *testing.T) {
	up := &ytUpstream{body: ytFeedXML}
	srv := httptest.NewServer(up.handler())
	defer srv.Close()
	svc := newYouTubeService(t, srv.URL)

	f, err := svc.GetYouTube(context.Background(), "@testchannel", 15, false)
	if err != nil {
		t.Fatalf("GetYouTube: %v", err)
	}
	if f.UID != ytChannelID || f.Title() != "Test Channel" {
		t.Fatalf("channel = %q / %q", f.UID, f.Title())
	}
	if f.Degraded || f.Cached {
		t.Fatalf("degraded/cached = %v/%v, want false/false", f.Degraded, f.Cached)
	}
	if len(f.Items) != 2 {
		t.Fatalf("items = %d, want 2", len(f.Items))
	}
	it := f.Items[0]
	if it.ID != "vid00000001" || it.Title != "First upload" {
		t.Fatalf("item = %+v", it)
	}
	if it.URL != "https://www.youtube.com/watch?v=vid00000001" {
		t.Fatalf("url = %q", it.URL)
	}
	if it.Views != 9862 {
		t.Fatalf("views = %d", it.Views)
	}
	if len(it.Images) != 1 || !strings.Contains(it.Images[0], "hqdefault.jpg") {
		t.Fatalf("images = %v", it.Images)
	}
	if !strings.Contains(it.HTML, "Watch on YouTube") || !strings.Contains(it.HTML, "9.9K views") {
		t.Fatalf("html = %s", it.HTML)
	}
	if it.CreatedAt.IsZero() {
		t.Fatal("created_at is zero")
	}

	// Second read comes from the cache, and the handle mapping is reused.
	if _, err := svc.GetYouTube(context.Background(), "@testchannel", 15, false); err != nil {
		t.Fatalf("second call: %v", err)
	}
	if got := atomic.LoadInt32(&up.feedHits); got != 1 {
		t.Fatalf("feed requests = %d, want 1", got)
	}
	if got := atomic.LoadInt32(&up.pageHits); got != 1 {
		t.Fatalf("channel page requests = %d, want 1 (mapping is cached)", got)
	}
}

func TestGetYouTubeClampsLimit(t *testing.T) {
	up := &ytUpstream{body: ytFeedXML}
	srv := httptest.NewServer(up.handler())
	defer srv.Close()
	svc := newYouTubeService(t, srv.URL)

	f, err := svc.GetYouTube(context.Background(), ytChannelID, 1, false)
	if err != nil {
		t.Fatalf("GetYouTube: %v", err)
	}
	if len(f.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(f.Items))
	}
	if got := atomic.LoadInt32(&up.pageHits); got != 0 {
		t.Fatalf("channel page requests = %d, want 0 for a raw id", got)
	}
}

func TestGetYouTubeDegradesToLastGoodList(t *testing.T) {
	up := &ytUpstream{body: ytFeedXML}
	srv := httptest.NewServer(up.handler())
	defer srv.Close()
	svc := newYouTubeService(t, srv.URL)

	if _, err := svc.GetYouTube(context.Background(), "@testchannel", 15, false); err != nil {
		t.Fatalf("warm: %v", err)
	}
	up.down.Store(true)

	f, err := svc.GetYouTube(context.Background(), "@testchannel", 15, true)
	if err != nil {
		t.Fatalf("degraded read must not fail: %v", err)
	}
	if !f.Degraded {
		t.Fatal("degraded = false, want true")
	}
	if !f.Cached || len(f.Items) != 2 {
		t.Fatalf("cached/items = %v/%d, want true/2", f.Cached, len(f.Items))
	}
}

// TestGetYouTubeSharesOneChannelMapping pins the cache key: the same channel
// pasted as a handle and as a URL must not cost a second channel-page read.
func TestGetYouTubeSharesOneChannelMapping(t *testing.T) {
	up := &ytUpstream{body: ytFeedXML}
	srv := httptest.NewServer(up.handler())
	defer srv.Close()
	svc := newYouTubeService(t, srv.URL)
	ctx := context.Background()

	if _, err := svc.GetYouTube(ctx, "@testchannel", 5, false); err != nil {
		t.Fatalf("handle read: %v", err)
	}
	if _, err := svc.GetYouTube(ctx, "https://www.youtube.com/@testchannel/videos", 5, false); err != nil {
		t.Fatalf("URL read: %v", err)
	}
	if got := atomic.LoadInt32(&up.pageHits); got != 1 {
		t.Fatalf("channel page requests = %d, want 1 for one channel", got)
	}
}

// TestGetYouTubeServesLastGoodListPastTTL covers the ordinary read path (no
// refresh=1): once the entry is older than the freshness window and the
// upstream is failing, the reader must still get the last good list, marked
// degraded. The cache layer deletes expired entries on a normal read, so a
// read path that goes through it loses the copy this fallback depends on.
func TestGetYouTubeServesLastGoodListPastTTL(t *testing.T) {
	up := &ytUpstream{body: ytFeedXML}
	srv := httptest.NewServer(up.handler())
	defer srv.Close()
	dir := t.TempDir()
	svc := newYouTubeServiceAt(t, srv.URL, dir)
	ctx := context.Background()

	if _, err := svc.GetYouTube(ctx, "@testchannel", 15, false); err != nil {
		t.Fatalf("warm read: %v", err)
	}
	ageYouTubeFeedEntry(t, dir)
	up.down.Store(true)

	f, err := svc.GetYouTube(ctx, "@testchannel", 15, false)
	if err != nil {
		t.Fatalf("read past the freshness window must serve the last good list: %v", err)
	}
	if !f.Degraded {
		t.Fatal("degraded = false, want true")
	}
	if !f.Cached || len(f.Items) != 2 {
		t.Fatalf("cached/items = %v/%d, want true/2", f.Cached, len(f.Items))
	}
}

func TestGetYouTubeFailsColdWithoutCache(t *testing.T) {
	up := &ytUpstream{body: ytFeedXML}
	up.down.Store(true)
	srv := httptest.NewServer(up.handler())
	defer srv.Close()
	svc := newYouTubeService(t, srv.URL)

	_, err := svc.GetYouTube(context.Background(), "@testchannel", 15, false)
	if err == nil {
		t.Fatal("want an error")
	}
	var we *weibo.Error
	if !errors.As(err, &we) || we.Code != weibo.CodeUpstreamUnavailable {
		t.Fatalf("err = %v, want UPSTREAM_UNAVAILABLE", err)
	}
}

func TestGetYouTubeUnknownChannel(t *testing.T) {
	up := &ytUpstream{body: ytFeedXML}
	up.noPage.Store(true)
	srv := httptest.NewServer(up.handler())
	defer srv.Close()
	svc := newYouTubeService(t, srv.URL)

	_, err := svc.GetYouTube(context.Background(), "@nosuchchannel", 15, false)
	if err == nil {
		t.Fatal("want an error")
	}
	var we *weibo.Error
	if !errors.As(err, &we) || we.Code != weibo.CodeUserNotFound {
		t.Fatalf("err = %v, want USER_NOT_FOUND", err)
	}
}

func TestGetYouTubeRefreshReplacesList(t *testing.T) {
	up := &ytUpstream{body: ytFeedXML}
	srv := httptest.NewServer(up.handler())
	defer srv.Close()
	svc := newYouTubeService(t, srv.URL)

	if _, err := svc.GetYouTube(context.Background(), ytChannelID, 15, false); err != nil {
		t.Fatalf("warm: %v", err)
	}
	up.setFeed(strings.Replace(ytFeedXML, "First upload", "Refreshed upload", 1))

	f, err := svc.GetYouTube(context.Background(), ytChannelID, 15, true)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if f.Cached || f.Degraded {
		t.Fatalf("cached/degraded = %v/%v, want false/false", f.Cached, f.Degraded)
	}
	if f.Items[0].Title != "Refreshed upload" {
		t.Fatalf("first title = %q", f.Items[0].Title)
	}
}

func TestGetYouTubeUsesBrowseWhenFeedFails(t *testing.T) {
	// The Atom endpoint is out; the browse endpoint answers instead, so the
	// reader still gets a feed (with approximate timestamps) rather than 504.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/@testchannel":
			_, _ = w.Write([]byte(ytChannelPage))
		case "/feeds/videos.xml":
			http.NotFound(w, r)
		case "/youtubei/v1/browse":
			_, _ = w.Write([]byte(ytBrowseJSON))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	svc := newYouTubeService(t, srv.URL)

	f, err := svc.GetYouTube(context.Background(), "@testchannel", 15, false)
	if err != nil {
		t.Fatalf("GetYouTube: %v", err)
	}
	if f.Degraded {
		t.Fatal("degraded = true, want false: the fallback is a live read")
	}
	if len(f.Items) != 1 || f.Items[0].Title != "Browse fallback upload" {
		t.Fatalf("items = %+v", f.Items)
	}
	if f.Items[0].Views != 7500 {
		t.Fatalf("views = %d, want 7500", f.Items[0].Views)
	}
	if !strings.Contains(f.Items[0].HTML, "7.5K views") {
		t.Fatalf("html = %s, want a compact view counter", f.Items[0].HTML)
	}
	if age := time.Since(f.Items[0].CreatedAt); age < 36*time.Hour || age > 60*time.Hour {
		t.Fatalf("created_at age = %v, want about 2 days", age)
	}
}

func TestGetYouTubeRejectsEmptyReference(t *testing.T) {
	up := &ytUpstream{body: ytFeedXML}
	srv := httptest.NewServer(up.handler())
	defer srv.Close()
	svc := newYouTubeService(t, srv.URL)

	_, err := svc.GetYouTube(context.Background(), "  ", 15, false)
	if err == nil {
		t.Fatal("want an error")
	}
	var we *weibo.Error
	if !errors.As(err, &we) || we.Code != weibo.CodeBadRequest {
		t.Fatalf("err = %v, want BAD_REQUEST", err)
	}
}
