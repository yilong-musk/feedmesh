package httpapi_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestBilibiliRSSRoute(t *testing.T) {
	h := newTestServer(t)
	for _, path := range []string{"/rss/bilibili/946974", "/rss/bilibili/946974.xml"} {
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
			"影视飓风 on Bilibili",
			"https://www.bilibili.com/video/BV1kZWX62ENY",
			"https://i2.hdslb.com/bfs/archive/a.jpg",
			"为什么是最后一期？",
		} {
			if !strings.Contains(body, want) {
				t.Fatalf("%s body missing %q", path, want)
			}
		}
	}
}

func TestBilibiliAPIRoute(t *testing.T) {
	h := newTestServer(t)
	rec := get(t, h, "/api/v1/bilibili?user=946974&limit=1")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Provider string `json:"provider"`
		Title    string `json:"title"`
		Cached   bool   `json:"cached"`
		Meta     struct {
			User string `json:"user"`
		} `json:"meta"`
		Items []struct {
			ID        string    `json:"id"`
			Title     string    `json:"title"`
			URL       string    `json:"url"`
			Author    string    `json:"author"`
			CreatedAt time.Time `json:"created_at"`
			Extra     struct {
				Views int64 `json:"views"`
			} `json:"extra"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Provider != "bilibili" || resp.Meta.User != "946974" || resp.Title != "影视飓风 on Bilibili" {
		t.Fatalf("envelope = %+v", resp)
	}
	if len(resp.Items) != 1 {
		t.Fatalf("items = %d", len(resp.Items))
	}
	item := resp.Items[0]
	if item.ID != "BV1kZWX62ENY" || item.URL != "https://www.bilibili.com/video/BV1kZWX62ENY" {
		t.Fatalf("item = %+v", item)
	}
	if item.Extra.Views != 6608624 {
		t.Fatalf("views = %d", item.Extra.Views)
	}
	if !item.CreatedAt.Equal(time.Unix(1800000000, 0).UTC()) {
		t.Fatalf("created_at = %v", item.CreatedAt)
	}
}

func TestBilibiliMissingUser(t *testing.T) {
	h := newTestServer(t)
	rec := get(t, h, "/api/v1/bilibili?user=")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("X-Feedmesh-Code"); got != "BAD_REQUEST" {
		t.Fatalf("error code = %q", got)
	}
}
