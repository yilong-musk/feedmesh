package httpapi_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

const douyinSecUID = "MS4wLjABAAAAtestsecuid0000000000"

func TestDouyinRSSRoute(t *testing.T) {
	h := newTestServer(t)
	for _, path := range []string{"/rss/douyin/" + douyinSecUID, "/rss/douyin/" + douyinSecUID + ".xml"} {
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
			"Test Creator on Douyin",
			"https://www.douyin.com/video/7002",
			"https://p3.douyinpic.com/c.jpeg",
		} {
			if !strings.Contains(body, want) {
				t.Fatalf("%s body missing %q", path, want)
			}
		}
	}
}

func TestDouyinAPIRoute(t *testing.T) {
	h := newTestServer(t)
	rec := get(t, h, "/api/v1/douyin?user="+douyinSecUID+"&limit=1")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		User   string `json:"user"`
		Title  string `json:"title"`
		Cached bool   `json:"cached"`
		Items  []struct {
			ID        string    `json:"id"`
			Title     string    `json:"title"`
			URL       string    `json:"url"`
			CreatedAt time.Time `json:"created_at"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.User != douyinSecUID || resp.Title != "Test Creator on Douyin" {
		t.Fatalf("envelope = %+v", resp)
	}
	if len(resp.Items) != 1 {
		t.Fatalf("items = %d", len(resp.Items))
	}
	if resp.Items[0].ID != "7002" || resp.Items[0].URL != "https://www.douyin.com/video/7002" {
		t.Fatalf("item = %+v", resp.Items[0])
	}
	if !resp.Items[0].CreatedAt.Equal(time.Unix(1800000000, 0).UTC()) {
		t.Fatalf("created_at = %v", resp.Items[0].CreatedAt)
	}
}

func TestDouyinTimelineRoute(t *testing.T) {
	h := newTestServer(t)
	rec := get(t, h, "/api/v1/douyin/timeline?user="+douyinSecUID)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Author string `json:"author"`
		SecUID string `json:"sec_uid"`
		Items  []struct {
			AwemeID    string `json:"aweme_id"`
			Desc       string `json:"desc"`
			CreateTime int64  `json:"create_time"`
			DurationMS int    `json:"duration_ms"`
			Width      int    `json:"width"`
			IsTop      int    `json:"is_top"`
			PlayURL    string `json:"play_url"`
			CoverURL   string `json:"cover_url"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Author != "Test Creator" || resp.SecUID != douyinSecUID {
		t.Fatalf("envelope = %+v", resp)
	}
	if len(resp.Items) != 2 {
		t.Fatalf("items = %d", len(resp.Items))
	}
	first := resp.Items[0]
	if first.AwemeID != "7002" || first.PlayURL != "https://v3-dy-o.zjcdn.com/a.mp4" || first.CoverURL == "" {
		t.Fatalf("first item = %+v", first)
	}
	if first.CreateTime != 1800000000 || first.DurationMS != 93000 || first.Width != 1080 {
		t.Fatalf("first item media = %+v", first)
	}
	if resp.Items[1].IsTop != 1 {
		t.Fatalf("pinned marker = %+v", resp.Items[1])
	}
}

func TestDouyinMissingUser(t *testing.T) {
	h := newTestServer(t)
	rec := get(t, h, "/api/v1/douyin")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if code := rec.Header().Get("X-Feedmesh-Code"); code != "BAD_REQUEST" {
		t.Fatalf("code header = %q", code)
	}
}
