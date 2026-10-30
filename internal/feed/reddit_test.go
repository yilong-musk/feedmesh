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
	"github.com/yilong-musk/feedmesh/internal/reddit"
	"github.com/yilong-musk/feedmesh/internal/weibo"
)

// redditCaptured is the Atom document the site answered for r/programming/hot
// on 2026-10-30, cut to two entries. It carries the real escaping: the entry
// content is double-escaped HTML, the author name has the /u/ prefix and there
// is no score, thumbnail or self text.
func redditCaptured(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "reddit", "testdata", "hot.rss"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return string(raw)
}

// rdUpstream is a controllable stand-in for reddit.com.
type rdUpstream struct {
	mu      sync.Mutex
	seen    []string
	body    string
	status  int
	header  http.Header
	down    atomic.Bool
	empty   atomic.Bool
	missing atomic.Bool
}

func (u *rdUpstream) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u.mu.Lock()
		u.seen = append(u.seen, r.URL.Path+"?"+r.URL.RawQuery)
		body, status, header := u.body, u.status, u.header
		u.mu.Unlock()

		for name, values := range header {
			for _, v := range values {
				w.Header().Add(name, v)
			}
		}
		switch {
		case u.down.Load():
			http.Error(w, "boom", http.StatusInternalServerError)
		case u.missing.Load():
			w.Header().Set("Location", "https://www.reddit.com/subreddits/search.rss?q="+r.URL.Path)
			w.WriteHeader(http.StatusFound)
		case u.empty.Load():
			w.Header().Set("Content-Type", "application/atom+xml")
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><feed xmlns="http://www.w3.org/2005/Atom"><title>empty</title></feed>`))
		default:
			if status == 0 {
				status = http.StatusOK
			}
			w.Header().Set("Content-Type", "application/atom+xml")
			w.WriteHeader(status)
			_, _ = w.Write([]byte(body))
		}
	}
}

func (u *rdUpstream) hits() int {
	u.mu.Lock()
	defer u.mu.Unlock()
	return len(u.seen)
}

func (u *rdUpstream) requests() []string {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]string(nil), u.seen...)
}

func newRedditService(t *testing.T, base string) *feed.Service {
	t.Helper()
	return newRedditServiceAt(t, base, t.TempDir(), -1)
}

// newRedditServiceAt is newRedditService with a caller-owned cache directory and
// an explicit minimum interval, so a test can age the stored listing on disk or
// exercise the global read slot.
func newRedditServiceAt(t *testing.T, base, dir string, minInterval time.Duration) *feed.Service {
	t.Helper()
	c, err := cache.New(dir)
	if err != nil {
		t.Fatalf("cache: %v", err)
	}
	client := reddit.New(reddit.Config{BaseURL: base, Timeout: 2 * time.Second, MinInterval: minInterval})
	return feed.NewService(feed.Clients{Reddit: client}, c, logx.Discard(), feed.DefaultTTLs())
}

// ageRedditEntry rewrites the stored listing so it looks fetched age ago: the
// entry expiry and the fetched_at inside the value move together, exactly as a
// write that long ago would leave them.
func ageRedditEntry(t *testing.T, dir string, age time.Duration) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, "reddit_list_*.json"))
	if err != nil || len(matches) != 1 {
		t.Fatalf("listing cache files = %v (err = %v), want exactly one", matches, err)
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
	value["fetched_at"] = time.Now().Add(-age).UTC().Format(time.RFC3339Nano)
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

func TestGetRedditMapsItems(t *testing.T) {
	up := &rdUpstream{body: redditCaptured(t)}
	srv := httptest.NewServer(up.handler())
	defer srv.Close()

	f, err := newRedditService(t, srv.URL).GetReddit(context.Background(), "programming", reddit.SortHot, reddit.WindowDay, 10, false)
	if err != nil {
		t.Fatalf("GetReddit: %v", err)
	}
	if f.UID != "programming" || f.Title() != "r/programming hot" {
		t.Fatalf("feed = %q / %q", f.UID, f.Title())
	}
	if f.Description != "Computer Programming" {
		t.Fatalf("description = %q", f.Description)
	}
	if f.Link() != "https://www.reddit.com/r/programming/hot/" {
		t.Fatalf("link = %q", f.Link())
	}
	if f.Cached || f.Degraded || f.Source != "atom" {
		t.Fatalf("cached/degraded/source = %v/%v/%q", f.Cached, f.Degraded, f.Source)
	}
	if len(f.Items) != 2 {
		t.Fatalf("items = %d, want 2", len(f.Items))
	}

	it := f.Items[0]
	if it.ID != "t3_1xiq8we" {
		t.Errorf("id = %q", it.ID)
	}
	if it.URL != "https://www.reddit.com/r/programming/comments/1xiq8we/linux_kernel_tcp_flaw_with_98_cvss_score_affects/" {
		t.Errorf("url = %q, want the permalink", it.URL)
	}
	if it.Text != "https://thehackernews.com/2026/10/linux-kernel-tcp-flaw-with-98-cvss.html" {
		t.Errorf("text = %q, want the outbound link", it.Text)
	}
	if it.Author != "CircumspectCapybara" {
		t.Errorf("author = %q", it.Author)
	}
	if !it.CreatedAt.Equal(time.Date(2026, 10, 29, 17, 22, 17, 0, time.UTC)) {
		t.Errorf("created_at = %s", it.CreatedAt)
	}
	if it.CreatedAt.Location() != time.UTC {
		t.Errorf("created_at location = %s, want UTC", it.CreatedAt.Location())
	}
	if it.Images == nil || len(it.Images) != 0 {
		t.Errorf("images = %v, want an empty array", it.Images)
	}
	if !strings.Contains(it.HTML, `href="https://thehackernews.com/2026/10/linux-kernel-tcp-flaw-with-98-cvss.html"`) {
		t.Errorf("html = %q, want the outbound link", it.HTML)
	}
	if !strings.Contains(it.HTML, "Comments on Reddit") {
		t.Errorf("html = %q, want the comment link", it.HTML)
	}
	if !strings.Contains(it.HTML, "submitted by u/CircumspectCapybara") {
		t.Errorf("html = %q, want the submitter", it.HTML)
	}
}

func TestGetRedditSecondReadIsCached(t *testing.T) {
	up := &rdUpstream{body: redditCaptured(t)}
	srv := httptest.NewServer(up.handler())
	defer srv.Close()
	svc := newRedditService(t, srv.URL)

	first, err := svc.GetReddit(context.Background(), "programming", reddit.SortHot, reddit.WindowDay, 10, false)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	if first.Cached {
		t.Fatal("first read reported cached")
	}
	second, err := svc.GetReddit(context.Background(), "programming", reddit.SortHot, reddit.WindowDay, 10, false)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if !second.Cached {
		t.Fatal("second read missed the cache")
	}
	if up.hits() != 1 {
		t.Fatalf("upstream reads = %d, want 1", up.hits())
	}
	if !second.FetchedAt.Equal(first.FetchedAt) {
		t.Fatalf("fetched_at moved on a cache hit: %s vs %s", second.FetchedAt, first.FetchedAt)
	}
}

func TestGetRedditWidensTheCache(t *testing.T) {
	up := &rdUpstream{body: redditCaptured(t)}
	srv := httptest.NewServer(up.handler())
	defer srv.Close()
	svc := newRedditService(t, srv.URL)

	if _, err := svc.GetReddit(context.Background(), "programming", reddit.SortHot, reddit.WindowDay, 1, false); err != nil {
		t.Fatalf("narrow read: %v", err)
	}
	wide, err := svc.GetReddit(context.Background(), "programming", reddit.SortHot, reddit.WindowDay, 2, false)
	if err != nil {
		t.Fatalf("wide read: %v", err)
	}
	if len(wide.Items) != 2 {
		t.Fatalf("items = %d, want the wider listing", len(wide.Items))
	}
	if up.hits() != 2 {
		t.Fatalf("upstream reads = %d, want 2 (the wider read must refetch)", up.hits())
	}
	if got := up.requests()[1]; got != "/r/programming/hot/.rss?limit=2" {
		t.Fatalf("wide request = %q", got)
	}

	// A narrower read after a wide one is answered from the cache.
	narrow, err := svc.GetReddit(context.Background(), "programming", reddit.SortHot, reddit.WindowDay, 1, false)
	if err != nil {
		t.Fatalf("second narrow read: %v", err)
	}
	if len(narrow.Items) != 1 || !narrow.Cached {
		t.Fatalf("narrow read = %d items, cached=%v", len(narrow.Items), narrow.Cached)
	}
	if up.hits() != 2 {
		t.Fatalf("upstream reads = %d, want 2", up.hits())
	}
}

func TestGetRedditSharesOneCacheKeyPerListing(t *testing.T) {
	up := &rdUpstream{body: redditCaptured(t)}
	srv := httptest.NewServer(up.handler())
	defer srv.Close()
	svc := newRedditService(t, srv.URL)

	if _, err := svc.GetReddit(context.Background(), "r/Programming", reddit.SortHot, reddit.WindowDay, 10, false); err != nil {
		t.Fatalf("prefixed name: %v", err)
	}
	if _, err := svc.GetReddit(context.Background(), "programming", reddit.SortHot, reddit.WindowDay, 10, false); err != nil {
		t.Fatalf("bare name: %v", err)
	}
	if up.hits() != 1 {
		t.Fatalf("upstream reads = %d, want 1: the two spellings name one listing", up.hits())
	}
}

func TestGetRedditSortsAndWindowsUseTheirOwnPaths(t *testing.T) {
	up := &rdUpstream{body: redditCaptured(t)}
	srv := httptest.NewServer(up.handler())
	defer srv.Close()
	svc := newRedditService(t, srv.URL)

	if _, err := svc.GetReddit(context.Background(), "programming", reddit.SortTop, reddit.WindowWeek, 10, false); err != nil {
		t.Fatalf("top: %v", err)
	}
	if got := up.requests()[0]; got != "/r/programming/top/.rss?limit=10&t=week" {
		t.Fatalf("top request = %q", got)
	}
	if _, err := svc.GetReddit(context.Background(), "programming", reddit.SortHot, reddit.WindowWeek, 10, false); err != nil {
		t.Fatalf("hot: %v", err)
	}
	if got := up.requests()[1]; got != "/r/programming/hot/.rss?limit=10" {
		t.Fatalf("hot request = %q", got)
	}
}

func TestGetRedditDegradesToLastGood(t *testing.T) {
	up := &rdUpstream{body: redditCaptured(t)}
	srv := httptest.NewServer(up.handler())
	defer srv.Close()
	svc := newRedditService(t, srv.URL)

	if _, err := svc.GetReddit(context.Background(), "programming", reddit.SortHot, reddit.WindowDay, 10, false); err != nil {
		t.Fatalf("warm read: %v", err)
	}
	up.down.Store(true)
	f, err := svc.GetReddit(context.Background(), "programming", reddit.SortHot, reddit.WindowDay, 10, true)
	if err != nil {
		t.Fatalf("degraded read: %v", err)
	}
	if !f.Degraded || !f.Cached || f.Source != "stale" {
		t.Fatalf("degraded/cached/source = %v/%v/%q", f.Degraded, f.Cached, f.Source)
	}
	if len(f.Items) != 2 {
		t.Fatalf("items = %d, want the last good listing", len(f.Items))
	}
}

func TestGetRedditFailsWithoutALastGoodListing(t *testing.T) {
	up := &rdUpstream{down: atomic.Bool{}}
	up.down.Store(true)
	srv := httptest.NewServer(up.handler())
	defer srv.Close()

	_, err := newRedditService(t, srv.URL).GetReddit(context.Background(), "programming", reddit.SortHot, reddit.WindowDay, 10, false)
	if code := weibo.CodeOf(err); code != weibo.CodeUpstreamUnavailable {
		t.Fatalf("code = %q (%v), want %q", code, err, weibo.CodeUpstreamUnavailable)
	}
}

func TestGetRedditEmptyListingKeepsTheLastGoodOne(t *testing.T) {
	up := &rdUpstream{body: redditCaptured(t)}
	srv := httptest.NewServer(up.handler())
	defer srv.Close()
	svc := newRedditService(t, srv.URL)

	if _, err := svc.GetReddit(context.Background(), "programming", reddit.SortHot, reddit.WindowDay, 10, false); err != nil {
		t.Fatalf("warm read: %v", err)
	}
	up.empty.Store(true)
	f, err := svc.GetReddit(context.Background(), "programming", reddit.SortHot, reddit.WindowDay, 10, true)
	if err != nil {
		t.Fatalf("empty read: %v", err)
	}
	if !f.Degraded || len(f.Items) != 2 {
		t.Fatalf("degraded = %v, items = %d; want the last good listing kept", f.Degraded, len(f.Items))
	}
}

func TestGetRedditUnknownSubredditIsNotFound(t *testing.T) {
	up := &rdUpstream{}
	up.missing.Store(true)
	srv := httptest.NewServer(up.handler())
	defer srv.Close()

	_, err := newRedditService(t, srv.URL).GetReddit(context.Background(), "zzz_no_such_sub", reddit.SortHot, reddit.WindowDay, 10, false)
	if code := weibo.CodeOf(err); code != weibo.CodeUserNotFound {
		t.Fatalf("code = %q (%v), want %q", code, err, weibo.CodeUserNotFound)
	}
}

func TestGetRedditRejectsInvalidNamesAndSorts(t *testing.T) {
	up := &rdUpstream{body: redditCaptured(t)}
	srv := httptest.NewServer(up.handler())
	defer srv.Close()
	svc := newRedditService(t, srv.URL)

	if _, err := svc.GetReddit(context.Background(), "a b", reddit.SortHot, reddit.WindowDay, 10, false); weibo.CodeOf(err) != weibo.CodeBadRequest {
		t.Fatalf("bad name code = %q (%v)", weibo.CodeOf(err), err)
	}
	if _, err := svc.GetReddit(context.Background(), "programming", "rising", reddit.WindowDay, 10, false); weibo.CodeOf(err) != weibo.CodeBadRequest {
		t.Fatalf("bad sort code = %q (%v)", weibo.CodeOf(err), err)
	}
	if _, err := svc.GetReddit(context.Background(), "programming", reddit.SortTop, "decade", 10, false); weibo.CodeOf(err) != weibo.CodeBadRequest {
		t.Fatalf("bad window code = %q (%v)", weibo.CodeOf(err), err)
	}
	if up.hits() != 0 {
		t.Fatalf("upstream reads = %d, want 0: local validation must not reach upstream", up.hits())
	}
}

func TestGetRedditLimiterRefusalIsNotAnUpstreamRead(t *testing.T) {
	up := &rdUpstream{body: redditCaptured(t)}
	srv := httptest.NewServer(up.handler())
	defer srv.Close()
	// One read per hour: the second subreddit cannot be read at all.
	svc := newRedditServiceAt(t, srv.URL, t.TempDir(), time.Hour)

	if _, err := svc.GetReddit(context.Background(), "programming", reddit.SortHot, reddit.WindowDay, 10, false); err != nil {
		t.Fatalf("first subreddit: %v", err)
	}
	_, err := svc.GetReddit(context.Background(), "golang", reddit.SortHot, reddit.WindowDay, 10, false)
	if !errors.Is(err, reddit.ErrBusy) {
		t.Fatalf("second subreddit error = %v, want the limiter refusal", err)
	}
	if code := weibo.CodeOf(err); code != weibo.CodeUpstreamUnavailable {
		t.Fatalf("code = %q, want %q", code, weibo.CodeUpstreamUnavailable)
	}
	if up.hits() != 1 {
		t.Fatalf("upstream reads = %d, want 1", up.hits())
	}
}

func TestGetRedditStaleWindowExpires(t *testing.T) {
	up := &rdUpstream{body: redditCaptured(t)}
	srv := httptest.NewServer(up.handler())
	defer srv.Close()
	dir := t.TempDir()
	svc := newRedditServiceAt(t, srv.URL, dir, -1)

	if _, err := svc.GetReddit(context.Background(), "programming", reddit.SortHot, reddit.WindowDay, 10, false); err != nil {
		t.Fatalf("warm read: %v", err)
	}
	// Twenty-five hours later the entry is outside the last-good window.
	ageRedditEntry(t, dir, 25*time.Hour)
	up.down.Store(true)

	_, err := svc.GetReddit(context.Background(), "programming", reddit.SortHot, reddit.WindowDay, 10, true)
	if code := weibo.CodeOf(err); code != weibo.CodeUpstreamUnavailable {
		t.Fatalf("code = %q (%v), want %q: the entry is older than the stale window", code, err, weibo.CodeUpstreamUnavailable)
	}
}

func TestGetRedditServesLastGoodInsideTheStaleWindow(t *testing.T) {
	up := &rdUpstream{body: redditCaptured(t)}
	srv := httptest.NewServer(up.handler())
	defer srv.Close()
	dir := t.TempDir()
	svc := newRedditServiceAt(t, srv.URL, dir, -1)

	if _, err := svc.GetReddit(context.Background(), "programming", reddit.SortHot, reddit.WindowDay, 10, false); err != nil {
		t.Fatalf("warm read: %v", err)
	}
	// Eleven minutes old: past the ten-minute freshness window, well inside the
	// last-good one. The default read path must serve it instead of failing.
	ageRedditEntry(t, dir, 11*time.Minute)
	up.down.Store(true)

	f, err := svc.GetReddit(context.Background(), "programming", reddit.SortHot, reddit.WindowDay, 10, false)
	if err != nil {
		t.Fatalf("stale read: %v", err)
	}
	if !f.Degraded || f.Source != "stale" || len(f.Items) != 2 {
		t.Fatalf("degraded/source/items = %v/%q/%d", f.Degraded, f.Source, len(f.Items))
	}
}
