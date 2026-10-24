package xnitter

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func fixture(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/timeline.xml")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return b
}

func testClient(t *testing.T, instances ...string) *Client {
	t.Helper()
	return New(Config{
		Instances: instances,
		// Point the directory at a path the test servers answer with 404 so
		// the baked-in list wins.
		StatusURL: "http://127.0.0.1:1/status",
		Timeout:   5 * time.Second,
	})
}

func TestParseRSS(t *testing.T) {
	tweets, err := parseRSS(fixture(t), "https://nitter.kareem.one", "elonmusk")
	if err != nil {
		t.Fatalf("parseRSS: %v", err)
	}
	if len(tweets) != 3 {
		t.Fatalf("want 3 tweets, got %d", len(tweets))
	}
	first := tweets[0]
	if first.ID != "2113508310267674684" {
		t.Errorf("first id = %q", first.ID)
	}
	if want := "https://x.com/elonmusk/status/2113508310267674684"; first.URL != want {
		t.Errorf("url = %q, want %q", first.URL, want)
	}
	if !strings.HasPrefix(first.Author, "") || first.Author != "elonmusk" {
		t.Errorf("author = %q", first.Author)
	}
	if first.Published.IsZero() {
		t.Error("published time not parsed")
	}
	if strings.Contains(first.HTML, "nitter.kareem.one") {
		t.Errorf("HTML still points at the mirror: %s", first.HTML)
	}
	if !strings.Contains(first.HTML, `href="https://x.com/`) {
		t.Errorf("HTML links were not rewritten: %s", first.HTML)
	}
	if first.Text == "" {
		t.Error("plain text is empty")
	}

	// The second item carries an image, the third is a retweet.
	if len(tweets[1].Images) == 0 {
		t.Errorf("second tweet should expose an image: %+v", tweets[1])
	}
	if !strings.HasPrefix(tweets[1].Images[0], "https://pbs.twimg.com/") {
		t.Errorf("image host = %q", tweets[1].Images[0])
	}
	if !tweets[2].IsRetweet {
		t.Errorf("third tweet should be flagged as a retweet: %q", tweets[2].Title)
	}
	if tweets[0].IsRetweet {
		t.Error("first tweet is not a retweet")
	}
}

func TestParseRSSTimeFormats(t *testing.T) {
	body := []byte(`<rss xmlns:dc="http://purl.org/dc/elements/1.1/"><channel>
<item><title>a</title><dc:creator>@someone</dc:creator><description>&lt;p&gt;hi&lt;/p&gt;</description>
<pubDate>Fri, 23 Oct 2026 05:50:22 GMT</pubDate><guid>1</guid><link>https://n.test/someone/status/1#m</link></item>
</channel></rss>`)
	tweets, err := parseRSS(body, "https://n.test", "someone")
	if err != nil {
		t.Fatalf("parseRSS: %v", err)
	}
	if got := tweets[0].Published.UTC().Format(time.RFC3339); got != "2026-10-23T05:50:22Z" {
		t.Errorf("published = %s", got)
	}
	if tweets[0].Text != "hi" {
		t.Errorf("text = %q", tweets[0].Text)
	}
}

func TestParseRSSUsesLinkIDWhenGUIDIsURL(t *testing.T) {
	// Older Nitter versions emit the full status URL in <guid>; using it
	// verbatim leaked the mirror host into Tweet.ID and doubled it in Tweet.URL.
	body := []byte(`<rss xmlns:dc="http://purl.org/dc/elements/1.1/"><channel>
<item><title>old post</title><dc:creator>@someone</dc:creator>
<description>&lt;p&gt;hello&lt;/p&gt;</description>
<pubDate>Fri, 23 Oct 2026 05:50:22 GMT</pubDate>
<guid>https://n.example/someone/status/123#m</guid>
<link>https://n.example/someone/status/123#m</link></item>
</channel></rss>`)
	tweets, err := parseRSS(body, "https://n.example", "someone")
	if err != nil {
		t.Fatalf("parseRSS: %v", err)
	}
	if got := tweets[0].ID; got != "123" {
		t.Errorf("id = %q, want the numeric status id", got)
	}
	if got, want := tweets[0].URL, "https://x.com/someone/status/123"; got != want {
		t.Errorf("url = %q, want %q", got, want)
	}
	if strings.Contains(tweets[0].URL, "n.example") {
		t.Errorf("mirror host leaked into tweet URL: %s", tweets[0].URL)
	}
}

func TestRewriteLinksKeepsImageHosts(t *testing.T) {
	in := `<p><a href="https://n.example.test/bot">@Bot</a>` +
		`<img src="https://pbs.twimg.com/media/x.jpg"/></p>` +
		`<a href="https://n.example.test/minchoi/status/99#m">link</a>`
	out := rewriteLinks(in, "https://n.example.test")
	if strings.Contains(out, "n.example.test") {
		t.Errorf("mirror host survived: %s", out)
	}
	if !strings.Contains(out, `href="https://x.com/bot"`) {
		t.Errorf("profile link not rewritten: %s", out)
	}
	if !strings.Contains(out, `https://pbs.twimg.com/media/x.jpg`) {
		t.Errorf("image host was rewritten: %s", out)
	}
	if !strings.Contains(out, `https://x.com/minchoi/status/99`) {
		t.Errorf("status link not rewritten: %s", out)
	}
}

func TestSolvePoW(t *testing.T) {
	nonce, digest, err := solvePoW(context.Background(), "seed", 3)
	if err != nil {
		t.Fatalf("solvePoW: %v", err)
	}
	sum := sha256.Sum256([]byte("seed" + fmt.Sprint(nonce)))
	if got := hex.EncodeToString(sum[:]); got != digest {
		t.Errorf("digest mismatch: %s vs %s", got, digest)
	}
	if !strings.HasPrefix(digest, "000") {
		t.Errorf("digest does not meet difficulty: %s", digest)
	}
}

func TestTimelineSolvesAnubisChallenge(t *testing.T) {
	var base string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/.within.website/x/cmd/anubis/api/pass-challenge"):
			c, err := r.Cookie("anubis-auth")
			if err == nil && c.Value == "ok" {
				w.WriteHeader(http.StatusOK)
				return
			}
			if r.URL.Query().Get("response") == "" || r.URL.Query().Get("nonce") == "" {
				http.Error(w, "bad challenge response", http.StatusBadRequest)
				return
			}
			http.SetCookie(w, &http.Cookie{Name: "anubis-auth", Value: "ok", Path: "/"})
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/elonmusk/rss":
			if c, err := r.Cookie("anubis-auth"); err != nil || c.Value != "ok" {
				w.Header().Set("Content-Type", "text/html")
				fmt.Fprintf(w, `<script id="anubis_challenge" type="application/json">`+
					`{"challenge":{"randomData":"abc","difficulty":1,"id":"cid"}}</script>`)
				return
			}
			w.Header().Set("Content-Type", "application/rss+xml")
			_, _ = w.Write(fixture(t))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	base = srv.URL

	c := testClient(t, base)
	tweets, err := c.Timeline(context.Background(), "@elonmusk")
	if err != nil {
		t.Fatalf("Timeline through Anubis: %v", err)
	}
	if len(tweets) != 3 {
		t.Fatalf("want 3 tweets, got %d", len(tweets))
	}
}

func TestTimelineFallsBackToNextInstance(t *testing.T) {
	broken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer broken.Close()

	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/elonmusk/rss" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write(fixture(t))
	}))
	defer good.Close()

	c := testClient(t, broken.URL, good.URL)
	tweets, err := c.Timeline(context.Background(), "elonmusk")
	if err != nil {
		t.Fatalf("Timeline with fallback: %v", err)
	}
	if len(tweets) != 3 {
		t.Fatalf("want 3 tweets from fallback instance, got %d", len(tweets))
	}
}

func TestTimelineTriesFallbackAfterFullPoolFails(t *testing.T) {
	var failing []string
	for i := 0; i < maxPoolAttempts; i++ {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "down", http.StatusInternalServerError)
		}))
		t.Cleanup(srv.Close)
		failing = append(failing, srv.URL)
	}
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/elonmusk/rss" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write(fixture(t))
	}))
	defer good.Close()

	c := testClient(t, good.URL)
	c.pool = failing
	c.pooledAt = c.now()
	if tweets, err := c.Timeline(context.Background(), "elonmusk"); err != nil || len(tweets) == 0 {
		t.Fatalf("Timeline with fallback after %d pool failures = %d, %v", len(failing), len(tweets), err)
	}
}

func TestTimelineBoundsEachInstanceAttempt(t *testing.T) {
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write(fixture(t))
	}))
	defer slow.Close()

	c := New(Config{
		Instances: []string{slow.URL},
		StatusURL: "http://127.0.0.1:1/status",
		Timeout:   50 * time.Millisecond,
	})
	start := time.Now()
	if _, err := c.Timeline(context.Background(), "elonmusk"); err == nil {
		t.Fatal("expected a timeout error")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("Timeline took %v, want it bounded by the per-attempt timeout", elapsed)
	}
}

func TestTimelineAllInstancesFail(t *testing.T) {
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusBadGateway)
	}))
	defer bad.Close()

	c := testClient(t, bad.URL)
	if _, err := c.Timeline(context.Background(), "elonmusk"); err == nil {
		t.Fatal("expected an error when every instance fails")
	} else if !strings.Contains(err.Error(), "all instances failed") {
		t.Errorf("error text = %v", err)
	}
}

func TestNormalizeHandle(t *testing.T) {
	for _, in := range []string{"elonmusk", "@elonmusk", " @_RSSHub "} {
		if _, err := NormalizeHandle(in); err != nil {
			t.Errorf("NormalizeHandle(%q): %v", in, err)
		}
	}
	for _, in := range []string{"", "@", "has space", strings.Repeat("a", 16), "bad-hyphen"} {
		if _, err := NormalizeHandle(in); err == nil {
			t.Errorf("NormalizeHandle(%q) should fail", in)
		}
	}
}

func TestFetchPoolFiltersAndSorts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		doc := map[string]any{"hosts": []map[string]any{
			{"url": "https://low.test", "rss": true, "points": 10, "healthy": true},
			{"url": "https://high.test", "rss": true, "points": 90, "healthy": true},
			{"url": "https://norRss.test", "rss": false, "points": 99, "healthy": true},
			{"url": "https://bad.test", "rss": true, "points": 99, "is_bad_host": true},
			{"url": "https://mid.test", "rss": true, "points": 70, "healthy": false},
		}}
		_ = json.NewEncoder(w).Encode(doc)
	}))
	defer srv.Close()

	c := New(Config{StatusURL: srv.URL, Timeout: 3 * time.Second})
	pool, err := c.fetchPool(context.Background())
	if err != nil {
		t.Fatalf("fetchPool: %v", err)
	}
	want := []string{"https://high.test", "https://mid.test"}
	if strings.Join(pool, ",") != strings.Join(want, ",") {
		t.Errorf("pool = %v, want %v", pool, want)
	}
}
