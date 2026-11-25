package httpapi_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

const pinterestUserFeedXML = `<?xml version="1.0" encoding="utf-8"?><rss version="2.0">
<channel><title>NASA</title><link>https://www.pinterest.com/nasa/</link>
<description>Official NASA boards.</description>
<item><title>Moon Base</title><link>https://www.pinterest.com/pin/142567144449130105/</link>
<description>&lt;a href=&quot;https://www.pinterest.com/pin/142567144449130105/&quot;&gt;&lt;img src=&quot;https://i.pinimg.com/236x/7f/b0/7c/7fb07c79a6280dd11eab9165b4120300.jpg&quot;&gt;&lt;/a&gt;Moon Base caption.</description>
<pubDate>Fri, 20 Nov 2026 16:39:34 GMT</pubDate><guid>https://www.pinterest.com/pin/142567144449130105/</guid></item>
<item><title> </title><link>https://www.pinterest.com/pin/142567144449081155/</link>
<description>&lt;a href=&quot;https://www.pinterest.com/pin/142567144449081155/&quot;&gt;&lt;img src=&quot;https://i.pinimg.com/236x/bb/a1/e7/bba1e7fb6fe37433e1314ad3cf4e3434.jpg&quot;&gt;&lt;/a&gt;</description>
<pubDate>Fri, 20 Nov 2026 15:00:00 GMT</pubDate><guid>https://www.pinterest.com/pin/142567144449081155/</guid></item>
</channel></rss>`

const pinterestBoardFeedXML = `<?xml version="1.0" encoding="utf-8"?><rss version="2.0">
<channel><title>Earth</title><link>https://www.pinterest.com/nasa/earth</link><description></description>
<item><title>Polar bears</title><link>https://www.pinterest.com/pin/e1_AfqPShwRDMi0aXUqBNuln_BzY7y-7nLKzl3QcRs0w1M1-vS9pbG9/</link>
<description>&lt;a href=&quot;https://www.pinterest.com/pin/e1_AfqPShwRDMi0aXUqBNuln_BzY7y-7nLKzl3QcRs0w1M1-vS9pbG9/&quot;&gt;&lt;img src=&quot;https://i.pinimg.com/236x/17/66/48/17664853e56a6b68e5fa9ad24f5732d0.jpg&quot;&gt;&lt;/a&gt;Polar bears are scrambling.</description>
<pubDate>Thu, 16 Jun 2022 18:20:42 GMT</pubDate><guid>https://www.pinterest.com/pin/e1_AfqPShwRDMi0aXUqBNuln_BzY7y-7nLKzl3QcRs0w1M1-vS9pbG9/</guid></item>
</channel></rss>`

func TestPinterestRSSRoute(t *testing.T) {
	h := newTestServer(t)
	for _, path := range []string{"/rss/pinterest/nasa", "/rss/pinterest/nasa.xml"} {
		rec := get(t, h, path)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d body=%s", path, rec.Code, rec.Body.String())
		}
		if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/rss+xml") {
			t.Fatalf("%s content-type = %q", path, ct)
		}
		if got := rec.Header().Get("Cache-Control"); got != "public, max-age=600" {
			t.Fatalf("%s cache-control = %q", path, got)
		}
		body := rec.Body.String()
		for _, want := range []string{
			"<title>NASA on Pinterest</title>",
			"https://www.pinterest.com/pin/142567144449130105/",
			"https://i.pinimg.com/736x/7f/b0/7c/7fb07c79a6280dd11eab9165b4120300.jpg",
			"Moon Base",
			"(no content)",
			"<ttl>10</ttl>",
		} {
			if !strings.Contains(body, want) {
				t.Fatalf("%s missing %q:\n%s", path, want, body)
			}
		}
	}
}

func TestPinterestBoardRSSRoute(t *testing.T) {
	h := newTestServer(t)
	for _, path := range []string{"/rss/pinterest/nasa/earth", "/rss/pinterest/nasa/earth.xml"} {
		rec := get(t, h, path)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d body=%s", path, rec.Code, rec.Body.String())
		}
		body := rec.Body.String()
		for _, want := range []string{
			"<title>Earth on Pinterest</title>",
			"https://www.pinterest.com/pin/e1_AfqPShwRDMi0aXUqBNuln_BzY7y-7nLKzl3QcRs0w1M1-vS9pbG9/",
			"Pinterest board nasa/earth",
		} {
			if !strings.Contains(body, want) {
				t.Fatalf("%s missing %q:\n%s", path, want, body)
			}
		}
	}
}

func TestPinterestAPIRoute(t *testing.T) {
	h := newTestServer(t)

	rec := get(t, h, "/api/v1/pinterest?user=nasa&limit=1")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var out struct {
		User        string `json:"user"`
		Title       string `json:"title"`
		Description string `json:"description"`
		Cached      bool   `json:"cached"`
		Degraded    bool   `json:"degraded"`
		Items       []struct {
			ID        string    `json:"id"`
			Title     string    `json:"title"`
			Text      string    `json:"text"`
			URL       string    `json:"url"`
			HTML      string    `json:"html"`
			Images    []string  `json:"images"`
			CreatedAt time.Time `json:"created_at"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v\n%s", err, rec.Body.String())
	}
	if out.User != "nasa" || out.Title != "NASA on Pinterest" || out.Description != "Official NASA boards." {
		t.Fatalf("channel = %+v", out)
	}
	if len(out.Items) != 1 {
		t.Fatalf("limit=1 returned %d items", len(out.Items))
	}
	it := out.Items[0]
	if it.ID != "142567144449130105" || it.Title != "Moon Base" {
		t.Fatalf("item = %+v", it)
	}
	if len(it.Images) != 1 || !strings.Contains(it.Images[0], "/736x/") {
		t.Fatalf("images = %v", it.Images)
	}
	if got := it.CreatedAt.Format(time.RFC3339); got != "2026-11-20T16:39:34Z" {
		t.Fatalf("created_at = %s", got)
	}
	if out.Cached {
		t.Fatal("first request must not be a cache hit")
	}

	rec = get(t, h, "/api/v1/pinterest?user=nasa&limit=1")
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode second call: %v", err)
	}
	if !out.Cached {
		t.Fatal("second call must be a cache hit")
	}
}

func TestPinterestAPIBoard(t *testing.T) {
	h := newTestServer(t)
	rec := get(t, h, "/api/v1/pinterest?user=nasa&board=earth")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"user":"nasa","board":"earth"`) {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestPinterestNotFoundRoute(t *testing.T) {
	h := newTestServer(t)
	rec := get(t, h, "/api/v1/pinterest?user=ghost")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if code := rec.Header().Get("X-Feedmesh-Code"); code != "USER_NOT_FOUND" {
		t.Fatalf("code = %q", code)
	}
}

func TestPinterestRejectsBadName(t *testing.T) {
	h := newTestServer(t)
	rec := get(t, h, "/api/v1/pinterest?user=nasa%2Fearth")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}
