package xnitter

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestPublicInstanceURLRejectsLocalTargets(t *testing.T) {
	for _, raw := range []string{
		"https://127.0.0.1:8443",
		"https://user@127.0.0.1:8443",
		"https://localhost",
		"https://localhost.",
		"https://localhost..",
		"https://localhost.:8443",
		"https://[::1]",
		"https://0x7f.0.0.1",
		"https://0177.0.0.1",
		"https://nitter.local",
		"https://nitter.local.",
		"https://127.0.0.1.nip.io",
		"https://10.0.0.1.sslip.io",
		"http://nitter.example",
	} {
		if _, err := publicInstanceURL(raw); err == nil {
			t.Errorf("publicInstanceURL(%q) unexpectedly accepted", raw)
		}
	}
	if got, err := publicInstanceURL("https://nitter.example./"); err != nil || got != "https://nitter.example" {
		t.Fatalf("public instance = %q, %v", got, err)
	}
}

func TestIsPublicIP(t *testing.T) {
	for _, raw := range []string{
		"127.0.0.1", "10.0.0.1", "172.16.0.1", "192.168.1.1",
		"169.254.169.254", "0.0.0.0", "0.1.2.3", "100.64.0.1",
		"192.0.2.1", "198.51.100.1", "203.0.113.1", "224.0.0.1",
		"::1", "fc00::1", "fe80::1", "ff02::1",
	} {
		if isPublicIP(net.ParseIP(raw)) {
			t.Errorf("isPublicIP(%q) = true", raw)
		}
	}
	for _, raw := range []string{"8.8.8.8", "1.1.1.1", "2606:4700:4700::1111"} {
		if !isPublicIP(net.ParseIP(raw)) {
			t.Errorf("isPublicIP(%q) = false", raw)
		}
	}
	if isPublicIP(nil) {
		t.Error("isPublicIP(nil) = true")
	}
}

func TestDialContextRejectsPrivateAddress(t *testing.T) {
	c := New(Config{Timeout: time.Second})
	// A proxy environment variable may legitimately mark loopback as trusted;
	// remove it so this test always exercises the directory-host path.
	delete(c.trusted, "127.0.0.1")
	if _, err := c.dialContext(context.Background(), "tcp", "127.0.0.1:443"); err == nil ||
		!strings.Contains(err.Error(), "non-public") {
		t.Fatalf("dialContext error = %v, want a non-public rejection", err)
	}
}

func TestSolvePoWRejectsHugeDifficultyAndData(t *testing.T) {
	if _, _, err := solvePoW(context.Background(), "x", maxDifficulty+1); err == nil {
		t.Fatal("expected difficulty limit error")
	}
	if _, _, err := solvePoW(context.Background(), strings.Repeat("x", maxRandomData+1), 1); err == nil {
		t.Fatal("expected large-data error")
	}
}

func TestRewriteLinksKeepsQueryAndMapsProxiedPictures(t *testing.T) {
	in := `<a href="https://n.example.test/search?f=tweets&amp;q=go">search</a>` +
		`<img src="https://n.example.test/pic/media%2Fabc.jpg"/>` +
		`<a href="/relative/status/1">relative</a>`
	out := rewriteLinks(in, "https://n.example.test")
	if strings.Contains(out, "n.example.test") {
		t.Fatalf("mirror host survived: %s", out)
	}
	if !strings.Contains(out, "https://x.com/search?f=tweets&amp;q=go") {
		t.Fatalf("query was not preserved/escaped: %s", out)
	}
	if !strings.Contains(out, "https://pbs.twimg.com/media/abc.jpg") {
		t.Fatalf("proxied image was not mapped: %s", out)
	}
	if !strings.Contains(out, "https://x.com/relative/status/1") {
		t.Fatalf("relative link was not absolutised: %s", out)
	}
}

func TestPreactRedirectMustStayOnInstance(t *testing.T) {
	c := New(Config{Timeout: time.Second})
	err := c.solve(context.Background(), "https://nitter.example", &challenge{
		kind: "preact", redir: "@127.0.0.1:8443/x", result: "deadbeef",
	}, "/")
	if err == nil || !strings.Contains(err.Error(), "unsafe") {
		t.Fatalf("expected unsafe redirect error, got %v", err)
	}
}

func TestStatusDirectoryFailureIsCached(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		http.Error(w, "down", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := New(Config{StatusURL: srv.URL, Timeout: time.Second, Instances: []string{"https://fallback.test"}})
	_ = c.instances(context.Background())
	_ = c.instances(context.Background())
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("status directory called %d times, want 1 with failure backoff", got)
	}
}

func TestFetchPoolRejectsPrivateHosts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"hosts": []map[string]any{
			{"url": "https://127.0.0.1:8443", "rss": true, "points": 99},
			{"url": "https://good.example", "rss": true, "points": 80},
			{"url": "https://user@127.0.0.1:8443", "rss": true, "points": 90},
		}})
	}))
	defer srv.Close()
	c := New(Config{StatusURL: srv.URL, Timeout: time.Second})
	pool, err := c.fetchPool(context.Background())
	if err != nil {
		t.Fatalf("fetchPool: %v", err)
	}
	if len(pool) != 1 || pool[0] != "https://good.example" {
		t.Fatalf("pool = %v", pool)
	}
}
