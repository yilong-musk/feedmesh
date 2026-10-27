package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yilong-musk/feedmesh/internal/cache"
)

func TestParseArgsMarksExplicitTimeout(t *testing.T) {
	_, _, o, err := parseArgs([]string{"fetch", "1", "--timeout", "2s"})
	if err != nil {
		t.Fatal(err)
	}
	if !o.timeoutSet {
		t.Fatal("explicit --timeout must be distinguished from the default")
	}
	_, _, o, err = parseArgs([]string{"fetch", "1"})
	if err != nil {
		t.Fatal(err)
	}
	if o.timeoutSet {
		t.Fatal("default timeout must not look explicit")
	}
}

func TestProviderTimeoutsClampX(t *testing.T) {
	cases := []struct {
		name        string
		opts        *options
		wantSspai   time.Duration
		wantV2ex    time.Duration
		wantX       time.Duration
		wantSmzdm   time.Duration
		wantYouTube time.Duration
	}{
		{"defaults", &options{}, 15 * time.Second, 15 * time.Second, 20 * time.Second, 15 * time.Second, 20 * time.Second},
		{"explicit", &options{timeout: 2 * time.Second, timeoutSet: true}, 2 * time.Second, 2 * time.Second, 2 * time.Second, 2 * time.Second, 2 * time.Second},
		{"x capped", &options{timeout: 60 * time.Second, timeoutSet: true}, 60 * time.Second, 60 * time.Second, 25 * time.Second, 60 * time.Second, 60 * time.Second},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sspaiTimeout, v2exTimeout, xTimeout, smzdmTimeout, youtubeTimeout := providerTimeouts(tc.opts)
			if sspaiTimeout != tc.wantSspai || v2exTimeout != tc.wantV2ex || xTimeout != tc.wantX ||
				smzdmTimeout != tc.wantSmzdm || youtubeTimeout != tc.wantYouTube {
				t.Fatalf("timeouts = %v/%v/%v/%v/%v, want %v/%v/%v/%v/%v",
					sspaiTimeout, v2exTimeout, xTimeout, smzdmTimeout, youtubeTimeout,
					tc.wantSspai, tc.wantV2ex, tc.wantX, tc.wantSmzdm, tc.wantYouTube)
			}
		})
	}
}

func TestCachePurgeUIDRemovesLongTextAndDomainAliases(t *testing.T) {
	dir := t.TempDir()
	c, err := cache.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	mustSet := func(key string, v any) {
		t.Helper()
		if err := c.Set(key, v, time.Hour); err != nil {
			t.Fatal(err)
		}
	}
	mustSet("profile:42", map[string]any{"screen_name": "x"})
	mustSet("timeline:42", map[string]any{"items": []any{
		map[string]any{"id": "1001", "isLongText": true},
	}})
	mustSet("longtext:1001", "old long body")
	mustSet("longtext:42:1002", "owned long body")
	mustSet("domain:alias", "42")
	mustSet("domain:other", "99")

	var out, errBuf bytes.Buffer
	if code := run([]string{"cache", "purge", "--uid", "42", "--json", "--cache-dir", dir}, &out, &errBuf); code != exitOK {
		t.Fatalf("exit = %d, stderr=%s", code, errBuf.String())
	}
	for _, key := range []string{"profile:42", "timeline:42", "longtext:1001", "longtext:42:1002", "domain:alias"} {
		var v any
		if hit, _, err := c.Get(key, &v); err != nil || hit {
			t.Errorf("%s still cached (hit=%v err=%v)", key, hit, err)
		}
	}
	var v any
	if hit, _, err := c.Get("domain:other", &v); err != nil || !hit {
		t.Errorf("unrelated domain entry was removed (hit=%v err=%v)", hit, err)
	}
}

func TestCachePurgeHandleIsScoped(t *testing.T) {
	dir := t.TempDir()
	c, err := cache.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	_ = c.Set("x:elonmusk", "one", time.Hour)
	_ = c.Set("x:other", "two", time.Hour)

	var out, errBuf bytes.Buffer
	if code := run([]string{"cache", "purge", "--handle", "@elonmusk", "--json", "--cache-dir", dir}, &out, &errBuf); code != exitOK {
		t.Fatalf("exit = %d, stderr=%s", code, errBuf.String())
	}
	var v any
	if hit, _, _ := c.Get("x:elonmusk", &v); hit {
		t.Fatal("target X cache survived")
	}
	if hit, _, _ := c.Get("x:other", &v); !hit {
		t.Fatal("unrelated X cache was removed")
	}
}

func TestCachePurgeDryRunDoesNotSweepExpiredEntries(t *testing.T) {
	dir := t.TempDir()
	c, err := cache.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Set("timeline:1", "x", time.Millisecond); err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)
	before, err := c.List()
	if err != nil {
		t.Fatal(err)
	}

	var out, errBuf bytes.Buffer
	if code := run([]string{"cache", "purge", "--all", "--dry-run", "--json", "--cache-dir", dir}, &out, &errBuf); code != exitOK {
		t.Fatalf("exit = %d, stderr=%s", code, errBuf.String())
	}
	after, err := c.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before) {
		t.Fatalf("dry-run deleted entries: before=%d after=%d", len(before), len(after))
	}
}

func TestCachePurgeAllLeavesUnrelatedJSONAlone(t *testing.T) {
	dir := t.TempDir()
	c, err := cache.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	_ = c.Set("profile:1", "p", time.Hour)
	unrelated := map[string]string{
		"user-notes.json": `{"private":true}`,
		"x_notes.json":    `{"name":"notes"}`,
	}
	for name, body := range unrelated {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	var out, errBuf bytes.Buffer
	if code := run([]string{"cache", "purge", "--all", "--json", "--cache-dir", dir}, &out, &errBuf); code != exitOK {
		t.Fatalf("exit = %d, stderr=%s", code, errBuf.String())
	}
	for name := range unrelated {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("purge --all deleted unrelated %s: %v", name, err)
		}
	}
	var v any
	if hit, _, _ := c.Get("profile:1", &v); hit {
		t.Fatal("managed cache was not purged")
	}
}

func TestDefaultCacheDirFallbackIsCleanedUp(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("XDG_CACHE_HOME", "")
	dir, cleanup := defaultCacheDir()
	if cleanup == nil {
		t.Skip("UserCacheDir is available in this environment")
	}
	if dir == "" {
		t.Fatal("fallback directory is empty")
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("fallback directory was not created: %v", err)
	}
	cleanup()
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("cleanup left %s behind: %v", dir, err)
	}
}

func TestListenIsLoopback(t *testing.T) {
	for _, addr := range []string{"127.0.0.1:3000", "localhost:3000", "[::1]:3000"} {
		if !listenIsLoopback(addr) {
			t.Errorf("%s should be loopback", addr)
		}
	}
	for _, addr := range []string{":3000", "0.0.0.0:3000", "[::]:3000", "192.168.1.2:3000"} {
		if listenIsLoopback(addr) {
			t.Errorf("%s should not be loopback", addr)
		}
	}
}

func TestHelpMentionsTokenAndHandlePurge(t *testing.T) {
	var out, errBuf bytes.Buffer
	if code := run([]string{"help"}, &out, &errBuf); code != exitOK {
		t.Fatalf("exit = %d", code)
	}
	for _, want := range []string{"--token", "--rate-limit", "cache purge --handle"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("help is missing %q", want)
		}
	}
}
