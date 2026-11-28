package httpapi_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/yilong-musk/feedmesh/internal/xiaoyuzhou"
)

type fakeXiaoyuzhou struct{}

func (fakeXiaoyuzhou) Episodes(_ context.Context, id string, limit int) (*xiaoyuzhou.Show, error) {
	when := time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC)
	eps := []xiaoyuzhou.Episode{{
		ID: "bbbbbbbbbbbbbbbbbbbbbbbb", Title: "Newer", Summary: "<p>Hello</p>",
		URL:      "https://www.xiaoyuzhoufm.com/episode/bbbbbbbbbbbbbbbbbbbbbbbb",
		AudioURL: "https://media.example/new.m4a", Published: when, Duration: time.Minute,
		Plays: 40, ShowTitle: "Sample Show", ShowAuthor: "Host",
		ShowURL: "https://www.xiaoyuzhoufm.com/podcast/" + id,
	}}
	if limit > 0 && limit < len(eps) {
		eps = eps[:limit]
	}
	return &xiaoyuzhou.Show{ID: id, Title: "Sample Show", URL: "https://www.xiaoyuzhoufm.com/podcast/" + id, Episodes: eps}, nil
}

func TestXiaoyuzhouRoutes(t *testing.T) {
	h := newTestServer(t)
	for _, path := range []string{"/rss/xiaoyuzhou/6021f949a789fca4eff4492c", "/rss/xiaoyuzhou/6021f949a789fca4eff4492c.xml"} {
		rec := get(t, h, path)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d body=%s", path, rec.Code, rec.Body.String())
		}
		if got := rec.Header().Get("Cache-Control"); got != "public, max-age=1800" {
			t.Fatalf("%s cache-control = %q", path, got)
		}
		body := rec.Body.String()
		for _, want := range []string{"Sample Show on Xiaoyuzhou", `type="audio/mp4"`, "https://media.example/new.m4a"} {
			if !strings.Contains(body, want) {
				t.Fatalf("%s missing %s\n%s", path, want, body)
			}
		}
	}
	rec := get(t, h, "/api/v1/xiaoyuzhou?id=6021f949a789fca4eff4492c")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"provider":"xiaoyuzhou"`) || !strings.Contains(rec.Body.String(), `"views":40`) {
		t.Fatalf("json = %d %s", rec.Code, rec.Body.String())
	}
	rec = get(t, h, "/api/v1/xiaoyuzhou?id=nope")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad id = %d %s", rec.Code, rec.Body.String())
	}
}
