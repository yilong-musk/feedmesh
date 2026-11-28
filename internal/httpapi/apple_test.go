package httpapi_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/yilong-musk/feedmesh/internal/apple"
)

type fakeApple struct{}

func (fakeApple) Episodes(_ context.Context, id string, limit int) (*apple.Show, error) {
	when := time.Date(2026, 11, 27, 11, 0, 0, 0, time.UTC)
	eps := []apple.Episode{{
		ID: "new", Title: "Newer", Summary: "<p>Hello</p>",
		URL: "https://www.nytimes.com/newer", EnclosureURL: "https://cdn.example/new.mp3",
		EnclosureType: "audio/mpeg", EnclosureLength: 2048, Published: when,
		Duration: time.Minute, ShowTitle: "The Daily", ShowAuthor: "The New York Times",
		ShowURL: "https://podcasts.apple.com/podcast/id" + id, Language: "en-us",
	}}
	if limit > 0 && limit < len(eps) {
		eps = eps[:limit]
	}
	return &apple.Show{ID: id, Title: "The Daily", URL: "https://podcasts.apple.com/podcast/id" + id, Episodes: eps}, nil
}

func (fakeApple) Chart(_ context.Context, country string, limit int) (*apple.Chart, error) {
	rows := []apple.ChartEntry{
		{ID: "1", Name: "One", Artist: "A", URL: "https://podcasts.apple.com/us/podcast/one/id1", Genre: "News"},
		{ID: "2", Name: "Two", Artist: "B", URL: "https://podcasts.apple.com/us/podcast/two/id2"},
	}
	if limit > 0 && limit < len(rows) {
		rows = rows[:limit]
	}
	return &apple.Chart{Country: country, Title: "Top Shows", Entries: rows}, nil
}

func TestAppleRSSRoute(t *testing.T) {
	h := newTestServer(t)
	for _, path := range []string{"/rss/apple/1200361736", "/rss/apple/1200361736.xml"} {
		rec := get(t, h, path)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d body=%s", path, rec.Code, rec.Body.String())
		}
		if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/rss+xml") {
			t.Fatalf("%s content-type = %q", path, ct)
		}
		if got := rec.Header().Get("Cache-Control"); got != "public, max-age=1800" {
			t.Fatalf("%s cache-control = %q", path, got)
		}
		body := rec.Body.String()
		for _, want := range []string{
			"<title>The Daily on Apple Podcasts</title>",
			`type="audio/mpeg"`,
			"https://cdn.example/new.mp3",
		} {
			if !strings.Contains(body, want) {
				t.Fatalf("%s missing %s\n%s", path, want, body)
			}
		}
	}
	rec := get(t, h, "/rss/apple/top?country=us&limit=1")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Top podcasts (US)") || !strings.Contains(rec.Body.String(), "One") {
		t.Fatalf("chart status=%d body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), ">Two<") {
		t.Fatalf("limit was ignored: %s", rec.Body.String())
	}
}

func TestAppleJSONRoute(t *testing.T) {
	h := newTestServer(t)
	rec := get(t, h, "/api/v1/apple?id=1200361736")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"provider":"apple"`) || !strings.Contains(rec.Body.String(), `"mime":"audio/mpeg"`) {
		t.Fatalf("show json = %d %s", rec.Code, rec.Body.String())
	}
	rec = get(t, h, "/api/v1/apple?chart=top&country=gb")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"country":"gb"`) {
		t.Fatalf("chart json = %d %s", rec.Code, rec.Body.String())
	}
	rec = get(t, h, "/api/v1/apple?id=nope")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad id = %d %s", rec.Code, rec.Body.String())
	}
}
