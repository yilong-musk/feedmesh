package httpapi_test

import (
	"compress/gzip"
	"encoding/json"
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

func TestUnifiedJSONEnvelopeAcrossProviders(t *testing.T) {
	h := newTestServer(t)
	cases := []struct {
		path      string
		provider  string
		wantExtra bool
	}{
		{"/api/v1/feed?uid=2803301701&limit=1", "weibo", true},
		{"/api/v1/hot?limit=1", "hot", true},
		{"/api/v1/sspai?limit=1", "sspai", false},
		{"/api/v1/v2ex?section=latest&limit=1", "v2ex", false},
		{"/api/v1/smzdm?channel=deals&limit=1", "smzdm", false},
		{"/api/v1/smzdm?channel=fenlei&slug=tixuhuli&limit=1", "smzdm", true},
		{"/api/v1/x?handle=elonmusk&limit=1", "x", true},
		{"/api/v1/youtube?ref=@testchannel&limit=1", "youtube", true},
		{"/api/v1/douyin?user=" + douyinSecUID + "&limit=1", "douyin", false},
		{"/api/v1/tiktok?user=nasa&limit=1", "tiktok", false},
		{"/api/v1/bilibili?user=946974&limit=1", "bilibili", true},
		{"/api/v1/telegram?channel=sspai&limit=1", "telegram", true},
		{"/api/v1/pinterest?user=nasa&limit=1", "pinterest", false},
		{"/api/v1/polymarket?feed=event&slug=event-one&limit=1", "polymarket", true},
		{"/api/v1/polymarket?feed=user&address=0x7c3db723f1d4d8cb9c550095203b686cb11e5c6b&limit=1", "polymarket", false},
		{"/api/v1/linuxdo?feed=latest&limit=1", "linuxdo", true},
	}
	for _, tc := range cases {
		rec := get(t, h, tc.path)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d body=%s", tc.path, rec.Code, rec.Body.String())
		}
		var env map[string]json.RawMessage
		if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
			t.Fatalf("%s decode: %v", tc.path, err)
		}
		for _, k := range []string{"provider", "title", "description", "fetched_at", "cached", "degraded", "items"} {
			if _, ok := env[k]; !ok {
				t.Errorf("%s missing envelope key %q", tc.path, k)
			}
		}
		if got := strings.Trim(string(env["provider"]), `"`); got != tc.provider {
			t.Errorf("%s provider = %q, want %q", tc.path, got, tc.provider)
		}
		var items []map[string]json.RawMessage
		if err := json.Unmarshal(env["items"], &items); err != nil || len(items) == 0 {
			t.Fatalf("%s items = %s (err %v)", tc.path, env["items"], err)
		}
		it := items[0]
		if got := string(it["images"]); got == "" || got == "null" {
			t.Errorf("%s item.images = %q, want a JSON array", tc.path, got)
		}
		_, hasExtra := it["extra"]
		if hasExtra != tc.wantExtra {
			t.Errorf("%s item has extra = %v, want %v (item = %s)", tc.path, hasExtra, tc.wantExtra, env["items"])
		}
	}
}
