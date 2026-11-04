package xnitter

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestProbeReportsPoolAndInstances(t *testing.T) {
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/someone/rss" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(`<rss><channel><item><title>x</title></item></channel></rss>`))
	}))
	defer good.Close()
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "blocked", http.StatusForbidden)
	}))
	defer bad.Close()

	c := New(Config{
		Instances:         []string{good.URL, bad.URL},
		StatusURL:         "http://127.0.0.1:1/status",
		Timeout:           2 * time.Second,
		DisableEnrichment: true,
	})
	steps := c.Probe(context.Background(), "@someone")
	if len(steps) != 3 {
		t.Fatalf("steps = %d, want pool + two instances: %+v", len(steps), steps)
	}
	if steps[0].Step != "pool" || !steps[0].OK {
		t.Errorf("pool step = %+v, want a fallback list marked usable", steps[0])
	}
	if steps[0].Error == "" {
		t.Errorf("pool step should keep the directory error as a note")
	}
	if steps[0].Items != 2 {
		t.Errorf("pool items = %d, want the two fallbacks", steps[0].Items)
	}
	if steps[1].Step != hostOf(good.URL) || !steps[1].OK || steps[1].Items != 1 {
		t.Errorf("good step = %+v", steps[1])
	}
	if steps[2].OK || steps[2].Code != "UPSTREAM_BLOCKED" || steps[2].HTTPStatus != http.StatusForbidden {
		t.Errorf("blocked step = %+v", steps[2])
	}
}

func TestProbeRejectsAnInvalidHandle(t *testing.T) {
	c := New(Config{Instances: []string{"https://example.test"}, StatusURL: "http://127.0.0.1:1/status", Timeout: time.Second, DisableEnrichment: true})
	steps := c.Probe(context.Background(), "bad handle")
	if len(steps) != 2 || steps[1].Code != "BAD_REQUEST" {
		t.Fatalf("steps = %+v", steps)
	}
}
