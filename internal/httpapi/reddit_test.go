package httpapi_test

import (
	"encoding/json"
	"fmt"
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
	"github.com/yilong-musk/feedmesh/internal/httpapi"
	"github.com/yilong-musk/feedmesh/internal/logx"
	"github.com/yilong-musk/feedmesh/internal/reddit"
)

// redditCapturedXML is the Atom document the site answered for
// r/programming/hot on 2026-10-30, cut to two entries. It is byte-faithful, so
// the double-escaped entry content is exercised exactly as the site sends it.
func redditCapturedXML(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "reddit", "testdata", "hot.rss"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return string(raw)
}

// redditListingXML builds a listing with n synthetic entries, for the cases the
// captured document cannot cover (page width, limit clamping).
func redditListingXML(n int) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?><feed xmlns="http://www.w3.org/2005/Atom">`)
	b.WriteString(`<title>programming</title><subtitle>Computer Programming</subtitle>`)
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, `<entry><author><name>/u/poster%d</name></author>`, i)
		fmt.Fprintf(&b, `<content type="html">&amp;#32; submitted by &amp;#32; &lt;a href=&quot;https://www.reddit.com/user/poster%d&quot;&gt; /u/poster%d &lt;/a&gt; &lt;br/&gt; &lt;span&gt;&lt;a href=&quot;https://example.com/story-%d&quot;&gt;[link]&lt;/a&gt;&lt;/span&gt; &amp;#32; &lt;span&gt;&lt;a href=&quot;https://www.reddit.com/r/programming/comments/aaa%03d/story_%d/&quot;&gt;[comments]&lt;/a&gt;&lt;/span&gt;</content>`, i, i, i, i, i)
		fmt.Fprintf(&b, `<id>t3_aaa%03d</id>`, i)
		fmt.Fprintf(&b, `<link href="https://www.reddit.com/r/programming/comments/aaa%03d/story_%d/" />`, i, i)
		fmt.Fprintf(&b, `<updated>2026-10-30T0%d:00:00+00:00</updated><published>2026-10-30T0%d:00:00+00:00</published>`, i%10, i%10)
		fmt.Fprintf(&b, `<title>Story %d</title></entry>`, i)
	}
	b.WriteString(`</feed>`)
	return b.String()
}

// redditUpstream is a controllable stand-in for reddit.com. It records every
// request, so a test can prove how many reads an answer cost.
type redditUpstream struct {
	mu     sync.Mutex
	seen   []string
	body   string
	down   atomic.Bool
	delay  time.Duration
	hitsMu sync.Mutex
	hits   int
}

func (u *redditUpstream) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u.hitsMu.Lock()
		u.hits++
		u.hitsMu.Unlock()
		u.mu.Lock()
		u.seen = append(u.seen, r.URL.Path+"?"+r.URL.RawQuery)
		body := u.body
		u.mu.Unlock()

		if u.delay > 0 {
			time.Sleep(u.delay)
		}
		if u.down.Load() {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/atom+xml")
		_, _ = w.Write([]byte(body))
	}
}

func (u *redditUpstream) readCount() int {
	u.hitsMu.Lock()
	defer u.hitsMu.Unlock()
	return u.hits
}

func (u *redditUpstream) requests() []string {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]string(nil), u.seen...)
}

// newRedditTestServer wires the HTTP layer onto one reddit upstream.
func newRedditTestServer(t *testing.T, up *redditUpstream, minInterval time.Duration) http.Handler {
	t.Helper()
	srv := httptest.NewServer(up.handler())
	t.Cleanup(srv.Close)

	c, err := cache.New(t.TempDir())
	if err != nil {
		t.Fatalf("cache: %v", err)
	}
	client := reddit.New(reddit.Config{BaseURL: srv.URL, Timeout: 2 * time.Second, MinInterval: minInterval})
	svc := feed.NewService(feed.Clients{Reddit: client}, c, logx.Discard(), feed.DefaultTTLs())
	api := httpapi.New(httpapi.Config{
		Version: "test",
		Service: svc,
		Cache:   c,
		Log:     logx.Discard(),
	})
	return api.Handler()
}

func TestRedditRSSRoute(t *testing.T) {
	up := &redditUpstream{body: redditCapturedXML(t)}
	h := newRedditTestServer(t, up, -1)

	rec := get(t, h, "/rss/reddit/programming?limit=5")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/rss+xml") {
		t.Errorf("content type = %q", ct)
	}
	if src := rec.Header().Get("X-Feedmesh-Source"); src != "atom" {
		t.Errorf("source = %q, want atom", src)
	}
	if deg := rec.Header().Get("X-Feedmesh-Degraded"); deg != "" {
		t.Errorf("degraded = %q, want none on a healthy read", deg)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "public, max-age=600" {
		t.Errorf("cache control = %q", cc)
	}

	body := rec.Body.String()
	for _, want := range []string{
		"<title>r/programming hot</title>",
		"<![CDATA[",
		"https://www.reddit.com/r/programming/comments/1xiq8we/",
		"Thu, 29 Oct 2026 17:22:17 +0000",
		"<ttl>10</ttl>",
		"Linux Kernel TCP Flaw With 9.8 CVSS Score",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body is missing %q", want)
		}
	}
	if got := strings.Count(body, "<item>"); got != 2 {
		t.Errorf("items = %d, want 2", got)
	}
	if got := up.requests(); len(got) != 1 || got[0] != "/r/programming/hot/.rss?limit=5" {
		t.Errorf("upstream requests = %v", got)
	}
}

func TestRedditRSSRouteAcceptsPrefixAndXMLSuffix(t *testing.T) {
	up := &redditUpstream{body: redditCapturedXML(t)}
	h := newRedditTestServer(t, up, -1)

	rec := get(t, h, "/rss/reddit/r/Programming.xml")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if got := up.requests(); len(got) != 1 || got[0] != "/r/programming/hot/.rss?limit=20" {
		t.Errorf("upstream requests = %v", got)
	}
	if !strings.Contains(rec.Body.String(), "<title>r/programming hot</title>") {
		t.Error("feed title is missing")
	}
}

func TestRedditJSONRoute(t *testing.T) {
	up := &redditUpstream{body: redditCapturedXML(t)}
	h := newRedditTestServer(t, up, -1)

	rec := get(t, h, "/api/v1/reddit?sub=programming&sort=top&window=week&limit=5")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got struct {
		Sub      string `json:"sub"`
		Sort     string `json:"sort"`
		Window   string `json:"window"`
		Title    string `json:"title"`
		Cached   bool   `json:"cached"`
		Degraded bool   `json:"degraded"`
		Source   string `json:"source"`
		Items    []struct {
			ID        string    `json:"id"`
			Title     string    `json:"title"`
			Author    string    `json:"author"`
			Text      string    `json:"text"`
			URL       string    `json:"url"`
			HTML      string    `json:"html"`
			Images    []string  `json:"images"`
			CreatedAt time.Time `json:"created_at"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v (body = %s)", err, rec.Body.String())
	}
	if got.Sub != "programming" || got.Sort != "top" || got.Window != "week" {
		t.Fatalf("listing = %q/%q/%q", got.Sub, got.Sort, got.Window)
	}
	if got.Title != "r/programming top (week)" {
		t.Errorf("title = %q", got.Title)
	}
	if got.Cached || got.Degraded || got.Source != "atom" {
		t.Errorf("cached/degraded/source = %v/%v/%q", got.Cached, got.Degraded, got.Source)
	}
	if len(got.Items) != 2 {
		t.Fatalf("items = %d", len(got.Items))
	}
	it := got.Items[0]
	if it.ID != "t3_1xiq8we" || it.Author != "CircumspectCapybara" {
		t.Errorf("item = %+v", it)
	}
	if it.URL != "https://www.reddit.com/r/programming/comments/1xiq8we/linux_kernel_tcp_flaw_with_98_cvss_score_affects/" {
		t.Errorf("url = %q", it.URL)
	}
	if it.Text != "https://thehackernews.com/2026/10/linux-kernel-tcp-flaw-with-98-cvss.html" {
		t.Errorf("text = %q", it.Text)
	}
	if it.Images == nil || len(it.Images) != 0 {
		t.Errorf("images = %v, want an empty array", it.Images)
	}
	if !it.CreatedAt.Equal(time.Date(2026, 10, 29, 17, 22, 17, 0, time.UTC)) {
		t.Errorf("created_at = %s", it.CreatedAt)
	}
	if got := up.requests(); len(got) != 1 || got[0] != "/r/programming/top/.rss?limit=5&t=week" {
		t.Errorf("upstream requests = %v", got)
	}
}

func TestRedditLimitClamping(t *testing.T) {
	up := &redditUpstream{body: redditListingXML(30)}
	h := newRedditTestServer(t, up, -1)

	rec := get(t, h, "/api/v1/reddit?sub=programming&limit=999")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var wide struct {
		Items []json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &wide); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(wide.Items) != 25 {
		t.Errorf("limit=999 returned %d items, want the 25-entry page width", len(wide.Items))
	}
	if got := up.requests()[0]; got != "/r/programming/hot/.rss?limit=25" {
		t.Errorf("limit=999 request = %q", got)
	}

	// limit=0 falls back to the default of 20.
	rec = get(t, h, "/api/v1/reddit?sub=golang&limit=0")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var dflt struct {
		Items []json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &dflt); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(dflt.Items) != 20 {
		t.Errorf("limit=0 returned %d items, want 20", len(dflt.Items))
	}
	if got := up.requests()[1]; got != "/r/golang/hot/.rss?limit=20" {
		t.Errorf("limit=0 request = %q", got)
	}
}

func TestRedditSecondRequestIsCached(t *testing.T) {
	up := &redditUpstream{body: redditCapturedXML(t)}
	h := newRedditTestServer(t, up, -1)

	if rec := get(t, h, "/api/v1/reddit?sub=programming"); rec.Code != http.StatusOK {
		t.Fatalf("first status = %d", rec.Code)
	}
	start := time.Now()
	rec := get(t, h, "/api/v1/reddit?sub=programming")
	elapsed := time.Since(start)
	if rec.Code != http.StatusOK {
		t.Fatalf("second status = %d", rec.Code)
	}
	if elapsed > 100*time.Millisecond {
		t.Errorf("cached answer took %s, want under 100ms", elapsed)
	}
	var got struct {
		Cached bool `json:"cached"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !got.Cached {
		t.Error("second answer reported cached=false")
	}
	if up.readCount() != 1 {
		t.Errorf("upstream reads = %d, want 1", up.readCount())
	}
}

func TestRedditDegradedAnswerCarriesItsHeaders(t *testing.T) {
	up := &redditUpstream{body: redditCapturedXML(t)}
	h := newRedditTestServer(t, up, -1)

	if rec := get(t, h, "/rss/reddit/programming"); rec.Code != http.StatusOK {
		t.Fatalf("warm status = %d", rec.Code)
	}
	up.down.Store(true)

	rec := get(t, h, "/rss/reddit/programming?refresh=1")
	if rec.Code != http.StatusOK {
		t.Fatalf("degraded status = %d, want 200 with the last good listing", rec.Code)
	}
	if deg := rec.Header().Get("X-Feedmesh-Degraded"); deg != "1" {
		t.Errorf("degraded header = %q", deg)
	}
	if src := rec.Header().Get("X-Feedmesh-Source"); src != "stale" {
		t.Errorf("source header = %q", src)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "public, max-age=60" {
		t.Errorf("cache control = %q, want the short degraded lifetime", cc)
	}
	if !strings.Contains(rec.Body.String(), "<item>") {
		t.Error("degraded answer carried no items")
	}
}

func TestRedditUpstreamFailureWithoutCacheIsAGatewayTimeout(t *testing.T) {
	up := &redditUpstream{body: redditCapturedXML(t)}
	up.down.Store(true)
	h := newRedditTestServer(t, up, -1)

	rec := get(t, h, "/api/v1/reddit?sub=programming")
	if rec.Code != http.StatusGatewayTimeout {
		t.Fatalf("status = %d, want 504", rec.Code)
	}
	if code := rec.Header().Get(httpapi.ErrorCodeHeader); code != "UPSTREAM_UNAVAILABLE" {
		t.Errorf("error code = %q", code)
	}
}

func TestRedditUnknownSubredditIsNotFound(t *testing.T) {
	// The site redirects an unknown subreddit to its search page.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "https://www.reddit.com/subreddits/search.rss?q="+r.URL.Path)
		w.WriteHeader(http.StatusFound)
	}))
	defer srv.Close()

	c, err := cache.New(t.TempDir())
	if err != nil {
		t.Fatalf("cache: %v", err)
	}
	client := reddit.New(reddit.Config{BaseURL: srv.URL, Timeout: 2 * time.Second, MinInterval: -1})
	api := httpapi.New(httpapi.Config{
		Version: "test",
		Service: feed.NewService(feed.Clients{Reddit: client}, c, logx.Discard(), feed.DefaultTTLs()),
		Cache:   c,
		Log:     logx.Discard(),
	})

	rec := get(t, api.Handler(), "/rss/reddit/zzz_no_such_sub")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if code := rec.Header().Get(httpapi.ErrorCodeHeader); code != "USER_NOT_FOUND" {
		t.Errorf("error code = %q", code)
	}
}

func TestRedditUnknownSortOrWindowIsBadRequest(t *testing.T) {
	up := &redditUpstream{body: redditCapturedXML(t)}
	h := newRedditTestServer(t, up, -1)

	for _, path := range []string{
		"/rss/reddit/programming?sort=rising",
		"/api/v1/reddit?sub=programming&window=decade",
		"/api/v1/reddit",
	} {
		rec := get(t, h, path)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s status = %d, want 400", path, rec.Code)
		}
		if code := rec.Header().Get(httpapi.ErrorCodeHeader); code != "BAD_REQUEST" {
			t.Errorf("%s error code = %q", path, code)
		}
	}
	if up.readCount() != 0 {
		t.Errorf("upstream reads = %d, want 0", up.readCount())
	}
}

func TestRedditConcurrentSubredditsShareOneUpstreamRead(t *testing.T) {
	up := &redditUpstream{body: redditCapturedXML(t), delay: 150 * time.Millisecond}
	// The production floor: one read at a time, at least eight seconds apart.
	h := newRedditTestServer(t, up, 8*time.Second)

	var wg sync.WaitGroup
	codes := make([]int, 3)
	for i, sub := range []string{"programming", "golang", "rust"} {
		wg.Add(1)
		go func(i int, sub string) {
			defer wg.Done()
			rec := get(t, h, "/api/v1/reddit?sub="+sub)
			codes[i] = rec.Code
		}(i, sub)
	}
	wg.Wait()

	if up.readCount() != 1 {
		t.Fatalf("upstream reads = %d, want exactly 1 within one interval", up.readCount())
	}
	ok, refused := 0, 0
	for _, code := range codes {
		switch code {
		case http.StatusOK:
			ok++
		case http.StatusGatewayTimeout:
			refused++
		}
	}
	if ok != 1 || refused != 2 {
		t.Fatalf("statuses = %v, want one 200 and two 504", codes)
	}
}
