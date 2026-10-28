package smzdm

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
)

// fenleiPage is a slice of a real /fenlei/tixuhuli/ response (2026-10-28): the
// Nuxt payload with three good-price cards and one article card, behind the real
// shell (which includes the site's probe script - that script is not a challenge
// marker). The card list lives in the flattened Nuxt payload, so this fixture is
// exactly the bytes the parser has to handle.
const fenleiPage = `<!DOCTYPE html>
<html ><script type="text/javascript" src="/C2WF946J0/probev3.js" r='m'></script>
<head><meta charset="utf-8">
<title>shaving category page</title>
<link rel="canonical" href="https://www.smzdm.com/fenlei/tixuhuli/">
</head>
<body><div class="feed-page-container"><div class="changwen-card-container"><a class="title-normal-box" href="https://post.smzdm.com/p/apv84z2x/">an article card</a></div></div>
<script type="application/json" id="__NUXT_DATA__">[[[1,7,13,19]],{"article_type":2,"article_url":3,"focus_pic_url":4,"subtitle":5,"title":6},"faxian","https://www.smzdm.com/p/184919040/","//y.zdmimg.com/202610/28/6ae18587532d59814.jpg_a100.jpg","7.9元（需用券）","吉列 威锋经典手动剃须刀 1刀架3刀头",{"article_type":8,"article_url":9,"focus_pic_url":10,"subtitle":11,"title":12},"youhui","https://www.smzdm.com/p/184908856/","//qny.smzdm.com/202107/20/60f6457cd8e757094.jpg_a100.jpg","86.44元（淘金币78.64元）","淘金币可用、88VIP：吉列 锋隐手动剃须刀5层刀片8只装",{"article_type":14,"article_url":15,"focus_pic_url":16,"subtitle":17,"title":18},"faxian","https://www.smzdm.com/p/184883568/","//y.zdmimg.com/202610/27/6ae0ca0bbe7741009.jpg_a100.jpg","7.12元（需买2件，共14.24元）","YingjiLi 英吉利（YingJiLi）剃须泡沫男士刮胡刀泡沫须膏刮刀泡沫手动适用润滑防刮伤 柠檬香型 240ml",{"article_type":20,"article_url":21,"article_title":22,"article_format_date":23,"article_no_format_date":24,"cell_type":25,"card_type":26},"原创","https://post.smzdm.com/p/apv84z2x/","手动剃须刀怎么选？吉列剃须刀总有一款适合你","22-04-22","2022-04-22 10:50:14","22001","长图文"]</script>
</body></html>`

// fenleiChallenge is the shape the site answers with when it decides the reader
// is a bot: a page that carries the captcha bootstrap.
const fenleiChallenge = `<!DOCTYPE html><html><head><title>verify</title>
<script src="https://ssl.captcha.qq.com/TCaptcha.js"></script></head>
<body><div id="TCaptcha"></div></body></html>`

// devaluePayload renders cards the way the Nuxt payload stores them: a flat
// array whose containers hold indices into it.
func devaluePayload(t *testing.T, cards ...map[string]string) string {
	t.Helper()
	nodes := []any{nil}
	add := func(v any) int {
		nodes = append(nodes, v)
		return len(nodes) - 1
	}
	root := make([]any, 0, len(cards))
	for _, card := range cards {
		obj := make(map[string]any, len(card))
		for k, v := range card {
			obj[k] = add(v)
		}
		root = append(root, add(obj))
	}
	nodes[0] = root
	out, err := json.Marshal(nodes)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return string(out)
}

// fenleiServer serves one canned body at the category path and counts requests.
func fenleiServer(t *testing.T, body string, hits *atomic.Int32) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/fenlei/") {
			http.NotFound(w, r)
			return
		}
		hits.Add(1)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newCategoryClient(base string) *Client {
	return New(Config{BaseURL: base, Timeout: 2 * time.Second, PageTimeout: 5 * time.Second})
}

func TestCategoryParsesGoodPriceCards(t *testing.T) {
	var hits atomic.Int32
	srv := fenleiServer(t, fenleiPage, &hits)
	c := newCategoryClient(srv.URL)

	cards, err := c.Category(context.Background(), "tixuhuli", 20)
	if err != nil {
		t.Fatalf("Category: %v", err)
	}
	if len(cards) != 3 {
		t.Fatalf("cards = %d, want 3 (the article card must be dropped): %+v", len(cards), cards)
	}
	want := []struct {
		id    string
		price float64
	}{
		{"184919040", 7.9},
		{"184908856", 86.44},
		{"184883568", 7.12},
	}
	for i, w := range want {
		got := cards[i]
		if got.ID != w.id {
			t.Errorf("cards[%d].ID = %q, want %q", i, got.ID, w.id)
		}
		if got.Price == nil || *got.Price != w.price {
			t.Errorf("cards[%d].Price = %v, want %v", i, got.Price, w.price)
		}
		if got.URL != "https://www.smzdm.com/p/"+w.id+"/" {
			t.Errorf("cards[%d].URL = %q", i, got.URL)
		}
		if got.Title == "" {
			t.Errorf("cards[%d].Title is empty", i)
		}
		if !strings.HasPrefix(got.Image, "https://") {
			t.Errorf("cards[%d].Image = %q, want an absolute https URL", i, got.Image)
		}
		if got.PriceText == "" {
			t.Errorf("cards[%d].PriceText is empty", i)
		}
	}
	for _, got := range cards {
		if strings.Contains(got.URL, "post.smzdm.com") {
			t.Fatalf("article card leaked into the list: %+v", got)
		}
	}
	if hits.Load() != 1 {
		t.Fatalf("upstream requests = %d, want 1", hits.Load())
	}
}

func TestCategoryClampsLimit(t *testing.T) {
	var hits atomic.Int32
	srv := fenleiServer(t, fenleiPage, &hits)
	c := newCategoryClient(srv.URL)

	cards, err := c.Category(context.Background(), "tixuhuli", 1)
	if err != nil {
		t.Fatalf("Category: %v", err)
	}
	if len(cards) != 1 {
		t.Fatalf("limit 1 returned %d cards", len(cards))
	}
	if cards, err = c.Category(context.Background(), "tixuhuli", 0); err != nil || len(cards) != 3 {
		t.Fatalf("limit 0 returned %d cards, err %v; want every card", len(cards), err)
	}
}

func TestCategoryRejectsInvalidSlug(t *testing.T) {
	cases := map[string]string{
		"too short":        "a",
		"path traversal":   "tixuhuli/../x",
		"query injection":  "tixuhuli?x=1",
		"empty":            "",
		"robots disallow":  "3cjiadian",
		"robots disallow2": "shishangyundong",
	}
	var hits atomic.Int32
	srv := fenleiServer(t, fenleiPage, &hits)
	c := newCategoryClient(srv.URL)
	for name, slug := range cases {
		_, err := c.Category(context.Background(), slug, 5)
		var invalid *InvalidSlugError
		if !errors.As(err, &invalid) {
			t.Errorf("%s: %q error = %v, want InvalidSlugError", name, slug, err)
		}
	}
	if hits.Load() != 0 {
		t.Fatalf("invalid slugs reached upstream %d times", hits.Load())
	}
}

// TestCategoryNormalisesSlugCase keeps the URL shape stable: slugs are lower
// case upstream, and a caller's capitalisation must not become a second cache
// key or a differently cased path.
func TestCategoryNormalisesSlugCase(t *testing.T) {
	var hits atomic.Int32
	srv := fenleiServer(t, fenleiPage, &hits)
	c := newCategoryClient(srv.URL)

	if _, err := c.Category(context.Background(), " TixuHuli ", 5); err != nil {
		t.Fatalf("Category: %v", err)
	}
	if hits.Load() != 1 {
		t.Fatalf("upstream requests = %d, want 1", hits.Load())
	}
}

func TestCategoryChallengeTripsTheSlugBreaker(t *testing.T) {
	var hits atomic.Int32
	srv := fenleiServer(t, fenleiChallenge, &hits)
	c := newCategoryClient(srv.URL)

	_, err := c.Category(context.Background(), "tixuhuli", 5)
	var blocked *BlockedError
	if !errors.As(err, &blocked) {
		t.Fatalf("first call error = %v, want BlockedError", err)
	}
	if blocked.Until.IsZero() {
		t.Fatal("BlockedError carries no cooldown end")
	}

	_, err = c.Category(context.Background(), "tixuhuli", 5)
	var cooling *CooldownError
	if !errors.As(err, &cooling) {
		t.Fatalf("second call error = %v, want CooldownError", err)
	}
	if hits.Load() != 1 {
		t.Fatalf("upstream requests = %d, want 1: a blocked slug must not be retried", hits.Load())
	}

	// The cooldown is per slug: another category is still readable.
	if _, err := c.Category(context.Background(), "tixuchumao", 5); !errors.As(err, &blocked) {
		t.Fatalf("second slug error = %v, want its own BlockedError", err)
	}
	if hits.Load() != 2 {
		t.Fatalf("upstream requests = %d, want 2", hits.Load())
	}
}

func TestCategoryEmptyShellIsBlocked(t *testing.T) {
	var hits atomic.Int32
	srv := fenleiServer(t, "<html><body>nothing here</body></html>", &hits)
	c := newCategoryClient(srv.URL)

	_, err := c.Category(context.Background(), "tixuhuli", 5)
	var blocked *BlockedError
	if !errors.As(err, &blocked) {
		t.Fatalf("error = %v, want BlockedError", err)
	}
	if !strings.Contains(blocked.Reason, "empty shell") {
		t.Fatalf("reason = %q, want an empty-shell reason", blocked.Reason)
	}
}

func TestCategoryWithoutCardsIsNotABlock(t *testing.T) {
	// A large page without a payload is a layout change, not a challenge: it
	// must not put the slug on a six-hour cooldown.
	body := "<html><body>" + strings.Repeat("<div>no payload here</div>", 2000) + "</body></html>"
	var hits atomic.Int32
	srv := fenleiServer(t, body, &hits)
	c := newCategoryClient(srv.URL)

	for i := 0; i < 2; i++ {
		_, err := c.Category(context.Background(), "tixuhuli", 5)
		if err == nil {
			t.Fatalf("call %d: expected an error", i)
		}
		var blocked *BlockedError
		if errors.As(err, &blocked) {
			t.Fatalf("call %d: a layout change must not be treated as a challenge: %v", i, err)
		}
	}
	if hits.Load() != 2 {
		t.Fatalf("upstream requests = %d, want 2 (no cooldown)", hits.Load())
	}
}

func TestCategoryReadsCardDateWhenThePayloadCarriesOne(t *testing.T) {
	payload := devaluePayload(t,
		map[string]string{
			"article_type":           "faxian",
			"title":                  "card with a date",
			"article_url":            "https://www.smzdm.com/p/184919040/",
			"subtitle":               "7.9",
			"article_no_format_date": "2026-10-27 09:18:05",
		},
		map[string]string{
			"article_type":        "youhui",
			"title":               "card with a short date",
			"article_url":         "https://www.smzdm.com/p/184908856/",
			"subtitle":            "86.44",
			"article_format_date": "01-02 08:30",
		},
	)
	var hits atomic.Int32
	srv := fenleiServer(t, `<html><script type="application/json" id="__NUXT_DATA__">`+payload+`</script></html>`, &hits)
	c := newCategoryClient(srv.URL)
	c.now = func() time.Time { return time.Date(2026, 1, 3, 12, 0, 0, 0, beijing) }

	cards, err := c.Category(context.Background(), "tixuhuli", 5)
	if err != nil {
		t.Fatalf("Category: %v", err)
	}
	if len(cards) != 2 {
		t.Fatalf("cards = %d, want 2", len(cards))
	}
	// Newest first: the full timestamp (2026-10-27) leads the cross-year short
	// one (2026-01-02, fetched 2026-01-03).
	if cards[0].ID != "184919040" || cards[1].ID != "184908856" {
		t.Fatalf("order = %s, %s", cards[0].ID, cards[1].ID)
	}
	if got := cards[0].Published.UTC(); !got.Equal(time.Date(2026, 10, 27, 1, 18, 5, 0, time.UTC)) {
		t.Fatalf("full date = %v", got)
	}
	if got := cards[1].Published.UTC(); !got.Equal(time.Date(2026, 1, 2, 0, 30, 0, 0, time.UTC)) {
		t.Fatalf("short date = %v", got)
	}
}

func TestParseCardDate(t *testing.T) {
	beijingNoon := time.Date(2026, 1, 3, 12, 0, 0, 0, beijing)
	cases := []struct {
		name  string
		full  string
		short string
		now   time.Time
		want  time.Time
	}{
		{"full stamp", "2026-10-27 09:18:05", "", beijingNoon, time.Date(2026, 10, 27, 9, 18, 5, 0, beijing)},
		{"same year", "", "10-27 09:18", time.Date(2026, 10, 28, 12, 0, 0, 0, beijing), time.Date(2026, 10, 27, 9, 18, 0, 0, beijing)},
		{"December seen in January", "", "12-31 23:50", beijingNoon, time.Date(2025, 12, 31, 23, 50, 0, 0, beijing)},
		{"year inside the short stamp", "", "22-11-21", beijingNoon, time.Date(2022, 11, 21, 0, 0, 0, 0, beijing)},
		{"garbage", "", "yesterday", beijingNoon, time.Time{}},
		{"empty", "", "", beijingNoon, time.Time{}},
	}
	for _, tc := range cases {
		got := parseCardDate(tc.full, tc.short, tc.now)
		if !got.Equal(tc.want) {
			t.Errorf("%s: parseCardDate = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestParsePrice(t *testing.T) {
	cases := map[string]float64{
		"7.9元":                7.9,
		"7.9元（需用券）":           7.9,
		"1,299元":              1299,
		"86.44元（淘金币78.64元）":   86.44,
		"7.12元（需买2件，共14.24元）": 7.12,
	}
	for text, want := range cases {
		got, ok := parsePrice(text)
		if !ok || got == nil || *got != want {
			t.Errorf("parsePrice(%q) = %v, %v; want %v", text, got, ok, want)
		}
	}
	for _, text := range []string{"", "券后好价", "元"} {
		if _, ok := parsePrice(text); ok {
			t.Errorf("parsePrice(%q) accepted", text)
		}
	}
}
