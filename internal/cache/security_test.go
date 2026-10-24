package cache

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGCRemovesExpiredManagedEntries(t *testing.T) {
	c, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Set("timeline:1", "x", 10*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	time.Sleep(30 * time.Millisecond)
	removed, err := c.GC()
	if err != nil {
		t.Fatal(err)
	}
	if removed != 1 {
		t.Fatalf("GC removed %d entries, want 1", removed)
	}
	if st, _ := c.Stat(); st.Entries != 0 {
		t.Fatalf("entries after GC = %d", st.Entries)
	}
}

func TestStatIgnoresUnmanagedJSON(t *testing.T) {
	dir := t.TempDir()
	c, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Set("profile:1", "p", time.Hour); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "user-notes.json"), []byte(`{"private":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	st, err := c.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if st.Entries != 1 {
		t.Fatalf("Stat counted an unrelated file: %+v", st)
	}
	list, err := c.List()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, ki := range list {
		if ki.Key == "user-notes" {
			found = true
		}
	}
	if !found {
		t.Fatal("List should still be able to see the file for explicit filtering")
	}
}

func TestGCIgnoresUnrelatedPrefixedFiles(t *testing.T) {
	dir := t.TempDir()
	c, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"x_notes.json":   `{"name":"notes"}`,
		"x_corrupt.json": `{not json`,
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	removed, err := c.GC()
	if err != nil {
		t.Fatal(err)
	}
	if removed != 0 {
		t.Fatalf("GC removed %d unrelated files", removed)
	}
	for name := range files {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("GC removed unrelated %s: %v", name, err)
		}
	}
}

func TestGetDoesNotConsumeForeignOrCorruptFiles(t *testing.T) {
	dir := t.TempDir()
	c, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"x_notes.json":   `{"name":"notes"}`,
		"x_corrupt.json": `{not json`,
		"x_spoof.json":   `{"key":"x_other","value":{"name":"spoof"}}`,
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	for _, key := range []string{"x:notes", "x:corrupt", "x:spoof"} {
		var out any
		hit, _, err := c.Get(key, &out)
		if err != nil {
			t.Fatalf("Get(%q): %v", key, err)
		}
		if hit {
			t.Errorf("Get(%q) treated an unowned file as a cache hit: %+v", key, out)
		}
	}
	for name := range files {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("Get deleted unowned %s: %v", name, err)
		}
	}
}

func TestPeekVerifiesEntryOwnership(t *testing.T) {
	dir := t.TempDir()
	c, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	body := `{"key":"x_other","value":{"name":"spoof"}}`
	if err := os.WriteFile(filepath.Join(dir, "x_spoof.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	var out map[string]string
	if hit, err := c.Peek("x:spoof", &out); err != nil || hit {
		t.Fatalf("Peek on an unowned entry = hit %v, err %v", hit, err)
	}
}

func TestPeekStoredReadsExpiredEntry(t *testing.T) {
	c, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Set("timeline:1", "old", time.Millisecond); err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)

	var fresh string
	if hit, err := c.Peek("timeline:1", &fresh); err != nil || hit {
		t.Fatalf("Peek on expired entry = hit %v, err %v", hit, err)
	}
	var stored string
	if hit, err := c.PeekStored("timeline:1", &stored); err != nil || !hit {
		t.Fatalf("PeekStored on expired entry = hit %v, err %v", hit, err)
	}
	if stored != "old" {
		t.Fatalf("PeekStored value = %q, want old", stored)
	}
}

func TestListManagedVerifiesEntryKey(t *testing.T) {
	dir := t.TempDir()
	c, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Set("profile:42", "p", time.Hour); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "x_notes.json"), []byte(`{"name":"notes"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	list, err := c.ListManaged()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Key != "profile_42" {
		t.Fatalf("ListManaged = %+v, want only profile_42", list)
	}
}

func TestNewRejectsSymlinkCacheDir(t *testing.T) {
	target := t.TempDir()
	link := filepath.Join(t.TempDir(), "cache-link")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := New(link); err == nil {
		t.Fatal("New accepted a symlinked cache directory")
	}
}
