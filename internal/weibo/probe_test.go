package weibo_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/yilong-musk/feedmesh/internal/weibo"
)

func TestProbeReportsVisitorHTTPStatus(t *testing.T) {
	srv := fakeUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/visitor" {
			http.Error(w, "blocked", http.StatusTeapot)
			return
		}
		http.NotFound(w, r)
	})

	steps := newClient(t, srv.URL).Probe(context.Background(), "1")
	if len(steps) == 0 {
		t.Fatal("Probe returned no steps")
	}
	first := steps[0]
	if first.OK {
		t.Fatal("visitor step unexpectedly succeeded")
	}
	if first.HTTPStatus != http.StatusTeapot {
		t.Fatalf("genvisitor HTTPStatus = %d, want %d", first.HTTPStatus, http.StatusTeapot)
	}
	if first.Code != weibo.CodeUpstreamBlocked {
		t.Fatalf("genvisitor code = %q, want %q", first.Code, weibo.CodeUpstreamBlocked)
	}
}
