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
	unrelated := filepath.Join(dir, "user-notes.json")
	if err := os.WriteFile(unrelated, []byte(`{"private":true}`), 0o600); err != nil {
		t.Fatal(err)
	}

	var out, errBuf bytes.Buffer
	if code := run([]string{"cache", "purge", "--all", "--json", "--cache-dir", dir}, &out, &errBuf); code != exitOK {
		t.Fatalf("exit = %d, stderr=%s", code, errBuf.String())
	}
	if _, err := os.Stat(unrelated); err != nil {
		t.Fatalf("purge --all deleted an unrelated file: %v", err)
	}
	var v any
	if hit, _, _ := c.Get("profile:1", &v); hit {
		t.Fatal("managed cache was not purged")
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
