package httpapi

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/yilong-musk/feedmesh/internal/logx"
	"github.com/yilong-musk/feedmesh/internal/weibo"
)

func TestRateLimiterUsesRollingWindow(t *testing.T) {
	now := time.Unix(0, 0)
	limiter := newRateLimiter(2)
	limiter.now = func() time.Time { return now }

	if !limiter.allow("ip") || !limiter.allow("ip") {
		t.Fatal("first two requests should be allowed")
	}
	if limiter.allow("ip") {
		t.Fatal("third request in the same window should be denied")
	}

	now = now.Add(59 * time.Second)
	if limiter.allow("ip") {
		t.Fatal("requests must still count until a full minute passes")
	}

	now = now.Add(2 * time.Second)
	if !limiter.allow("ip") || !limiter.allow("ip") {
		t.Fatal("requests older than one minute should no longer count")
	}
	if limiter.allow("ip") {
		t.Fatal("window did not reset after the old requests expired")
	}
}

func TestRateLimiterSweepsIdleKeys(t *testing.T) {
	now := time.Unix(0, 0)
	limiter := newRateLimiter(1)
	limiter.now = func() time.Time { return now }
	_ = limiter.allow("old")

	now = now.Add(2 * time.Minute)
	_ = limiter.allow("new")
	if len(limiter.windows) != 1 {
		t.Fatalf("windows = %v, want only the current key", limiter.windows)
	}
}

func TestRSSCacheControlPrivateWhenTokenSet(t *testing.T) {
	s := New(Config{Token: "secret"})
	rec := httptest.NewRecorder()
	s.setRSSCache(rec, 300)
	if got := rec.Header().Get("Cache-Control"); !strings.Contains(got, "private") || !strings.Contains(got, "300") {
		t.Fatalf("authenticated Cache-Control = %q", got)
	}
	if got := rec.Header().Get("Vary"); got != "Authorization" {
		t.Fatalf("Vary = %q, want Authorization", got)
	}

	s = New(Config{})
	rec = httptest.NewRecorder()
	s.setRSSCache(rec, 300)
	if got := rec.Header().Get("Cache-Control"); !strings.Contains(got, "public") || !strings.Contains(got, "300") {
		t.Fatalf("unauthenticated Cache-Control = %q", got)
	}
	if got := rec.Header().Get("Vary"); got != "" {
		t.Fatalf("Vary = %q, want empty", got)
	}
}

func TestErrorLogRecordsServerFailuresOnly(t *testing.T) {
	var buf bytes.Buffer
	s := New(Config{Log: logx.New(&buf)})

	fail := s.withErrorLog(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.writeErrorStatus(w, http.StatusGatewayTimeout,
			weibo.NewError(weibo.CodeUpstreamUnavailable, 0, "xnitter: all instances failed (5 tried)"))
	}))
	rec := httptest.NewRecorder()
	fail.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/rss/x/elonmusk", nil))
	if rec.Code != http.StatusGatewayTimeout {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusGatewayTimeout)
	}
	out := buf.String()
	for _, want := range []string{
		"request failed",
		`"path":"/rss/x/elonmusk"`,
		`"status":504`,
		"UPSTREAM_UNAVAILABLE",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("journal is missing %q: %s", want, out)
		}
	}

	buf.Reset()
	ok := s.withErrorLog(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	ok.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if buf.Len() != 0 {
		t.Fatalf("a successful request was logged: %s", buf.String())
	}
}
