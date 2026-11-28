package httpapi_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/yilong-musk/feedmesh/internal/trumpstruth"
)

type fakeTrumpTruth struct{}

func (fakeTrumpTruth) Latest(context.Context, int) (*trumpstruth.Result, error) {
	when := time.Date(2026, 11, 27, 21, 56, 39, 0, time.UTC)
	return &trumpstruth.Result{
		Title: "Trump's Truth - Latest Posts", Link: "https://www.trumpstruth.org/",
		Description: "Archive", FetchedAt: when,
		Posts: []trumpstruth.Post{{
			ID: "43761", Title: "A statement", Text: "Hello archive",
			URL:         "https://www.trumpstruth.org/statuses/43761",
			OriginalURL: "https://truthsocial.com/@realDonaldTrump/1",
			Image:       "https://cdn.example/a.jpg", Published: when,
			PostType: "original", Author: "Donald J. Trump",
		}},
	}, nil
}

func TestTrumpTruthRoutes(t *testing.T) {
	h := newTestServer(t)
	for _, path := range []string{"/rss/trumpstruth", "/rss/trumpstruth.xml"} {
		rec := get(t, h, path)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d body=%s", path, rec.Code, rec.Body.String())
		}
		if got := rec.Header().Get("Cache-Control"); got != "public, max-age=600" {
			t.Fatalf("%s cache-control = %q", path, got)
		}
		body := rec.Body.String()
		for _, want := range []string{"Latest Posts", "https://www.trumpstruth.org/statuses/43761", "https://cdn.example/a.jpg"} {
			if !strings.Contains(body, want) {
				t.Fatalf("%s missing %s\n%s", path, want, body)
			}
		}
	}
	rec := get(t, h, "/api/v1/trumpstruth?limit=5")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"provider":"trumpstruth"`) || !strings.Contains(rec.Body.String(), `"post_type":"original"`) {
		t.Fatalf("json = %d %s", rec.Code, rec.Body.String())
	}
}
