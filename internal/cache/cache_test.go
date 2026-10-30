package cache

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func newTestCache(t *testing.T) *Cache {
	t.Helper()
	c, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

// fakeClock pins c's clock to t; move *t forward to advance time.
func fakeClock(c *Cache, t *time.Time) {
	c.now = func() time.Time { return *t }
}

func TestSetGetRoundTrip(t *testing.T) {
	c := newTestCache(t)
	type payload struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	}
	want := payload{Name: "人民日报", Count: 10}
	if err := c.Set("profile:2803301701", want, time.Hour); err != nil {
		t.Fatalf("Set: %v", err)
	}

	var got payload
	hit, storedAt, err := c.Get("profile:2803301701", &got)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !hit {
		t.Fatal("expected a cache hit")
	}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	if storedAt.IsZero() {
		t.Fatal("stored_at must be recorded")
	}
}

func TestGetMissingIsMiss(t *testing.T) {
	c := newTestCache(t)
	var out map[string]any
	hit, _, err := c.Get("nope", &out)
	if err != nil {
		t.Fatalf("Get on missing key must not error: %v", err)
	}
	if hit {
		t.Fatal("missing key must be a miss")
	}
}

func TestExpiredEntryIsMissAndRemoved(t *testing.T) {
	c := newTestCache(t)
	now := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	fakeClock(c, &now)

	if err := c.Set("timeline:1", []string{"a"}, 30*time.Second); err != nil {
		t.Fatalf("Set: %v", err)
	}

	var out []string
	if hit, _, err := c.Get("timeline:1", &out); err != nil || !hit {
		t.Fatalf("expected a fresh hit, got hit=%v err=%v", hit, err)
	}

	// Move the cache clock past the TTL instead of sleeping: the check must not
	// depend on scheduling delays between Set and Get.
	now = now.Add(time.Minute)

	out = nil
	hit, _, err := c.Get("timeline:1", &out)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if hit {
		t.Fatal("expired entry must not be a hit")
	}
	if _, err := os.Stat(filepath.Join(c.Dir(), SafeKey("timeline:1")+".json")); !os.IsNotExist(err) {
		t.Fatalf("expired file should be removed, stat err = %v", err)
	}
}

func TestPeekHonoursTheClock(t *testing.T) {
	c := newTestCache(t)
	now := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	fakeClock(c, &now)

	if err := c.Set("timeline:2", []string{"b"}, 30*time.Second); err != nil {
		t.Fatalf("Set: %v", err)
	}

	var out []string
	if hit, err := c.Peek("timeline:2", &out); err != nil || !hit {
		t.Fatalf("expected a fresh peek hit, got hit=%v err=%v", hit, err)
	}

	// Read-only peeks must not depend on wall-clock time any more than Get does.
	now = now.Add(time.Minute)

	out = nil
	if hit, err := c.Peek("timeline:2", &out); err != nil || hit {
		t.Fatalf("expired entry must not peek as a hit, got hit=%v err=%v", hit, err)
	}

	hit, err := c.PeekStored("timeline:2", &out)
	if err != nil || !hit {
		t.Fatalf("PeekStored must still read an expired entry, got hit=%v err=%v", hit, err)
	}
	if len(out) != 1 || out[0] != "b" {
		t.Fatalf("PeekStored decoded %v, want [b]", out)
	}
	if _, err := os.Stat(filepath.Join(c.Dir(), SafeKey("timeline:2")+".json")); err != nil {
		t.Fatalf("Peek must leave the expired file in place: %v", err)
	}
}

func TestWriteIsAtomicAndLeavesNoTempFiles(t *testing.T) {
	c := newTestCache(t)
	for i := 0; i < 20; i++ {
		if err := c.Set("feed:1", map[string]int{"n": i}, time.Hour); err != nil {
			t.Fatalf("Set: %v", err)
		}
	}
	entries, err := os.ReadDir(c.Dir())
	if err != nil {
		t.Fatal(err)
	}
	for _, de := range entries {
		if strings.Contains(de.Name(), ".tmp") {
			t.Fatalf("temp file left behind: %s", de.Name())
		}
	}
	if len(entries) != 1 {
		t.Fatalf("got %d files, want exactly 1", len(entries))
	}

	// The file on disk must always be complete, valid JSON.
	raw, err := os.ReadFile(filepath.Join(c.Dir(), SafeKey("feed:1")+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var e Entry
	if err := json.Unmarshal(raw, &e); err != nil {
		t.Fatalf("on-disk entry is not valid JSON: %v", err)
	}
	if e.Key != "feed:1" || e.ExpiresAt.IsZero() {
		t.Fatalf("entry = %+v, want key and expires_at", e)
	}
}

func TestConcurrentWritesStayValid(t *testing.T) {
	c := newTestCache(t)
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = c.Set("feed:2803301701", map[string]int{"n": i}, time.Hour)
		}(i)
	}
	wg.Wait()

	raw, err := os.ReadFile(filepath.Join(c.Dir(), SafeKey("feed:2803301701")+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var e Entry
	if err := json.Unmarshal(raw, &e); err != nil {
		t.Fatalf("concurrent writes corrupted the entry: %v", err)
	}
}

func TestStatListAndDelete(t *testing.T) {
	c := newTestCache(t)
	_ = c.Set("profile:1", "p", time.Hour)
	_ = c.Set("feed:1:10", "f", time.Hour)
	_ = c.Set("timeline:1", "t", time.Hour)
	_ = c.Set("profile:2", "p2", time.Hour)

	st, err := c.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if st.Entries != 4 || st.Bytes == 0 {
		t.Fatalf("stat = %+v, want 4 entries with bytes", st)
	}

	list, err := c.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 4 {
		t.Fatalf("list = %d entries, want 4", len(list))
	}
	for i := 1; i < len(list); i++ {
		if list[i-1].Key > list[i].Key {
			t.Fatalf("list is not sorted: %v", list)
		}
	}
	for _, ki := range list {
		if strings.Contains(ki.Key, ":") {
			t.Fatalf("key %q was not sanitised", ki.Key)
		}
		if ki.Bytes == 0 {
			t.Fatalf("key %q has zero size", ki.Key)
		}
	}

	// An exact-name delete must not touch a neighbouring uid.
	ok, err := c.Delete("profile:1")
	if err != nil || !ok {
		t.Fatalf("Delete = %v, %v; want true, nil", ok, err)
	}
	ok, err = c.Delete("profile:1")
	if err != nil || ok {
		t.Fatalf("second Delete = %v, %v; want false, nil", ok, err)
	}
	if st, _ := c.Stat(); st.Entries != 3 {
		t.Fatalf("entries = %d after delete, want 3", st.Entries)
	}
	var survivors []string
	for _, ki := range mustList(t, c) {
		survivors = append(survivors, ki.Key)
	}
	if !contains(survivors, "profile_2") {
		t.Fatalf("deleting profile:1 also removed profile:2: %v", survivors)
	}
}

// TestEveryProviderPrefixIsManaged guards the maintenance paths: a key that no
// prefix matches is invisible to Stat, List, GC and purge, so its entries stay
// on disk forever and /healthz undercounts. Every provider's key shape belongs
// in this list.
func TestEveryProviderPrefixIsManaged(t *testing.T) {
	keys := []string{
		"profile:2803301701",
		"timeline:2803301701",
		"longtext:2803301701:Q4rX",
		"domain:testdomain",
		"hot:top",
		"sspai_list",
		"sspai_detail:12345",
		"v2ex_latest",
		"v2ex_hot",
		"x:handle",
		"feed:2803301701:10",
		"youtube:feed:UC_x5XG1OV2P6uZZ5FSM9Ttw",
		"youtube:channel:@googledevelopers",
		"smzdm:deals",
		"smzdm:fenlei:tixuhuli",
		"reddit:list:programming:hot:day",
	}
	for _, key := range keys {
		// ManagedKey takes the filename stem, so the key goes through SafeKey
		// exactly as the maintenance paths pass it.
		if stem := SafeKey(key); !ManagedKey(stem) {
			t.Errorf("ManagedKey(SafeKey(%q)) = false (%q): Stat, List, GC and purge would ignore this entry", key, stem)
		}
	}

	c := newTestCache(t)
	now := time.Now()
	c.now = func() time.Time { return now }
	for _, key := range keys {
		if err := c.Set(key, "value", time.Minute); err != nil {
			t.Fatalf("Set(%q): %v", key, err)
		}
	}
	if st, err := c.Stat(); err != nil || st.Entries != len(keys) {
		t.Fatalf("Stat = %+v, %v; want %d entries", st, err, len(keys))
	}

	now = now.Add(2 * time.Minute)
	removed, err := c.GC()
	if err != nil {
		t.Fatalf("GC: %v", err)
	}
	if removed != len(keys) {
		t.Fatalf("GC removed %d expired entries, want %d", removed, len(keys))
	}
}

func mustList(t *testing.T, c *Cache) []KeyInfo {
	t.Helper()
	list, err := c.List()
	if err != nil {
		t.Fatal(err)
	}
	return list
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

func TestSafeKeySanitises(t *testing.T) {
	cases := map[string]string{
		"feed:2803301701": "feed_2803301701",
		// Leading dots are trimmed so a key can never name a traversal path.
		"../escape":      "_escape",
		"a/b":            "a_b",
		"UPPER-case_1.2": "UPPER-case_1.2",
		"":               "",
		"中文":             "__",
	}
	for in, want := range cases {
		if got := SafeKey(in); got != want {
			t.Errorf("SafeKey(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestInvalidKeyRejected(t *testing.T) {
	c := newTestCache(t)
	if err := c.Set("", 1, time.Hour); err == nil {
		t.Fatal("empty key must be rejected")
	}
	if _, _, err := c.Get("", nil); err == nil {
		t.Fatal("empty key get must be rejected")
	}
}

func TestCorruptEntryIsTreatedAsMiss(t *testing.T) {
	c := newTestCache(t)
	p := filepath.Join(c.Dir(), SafeKey("bad")+".json")
	if err := os.WriteFile(p, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	hit, _, err := c.Get("bad", &out)
	if err != nil {
		t.Fatalf("corrupt entry must not error out: %v", err)
	}
	if hit {
		t.Fatal("corrupt entry must be a miss")
	}
}

func TestGCCleansStaleTempFiles(t *testing.T) {
	c := newTestCache(t)
	oldPath := filepath.Join(c.Dir(), "profile_1.json.tmpold")
	freshPath := filepath.Join(c.Dir(), "profile_2.json.tmpfresh")
	for _, p := range []string{oldPath, freshPath} {
		if err := os.WriteFile(p, []byte("partial"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(oldPath, old, old); err != nil {
		t.Fatal(err)
	}

	removed, err := c.GC()
	if err != nil {
		t.Fatalf("GC: %v", err)
	}
	if removed != 1 {
		t.Fatalf("GC removed %d files, want 1 stale temp file", removed)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("stale temp file survived: %v", err)
	}
	if _, err := os.Stat(freshPath); err != nil {
		t.Fatalf("fresh temp file was removed: %v", err)
	}
}

func TestRemoveIfUnchangedKeepsReplacedFile(t *testing.T) {
	c := newTestCache(t)
	key := "profile:1"
	p := filepath.Join(c.Dir(), SafeKey(key)+".json")
	old := []byte("old contents")
	if err := os.WriteFile(p, old, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := c.Set(key, "fresh", time.Hour); err != nil {
		t.Fatal(err)
	}

	// Simulate Get having read the old file, then a concurrent Set replacing it.
	c.removeIfUnchanged(p, old)

	var got string
	hit, _, err := c.Get(key, &got)
	if err != nil || !hit || got != "fresh" {
		t.Fatalf("fresh entry was lost: hit=%v err=%v got=%q", hit, err, got)
	}
}
