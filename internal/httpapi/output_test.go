package httpapi_test

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func getWith(t *testing.T, h http.Handler, path string, header map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	for k, v := range header {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestRSSIsGzippedWhenAccepted(t *testing.T) {
	h := newTestServer(t)

	rec := getWith(t, h, "/rss/user/2803301701", map[string]string{"Accept-Encoding": "br, gzip;q=0.8"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("Content-Encoding = %q, want gzip", got)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/rss+xml") {
		t.Fatalf("Content-Type = %q", ct)
	}
	if vary := strings.Join(rec.Header().Values("Vary"), ","); !strings.Contains(vary, "Accept-Encoding") {
		t.Fatalf("Vary = %q, want Accept-Encoding", vary)
	}
	zr, err := gzip.NewReader(rec.Body)
	if err != nil {
		t.Fatalf("body is not gzip: %v", err)
	}
	plain, err := io.ReadAll(zr)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(plain), `<rss version="2.0"`) {
		t.Fatalf("decompressed body is not RSS: %.120s", plain)
	}

	identity := getWith(t, h, "/rss/user/2803301701", nil)
	if got := identity.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("Content-Encoding without Accept-Encoding = %q", got)
	}
	refused := getWith(t, h, "/rss/user/2803301701", map[string]string{"Accept-Encoding": "gzip;q=0"})
	if got := refused.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("Content-Encoding with gzip;q=0 = %q", got)
	}
}

func TestRSSAnswersMatchingIfNoneMatchWith304(t *testing.T) {
	h := newTestServer(t)

	first := getWith(t, h, "/rss/user/2803301701", nil)
	etag := first.Header().Get("ETag")
	if first.Code != http.StatusOK || !strings.HasPrefix(etag, `W/"`) {
		t.Fatalf("first: status %d, ETag %q", first.Code, etag)
	}

	// The Weibo feed stamps the read time into lastBuildDate on every
	// request; the validator must not change because of that alone.
	again := getWith(t, h, "/rss/user/2803301701", nil)
	if got := again.Header().Get("ETag"); got != etag {
		t.Fatalf("ETag changed between identical documents: %q then %q", etag, got)
	}

	for _, inm := range []string{etag, strings.TrimPrefix(etag, "W/"), `"other", ` + etag, "*"} {
		rec := getWith(t, h, "/rss/user/2803301701", map[string]string{
			"If-None-Match":   inm,
			"Accept-Encoding": "gzip",
		})
		if rec.Code != http.StatusNotModified {
			t.Fatalf("If-None-Match %q: status = %d, want 304", inm, rec.Code)
		}
		if rec.Body.Len() != 0 {
			t.Fatalf("If-None-Match %q: 304 carried %d body bytes", inm, rec.Body.Len())
		}
		if rec.Header().Get("Content-Encoding") != "" {
			t.Fatalf("If-None-Match %q: 304 declared a content coding", inm)
		}
		if rec.Header().Get("ETag") != etag || rec.Header().Get("Cache-Control") == "" {
			t.Fatalf("If-None-Match %q: 304 headers = %v", inm, rec.Header())
		}
	}

	stale := getWith(t, h, "/rss/user/2803301701", map[string]string{"If-None-Match": `W/"stale"`})
	if stale.Code != http.StatusOK {
		t.Fatalf("non-matching If-None-Match: status = %d, want 200", stale.Code)
	}
}

func TestJSONErrorsAreGzippedToo(t *testing.T) {
	h := newTestServer(t)
	rec := getWith(t, h, "/no/such/route", map[string]string{"Accept-Encoding": "gzip"})
	if rec.Header().Get("Content-Encoding") != "gzip" {
		t.Fatalf("Content-Encoding = %q", rec.Header().Get("Content-Encoding"))
	}
	zr, err := gzip.NewReader(rec.Body)
	if err != nil {
		t.Fatal(err)
	}
	plain, _ := io.ReadAll(zr)
	if !strings.Contains(string(plain), `"code"`) {
		t.Fatalf("error body = %s", plain)
	}
}
