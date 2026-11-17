package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/yilong-musk/feedmesh/internal/linuxdo"
)

type fakeLinuxDo struct{}

func (fakeLinuxDo) Fetch(_ context.Context, ref linuxdo.Ref, limit int) (*linuxdo.Feed, error) {
	at := time.Date(2026, 11, 17, 8, 0, 0, 0, time.UTC)
	topics := []linuxdo.Topic{
		{
			ID: "3191226", Title: "First topic", URL: "https://linux.do/t/topic/3191226",
			Author: "alice", Category: "Development", CreatedAt: at.Add(2 * time.Minute),
			HTML:   `<p>First paragraph</p><p>Second paragraph</p><p><img src="https://cdn3.ldstatic.com/a.jpg"></p>`,
			Images: []string{"https://cdn3.ldstatic.com/a.jpg"},
		},
		{
			ID: "3191225", Title: "Second topic", URL: "https://linux.do/t/topic/3191225",
			Author: "bob", Category: "Chat", CreatedAt: at.Add(time.Minute),
			Pinned: true, Closed: true,
			HTML: `<p>Body</p>`,
		},
		{
			ID: "3191224", Title: "Third topic", URL: "https://linux.do/t/topic/3191224",
			Author: "carol", Category: "Development", CreatedAt: at,
			HTML: `<p>Body</p>`,
		},
	}
	if limit > 0 && limit < len(topics) {
		topics = topics[:limit]
	}
	return &linuxdo.Feed{
		Kind: ref.Kind, Channel: "LINUX DO - test", Link: "https://linux.do/",
		Language: "zh-CN", FetchedAt: time.Unix(1800000100, 0).UTC(), Topics: topics,
	}, nil
}

func (fakeLinuxDo) Budget() time.Duration { return 2 * time.Second }

func TestLinuxDoRSSRoutes(t *testing.T) {
	h := newTestServer(t)
	cases := []struct {
		path  string
		title string
	}{
		{"/rss/linuxdo", "LINUX DO · Latest topics"},
		{"/rss/linuxdo.xml", "LINUX DO · Latest topics"},
		{"/rss/linuxdo?order=activity", "LINUX DO · Latest activity"},
		{"/rss/linuxdo/top?period=weekly", "LINUX DO · Top Weekly"},
		{"/rss/linuxdo/top.xml", "LINUX DO · Top Daily"},
		{"/rss/linuxdo/hot", "LINUX DO · Hot"},
		{"/rss/linuxdo/c/develop/4", "LINUX DO · Development"},
		{"/rss/linuxdo/c/4", "LINUX DO · Development"},
		{"/rss/linuxdo/tag/chatgpt.xml", "LINUX DO · #chatgpt"},
	}
	for _, tc := range cases {
		rec := get(t, h, tc.path)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d body=%s", tc.path, rec.Code, rec.Body.String())
		}
		if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/rss+xml") {
			t.Fatalf("%s content-type = %q", tc.path, ct)
		}
		if cc := rec.Header().Get("Cache-Control"); cc != "public, max-age=900" {
			t.Fatalf("%s cache-control = %q", tc.path, cc)
		}
		body := rec.Body.String()
		for _, want := range []string{tc.title, "https://linux.do/t/topic/3191226", "https://cdn3.ldstatic.com/a.jpg"} {
			if !strings.Contains(body, want) {
				t.Fatalf("%s body missing %q", tc.path, want)
			}
		}
	}
}

func TestLinuxDoAPIRoute(t *testing.T) {
	h := newTestServer(t)
	rec := get(t, h, "/api/v1/linuxdo?feed=latest&limit=2")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Feed        string `json:"feed"`
		Title       string `json:"title"`
		Description string `json:"description"`
		Cached      bool   `json:"cached"`
		Items       []struct {
			ID        string    `json:"id"`
			Title     string    `json:"title"`
			Text      string    `json:"text"`
			URL       string    `json:"url"`
			HTML      string    `json:"html"`
			Images    []string  `json:"images"`
			Author    string    `json:"author"`
			Category  string    `json:"category"`
			Pinned    bool      `json:"pinned"`
			Closed    bool      `json:"closed"`
			CreatedAt time.Time `json:"created_at"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Feed != "latest:created" || resp.Title != "LINUX DO · Latest topics" {
		t.Fatalf("envelope = %+v", resp)
	}
	if len(resp.Items) != 2 {
		t.Fatalf("items = %d", len(resp.Items))
	}
	first := resp.Items[0]
	if first.ID != "3191226" || first.URL != "https://linux.do/t/topic/3191226" {
		t.Fatalf("first = %+v", first)
	}
	if first.Text != "First paragraph\nSecond paragraph" {
		t.Fatalf("text = %q", first.Text)
	}
	if first.Category != "Development" || first.Author != "alice" || len(first.Images) != 1 {
		t.Fatalf("first metadata = %+v", first)
	}
	second := resp.Items[1]
	if !second.Pinned || !second.Closed {
		t.Fatalf("state flags missing: %+v", second)
	}
}

func TestLinuxDoAPICategoryAndTag(t *testing.T) {
	h := newTestServer(t)
	for _, path := range []string{
		"/api/v1/linuxdo?feed=category&category=develop/4",
		"/api/v1/linuxdo?feed=category&category=4",
		"/api/v1/linuxdo?feed=tag&tag=chatgpt",
		"/api/v1/linuxdo?feed=top&period=weekly",
		"/api/v1/linuxdo?feed=hot",
	} {
		rec := get(t, h, path)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d body=%s", path, rec.Code, rec.Body.String())
		}
	}
}

func TestLinuxDoErrors(t *testing.T) {
	h := newTestServer(t)
	cases := []struct {
		path string
		code string
	}{
		{"/rss/linuxdo/top?period=hourly", "BAD_REQUEST"},
		{"/rss/linuxdo?order=newest", "BAD_REQUEST"},
		{"/rss/linuxdo/c/not-an-id", "BAD_REQUEST"},
		{"/rss/linuxdo/tag/", "BAD_REQUEST"},
		{"/api/v1/linuxdo?feed=nope", "BAD_REQUEST"},
		{"/api/v1/linuxdo?feed=category", "BAD_REQUEST"},
	}
	for _, tc := range cases {
		rec := get(t, h, tc.path)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s status = %d body=%s", tc.path, rec.Code, rec.Body.String())
		}
		if got := rec.Header().Get("X-Feedmesh-Code"); got != tc.code {
			t.Fatalf("%s code = %q", tc.path, got)
		}
	}
}

func TestLinuxDoCachedSecondRead(t *testing.T) {
	h := newTestServer(t)
	first := get(t, h, "/api/v1/linuxdo?feed=latest&limit=1")
	if first.Code != http.StatusOK {
		t.Fatalf("first status = %d", first.Code)
	}
	second := get(t, h, "/api/v1/linuxdo?feed=latest&limit=1")
	if second.Code != http.StatusOK {
		t.Fatalf("second status = %d", second.Code)
	}
	var resp struct {
		Cached bool `json:"cached"`
	}
	if err := json.Unmarshal(second.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.Cached {
		t.Fatal("second read was not served from cache")
	}
}
