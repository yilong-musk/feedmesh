package cache

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Two Cache values on one directory stand in for the server and a CLI run.

// settleFile moves key's modification time past mtimeSafetyWindow so the next
// read promotes it into the memory layer without sleeping.
func settleFile(t testing.TB, dir, key string) {
	t.Helper()
	p := filepath.Join(dir, SafeKey(key)+".json")
	past := time.Now().Add(-time.Second)
	if err := os.Chtimes(p, past, past); err != nil {
		t.Fatalf("chtimes %s: %v", p, err)
	}
}

func TestMemorySeesAWriteFromAnotherProcess(t *testing.T) {
	dir := t.TempDir()
	server, _ := New(dir)
	cli, _ := New(dir)

	if err := server.Set("timeline:1", "v1", time.Hour); err != nil {
		t.Fatal(err)
	}
	settleFile(t, dir, "timeline:1")
	var got string
	if hit, _, _ := server.Get("timeline:1", &got); !hit || got != "v1" {
		t.Fatalf("first read = %v %q", hit, got)
	}
	if server.mem.len() != 1 {
		t.Fatalf("memory entries = %d, want 1", server.mem.len())
	}

	if err := cli.Set("timeline:1", "v2-from-the-cli", time.Hour); err != nil {
		t.Fatal(err)
	}
	if hit, _, _ := server.Get("timeline:1", &got); !hit || got != "v2-from-the-cli" {
		t.Fatalf("read after a foreign write = %v %q, want the new value", hit, got)
	}
}

func TestMemorySeesAPurgeFromAnotherProcess(t *testing.T) {
	dir := t.TempDir()
	server, _ := New(dir)
	cli, _ := New(dir)

	if err := server.Set("timeline:1", "v1", time.Hour); err != nil {
		t.Fatal(err)
	}
	settleFile(t, dir, "timeline:1")
	var got string
	if hit, _, _ := server.Get("timeline:1", &got); !hit {
		t.Fatal("expected a hit before the purge")
	}
	if server.mem.len() != 1 {
		t.Fatalf("memory entries = %d, want 1", server.mem.len())
	}
	if ok, err := cli.Delete("timeline:1"); err != nil || !ok {
		t.Fatalf("Delete = %v, %v", ok, err)
	}
	if hit, _, _ := server.Get("timeline:1", &got); hit {
		t.Fatal("a purged entry was still served from memory")
	}
}

func TestMemoryIgnoresAFileRewrittenInPlace(t *testing.T) {
	c := newTestCache(t)
	if err := c.Set("timeline:1", "v1", time.Hour); err != nil {
		t.Fatal(err)
	}
	var got string
	_, _, _ = c.Get("timeline:1", &got)

	// Same-size in-place rewrite immediately after write (within the coarse
	// filesystem mtime tick): inode, size and mtime can all match, so the
	// entry must not have been cached yet.
	p := filepath.Join(c.Dir(), "timeline_1.json")
	raw, _ := os.ReadFile(p)
	sameSize := strings.Replace(string(raw), `"v1"`, `"v2"`, 1)
	if len(sameSize) != len(raw) {
		t.Fatalf("expected same-size rewrite, got %d vs %d", len(sameSize), len(raw))
	}
	if err := os.WriteFile(p, []byte(sameSize), 0o600); err != nil {
		t.Fatal(err)
	}
	if hit, _, _ := c.Get("timeline:1", &got); !hit || got != "v2" {
		t.Fatalf("read after a same-tick same-size rewrite = %v %q, want v2", hit, got)
	}

	// Once mtime settles and the entry is promoted to memory, a later same-size
	// in-place rewrite advances mtime and is still seen.
	settleFile(t, c.Dir(), "timeline:1")
	if hit, _, _ := c.Get("timeline:1", &got); !hit || got != "v2" {
		t.Fatalf("settled read = %v %q", hit, got)
	}
	if c.mem.len() != 1 {
		t.Fatalf("memory entries = %d, want 1", c.mem.len())
	}
	sameSize2 := strings.Replace(sameSize, `"v2"`, `"v3"`, 1)
	if err := os.WriteFile(p, []byte(sameSize2), 0o600); err != nil {
		t.Fatal(err)
	}
	if hit, _, _ := c.Get("timeline:1", &got); !hit || got != "v3" {
		t.Fatalf("read after a settled in-place rewrite = %v %q, want v3", hit, got)
	}
}

func TestMemoryStaysWithinItsBudget(t *testing.T) {
	m := newMemory(64 << 10)
	info, err := os.Stat(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	raw := make([]byte, 4<<10)
	for i := 0; i < 100; i++ {
		m.put(fmt.Sprintf("/k%d", i), info, raw, Entry{})
	}
	if m.used > m.max {
		t.Fatalf("used %d bytes, budget %d", m.used, m.max)
	}
	if _, ok := m.get("/k99"); !ok {
		t.Fatal("the newest entry was evicted")
	}
	if _, ok := m.get("/k0"); ok {
		t.Fatal("the oldest entry survived past the budget")
	}

	big := make([]byte, m.max/2)
	m.put("/big", info, big, Entry{})
	if _, ok := m.get("/big"); ok {
		t.Fatal("an entry larger than a quarter of the budget was kept")
	}
}

func TestGCDoesNotFillMemory(t *testing.T) {
	c := newTestCache(t)
	other, _ := New(c.Dir())
	for i := 0; i < 5; i++ {
		if err := other.Set(fmt.Sprintf("timeline:%d", i), i, time.Hour); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := c.GC(); err != nil {
		t.Fatal(err)
	}
	if n := c.mem.len(); n != 0 {
		t.Fatalf("GC loaded %d entries into memory", n)
	}
}

func BenchmarkGetHotKey(b *testing.B) {
	payload := make([]map[string]string, 20)
	for i := range payload {
		payload[i] = map[string]string{"id": fmt.Sprint(i), "html": strings.Repeat("<p>body</p>", 200)}
	}
	for _, mode := range []struct {
		name string
		mem  bool
	}{{"disk", false}, {"memory", true}} {
		b.Run(mode.name, func(b *testing.B) {
			c, err := New(b.TempDir())
			if err != nil {
				b.Fatal(err)
			}
			if !mode.mem {
				c.mem = nil
			}
			if err := c.Set("timeline:1", payload, time.Hour); err != nil {
				b.Fatal(err)
			}
			settleFile(b, c.Dir(), "timeline:1")
			var warm []map[string]string
			if hit, _, _ := c.Get("timeline:1", &warm); !hit {
				b.Fatal("warm miss")
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				var out []map[string]string
				if hit, _, _ := c.Get("timeline:1", &out); !hit {
					b.Fatal("miss")
				}
			}
		})
	}
}
