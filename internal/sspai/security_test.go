package sspai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestListStopsWhenAPageAddsNoValidArticles(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		data := make([]map[string]any, PageSize)
		for i := range data {
			data[i] = map[string]any{"id": 0, "title": ""}
		}
		if r.URL.Path != "/api/v1/article/index/page/get" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"error": 0, "data": data})
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, Timeout: time.Second})
	if _, err := c.List(context.Background(), 10); err == nil {
		t.Fatal("expected an error when no valid article arrives")
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("made %d requests, want 1 before stopping on no progress", got)
	}
}

func TestClientLimitsConcurrentRequests(t *testing.T) {
	var active, peak int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&active, 1)
		for {
			old := atomic.LoadInt32(&peak)
			if n <= old || atomic.CompareAndSwapInt32(&peak, old, n) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
		atomic.AddInt32(&active, -1)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": 0,
			"data":  map[string]any{"body": "<p>x</p>", "is_free": true},
		})
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, Timeout: time.Second})
	done := make(chan struct{}, 8)
	for i := 0; i < 8; i++ {
		go func(id int) {
			_, _ = c.Detail(context.Background(), id)
			done <- struct{}{}
		}(i)
	}
	for i := 0; i < 8; i++ {
		<-done
	}
	if got := atomic.LoadInt32(&peak); got > maxConcurrent {
		t.Fatalf("peak concurrency = %d, want <= %d", got, maxConcurrent)
	}
}
