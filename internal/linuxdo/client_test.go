package linuxdo

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestRefValidate(t *testing.T) {
	good := []struct {
		in   Ref
		want string
	}{
		{Ref{Kind: KindLatest}, "latest:created"},
		{Ref{Kind: KindLatest, Order: "activity"}, "latest:activity"},
		{Ref{Kind: KindTop}, "top:daily"},
		{Ref{Kind: KindTop, Period: "weekly"}, "top:weekly"},
		{Ref{Kind: KindHot}, "hot"},
		{Ref{Kind: KindCategory, Slug: "develop", ID: "4"}, "category:develop:4"},
		{Ref{Kind: KindCategory, ID: "4"}, "category:-:4"},
		{Ref{Kind: KindTag, Tag: "ChatGPT"}, "tag:chatgpt"},
	}
	for _, tc := range good {
		got, err := tc.in.Validate()
		if err != nil {
			t.Errorf("Validate(%+v) error = %v", tc.in, err)
			continue
		}
		if key := got.Key(); key != tc.want {
			t.Errorf("Validate(%+v).Key() = %q, want %q", tc.in, key, tc.want)
		}
	}
	bad := []Ref{
		{Kind: "nope"},
		{Kind: KindTop, Period: "hourly"},
		{Kind: KindLatest, Order: "newest"},
		{Kind: KindCategory},
		{Kind: KindCategory, Slug: "../x", ID: "4"},
		{Kind: KindCategory, ID: "abc"},
		{Kind: KindTag, Tag: ""},
		{Kind: KindTag, Tag: "bad tag"},
	}
	for _, ref := range bad {
		if _, err := ref.Validate(); !errors.Is(err, ErrInvalidRef) {
			t.Errorf("Validate(%+v) error = %v, want ErrInvalidRef", ref, err)
		}
	}
}

func TestParseLatest(t *testing.T) {
	f, err := parseFeed(KindLatest, []byte(latestFixture))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(f.Channel, "LINUX DO") {
		t.Errorf("channel = %q", f.Channel)
	}
	if f.Language != "zh-CN" {
		t.Errorf("language = %q", f.Language)
	}
	if !strings.HasPrefix(f.Link, "https://linux.do/latest") {
		t.Errorf("channel link = %q", f.Link)
	}
	if len(f.Topics) != 3 {
		t.Fatalf("topics = %d, want 3", len(f.Topics))
	}
	first := f.Topics[0]
	if first.ID == "" || first.Title == "" || first.Author == "" || first.Category == "" {
		t.Fatalf("incomplete topic: %+v", first)
	}
	if !strings.HasPrefix(first.URL, "https://linux.do/t/") {
		t.Errorf("url = %q", first.URL)
	}
	if first.CreatedAt.IsZero() {
		t.Error("created_at is zero")
	}
	if first.HTML == "" {
		t.Error("body is empty")
	}
}

func TestParseTopPreservesRanking(t *testing.T) {
	f, err := parseFeed(KindTop, []byte(topFixture))
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Topics) != 3 {
		t.Fatalf("topics = %d", len(f.Topics))
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(topFixture))
	}))
	defer srv.Close()
	c := New(Config{BaseURL: srv.URL, Timeout: 2 * time.Second, MinInterval: time.Millisecond})
	got, err := c.Fetch(context.Background(), Ref{Kind: KindTop, Period: "daily"}, 3)
	if err != nil {
		t.Fatal(err)
	}
	for i := range got.Topics {
		if got.Topics[i].ID != f.Topics[i].ID {
			t.Fatalf("top topic %d = %s, want ranking order %s", i, got.Topics[i].ID, f.Topics[i].ID)
		}
	}
}

func makeRSS(page, count int) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?><rss version="2.0" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:discourse="http://www.discourse.org/"><channel><title>LINUX DO - test</title><link>https://linux.do/latest</link><language>zh-CN</language>`)
	for i := 0; i < count; i++ {
		id := page*1000 + i
		at := time.Date(2026, 11, 17, 8, 0, 0, 0, time.UTC).Add(time.Duration(id) * time.Second)
		fmt.Fprintf(&b, `<item><title>Topic %d</title><dc:creator>user</dc:creator><category>dev</category><description><![CDATA[<p>body %d</p>]]></description><link>https://linux.do/t/topic/%d</link><pubDate>%s</pubDate><discourse:topicPinned>No</discourse:topicPinned><guid isPermaLink="false">linux.do-topic-%d</guid></item>`,
			id, id, id, at.Format(time.RFC1123Z), id)
	}
	b.WriteString(`</channel></rss>`)
	return b.String()
}

func TestFetchPaginates(t *testing.T) {
	var pages []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		if page == "" {
			page = "1"
		}
		pages = append(pages, page)
		p, _ := strconv.Atoi(page)
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(makeRSS(p, 30)))
	}))
	defer srv.Close()
	c := New(Config{BaseURL: srv.URL, Timeout: 2 * time.Second, MinInterval: time.Millisecond})
	f, err := c.Fetch(context.Background(), Ref{Kind: KindLatest}, 90)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Topics) != 90 {
		t.Fatalf("topics = %d, want 90", len(f.Topics))
	}
	if strings.Join(pages, ",") != "1,2,3" {
		t.Fatalf("pages = %v, want 1,2,3", pages)
	}
}

func TestFetchStopsOnShortPage(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		n := 30
		if r.URL.Query().Get("page") == "2" {
			n = 5
		}
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(makeRSS(calls, n)))
	}))
	defer srv.Close()
	c := New(Config{BaseURL: srv.URL, Timeout: 2 * time.Second, MinInterval: time.Millisecond})
	f, err := c.Fetch(context.Background(), Ref{Kind: KindLatest}, 60)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Topics) != 35 {
		t.Fatalf("topics = %d, want 35", len(f.Topics))
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
}

func TestChallengeSetsGlobalCooldown(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("cf-mitigated", "challenge")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`<html><title>Just a moment...</title></html>`))
	}))
	defer srv.Close()
	c := New(Config{BaseURL: srv.URL, Timeout: 2 * time.Second, MinInterval: time.Millisecond, Cooldown: time.Minute})
	if _, err := c.Fetch(context.Background(), Ref{Kind: KindLatest}, 10); !errors.Is(err, ErrChallenge) {
		t.Fatalf("first error = %v, want ErrChallenge", err)
	}
	if _, err := c.Fetch(context.Background(), Ref{Kind: KindLatest}, 10); !errors.Is(err, ErrChallenge) {
		t.Fatalf("second error = %v, want ErrChallenge", err)
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1 (cooldown must answer without a request)", calls)
	}
}

func TestHTTPErrors(t *testing.T) {
	cases := []struct {
		name     string
		status   int
		ctype    string
		body     string
		want     error
		cooldown bool
	}{
		{"not found", http.StatusNotFound, "text/html", "<html>not found</html>", ErrNotFound, false},
		{"html login", http.StatusOK, "text/html", "<html>login</html>", ErrUnexpectedPage, false},
		{"challenge page", http.StatusOK, "text/html", "<html>Just a moment...</html>", ErrChallenge, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", tc.ctype)
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			c := New(Config{BaseURL: srv.URL, Timeout: 2 * time.Second, MinInterval: time.Millisecond})
			if _, err := c.Fetch(context.Background(), Ref{Kind: KindLatest}, 10); !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
			if tc.cooldown {
				c.mu.Lock()
				active := time.Now().Before(c.cooldownUntil)
				c.mu.Unlock()
				if !active {
					t.Fatal("challenge did not open the cooldown window")
				}
			}
		})
	}
}

func TestPageURL(t *testing.T) {
	c := New(Config{BaseURL: DefaultBaseURL, Timeout: time.Second})
	cases := []struct {
		ref  Ref
		page int
		want string
	}{
		{Ref{Kind: KindLatest, Order: "created"}, 1, "https://linux.do/latest.rss?order=created"},
		{Ref{Kind: KindLatest, Order: "activity"}, 2, "https://linux.do/latest.rss?order=activity&page=2"},
		{Ref{Kind: KindTop, Period: "daily"}, 1, "https://linux.do/top.rss?period=daily"},
		{Ref{Kind: KindHot}, 2, "https://linux.do/hot.rss?page=2"},
		{Ref{Kind: KindCategory, Slug: "develop", ID: "4"}, 1, "https://linux.do/c/develop/4.rss"},
		{Ref{Kind: KindCategory, ID: "4"}, 1, "https://linux.do/c/-/4.rss"},
		{Ref{Kind: KindTag, Tag: "chatgpt"}, 1, "https://linux.do/tag/chatgpt.rss"},
	}
	for _, tc := range cases {
		got := c.pageURL(tc.ref, tc.page)
		if got != tc.want {
			t.Errorf("pageURL(%+v, %d) = %q, want %q", tc.ref, tc.page, got, tc.want)
		}
	}
}

func TestFeedImages(t *testing.T) {
	body := `<p><img src="https://cdn3.ldstatic.com/a.jpg"><img src="https://cdn3.ldstatic.com/a.jpg"><img src="https://cdn.ldstatic.com/images/emoji/twemoji/x.png"><img src="//cdn3.ldstatic.com/b.png"></p>`
	got := feedImages(body)
	if len(got) != 2 || got[0] != "https://cdn3.ldstatic.com/a.jpg" || got[1] != "https://cdn3.ldstatic.com/b.png" {
		t.Fatalf("images = %v", got)
	}
}

func TestTopicID(t *testing.T) {
	cases := map[string]string{
		"https://linux.do/t/topic/3191226":              "3191226",
		"https://linux.do/t/slug/3191226?page=2#post_3": "3191226",
		"https://linux.do/t/topic/abc":                  "",
		"":                                              "",
	}
	for in, want := range cases {
		if got := topicID(in); got != want {
			t.Errorf("topicID(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLatestActivityKeepsUpstreamOrder(t *testing.T) {
	page := `<?xml version="1.0" encoding="UTF-8"?><rss version="2.0" xmlns:dc="http://purl.org/dc/elements/1.1/"><channel><title>t</title><link>https://linux.do/latest</link><language>zh-CN</language>
<item><title>A</title><dc:creator>u</dc:creator><category>c</category><description><![CDATA[<p>a</p>]]></description><link>https://linux.do/t/topic/1</link><pubDate>Tue, 17 Nov 2026 10:00:00 +0000</pubDate></item>
<item><title>B</title><dc:creator>u</dc:creator><category>c</category><description><![CDATA[<p>b</p>]]></description><link>https://linux.do/t/topic/2</link><pubDate>Tue, 17 Nov 2026 09:00:00 +0000</pubDate></item>
<item><title>C</title><dc:creator>u</dc:creator><category>c</category><description><![CDATA[<p>c</p>]]></description><link>https://linux.do/t/topic/3</link><pubDate>Tue, 17 Nov 2026 11:00:00 +0000</pubDate></item>
</channel></rss>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(page))
	}))
	defer srv.Close()
	c := New(Config{BaseURL: srv.URL, Timeout: 2 * time.Second, MinInterval: time.Millisecond})

	activity, err := c.Fetch(context.Background(), Ref{Kind: KindLatest, Order: "activity"}, 3)
	if err != nil {
		t.Fatal(err)
	}
	if got := activity.Topics[0].ID; got != "1" {
		t.Fatalf("activity first id = %s, want 1 (upstream order)", got)
	}
	created, err := c.Fetch(context.Background(), Ref{Kind: KindLatest, Order: "created"}, 3)
	if err != nil {
		t.Fatal(err)
	}
	if got := created.Topics[0].ID; got != "3" {
		t.Fatalf("created first id = %s, want 3 (newest publication)", got)
	}
}
