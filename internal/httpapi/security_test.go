package httpapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/yilong-musk/feedmesh/internal/httpapi"
	"github.com/yilong-musk/feedmesh/internal/weibo"
)

func TestXMLSuffixRoutes(t *testing.T) {
	h := newTestServer(t)
	for _, path := range []string{
		"/rss/hot.xml",
		"/rss/sspai.xml",
		"/rss/v2ex.xml",
		"/rss/v2ex/hot.xml",
		"/rss/x/elonmusk.xml",
	} {
		rec := get(t, h, path)
		if rec.Code != http.StatusOK {
			t.Errorf("%s status = %d body=%s", path, rec.Code, rec.Body.String())
		}
	}
}

func TestBearerTokenRequiredWhenConfigured(t *testing.T) {
	h := httpapi.New(httpapi.Config{Token: "secret"}).Handler()

	rec := get(t, h, "/healthz")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("without token status = %d", rec.Code)
	}
	if rec.Header().Get("WWW-Authenticate") == "" {
		t.Fatal("missing WWW-Authenticate header")
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("401 Cache-Control = %q, want no-store", got)
	}

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("with token status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestRateLimitReturnsJSONEnvelope(t *testing.T) {
	h := httpapi.New(httpapi.Config{RateLimit: 1}).Handler()
	if rec := get(t, h, "/healthz"); rec.Code != http.StatusOK {
		t.Fatalf("first status = %d", rec.Code)
	}
	rec := get(t, h, "/healthz")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("second status = %d", rec.Code)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("429 Cache-Control = %q, want no-store", got)
	}
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body is not JSON: %v", err)
	}
	if body.Error.Code != weibo.CodeRateLimited {
		t.Fatalf("code = %q", body.Error.Code)
	}
}

func TestPanicRecoveryReturns500(t *testing.T) {
	// A nil service makes /rss/x/foo panic in the normal handler.
	h := httpapi.New(httpapi.Config{}).Handler()
	rec := get(t, h, "/rss/x/foo")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get(httpapi.ErrorCodeHeader) != weibo.CodeInternal {
		t.Fatalf("code header = %q", rec.Header().Get(httpapi.ErrorCodeHeader))
	}
}
