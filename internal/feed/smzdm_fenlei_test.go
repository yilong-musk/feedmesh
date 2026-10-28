package feed

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yilong-musk/feedmesh/internal/cache"
	"github.com/yilong-musk/feedmesh/internal/logx"
	"github.com/yilong-musk/feedmesh/internal/smzdm"
	"github.com/yilong-musk/feedmesh/internal/weibo"
)

// fenleiCard describes one card for the page builder below.
type fenleiCard struct {
	id       string
	title    string
	price    string
	kind     string
	host     string
	focusPic string
}

// buildFenleiPage renders a category page the way the site stores it: the card
// list goes into the flattened Nuxt payload, containers hold indices into it.
func buildFenleiPage(t *testing.T, cards ...fenleiCard) string {
	t.Helper()
	nodes := []any{nil}
	add := func(v any) int {
		nodes = append(nodes, v)
		return len(nodes) - 1
	}
	root := make([]any, 0, len(cards))
	for i, c := range cards {
		kind, host := c.kind, c.host
		if kind == "" {
			kind = "faxian"
		}
		if host == "" {
			host = "https://www.smzdm.com/p/" + c.id + "/"
		}
		obj := map[string]any{
			"article_type":   add(kind),
			"article_url":    add(host),
			"focus_pic_url":  add(c.focusPic),
			"subtitle":       add(c.price),
			"title":          add(c.title),
			"article_int_id": add(i + 1),
		}
		root = append(root, add(obj))
	}
	nodes[0] = root
	payload, err := json.Marshal(nodes)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return `<!DOCTYPE html><html><head><title>category</title></head><body>` +
		`<script type="application/json" id="__NUXT_DATA__">` + string(payload) + `</script>` +
		`</body></html>`
}

func fenleiCards() []fenleiCard {
	return []fenleiCard{
		{id: "184919040", title: "Gillette classic razor", price: "7.9 CNY (coupon)", focusPic: "//y.zdmimg.com/a.jpg_a100.jpg"},
		{id: "184908856", title: "Gillette 5-blade 8 pack", price: "86.44 CNY", focusPic: "//qny.smzdm.com/b.jpg_a100.jpg"},
		{id: "184883568", title: "Shaving foam 240ml", price: "7.12 CNY (buy 2)", focusPic: "//y.zdmimg.com/c.jpg_a100.jpg"},
	}
}

// fenleiUpstream serves a category page and counts the reads.
func fenleiUpstream(t *testing.T, body string, hits *atomic.Int32) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/fenlei/") {
			http.NotFound(w, r)
			return
		}
		hits.Add(1)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// fenleiService wires a Service against one fake upstream, with its own cache.
func fenleiService(t *testing.T, base string, now func() time.Time) *Service {
	t.Helper()
	c, err := cache.New(t.TempDir())
	if err != nil {
		t.Fatalf("cache: %v", err)
	}
	client := smzdm.New(smzdm.Config{BaseURL: base, Timeout: 2 * time.Second, PageTimeout: 5 * time.Second})
	s := NewService(Clients{Smzdm: client}, c, logx.Discard(), DefaultTTLs())
	if now != nil {
		s.now = now
	}
	return s
}

func TestGetSmzdmFenleiReturnsCards(t *testing.T) {
	var hits atomic.Int32
	srv := fenleiUpstream(t, buildFenleiPage(t, fenleiCards()...), &hits)
	s := fenleiService(t, srv.URL, nil)

	f, err := s.GetSmzdmFenlei(context.Background(), "tixuhuli", 20, false)
	if err != nil {
		t.Fatalf("GetSmzdmFenlei: %v", err)
	}
	if f.Cached {
		t.Fatal("first call must not report a cache hit")
	}
	if len(f.Items) != 3 {
		t.Fatalf("items = %d, want 3", len(f.Items))
	}
	if f.TTL != 60 {
		t.Fatalf("ttl = %d minutes, want 60", f.TTL)
	}
	if got := f.Title(); got != "SMZDM Deals: tixuhuli" {
		t.Fatalf("title = %q", got)
	}
	for i, it := range f.Items {
		if it.ID == "" || it.Title == "" || it.URL == "" {
			t.Fatalf("items[%d] is missing an identity: %+v", i, it)
		}
		if it.Price == nil {
			t.Fatalf("items[%d] has no price: %+v", i, it)
		}
		if it.Slug != "tixuhuli" {
			t.Fatalf("items[%d].Slug = %q", i, it.Slug)
		}
		if it.CreatedAt.IsZero() || it.CreatedAt.Location() != time.UTC {
			t.Fatalf("items[%d].CreatedAt = %v, want a UTC timestamp", i, it.CreatedAt)
		}
		if !strings.HasPrefix(it.Images[0], "https://") {
			t.Fatalf("items[%d].Images = %v", i, it.Images)
		}
	}
	if hits.Load() != 1 {
		t.Fatalf("upstream reads = %d, want 1", hits.Load())
	}
}

func TestGetSmzdmFenleiDropsArticleCards(t *testing.T) {
	body := buildFenleiPage(t,
		fenleiCard{id: "184919040", title: "deal", price: "7.9 CNY"},
		fenleiCard{
			id: "apv84z2x", kind: "原创", title: "an article",
			host:  "https://post.smzdm.com/p/apv84z2x/",
			price: "a blurb, no price",
		},
		fenleiCard{
			id: "184700000", title: "card without a price", price: "",
		},
	)
	var hits atomic.Int32
	srv := fenleiUpstream(t, body, &hits)
	s := fenleiService(t, srv.URL, nil)

	f, err := s.GetSmzdmFenlei(context.Background(), "tixuhuli", 20, false)
	if err != nil {
		t.Fatalf("GetSmzdmFenlei: %v", err)
	}
	if len(f.Items) != 1 || f.Items[0].ID != "184919040" {
		t.Fatalf("items = %+v, want only the priced deal", f.Items)
	}
}

func TestGetSmzdmFenleiReadsThePageOnceAnHour(t *testing.T) {
	var hits atomic.Int32
	srv := fenleiUpstream(t, buildFenleiPage(t, fenleiCards()...), &hits)
	s := fenleiService(t, srv.URL, nil)

	for i := 0; i < 3; i++ {
		f, err := s.GetSmzdmFenlei(context.Background(), "tixuhuli", 20, false)
		if err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
		if i > 0 && !f.Cached {
			t.Fatalf("call %d was not served from the cache", i)
		}
		if len(f.Items) != 3 {
			t.Fatalf("call %d: items = %d, want 3", i, len(f.Items))
		}
	}
	if hits.Load() != 1 {
		t.Fatalf("upstream reads = %d, want 1 for any number of reads inside the hour", hits.Load())
	}
}

func TestGetSmzdmFenleiWiderLimitDoesNotReadAgain(t *testing.T) {
	var hits atomic.Int32
	srv := fenleiUpstream(t, buildFenleiPage(t, fenleiCards()...), &hits)
	s := fenleiService(t, srv.URL, nil)

	if _, err := s.GetSmzdmFenlei(context.Background(), "tixuhuli", 1, false); err != nil {
		t.Fatalf("narrow read: %v", err)
	}
	f, err := s.GetSmzdmFenlei(context.Background(), "tixuhuli", 30, false)
	if err != nil {
		t.Fatalf("wide read: %v", err)
	}
	if len(f.Items) != 3 {
		t.Fatalf("wide read returned %d items, want 3", len(f.Items))
	}
	if !f.Cached {
		t.Fatal("wide read must be served from the cache the narrow read filled")
	}
	if hits.Load() != 1 {
		t.Fatalf("upstream reads = %d, want 1: the page is always read at full width", hits.Load())
	}
}

func TestGetSmzdmFenleiRefreshRespectsTheHourlyBudget(t *testing.T) {
	var hits atomic.Int32
	srv := fenleiUpstream(t, buildFenleiPage(t, fenleiCards()...), &hits)
	// The cache stamps entries with the real clock, so the service clock has to
	// start there as well; only the advance below is simulated.
	now := time.Now().UTC()
	s := fenleiService(t, srv.URL, func() time.Time { return now })

	if _, err := s.GetSmzdmFenlei(context.Background(), "tixuhuli", 20, false); err != nil {
		t.Fatalf("first read: %v", err)
	}

	// refresh inside the hour is served from the cache: the budget is spent.
	f, err := s.GetSmzdmFenlei(context.Background(), "tixuhuli", 20, true)
	if err != nil {
		t.Fatalf("refresh inside the hour: %v", err)
	}
	if !f.Cached {
		t.Fatal("refresh inside the hour must be served from the cache")
	}
	if hits.Load() != 1 {
		t.Fatalf("upstream reads = %d after a suppressed refresh, want 1", hits.Load())
	}

	// Past the hour the refresh reads upstream again.
	now = now.Add(smzdmFenleiTTL + time.Minute)
	if _, err := s.GetSmzdmFenlei(context.Background(), "tixuhuli", 20, true); err != nil {
		t.Fatalf("refresh after the hour: %v", err)
	}
	if hits.Load() != 2 {
		t.Fatalf("upstream reads = %d, want 2", hits.Load())
	}
}

// TestGetSmzdmFenleiCacheHitKeepsTheReadTime pins the read-time rule: the cards
// carry no timestamp of their own, so created_at is the page read time. A cache
// hit must report that instant, not the moment the request arrived — otherwise
// every poll re-stamps the same batch as new.
func TestGetSmzdmFenleiCacheHitKeepsTheReadTime(t *testing.T) {
	var hits atomic.Int32
	srv := fenleiUpstream(t, buildFenleiPage(t, fenleiCards()...), &hits)
	clock := time.Now().UTC()
	s := fenleiService(t, srv.URL, func() time.Time { return clock })

	cold, err := s.GetSmzdmFenlei(context.Background(), "tixuhuli", 20, false)
	if err != nil {
		t.Fatalf("cold read: %v", err)
	}
	readAt := cold.FetchedAt
	if readAt.IsZero() || !readAt.Equal(clock) {
		t.Fatalf("cold fetched_at = %v, want the read time %v", readAt, clock)
	}
	for i, it := range cold.Items {
		if !it.CreatedAt.Equal(readAt) {
			t.Fatalf("cold items[%d].created_at = %v, want %v", i, it.CreatedAt, readAt)
		}
		if it.DateSource != "fetched" {
			t.Fatalf("cold items[%d].date_source = %q, want \"fetched\"", i, it.DateSource)
		}
	}

	// Well inside the hour: the list comes from the cache and keeps its time.
	clock = clock.Add(5 * time.Minute)
	warm, err := s.GetSmzdmFenlei(context.Background(), "tixuhuli", 20, false)
	if err != nil {
		t.Fatalf("cached read: %v", err)
	}
	if !warm.Cached {
		t.Fatal("second read was not served from the cache")
	}
	if !warm.FetchedAt.Equal(readAt) {
		t.Fatalf("cached fetched_at = %v, want the read time %v", warm.FetchedAt, readAt)
	}
	if len(warm.Items) != len(cold.Items) {
		t.Fatalf("cached items = %d, want %d", len(warm.Items), len(cold.Items))
	}
	for i, it := range warm.Items {
		if !it.CreatedAt.Equal(readAt) {
			t.Fatalf("cached items[%d].created_at = %v, want the read time %v", i, it.CreatedAt, readAt)
		}
	}
	if hits.Load() != 1 {
		t.Fatalf("upstream reads = %d, want 1", hits.Load())
	}
}

// TestGetSmzdmFenleiFailureSpendsTheHourlyBudget pins the other half of the
// budget: a failed read costs one upstream request, and the rest of the window
// answers with the recorded failure instead of re-reading the page on every
// poll.
func TestGetSmzdmFenleiFailureSpendsTheHourlyBudget(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		http.Error(w, "upstream exploded", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	clock := time.Now().UTC()
	s := fenleiService(t, srv.URL, func() time.Time { return clock })

	_, err := s.GetSmzdmFenlei(context.Background(), "tixuhuli", 20, false)
	if got := weibo.CodeOf(err); got != weibo.CodeUpstreamUnavailable {
		t.Fatalf("first read code = %q (%v), want %q", got, err, weibo.CodeUpstreamUnavailable)
	}
	if hits.Load() != 1 {
		t.Fatalf("upstream reads = %d, want 1", hits.Load())
	}

	clock = clock.Add(2 * time.Minute)
	_, err = s.GetSmzdmFenlei(context.Background(), "tixuhuli", 20, false)
	if got := weibo.CodeOf(err); got != weibo.CodeUpstreamUnavailable {
		t.Fatalf("suppressed read code = %q (%v), want %q", got, err, weibo.CodeUpstreamUnavailable)
	}
	if !strings.Contains(err.Error(), "is not re-read until") {
		t.Fatalf("suppressed read error = %q, want the retry time", err)
	}
	if hits.Load() != 1 {
		t.Fatalf("upstream reads = %d, want 1: a failed read spends the hour", hits.Load())
	}

	// Past the window the page is read again — the marker is not a six-hour
	// cooldown, it is exactly the budget.
	clock = clock.Add(smzdmFenleiTTL)
	if _, err := s.GetSmzdmFenlei(context.Background(), "tixuhuli", 20, false); err == nil {
		t.Fatal("read after the window: want the upstream error to repeat")
	}
	if hits.Load() != 2 {
		t.Fatalf("upstream reads = %d, want 2 after the window", hits.Load())
	}
}

func TestGetSmzdmFenleiChallengeIsAnErrorAndCoolsTheSlugDown(t *testing.T) {
	var hits atomic.Int32
	body := `<html><body><script src="https://ssl.captcha.qq.com/TCaptcha.js"></script></body></html>`
	srv := fenleiUpstream(t, body, &hits)
	s := fenleiService(t, srv.URL, nil)

	_, err := s.GetSmzdmFenlei(context.Background(), "tixuhuli", 20, false)
	if got := weibo.CodeOf(err); got != weibo.CodeUpstreamBlocked {
		t.Fatalf("first call code = %q (%v), want %q", got, err, weibo.CodeUpstreamBlocked)
	}
	_, err = s.GetSmzdmFenlei(context.Background(), "tixuhuli", 20, false)
	if got := weibo.CodeOf(err); got != weibo.CodeBreakerOpen {
		t.Fatalf("second call code = %q (%v), want %q", got, err, weibo.CodeBreakerOpen)
	}
	if hits.Load() != 1 {
		t.Fatalf("upstream reads = %d, want 1: a blocked slug is not retried", hits.Load())
	}
}

func TestGetSmzdmFenleiRejectsBadSlug(t *testing.T) {
	s := NewService(Clients{Smzdm: smzdm.New(smzdm.Config{})}, nil, logx.Discard(), DefaultTTLs())
	for _, slug := range []string{"", "a", "../etc/passwd", "jujiashenghuo"} {
		_, err := s.GetSmzdmFenlei(context.Background(), slug, 20, false)
		if got := weibo.CodeOf(err); got != weibo.CodeBadRequest {
			t.Errorf("slug %q code = %q (%v), want %q", slug, got, err, weibo.CodeBadRequest)
		}
	}
}

func TestGetSmzdmFenleiWithoutProvider(t *testing.T) {
	s := NewService(Clients{}, nil, logx.Discard(), DefaultTTLs())
	_, err := s.GetSmzdmFenlei(context.Background(), "tixuhuli", 20, false)
	if err == nil || weibo.CodeOf(err) != weibo.CodeInternal {
		t.Fatalf("error = %v, want an internal error", err)
	}
}

// TestGetSmzdmFenleiReadsOneSlugAtATime pins the politeness rule: the upstream
// budget is per slug, but the site only tolerates one category read at a time.
func TestGetSmzdmFenleiReadsOneSlugAtATime(t *testing.T) {
	page := buildFenleiPage(t, fenleiCards()...)
	var active, peak atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/fenlei/") {
			http.NotFound(w, r)
			return
		}
		cur := active.Add(1)
		for {
			old := peak.Load()
			if cur <= old || peak.CompareAndSwap(old, cur) {
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
		active.Add(-1)
		_, _ = w.Write([]byte(page))
	}))
	t.Cleanup(srv.Close)
	s := fenleiService(t, srv.URL, nil)

	done := make(chan error, 2)
	for _, slug := range []string{"tixuhuli", "tixuchumao"} {
		go func(slug string) {
			_, err := s.GetSmzdmFenlei(context.Background(), slug, 20, false)
			done <- err
		}(slug)
	}
	for i := 0; i < 2; i++ {
		if err := <-done; err != nil {
			t.Fatalf("concurrent read: %v", err)
		}
	}
	if peak.Load() != 1 {
		t.Fatalf("peak concurrent upstream reads = %d, want 1", peak.Load())
	}
}

// TestGetSmzdmFenleiNoCardsIsUnavailable checks that a layout change (a page
// with no cards) is reported as unavailable, not as a cooldown — and that it
// spends the hour's budget like any other failed read, so a layout change
// cannot turn a polling consumer into one 600 KB read per poll.
func TestGetSmzdmFenleiNoCardsIsUnavailable(t *testing.T) {
	var hits atomic.Int32
	srv := fenleiUpstream(t, "<html><body>"+strings.Repeat("<p>no payload</p>", 2000)+"</body></html>", &hits)
	s := fenleiService(t, srv.URL, nil)

	_, err := s.GetSmzdmFenlei(context.Background(), "tixuhuli", 20, false)
	if got := weibo.CodeOf(err); got != weibo.CodeUpstreamUnavailable {
		t.Fatalf("code = %q (%v), want %q", got, err, weibo.CodeUpstreamUnavailable)
	}
	var blocked *smzdm.BlockedError
	if errors.As(err, &blocked) {
		t.Fatal("a layout change must not be classified as a challenge")
	}

	if _, err := s.GetSmzdmFenlei(context.Background(), "tixuhuli", 20, false); err == nil {
		t.Fatal("second read: want the recorded failure")
	}
	if hits.Load() != 1 {
		t.Fatalf("upstream reads = %d, want 1: a failed read spends the hour", hits.Load())
	}
}
