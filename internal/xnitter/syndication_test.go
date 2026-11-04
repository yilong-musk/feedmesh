package xnitter

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yilong-musk/feedmesh/internal/logx"
)

const enrichRSS = `<?xml version="1.0" encoding="UTF-8"?>
<rss xmlns:dc="http://purl.org/dc/elements/1.1/" version="2.0"><channel>
<item><title>video post</title><dc:creator>@someone</dc:creator>
<description><![CDATA[<p>hello</p><a href="https://mirror.test/someone/status/111#m"><br>Video<br><img src="https://pbs.twimg.com/amplify_video_thumb/9/img/p.jpg" /></a>]]></description>
<pubDate>Wed, 04 Nov 2026 06:49:32 GMT</pubDate><guid>111</guid>
<link>https://mirror.test/someone/status/111#m</link></item>
<item><title>text post</title><dc:creator>@someone</dc:creator>
<description><![CDATA[<p>plain</p>]]></description>
<pubDate>Wed, 04 Nov 2026 06:40:00 GMT</pubDate><guid>222</guid>
<link>https://mirror.test/someone/status/222#m</link></item>
</channel></rss>`

func enrichmentUpstream(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/someone/rss" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(enrichRSS))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestTimelineEnrichesVideoCountsAndQuoted(t *testing.T) {
	nitter := enrichmentUpstream(t)
	var calls atomic.Int32
	synd := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		id := r.URL.Query().Get("id")
		if id == "222" {
			http.Error(w, "boom", http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{
			"id_str": %q,
			"favorite_count": 17,
			"conversation_count": 3,
			"user": {"id_str": "42", "screen_name": "someone", "name": "Some One"},
			"video": {
				"durationMs": 4308,
				"viewCount": 1234,
				"poster": "https://pbs.twimg.com/amplify_video_thumb/9/img/p.jpg",
				"variants": [
					{"type": "application/x-mpegURL", "src": "https://video.twimg.com/v/pl.m3u8"},
					{"type": "video/mp4", "src": "https://video.twimg.com/vid/320x692/a.mp4"},
					{"type": "video/mp4", "src": "https://video.twimg.com/vid/720x1558/b.mp4"}
				]
			},
			"quoted_tweet": {
				"id_str": "999",
				"text": "quoted text",
				"user": {"screen_name": "quoter", "name": "Quoter"},
				"photos": [{"url": "https://pbs.twimg.com/media/q.jpg"}]
			}
		}`, id)
	}))
	defer synd.Close()

	c := New(Config{
		Instances:      []string{nitter.URL},
		StatusURL:      "http://127.0.0.1:1/status",
		Timeout:        5 * time.Second,
		SyndicationURL: synd.URL,
	})
	tweets, err := c.Timeline(context.Background(), "someone")
	if err != nil {
		t.Fatalf("Timeline: %v", err)
	}
	if len(tweets) != 2 {
		t.Fatalf("tweets = %d", len(tweets))
	}
	first := tweets[0]
	if first.Video == nil || first.Video.URL != "https://video.twimg.com/vid/720x1558/b.mp4" {
		t.Errorf("video = %+v, want the highest-resolution mp4", first.Video)
	}
	if first.Video != nil && (first.Video.DurationMS != 4308 || first.Video.Views != 1234) {
		t.Errorf("video meta = %+v", first.Video)
	}
	if first.Likes != 17 || first.Replies != 3 || first.AuthorID != "42" {
		t.Errorf("counts = likes %d replies %d author %q", first.Likes, first.Replies, first.AuthorID)
	}
	if first.Quoted == nil {
		t.Fatal("quoted post missing")
	}
	if first.Quoted.ID != "999" || first.Quoted.Author != "quoter" {
		t.Errorf("quoted = %+v", first.Quoted)
	}
	if len(first.Quoted.Images) != 1 || first.Quoted.Images[0] != "https://pbs.twimg.com/media/q.jpg?name=orig" {
		t.Errorf("quoted images = %v", first.Quoted.Images)
	}
	// The embed endpoint failed for this one: the RSS item must survive.
	if tweets[1].Likes != 0 || tweets[1].Video != nil {
		t.Errorf("failed enrichment changed the item: %+v", tweets[1])
	}
	if got := calls.Load(); got != 2 {
		t.Errorf("embed calls = %d, want one per item", got)
	}
}

func TestTimelineEnrichmentCanBeDisabled(t *testing.T) {
	nitter := enrichmentUpstream(t)
	var calls atomic.Int32
	synd := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		http.Error(w, "must not be called", http.StatusInternalServerError)
	}))
	defer synd.Close()

	c := New(Config{
		Instances:         []string{nitter.URL},
		StatusURL:         "http://127.0.0.1:1/status",
		Timeout:           5 * time.Second,
		SyndicationURL:    synd.URL,
		DisableEnrichment: true,
	})
	tweets, err := c.Timeline(context.Background(), "someone")
	if err != nil {
		t.Fatalf("Timeline: %v", err)
	}
	if got := calls.Load(); got != 0 {
		t.Fatalf("embed calls = %d, want 0 with enrichment disabled", got)
	}
	if tweets[0].Video == nil || tweets[0].Video.URL != "" {
		t.Errorf("video = %+v, want the RSS poster only", tweets[0].Video)
	}
	if tweets[0].Video == nil || !strings.Contains(tweets[0].Video.Thumbnail, "amplify_video_thumb") {
		t.Errorf("poster = %+v", tweets[0].Video)
	}
}

func TestTimelineWarnsWhenEveryEnrichmentFails(t *testing.T) {
	nitter := enrichmentUpstream(t)
	synd := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "down", http.StatusInternalServerError)
	}))
	defer synd.Close()

	var buf bytes.Buffer
	c := New(Config{
		Instances:      []string{nitter.URL},
		StatusURL:      "http://127.0.0.1:1/status",
		Timeout:        5 * time.Second,
		SyndicationURL: synd.URL,
		Log:            logx.New(&buf),
	})
	tweets, err := c.Timeline(context.Background(), "someone")
	if err != nil {
		t.Fatalf("Timeline: %v", err)
	}
	if len(tweets) != 2 {
		t.Fatalf("tweets = %d", len(tweets))
	}
	if !strings.Contains(buf.String(), "x enrichment failed for every item") {
		t.Fatalf("journal is missing the enrichment warning: %s", buf.String())
	}
}

func TestUpgradeMediaURL(t *testing.T) {
	cases := map[string]string{
		"https://pbs.twimg.com/media/a.jpg":                          "https://pbs.twimg.com/media/a.jpg?name=orig",
		"https://pbs.twimg.com/media/a.jpg?name=small":               "https://pbs.twimg.com/media/a.jpg?name=small",
		"https://pbs.twimg.com/card_img/1/2?format=jpg&name=800x419": "https://pbs.twimg.com/card_img/1/2?format=jpg&name=800x419",
		"https://pbs.twimg.com/amplify_video_thumb/1/img/p.jpg":      "https://pbs.twimg.com/amplify_video_thumb/1/img/p.jpg",
		"https://example.com/media/a.jpg":                            "https://example.com/media/a.jpg",
	}
	for in, want := range cases {
		if got := upgradeMediaURL(in); got != want {
			t.Errorf("upgradeMediaURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestClassifyImagesSeparatesCardAndVideo(t *testing.T) {
	html := `<img src="https://pbs.twimg.com/media/a.jpg"/>` +
		`<img src="https://pbs.twimg.com/card_img/1/2?format=jpg&name=800x419"/>` +
		`<img src="https://pbs.twimg.com/amplify_video_thumb/3/img/p.jpg"/>`
	photos, poster, card := classifyImages(html)
	if len(photos) != 1 || photos[0] != "https://pbs.twimg.com/media/a.jpg?name=orig" {
		t.Errorf("photos = %v", photos)
	}
	if !strings.Contains(poster, "amplify_video_thumb") {
		t.Errorf("poster = %q", poster)
	}
	if !strings.Contains(card, "card_img") {
		t.Errorf("card = %q", card)
	}
}
