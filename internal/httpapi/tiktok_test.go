package httpapi_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestTikTokRSSRoute(t *testing.T) {
	h := newTestServer(t)
	for _, path := range []string{"/rss/tiktok/nasa", "/rss/tiktok/nasa.xml"} {
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
			"NASA on TikTok",
			"https://www.tiktok.com/@nasa/video/7708408003065580831",
			"https://p16-common-sign.tiktokcdn.com/c.jpeg",
			"first tiktok caption",
		} {
			if !strings.Contains(body, want) {
				t.Fatalf("%s body missing %q", path, want)
			}
		}
	}
}

func TestTikTokAPIRoute(t *testing.T) {
	h := newTestServer(t)
	rec := get(t, h, "/api/v1/tiktok?user=nasa&limit=1")
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
		} `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Provider != "tiktok" || resp.Meta.User != "nasa" || resp.Title != "NASA on TikTok" {
		t.Fatalf("envelope = %+v", resp)
	}
	if len(resp.Items) != 1 {
		t.Fatalf("items = %d", len(resp.Items))
	}
	item := resp.Items[0]
	if item.ID != "7708408003065580831" || item.URL != "https://www.tiktok.com/@nasa/video/7708408003065580831" {
		t.Fatalf("item = %+v", item)
	}
	if item.Author != "nasa" {
		t.Fatalf("author = %q", item.Author)
	}
	if !item.CreatedAt.Equal(time.Unix(1800000000, 0).UTC()) {
		t.Fatalf("created_at = %v", item.CreatedAt)
	}
}

func TestTikTokTimelineRoute(t *testing.T) {
	h := newTestServer(t)
	rec := get(t, h, "/api/v1/tiktok/timeline?user=nasa")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Author string `json:"author"`
		User   string `json:"user"`
		Source string `json:"source"`
		Items  []struct {
			AwemeID    string `json:"aweme_id"`
			Desc       string `json:"desc"`
			Author     string `json:"author"`
			CreateTime int64  `json:"create_time"`
			DurationMS int    `json:"duration_ms"`
			Width      int    `json:"width"`
			Height     int    `json:"height"`
			IsTop      int    `json:"is_top"`
			PlayURL    string `json:"play_url"`
			CoverURL   string `json:"cover_url"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Author != "NASA" || resp.User != "nasa" || resp.Source != "tiktok" {
		t.Fatalf("envelope = %+v", resp)
	}
	if len(resp.Items) != 2 {
		t.Fatalf("items = %d", len(resp.Items))
	}
	first := resp.Items[0]
	if first.AwemeID != "7708408003065580831" || first.Desc != "first tiktok caption" {
		t.Fatalf("item = %+v", first)
	}
	if first.CreateTime != 1800000000 || first.DurationMS != 79000 || first.Width != 1080 {
		t.Fatalf("timeline fields = %+v", first)
	}
	if first.PlayURL != "https://v16-webapp-prime.tiktok.com/a.mp4" {
		t.Fatalf("play_url = %q", first.PlayURL)
	}
	if resp.Items[1].IsTop != 1 {
		t.Fatalf("is_top = %d", resp.Items[1].IsTop)
	}
}

func TestTikTokMissingUser(t *testing.T) {
	h := newTestServer(t)
	rec := get(t, h, "/api/v1/tiktok?user=")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("X-Feedmesh-Code"); got != "BAD_REQUEST" {
		t.Fatalf("error code = %q", got)
	}
}
