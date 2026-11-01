package httpapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yilong-musk/feedmesh/internal/cache"
	"github.com/yilong-musk/feedmesh/internal/feed"
	"github.com/yilong-musk/feedmesh/internal/httpapi"
	"github.com/yilong-musk/feedmesh/internal/logx"
	"github.com/yilong-musk/feedmesh/internal/smzdm"
)

// fenleiPageForTest renders a category page the way the site stores it: the
// card list goes into the flattened Nuxt payload, containers hold indices.
func fenleiPageForTest(t *testing.T) string {
	t.Helper()
	nodes := []any{nil}
	add := func(v any) int {
		nodes = append(nodes, v)
		return len(nodes) - 1
	}
	root := make([]any, 0, 2)
	for i, title := range []string{"Gillette classic razor", "Shaving foam 240ml"} {
		root = append(root, add(map[string]any{
			"article_type":   add("faxian"),
			"article_url":    add("https://www.smzdm.com/p/" + title[:4] + "/"),
			"focus_pic_url":  add("//y.zdmimg.com/a.jpg_a100.jpg"),
			"subtitle":       add("7.9 CNY"),
			"title":          add(title),
			"article_int_id": add(i + 1),
		}))
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

// A category page served from its last good entry has to announce it, exactly
// like every other degraded document: the header and the short max-age keep a
// reader polling the real page instead of treating the feed as broken.
func TestRSSFenleiDegradedHeader(t *testing.T) {
	var down atomic.Bool
	page := fenleiPageForTest(t)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/fenlei/") {
			http.NotFound(w, r)
			return
		}
		if down.Load() {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(page))
	}))
	defer upstream.Close()

	dir := t.TempDir()
	c, err := cache.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	client := smzdm.New(smzdm.Config{BaseURL: upstream.URL, Timeout: 2 * time.Second, PageTimeout: 5 * time.Second})
	svc := feed.NewService(feed.Clients{Smzdm: client}, c, logx.Discard(), feed.DefaultTTLs())
	api := httpapi.New(httpapi.Config{Version: "test", Service: svc, Cache: c, Log: logx.Discard()})
	h := api.Handler()

	warm := get(t, h, "/rss/smzdm/fenlei/tixuhuli")
	if warm.Code != http.StatusOK {
		t.Fatalf("warm status = %d, want 200", warm.Code)
	}
	if warm.Header().Get("X-Feedmesh-Degraded") != "" {
		t.Fatal("degraded header on a healthy read")
	}

	ageStoredEntry(t, dir, "smzdm:fenlei:tixuhuli", 2*time.Hour)
	down.Store(true)

	rec := get(t, h, "/rss/smzdm/fenlei/tixuhuli")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want the last good category page", rec.Code)
	}
	if rec.Header().Get("X-Feedmesh-Degraded") != "1" {
		t.Fatal("missing X-Feedmesh-Degraded on a last-good category page")
	}
	if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "max-age=60") {
		t.Fatalf("Cache-Control = %q, want max-age=60", cc)
	}
	if !strings.Contains(rec.Body.String(), "Gillette classic razor") {
		t.Fatal("body does not carry the cached cards")
	}

	apiRec := get(t, h, "/api/v1/smzdm?channel=fenlei&slug=tixuhuli")
	if apiRec.Code != http.StatusOK {
		t.Fatalf("api status = %d, want 200", apiRec.Code)
	}
	var body struct {
		Degraded bool `json:"degraded"`
		Items    []struct {
			Title string `json:"title"`
		} `json:"items"`
	}
	if err := json.Unmarshal(apiRec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode api body: %v", err)
	}
	if !body.Degraded {
		t.Fatal("api degraded = false, want true for a last-good category page")
	}
	if len(body.Items) == 0 {
		t.Fatal("api response carries no items")
	}
}
