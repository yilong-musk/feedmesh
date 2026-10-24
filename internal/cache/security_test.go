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
