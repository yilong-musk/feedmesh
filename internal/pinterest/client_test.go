package pinterest

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// userFeedXML is a trimmed copy of a live profile feed (nasa/feed.rss): two
// pins with a title, one with an empty title, all carrying a 236px thumbnail.
const userFeedXML = `<?xml version="1.0" encoding="utf-8"?><rss xmlns:atom="http://www.w3.org/2005/Atom" version="2.0">
    <channel>
        <title>NASA</title>
        <link>https://www.pinterest.com/nasa/</link>
        <description>Explore the universe and discover our home planet with the official NASA boards on Pinterest.</description>
        <language>en-us</language>
        <lastBuildDate>Wed, 25 Nov 2026 07:26:52 GMT</lastBuildDate>
        <item>
            <title>NASA is returning astronauts to the Moon.</title>
            <link>https://www.pinterest.com/pin/142567144449130105/</link>
            <description>&lt;a href=&quot;https://www.pinterest.com/pin/142567144449130105/&quot;&gt;&lt;img src=&quot;https://i.pinimg.com/236x/7f/b0/7c/7fb07c79a6280dd11eab9165b4120300.jpg&quot;&gt;&lt;/a&gt;NASA is returning astronauts to the Moon.</description>
            <pubDate>Fri, 20 Nov 2026 16:39:34 GMT</pubDate>
            <guid>https://www.pinterest.com/pin/142567144449130105/</guid>
        </item>
        <item>
            <title> </title>
            <link>https://www.pinterest.com/pin/142567144449081155/</link>
            <description>&lt;a href=&quot;https://www.pinterest.com/pin/142567144449081155/&quot;&gt;&lt;img src=&quot;https://i.pinimg.com/236x/bb/a1/e7/bba1e7fb6fe37433e1314ad3cf4e3434.jpg&quot;&gt;&lt;/a&gt;Next stop: Titan! NASA&apos;s Dragonfly mission is heading to Saturn&apos;s largest moon.</description>
            <pubDate>Fri, 20 Nov 2026 15:00:00 GMT</pubDate>
            <guid>https://www.pinterest.com/pin/142567144449081155/</guid>
        </item>
        <item>
            <title>Voyager 1 at the edge of the solar system</title>
            <link>https://www.pinterest.com/pin/142567144449027286/</link>
            <description>&lt;a href=&quot;https://www.pinterest.com/pin/142567144449027286/&quot;&gt;&lt;img src=&quot;https://i.pinimg.com/236x/5b/cb/1d/5bcb1d194cab7dc148540cbbd54319f5.jpg&quot;&gt;&lt;/a&gt;Ever since Voyager 1 crossed the heliopause.</description>
            <pubDate>Thu, 19 Nov 2026 10:00:00 GMT</pubDate>
            <guid>https://www.pinterest.com/pin/142567144449027286/</guid>
        </item>
    </channel></rss>`

// boardFeedXML is a trimmed copy of a live board feed (nasa/earth.rss). Board
// pins carry opaque ids and can have much older creation dates; the order is
// Pinterest's own and must be preserved.
const boardFeedXML = `<?xml version="1.0" encoding="utf-8"?><rss xmlns:atom="http://www.w3.org/2005/Atom" version="2.0">
    <channel>
        <title>Earth</title>
        <link>https://www.pinterest.com/nasa/earth</link>
        <description></description>
        <language>en-us</language>
        <lastBuildDate>Wed, 25 Nov 2026 07:31:10 GMT</lastBuildDate>
        <item>
            <title>Polar bears are scrambling to find ground to stand on.</title>
            <link>https://www.pinterest.com/pin/e1_AfqPShwRDMi0aXUqBNuln_BzY7y-7nLKzl3QcRs0w1M1-vS9pbG9/</link>
            <description>&lt;a href=&quot;https://www.pinterest.com/pin/e1_AfqPShwRDMi0aXUqBNuln_BzY7y-7nLKzl3QcRs0w1M1-vS9pbG9/&quot;&gt;&lt;img src=&quot;https://i.pinimg.com/236x/17/66/48/17664853e56a6b68e5fa9ad24f5732d0.jpg&quot;&gt;&lt;/a&gt;Polar bears are scrambling to find ground to stand on.</description>
            <pubDate>Thu, 16 Jun 2022 18:20:42 GMT</pubDate>
            <guid>https://www.pinterest.com/pin/e1_AfqPShwRDMi0aXUqBNuln_BzY7y-7nLKzl3QcRs0w1M1-vS9pbG9/</guid>
        </item>
        <item>
            <title>NASA selected two companies for the GEO-XO Imager study.</title>
            <link>https://www.pinterest.com/pin/e1_AfqPShy3JKii1RUTjzwyxIWHNRfzYogXCNSTAtmVDlPecUwHGqek/</link>
            <description>&lt;a href=&quot;https://www.pinterest.com/pin/e1_AfqPShy3JKii1RUTjzwyxIWHNRfzYogXCNSTAtmVDlPecUwHGqek/&quot;&gt;&lt;img src=&quot;https://i.pinimg.com/236x/4b/07/3e/4b073e59b72b96a21e4bea6c24b66ab7.jpg&quot;&gt;&lt;/a&gt;NASA has selected two companies.</description>
            <pubDate>Wed, 31 Mar 2021 20:03:28 GMT</pubDate>
            <guid>https://www.pinterest.com/pin/e1_AfqPShy3JKii1RUTjzwyxIWHNRfzYogXCNSTAtmVDlPecUwHGqek/</guid>
        </item>
    </channel></rss>`

func newTestClient(t *testing.T) *Client {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/nasa/feed.rss", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/xml; charset=utf-8")
		_, _ = w.Write([]byte(userFeedXML))
	})
	mux.HandleFunc("/nasa/earth.rss", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/xml; charset=utf-8")
		_, _ = w.Write([]byte(boardFeedXML))
	})
	mux.HandleFunc("/ghost/feed.rss", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("User not found"))
	})
	mux.HandleFunc("/nasa/nope.rss", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("Board not found"))
	})
	mux.HandleFunc("/empty/feed.rss", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/xml; charset=utf-8")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="utf-8"?><rss version="2.0"><channel><title>Empty</title><link>https://www.pinterest.com/empty/</link><description></description></channel></rss>`))
	})
	mux.HandleFunc("/shell/feed.rss", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("<!doctype html><html><body>app shell</body></html>"))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return New(Config{BaseURL: srv.URL, Timeout: 2 * time.Second})
}

func TestNewDefaultsBaseURL(t *testing.T) {
	// An empty BaseURL used to build relative request URLs, so every
	// production read failed with "unsupported protocol scheme".
	c := New(Config{})
	if c.base != DefaultBaseURL {
		t.Fatalf("base = %q, want %q", c.base, DefaultBaseURL)
	}
}

func TestFetchUserFeed(t *testing.T) {
	c := newTestClient(t)
	res, err := c.Fetch(context.Background(), "nasa", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if res.User != "nasa" || res.Board != "" {
		t.Fatalf("target = %q/%q", res.User, res.Board)
	}
	if res.Title != "NASA" {
		t.Fatalf("title = %q", res.Title)
	}
	if res.Link != "https://www.pinterest.com/nasa/" {
		t.Fatalf("link = %q", res.Link)
	}
	if len(res.Pins) != 3 {
		t.Fatalf("pins = %d", len(res.Pins))
	}
	first := res.Pins[0]
	if first.ID != "142567144449130105" {
		t.Fatalf("id = %q", first.ID)
	}
	if first.Image != "https://i.pinimg.com/736x/7f/b0/7c/7fb07c79a6280dd11eab9165b4120300.jpg" {
		t.Fatalf("image not upgraded to 736x: %q", first.Image)
	}
	if got := first.Published.Format(time.RFC3339); got != "2026-11-20T16:39:34Z" {
		t.Fatalf("published = %s", got)
	}
	if res.Pins[1].Title != "" {
		t.Fatalf("blank title must stay blank for the feed layer, got %q", res.Pins[1].Title)
	}
	if res.Pins[1].Text != "Next stop: Titan! NASA's Dragonfly mission is heading to Saturn's largest moon." {
		t.Fatalf("text = %q", res.Pins[1].Text)
	}
}

func TestFetchBoardFeed(t *testing.T) {
	c := newTestClient(t)
	res, err := c.Fetch(context.Background(), "nasa", "earth", 0)
	if err != nil {
		t.Fatal(err)
	}
	if res.Board != "earth" || res.Title != "Earth" {
		t.Fatalf("board = %q title = %q", res.Board, res.Title)
	}
	if res.Link != "https://www.pinterest.com/nasa/earth/" {
		t.Fatalf("link = %q", res.Link)
	}
	if len(res.Pins) != 2 {
		t.Fatalf("pins = %d", len(res.Pins))
	}
	if res.Pins[0].ID != "e1_AfqPShwRDMi0aXUqBNuln_BzY7y-7nLKzl3QcRs0w1M1-vS9pbG9" {
		t.Fatalf("opaque id = %q", res.Pins[0].ID)
	}
	// Old creation dates must keep Pinterest's order, not move to the top or
	// bottom.
	if got := res.Pins[0].Published.Year(); got != 2022 {
		t.Fatalf("first pin year = %d, want upstream order preserved", got)
	}
}

func TestFetchClampsLimit(t *testing.T) {
	c := newTestClient(t)
	res, err := c.Fetch(context.Background(), "nasa", "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Pins) != 1 {
		t.Fatalf("limit=1 returned %d pins", len(res.Pins))
	}
	res, err = c.Fetch(context.Background(), "nasa", "", 500)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Pins) != 3 {
		t.Fatalf("oversized limit returned %d pins", len(res.Pins))
	}
}

func TestFetchNotFound(t *testing.T) {
	c := newTestClient(t)
	if _, err := c.Fetch(context.Background(), "ghost", "", 5); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("user 404 = %v", err)
	}
	if _, err := c.Fetch(context.Background(), "nasa", "nope", 5); !errors.Is(err, ErrBoardNotFound) {
		t.Fatalf("board 404 = %v", err)
	}
}

func TestFetchEmptyBoard(t *testing.T) {
	// A board with no pins is a real (if dull) feed, not an HTML shell: the
	// channel title proves the document parsed, so it must not be a 504.
	c := newTestClient(t)
	res, err := c.Fetch(context.Background(), "empty", "", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Pins) != 0 || res.Title != "Empty" {
		t.Fatalf("result = %+v", res)
	}
}

func TestFetchRejectsHTMLShell(t *testing.T) {
	c := newTestClient(t)
	if _, err := c.Fetch(context.Background(), "shell", "", 5); !errors.Is(err, ErrUnexpectedPage) {
		t.Fatalf("html shell = %v", err)
	}
}

func TestNormalizeUser(t *testing.T) {
	for _, raw := range []string{"nasa", "@nasa", "https://www.pinterest.com/nasa/", "pinterest.com/nasa", " www.pinterest.com/nasa "} {
		got, err := NormalizeUser(raw)
		if err != nil || got != "nasa" {
			t.Fatalf("NormalizeUser(%q) = %q, %v", raw, got, err)
		}
	}
	for _, raw := range []string{"", "na sa", "nasa/earth", "../etc", "a?b"} {
		if _, err := NormalizeUser(raw); err == nil {
			t.Fatalf("NormalizeUser(%q) must fail", raw)
		}
	}
}

func TestNormalizeBoard(t *testing.T) {
	cases := map[string]string{
		"earth":      "earth",
		"@earth":     "earth",
		"earth-2026": "earth-2026",
		"earth_2.0":  "earth_2.0",
	}
	for raw, want := range cases {
		if got, err := NormalizeBoard(raw); err != nil || got != want {
			t.Fatalf("NormalizeBoard(%q) = %q, %v", raw, got, err)
		}
	}
	for _, raw := range []string{"", "nasa/earth", "a b"} {
		if _, err := NormalizeBoard(raw); err == nil {
			t.Fatalf("NormalizeBoard(%q) must fail", raw)
		}
	}
}

func TestHiResThumb(t *testing.T) {
	cases := map[string]string{
		"https://i.pinimg.com/236x/aa/bb/cc.jpg":      "https://i.pinimg.com/736x/aa/bb/cc.jpg",
		"https://i.pinimg.com/736x/aa/bb/cc.jpg":      "https://i.pinimg.com/736x/aa/bb/cc.jpg",
		"https://i.pinimg.com/originals/aa/bb/cc.jpg": "https://i.pinimg.com/originals/aa/bb/cc.jpg",
		"https://example.com/236x/aa.jpg":             "https://example.com/236x/aa.jpg",
	}
	for in, want := range cases {
		if got := hiResThumb(in); got != want {
			t.Fatalf("hiResThumb(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCleanText(t *testing.T) {
	got := cleanText(`<a href="https://www.pinterest.com/pin/1/"><img src="https://i.pinimg.com/236x/a.jpg"></a>
		Hello &amp; welcome`)
	if got != "Hello & welcome" {
		t.Fatalf("cleanText = %q", got)
	}
}
