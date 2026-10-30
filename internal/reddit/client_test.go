package reddit_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yilong-musk/feedmesh/internal/reddit"
)

// readFixture returns one captured Atom document. The files are byte-faithful
// cuts of the production replies, so the parser is tested against the shape the
// site actually sends (entity-escaped content, /u/ author names, no score).
func readFixture(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return string(raw)
}

// upstream is a controllable stand-in for reddit.com.
type upstream struct {
	mu       sync.Mutex
	hits     int
	seen     []string
	headers  []http.Header
	status   int
	body     string
	location string
	respond  func(w http.ResponseWriter, r *http.Request)
}

func (u *upstream) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u.mu.Lock()
		u.hits++
		u.seen = append(u.seen, r.URL.Path+"?"+r.URL.RawQuery)
		u.headers = append(u.headers, r.Header.Clone())
		respond, status, body, location := u.respond, u.status, u.body, u.location
		u.mu.Unlock()

		if respond != nil {
			respond(w, r)
			return
		}
		if location != "" {
			w.Header().Set("Location", location)
		}
		if body != "" {
			w.Header().Set("Content-Type", "application/atom+xml; charset=UTF-8")
		}
		if status == 0 {
			status = http.StatusOK
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}
}

func (u *upstream) count() int {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.hits
}

func (u *upstream) requests() []string {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]string(nil), u.seen...)
}

// newClient builds a client whose minimum interval is disabled: the limiter has
// its own test.
func newClient(t *testing.T, base string) *reddit.Client {
	t.Helper()
	return reddit.New(reddit.Config{BaseURL: base, Timeout: 2 * time.Second, MinInterval: -1})
}

func TestListParsesCapturedListing(t *testing.T) {
	up := &upstream{body: readFixture(t, "hot.rss")}
	srv := httptest.NewServer(up.handler())
	defer srv.Close()

	listing, err := newClient(t, srv.URL).List(context.Background(), "programming", reddit.SortHot, reddit.WindowDay, 20)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if listing.Sub != "programming" || listing.Title != "programming" {
		t.Fatalf("listing = %q / %q", listing.Sub, listing.Title)
	}
	if listing.Description != "Computer Programming" {
		t.Fatalf("description = %q", listing.Description)
	}
	if len(listing.Posts) != 2 {
		t.Fatalf("posts = %d, want 2", len(listing.Posts))
	}

	first := listing.Posts[0]
	if first.ID != "t3_1xiq8we" {
		t.Errorf("id = %q", first.ID)
	}
	if !strings.HasPrefix(first.Title, "Linux Kernel TCP Flaw With 9.8 CVSS Score") {
		t.Errorf("title = %q", first.Title)
	}
	if first.Author != "CircumspectCapybara" {
		t.Errorf("author = %q, want the name without the /u/ prefix", first.Author)
	}
	if first.Permalink != "https://www.reddit.com/r/programming/comments/1xiq8we/linux_kernel_tcp_flaw_with_98_cvss_score_affects/" {
		t.Errorf("permalink = %q", first.Permalink)
	}
	if first.ExternalURL != "https://thehackernews.com/2026/10/linux-kernel-tcp-flaw-with-98-cvss.html" {
		t.Errorf("external = %q", first.ExternalURL)
	}
	want := time.Date(2026, 10, 29, 17, 22, 17, 0, time.UTC)
	if !first.Published.Equal(want) {
		t.Errorf("published = %s, want %s", first.Published, want)
	}

	second := listing.Posts[1]
	if second.ExternalURL != "https://toji.dev/2026/10/06/webgpu-bindless.html" {
		t.Errorf("second external = %q", second.ExternalURL)
	}
	if got := second.Published; !got.Equal(time.Date(2026, 10, 30, 5, 52, 40, 0, time.UTC)) {
		t.Errorf("second published = %s", got)
	}
}

func TestListKeepsUpstreamOrder(t *testing.T) {
	// The hot listing is a ranking, not a time series. In the captured document
	// the first entry is older than the second, so a provider that sorted by
	// time would swap them and destroy the ranking the reader asked for.
	up := &upstream{body: readFixture(t, "hot.rss")}
	srv := httptest.NewServer(up.handler())
	defer srv.Close()

	listing, err := newClient(t, srv.URL).List(context.Background(), "programming", reddit.SortHot, reddit.WindowDay, 20)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if !listing.Posts[0].Published.Before(listing.Posts[1].Published) {
		t.Fatalf("fixture no longer proves anything: first entry must be the older one")
	}
	if listing.Posts[0].ID != "t3_1xiq8we" || listing.Posts[1].ID != "t3_1xj7d21" {
		t.Fatalf("order = %s, %s; want the document order", listing.Posts[0].ID, listing.Posts[1].ID)
	}
}

func TestListPathCarriesSortAndWindow(t *testing.T) {
	up := &upstream{body: readFixture(t, "hot.rss")}
	srv := httptest.NewServer(up.handler())
	defer srv.Close()
	client := newClient(t, srv.URL)

	cases := []struct {
		sort   reddit.Sort
		window reddit.Window
		want   string
	}{
		{reddit.SortHot, reddit.WindowDay, "/r/programming/hot/.rss?limit=20"},
		{reddit.SortNew, reddit.WindowDay, "/r/programming/new/.rss?limit=20"},
		// The window belongs to top only.
		{reddit.SortTop, reddit.WindowDay, "/r/programming/top/.rss?limit=20&t=day"},
		{reddit.SortTop, reddit.WindowAll, "/r/programming/top/.rss?limit=20&t=all"},
	}
	for _, tc := range cases {
		if _, err := client.List(context.Background(), "programming", tc.sort, tc.window, 20); err != nil {
			t.Fatalf("List(%s): %v", tc.sort, err)
		}
	}
	got := up.requests()
	if len(got) != len(cases) {
		t.Fatalf("requests = %v", got)
	}
	for i, tc := range cases {
		if got[i] != tc.want {
			t.Errorf("request %d = %q, want %q", i, got[i], tc.want)
		}
	}
	if ua := up.headers[0].Get("User-Agent"); ua != reddit.DefaultUserAgent {
		t.Errorf("user agent = %q", ua)
	}
	if accept := up.headers[0].Get("Accept"); accept != "application/atom+xml" {
		t.Errorf("accept = %q", accept)
	}
}

func TestListClampsLimit(t *testing.T) {
	up := &upstream{body: readFixture(t, "hot.rss")}
	srv := httptest.NewServer(up.handler())
	defer srv.Close()
	client := newClient(t, srv.URL)

	for _, limit := range []int{0, 999} {
		if _, err := client.List(context.Background(), "programming", reddit.SortHot, reddit.WindowDay, limit); err != nil {
			t.Fatalf("List(%d): %v", limit, err)
		}
	}
	got := up.requests()
	if got[0] != "/r/programming/hot/.rss?limit=20" {
		t.Errorf("limit=0 request = %q, want the default 20", got[0])
	}
	if got[1] != "/r/programming/hot/.rss?limit=25" {
		t.Errorf("limit=999 request = %q, want the page width 25", got[1])
	}
}

func TestListTruncatesToLimit(t *testing.T) {
	up := &upstream{body: readFixture(t, "hot.rss")}
	srv := httptest.NewServer(up.handler())
	defer srv.Close()

	listing, err := newClient(t, srv.URL).List(context.Background(), "programming", reddit.SortHot, reddit.WindowDay, 1)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(listing.Posts) != 1 {
		t.Fatalf("posts = %d, want 1", len(listing.Posts))
	}
}

func TestListRefusesSecondReadInsideTheInterval(t *testing.T) {
	up := &upstream{body: readFixture(t, "hot.rss")}
	srv := httptest.NewServer(up.handler())
	defer srv.Close()
	client := reddit.New(reddit.Config{BaseURL: srv.URL, Timeout: 2 * time.Second, MinInterval: time.Hour})

	if _, err := client.List(context.Background(), "programming", reddit.SortHot, reddit.WindowDay, 20); err != nil {
		t.Fatalf("first List: %v", err)
	}
	// A different subreddit shares the same slot: the floor is global.
	_, err := client.List(context.Background(), "golang", reddit.SortNew, reddit.WindowDay, 20)
	if !errors.Is(err, reddit.ErrBusy) {
		t.Fatalf("second List error = %v, want ErrBusy", err)
	}
	if up.count() != 1 {
		t.Fatalf("upstream reads = %d, want 1", up.count())
	}
}

func TestRateLimitWindowBlocksWithoutReading(t *testing.T) {
	var mu sync.Mutex
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	attempts := 0
	up := &upstream{respond: func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		attempts++
		n := attempts
		mu.Unlock()
		if n == 1 {
			// The site's shape: 429, empty body, reset in seconds.
			w.Header().Set("X-Ratelimit-Remaining", "0.0")
			w.Header().Set("X-Ratelimit-Reset", "6")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/atom+xml")
		_, _ = w.Write([]byte(readFixture(t, "hot.rss")))
	}}
	srv := httptest.NewServer(up.handler())
	defer srv.Close()
	client := reddit.New(reddit.Config{
		BaseURL:     srv.URL,
		Timeout:     2 * time.Second,
		MinInterval: -1,
		Now:         func() time.Time { mu.Lock(); defer mu.Unlock(); return now },
	})

	_, err := client.List(context.Background(), "programming", reddit.SortHot, reddit.WindowDay, 20)
	if !errors.Is(err, reddit.ErrRateLimited) {
		t.Fatalf("first List error = %v, want ErrRateLimited", err)
	}
	if up.count() != 1 {
		t.Fatalf("upstream reads = %d after a 429, want 1 (no retry storm)", up.count())
	}

	// Inside the window every caller is refused without touching the endpoint.
	for i := 0; i < 3; i++ {
		if _, err := client.List(context.Background(), "golang", reddit.SortHot, reddit.WindowDay, 20); !errors.Is(err, reddit.ErrRateLimited) {
			t.Fatalf("read %d inside the window error = %v, want ErrRateLimited", i, err)
		}
	}
	if up.count() != 1 {
		t.Fatalf("upstream reads inside the window = %d, want 1", up.count())
	}

	// Once the reset has passed the next read is allowed again.
	mu.Lock()
	now = now.Add(7 * time.Second)
	mu.Unlock()
	if _, err := client.List(context.Background(), "programming", reddit.SortHot, reddit.WindowDay, 20); err != nil {
		t.Fatalf("List after reset: %v", err)
	}
	if up.count() != 2 {
		t.Fatalf("upstream reads after reset = %d, want 2", up.count())
	}
}

func TestSpentQuotaOnSuccessIsHonoured(t *testing.T) {
	// The site reports the exhausted quota on the 200 that spends it. Reading
	// the header only on 429 would turn every second read into a wasted
	// request.
	var mu sync.Mutex
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	up := &upstream{respond: func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/atom+xml")
		w.Header().Set("X-Ratelimit-Remaining", "0.0")
		w.Header().Set("X-Ratelimit-Reset", "57")
		_, _ = w.Write([]byte(readFixture(t, "hot.rss")))
	}}
	srv := httptest.NewServer(up.handler())
	defer srv.Close()
	client := reddit.New(reddit.Config{
		BaseURL:     srv.URL,
		Timeout:     2 * time.Second,
		MinInterval: -1,
		Now:         func() time.Time { mu.Lock(); defer mu.Unlock(); return now },
	})

	if _, err := client.List(context.Background(), "programming", reddit.SortHot, reddit.WindowDay, 20); err != nil {
		t.Fatalf("first List: %v", err)
	}
	if _, err := client.List(context.Background(), "golang", reddit.SortHot, reddit.WindowDay, 20); !errors.Is(err, reddit.ErrRateLimited) {
		t.Fatalf("second List error = %v, want ErrRateLimited", err)
	}
	if up.count() != 1 {
		t.Fatalf("upstream reads = %d, want 1", up.count())
	}

	mu.Lock()
	now = now.Add(58 * time.Second)
	mu.Unlock()
	if _, err := client.List(context.Background(), "golang", reddit.SortHot, reddit.WindowDay, 20); err != nil {
		t.Fatalf("List after the reported reset: %v", err)
	}
}

func TestUnknownSubredditRedirectIsNotFound(t *testing.T) {
	// The site answers an unknown subreddit with a redirect to its search page
	// rather than a 404, and following that redirect would spend a second
	// request inside the same quota window.
	up := &upstream{
		status:   http.StatusFound,
		location: "https://www.reddit.com/subreddits/search.rss?q=zzz_no_such_sub",
	}
	srv := httptest.NewServer(up.handler())
	defer srv.Close()

	_, err := newClient(t, srv.URL).List(context.Background(), "zzz_no_such_sub", reddit.SortHot, reddit.WindowDay, 20)
	if !errors.Is(err, reddit.ErrSubredditNotFound) {
		t.Fatalf("error = %v, want ErrSubredditNotFound", err)
	}
	if up.count() != 1 {
		t.Fatalf("upstream reads = %d, want 1 (the redirect must not be followed)", up.count())
	}
}

func TestNotFoundStatusIsNotFound(t *testing.T) {
	up := &upstream{status: http.StatusNotFound}
	srv := httptest.NewServer(up.handler())
	defer srv.Close()

	_, err := newClient(t, srv.URL).List(context.Background(), "gone", reddit.SortHot, reddit.WindowDay, 20)
	if !errors.Is(err, reddit.ErrSubredditNotFound) {
		t.Fatalf("error = %v, want ErrSubredditNotFound", err)
	}
}

func TestServerErrorIsReported(t *testing.T) {
	up := &upstream{status: http.StatusInternalServerError}
	srv := httptest.NewServer(up.handler())
	defer srv.Close()

	_, err := newClient(t, srv.URL).List(context.Background(), "programming", reddit.SortHot, reddit.WindowDay, 20)
	var he *reddit.HTTPError
	if !errors.As(err, &he) || he.StatusCode != http.StatusInternalServerError {
		t.Fatalf("error = %v, want an HTTPError with 500", err)
	}
}

func TestRejectsUnknownSortAndWindow(t *testing.T) {
	up := &upstream{body: readFixture(t, "hot.rss")}
	srv := httptest.NewServer(up.handler())
	defer srv.Close()
	client := newClient(t, srv.URL)

	if _, err := client.List(context.Background(), "programming", "rising", reddit.WindowDay, 20); !errors.Is(err, reddit.ErrBadRequest) {
		t.Fatalf("unknown sort error = %v", err)
	}
	if _, err := client.List(context.Background(), "programming", reddit.SortTop, "decade", 20); !errors.Is(err, reddit.ErrBadRequest) {
		t.Fatalf("unknown window error = %v", err)
	}
	if up.count() != 0 {
		t.Fatalf("upstream reads = %d, want 0", up.count())
	}
}

func TestListParsesTopListing(t *testing.T) {
	// The top listing carries a different channel title and its own entries.
	// The provider keeps the document title in the listing and builds the feed
	// title itself, so both are checked here.
	up := &upstream{body: readFixture(t, "top_day.rss")}
	srv := httptest.NewServer(up.handler())
	defer srv.Close()

	listing, err := newClient(t, srv.URL).List(context.Background(), "programming", reddit.SortTop, reddit.WindowDay, 20)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if listing.Title != "top scoring links : programming" {
		t.Errorf("listing title = %q", listing.Title)
	}
	if listing.Sort != reddit.SortTop || listing.Window != reddit.WindowDay {
		t.Errorf("listing sort/window = %q/%q", listing.Sort, listing.Window)
	}
	if len(listing.Posts) != 2 || listing.Posts[1].Title != "It's the memory latency, stupid" {
		t.Fatalf("posts = %+v", listing.Posts)
	}
	if got := up.requests()[0]; got != "/r/programming/top/.rss?limit=20&t=day" {
		t.Errorf("request = %q", got)
	}
}

func TestNormalizeSub(t *testing.T) {
	cases := map[string]string{
		"programming":    "programming",
		"Programming":    "programming",
		"r/programming":  "programming",
		"/r/programming": "programming",
		"R/Programming/": "programming",
		"  golang  ":     "golang",
		"some_sub-2":     "some_sub-2",
	}
	for in, want := range cases {
		got, err := reddit.NormalizeSub(in)
		if err != nil || got != want {
			t.Errorf("NormalizeSub(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "a b", "../etc", "r/", "sub/name", "r/r/x"} {
		if got, err := reddit.NormalizeSub(bad); err == nil {
			t.Errorf("NormalizeSub(%q) = %q, want an error", bad, got)
		}
	}
}

func TestParseSortAndWindow(t *testing.T) {
	for in, want := range map[string]reddit.Sort{"": reddit.SortHot, "hot": reddit.SortHot, "NEW": reddit.SortNew, "top": reddit.SortTop} {
		got, ok := reddit.ParseSort(in)
		if !ok || got != want {
			t.Errorf("ParseSort(%q) = %q, %v", in, got, ok)
		}
	}
	if _, ok := reddit.ParseSort("rising"); ok {
		t.Error("ParseSort accepted rising")
	}
	for in, want := range map[string]reddit.Window{"": reddit.WindowDay, "day": reddit.WindowDay, "All": reddit.WindowAll} {
		got, ok := reddit.ParseWindow(in)
		if !ok || got != want {
			t.Errorf("ParseWindow(%q) = %q, %v", in, got, ok)
		}
	}
	if _, ok := reddit.ParseWindow("hour"); ok {
		t.Error("ParseWindow accepted hour")
	}
}
