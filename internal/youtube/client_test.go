package youtube

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const (
	testChannelID = "UC_x5XG1OV2P6uZZ5FSM9Ttw"
	testHandle    = "GoogleDevelopers"
)

func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	body, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return body
}

func newClient(t *testing.T, url string) *Client {
	t.Helper()
	return New(Config{BaseURL: url, Timeout: 2 * time.Second, RetryDelay: time.Millisecond})
}

// newFeedOnlyClient keeps the browse fallback out of the way so a test can
// count Atom-feed requests without the fallback adding its own.
func newFeedOnlyClient(t *testing.T, url string) *Client {
	t.Helper()
	return New(Config{BaseURL: url, Timeout: 2 * time.Second, RetryDelay: time.Millisecond, DisableFallback: true})
}

func TestVideosParsesFeed(t *testing.T) {
	fixture := loadFixture(t, "testdata/feed.xml")
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		if r.URL.Path != "/feeds/videos.xml" {
			t.Errorf("path = %q, want /feeds/videos.xml", r.URL.Path)
		}
		if got := r.URL.Query().Get("channel_id"); got != testChannelID {
			t.Errorf("channel_id = %q, want %q", got, testChannelID)
		}
		_, _ = w.Write(fixture)
	}))
	defer srv.Close()

	f, err := newClient(t, srv.URL).Videos(context.Background(), testChannelID)
	if err != nil {
		t.Fatalf("Videos: %v", err)
	}
	if hits != 1 {
		t.Fatalf("requests = %d, want 1", hits)
	}
	if f.ChannelID != testChannelID {
		t.Fatalf("channel id = %q", f.ChannelID)
	}
	if f.Title != "Google for Developers" {
		t.Fatalf("channel title = %q", f.Title)
	}
	if len(f.Videos) != 2 {
		t.Fatalf("videos = %d, want 2", len(f.Videos))
	}
	v := f.Videos[0]
	if v.ID != "CsF5oqVV5YE" {
		t.Fatalf("id = %q", v.ID)
	}
	if v.Title == "" || v.Description == "" {
		t.Fatalf("empty title/description: %+v", v)
	}
	if v.URL != "https://www.youtube.com/watch?v=CsF5oqVV5YE" {
		t.Fatalf("url = %q", v.URL)
	}
	if v.Thumbnail == "" || v.Views == 0 {
		t.Fatalf("missing thumbnail/views: %+v", v)
	}
	if v.Published.IsZero() || v.Published.Year() < 2020 {
		t.Fatalf("published = %v", v.Published)
	}
}

func TestVideosRetriesTransientFailures(t *testing.T) {
	fixture := loadFixture(t, "testdata/feed.xml")
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch atomic.AddInt32(&hits, 1) {
		case 1:
			http.Error(w, "not found", http.StatusNotFound)
		case 2:
			http.Error(w, "boom", http.StatusInternalServerError)
		default:
			_, _ = w.Write(fixture)
		}
	}))
	defer srv.Close()

	f, err := newClient(t, srv.URL).Videos(context.Background(), testChannelID)
	if err != nil {
		t.Fatalf("Videos: %v", err)
	}
	if len(f.Videos) != 2 {
		t.Fatalf("videos = %d, want 2", len(f.Videos))
	}
	if hits != 3 {
		t.Fatalf("requests = %d, want 3", hits)
	}
}

func TestVideosReportsExhaustedRetries(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	_, err := newFeedOnlyClient(t, srv.URL).Videos(context.Background(), testChannelID)
	if err == nil {
		t.Fatal("want an error after every attempt failed")
	}
	var he *HTTPError
	if !errors.As(err, &he) || he.StatusCode != http.StatusNotFound {
		t.Fatalf("err = %v, want HTTP 404", err)
	}
	if hits != MaxAttempts {
		t.Fatalf("requests = %d, want %d", hits, MaxAttempts)
	}
}

func TestVideosDoesNotRetryBadRequest(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		http.Error(w, "bad request", http.StatusBadRequest)
	}))
	defer srv.Close()

	if _, err := newFeedOnlyClient(t, srv.URL).Videos(context.Background(), testChannelID); err == nil {
		t.Fatal("want an error")
	}
	if hits != 1 {
		t.Fatalf("requests = %d, want 1 (400 is not retried)", hits)
	}
}

func TestVideosRejectsMalformedChannelID(t *testing.T) {
	if _, err := New(Config{}).Videos(context.Background(), "notachannel"); err == nil {
		t.Fatal("want an error for a malformed channel id")
	}
}

func TestChannelIDPassthroughNeedsNoRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request to %s", r.URL.Path)
	}))
	defer srv.Close()

	for _, ref := range []string{
		testChannelID,
		"https://www.youtube.com/channel/" + testChannelID,
		"www.youtube.com/channel/" + testChannelID + "/videos",
	} {
		got, err := newClient(t, srv.URL).ChannelID(context.Background(), ref)
		if err != nil {
			t.Fatalf("ChannelID(%q): %v", ref, err)
		}
		if got != testChannelID {
			t.Fatalf("ChannelID(%q) = %q", ref, got)
		}
	}
}

func TestChannelIDResolvesHandle(t *testing.T) {
	page := []byte(`<html><head><script>{"metadata":{"channelMetadataRenderer":{"externalId":"` + testChannelID + `","title":"Google for Developers"}}}</script></head></html>`)
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		if r.URL.Path != "/@"+testHandle {
			t.Errorf("path = %q, want /@%s", r.URL.Path, testHandle)
		}
		_, _ = w.Write(page)
	}))
	defer srv.Close()

	c := newClient(t, srv.URL)
	for _, ref := range []string{testHandle, "@" + testHandle, "https://www.youtube.com/@" + testHandle, "https://www.youtube.com/@" + testHandle + "/videos"} {
		got, err := c.ChannelID(context.Background(), ref)
		if err != nil {
			t.Fatalf("ChannelID(%q): %v", ref, err)
		}
		if got != testChannelID {
			t.Fatalf("ChannelID(%q) = %q", ref, got)
		}
	}
	if hits != 4 {
		t.Fatalf("requests = %d, want 4 (no client-side cache)", hits)
	}
}

func TestChannelIDWithoutExternalID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<html>no channel here</html>"))
	}))
	defer srv.Close()

	if _, err := newClient(t, srv.URL).ChannelID(context.Background(), "nosuchchannel"); err == nil {
		t.Fatal("want an error when the page carries no channel id")
	}
}

func TestChannelIDRejectsEmptyReference(t *testing.T) {
	if _, err := New(Config{}).ChannelID(context.Background(), "   "); err == nil {
		t.Fatal("want an error for an empty reference")
	}
}

func TestVideosFallsBackToBrowse(t *testing.T) {
	browse := loadFixture(t, "testdata/browse.json")
	var feedHits, browseHits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/feeds/videos.xml":
			atomic.AddInt32(&feedHits, 1)
			http.NotFound(w, r)
		case r.URL.Path == "/youtubei/v1/browse":
			atomic.AddInt32(&browseHits, 1)
			if r.Method != http.MethodPost {
				t.Errorf("browse method = %s, want POST", r.Method)
			}
			if r.URL.Query().Get("key") == "" {
				t.Error("browse request has no key")
			}
			if r.Header.Get("X-YouTube-Client-Name") != "1" {
				t.Errorf("client name header = %q", r.Header.Get("X-YouTube-Client-Name"))
			}
			if r.Header.Get("X-YouTube-Client-Version") == "" {
				t.Error("client version header missing")
			}
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), testChannelID) || !strings.Contains(string(body), videosTabParams) {
				t.Errorf("browse body = %s", body)
			}
			_, _ = w.Write(browse)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	f, err := newClient(t, srv.URL).Videos(context.Background(), testChannelID)
	if err != nil {
		t.Fatalf("Videos: %v", err)
	}
	if feedHits != MaxAttempts || browseHits != 1 {
		t.Fatalf("feed/browse requests = %d/%d, want %d/1", feedHits, browseHits, MaxAttempts)
	}
	if f.Title != "Google for Developers" {
		t.Fatalf("title = %q", f.Title)
	}
	if len(f.Videos) != 2 {
		t.Fatalf("videos = %d, want 2", len(f.Videos))
	}
	v := f.Videos[0]
	if v.ID != "SOF3_evPje8" || v.URL != "https://www.youtube.com/watch?v=SOF3_evPje8" {
		t.Fatalf("video = %+v", v)
	}
	if v.Views != 32000 {
		t.Fatalf("views = %d, want 32000", v.Views)
	}
	if !strings.Contains(v.Thumbnail, "hq720") {
		t.Fatalf("thumbnail = %q, want the widest source", v.Thumbnail)
	}
	if age := time.Since(v.Published); age < 4*24*time.Hour || age > 6*24*time.Hour {
		t.Fatalf("published age = %v, want about 5 days", age)
	}
}

func TestVideosPrefersAtomFeed(t *testing.T) {
	feed := loadFixture(t, "testdata/feed.xml")
	browse := loadFixture(t, "testdata/browse.json")
	var browseHits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/feeds/videos.xml":
			_, _ = w.Write(feed)
		case "/youtubei/v1/browse":
			atomic.AddInt32(&browseHits, 1)
			_, _ = w.Write(browse)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	f, err := newClient(t, srv.URL).Videos(context.Background(), testChannelID)
	if err != nil {
		t.Fatalf("Videos: %v", err)
	}
	if browseHits != 0 {
		t.Fatalf("browse requests = %d, want 0 while the feed is healthy", browseHits)
	}
	if f.Videos[0].ID != "CsF5oqVV5YE" {
		t.Fatalf("first video = %q, want the Atom feed entry", f.Videos[0].ID)
	}
}

func TestVideosFallbackFailureReturnsFeedError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	_, err := newClient(t, srv.URL).Videos(context.Background(), testChannelID)
	var he *HTTPError
	if !errors.As(err, &he) || he.StatusCode != http.StatusNotFound {
		t.Fatalf("err = %v, want the feed's 404", err)
	}
}

func TestParseRelativeText(t *testing.T) {
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	cases := map[string]time.Duration{
		"5 days ago":          5 * 24 * time.Hour,
		"1 hour ago":          time.Hour,
		"an hour ago":         time.Hour,
		"2 weeks ago":         14 * 24 * time.Hour,
		"Streamed 3 days ago": 3 * 24 * time.Hour,
		"Premiered 1 day ago": 24 * time.Hour,
		"11 months ago":       11 * 30 * 24 * time.Hour,
		"1 year ago":          365 * 24 * time.Hour,
		"32K views":           0,
	}
	for text, want := range cases {
		got := parseRelativeText(text, now)
		if want == 0 {
			if !got.IsZero() {
				t.Fatalf("parseRelativeText(%q) = %v, want zero", text, got)
			}
			continue
		}
		if age := now.Sub(got); age != want {
			t.Fatalf("parseRelativeText(%q) age = %v, want %v", text, age, want)
		}
	}
}

func TestParseViews(t *testing.T) {
	cases := map[string]int64{
		"32K views":   32000,
		"1,234 views": 1234,
		"1.2M views":  1200000,
		"2B views":    2000000000,
		"No views":    0,
		"5 days ago":  0,
	}
	for text, want := range cases {
		if got := parseViews([]string{text}); got != want {
			t.Fatalf("parseViews(%q) = %d, want %d", text, got, want)
		}
	}
	if got := parseViews(nil); got != 0 {
		t.Fatalf("parseViews(nil) = %d", got)
	}
}

func TestParseHelpers(t *testing.T) {
	if got := parseInt("1234"); got != 1234 {
		t.Fatalf("parseInt = %d", got)
	}
	if got := parseInt(""); got != 0 {
		t.Fatalf("parseInt(empty) = %d", got)
	}
	if got := parseTime("2026-10-21T23:00:08+00:00"); got.UTC().Format(time.RFC3339) != "2026-10-21T23:00:08Z" {
		t.Fatalf("parseTime = %v", got)
	}
	if got := parseTime("nonsense"); !got.IsZero() {
		t.Fatalf("parseTime(nonsense) = %v", got)
	}
	thumbs := []atomThumbnail{{URL: "small", Width: "120"}, {URL: "big", Width: "480"}, {URL: "wide", Width: "1280"}}
	if got := pickThumbnail(thumbs); got != "wide" {
		t.Fatalf("pickThumbnail = %q", got)
	}
	if got := pickThumbnail(nil); got != "" {
		t.Fatalf("pickThumbnail(nil) = %q", got)
	}
	if _, err := strconv.Atoi("480"); err != nil {
		t.Fatal(err)
	}
}
