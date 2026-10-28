package httpapi_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

// fenleiCard is one card the fake category page carries.
type fenleiCard struct {
	ID    string
	Title string
	Price string
	Type  string
	URL   string
}

// fenleiPageHTML renders a category page in the shape the site uses: the card
// list lives in the flattened Nuxt payload, whose containers hold indices into
// the array.
func fenleiPageHTML(cards []fenleiCard) string {
	nodes := []any{nil}
	add := func(v any) int {
		nodes = append(nodes, v)
		return len(nodes) - 1
	}
	root := make([]any, 0, len(cards))
	for _, c := range cards {
		kind := c.Type
		if kind == "" {
			kind = "faxian"
		}
		link := c.URL
		if link == "" {
			link = "https://www.smzdm.com/p/" + c.ID + "/"
		}
		obj := map[string]any{
			"article_type":  add(kind),
			"article_url":   add(link),
			"focus_pic_url": add("//y.zdmimg.com/" + c.ID + ".jpg_a100.jpg"),
			"subtitle":      add(c.Price),
			"title":         add(c.Title),
		}
		root = append(root, add(obj))
	}
	nodes[0] = root
	payload, _ := json.Marshal(nodes)
	return `<!DOCTYPE html><html><head><title>category</title></head><body>` +
		`<div class="changwen-card-container">an article card</div>` +
		`<script type="application/json" id="__NUXT_DATA__">` + string(payload) + `</script>` +
		`</body></html>`
}

// fenleiTestPage is what the fake upstream serves for every category path.
var fenleiTestPage = fenleiPageHTML([]fenleiCard{
	{ID: "184919040", Title: "Gillette classic razor", Price: "7.9 CNY (coupon)"},
	{ID: "184908856", Title: "Gillette 5-blade 8 pack", Price: "86.44 CNY", Type: "youhui"},
	{ID: "apv84z2x", Title: "an article card", Price: "a blurb without a price", Type: "原创"},
})

func TestSmzdmFenleiRSSRoutes(t *testing.T) {
	h := newTestServer(t)

	for _, path := range []string{"/rss/smzdm/fenlei/tixuhuli", "/rss/smzdm/fenlei/tixuhuli.xml"} {
		rec := get(t, h, path)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d body=%s", path, rec.Code, rec.Body.String())
		}
		if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/rss+xml") {
			t.Fatalf("%s content-type = %q", path, ct)
		}
		if cc := rec.Header().Get("Cache-Control"); cc != "public, max-age=3600" {
			t.Fatalf("%s cache-control = %q, want the hourly upstream budget", path, cc)
		}
		body := rec.Body.String()
		for _, want := range []string{
			"<title>SMZDM Deals: tixuhuli</title>",
			"https://www.smzdm.com/p/184919040/",
			"7.9 CNY (coupon)",
			"<ttl>60</ttl>",
		} {
			if !strings.Contains(body, want) {
				t.Fatalf("%s missing %q:\n%s", path, want, body)
			}
		}
		if strings.Contains(body, "an article card") {
			t.Fatalf("%s carries the article card:\n%s", path, body)
		}
	}
}

func TestSmzdmFenleiAPIRoute(t *testing.T) {
	h := newTestServer(t)

	rec := get(t, h, "/api/v1/smzdm?channel=fenlei&slug=tixuhuli&limit=20")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var out struct {
		Channel   string    `json:"channel"`
		Slug      string    `json:"slug"`
		Title     string    `json:"title"`
		Cached    bool      `json:"cached"`
		FetchedAt time.Time `json:"fetched_at"`
		Items     []struct {
			ID         string    `json:"id"`
			Title      string    `json:"title"`
			Text       string    `json:"text"`
			URL        string    `json:"url"`
			HTML       string    `json:"html"`
			Images     []string  `json:"images"`
			Price      *float64  `json:"price"`
			Slug       string    `json:"slug"`
			DateSource string    `json:"date_source"`
			CreatedAt  time.Time `json:"created_at"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v\n%s", err, rec.Body.String())
	}
	if out.Channel != "fenlei" || out.Slug != "tixuhuli" {
		t.Fatalf("channel/slug = %q/%q", out.Channel, out.Slug)
	}
	if len(out.Items) != 2 {
		t.Fatalf("items = %d, want 2 (the article card is dropped): %s", len(out.Items), rec.Body.String())
	}
	first := out.Items[0]
	if first.ID != "184919040" || first.Price == nil || *first.Price != 7.9 {
		t.Fatalf("items[0] = %+v, want id 184919040 with price 7.9", first)
	}
	if first.Slug != "tixuhuli" {
		t.Fatalf("items[0].slug = %q", first.Slug)
	}
	if first.Text != "7.9 CNY (coupon)" {
		t.Fatalf("items[0].text = %q, want the display price", first.Text)
	}
	if first.CreatedAt.IsZero() || first.CreatedAt.Location() != time.UTC {
		t.Fatalf("items[0].created_at = %v, want a UTC timestamp", first.CreatedAt)
	}
	if !strings.Contains(first.HTML, "7.9 CNY (coupon)") {
		t.Fatalf("items[0].html = %q", first.HTML)
	}
	if len(first.Images) != 1 || !strings.HasPrefix(first.Images[0], "https://") {
		t.Fatalf("items[0].images = %v", first.Images)
	}
	if first.DateSource != "fetched" {
		t.Fatalf("items[0].date_source = %q, want the value that says the time is the read time", first.DateSource)
	}

	// The page is read once an hour: the second read is a cache hit, and it
	// keeps the read time instead of re-stamping the batch.
	rec = get(t, h, "/api/v1/smzdm?channel=fenlei&slug=tixuhuli&limit=20")
	var again struct {
		Cached    bool      `json:"cached"`
		FetchedAt time.Time `json:"fetched_at"`
		Items     []struct {
			CreatedAt time.Time `json:"created_at"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &again); err != nil {
		t.Fatalf("decode second: %v", err)
	}
	if !again.Cached {
		t.Fatal("second read is not a cache hit")
	}
	if !again.FetchedAt.Equal(out.FetchedAt) {
		t.Fatalf("cached fetched_at = %v, want the read time %v", again.FetchedAt, out.FetchedAt)
	}
	for i, it := range again.Items {
		if !it.CreatedAt.Equal(first.CreatedAt) {
			t.Fatalf("cached items[%d].created_at = %v, want the read time %v", i, it.CreatedAt, first.CreatedAt)
		}
	}
}

func TestSmzdmFenleiAPIRequiresSlug(t *testing.T) {
	h := newTestServer(t)
	rec := get(t, h, "/api/v1/smzdm?channel=fenlei")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if code := rec.Header().Get("X-Feedmesh-Code"); code != "BAD_REQUEST" {
		t.Fatalf("code = %q", code)
	}
}

func TestSmzdmFenleiAPIRejectsBadSlug(t *testing.T) {
	h := newTestServer(t)
	for _, path := range []string{
		"/api/v1/smzdm?channel=fenlei&slug=../etc/passwd",
		"/api/v1/smzdm?channel=fenlei&slug=jujiashenghuo",
		"/rss/smzdm/fenlei/Bad_Slug",
	} {
		rec := get(t, h, path)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s status = %d body=%s", path, rec.Code, rec.Body.String())
		}
	}
}

func TestSmzdmFenleiAPILimitIsClamped(t *testing.T) {
	h := newTestServer(t)
	rec := get(t, h, "/api/v1/smzdm?channel=fenlei&slug=tixuhuli&limit=500")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var out struct {
		Items []json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.Items) != 2 {
		t.Fatalf("items = %d, want the 2 cards the page holds", len(out.Items))
	}
}

// TestSmzdmFenleiCacheEntriesAreManaged pins the maintenance contract: the new
// key shape must be counted by /healthz and swept by the cache commands.
func TestSmzdmFenleiCacheEntriesAreManaged(t *testing.T) {
	h := newTestServer(t)
	if rec := get(t, h, "/api/v1/smzdm?channel=fenlei&slug=tixuhuli&limit=20"); rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	rec := get(t, h, "/healthz")
	var body struct {
		Cache struct {
			Entries int `json:"entries"`
		} `json:"cache"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode healthz: %v", err)
	}
	if body.Cache.Entries == 0 {
		t.Fatal("the category cache entry is invisible to /healthz")
	}
}
