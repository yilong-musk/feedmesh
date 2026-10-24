// Package cache is a file-per-key JSON cache with atomic writes.
//
// One key maps to one file in the cache directory. Files are written to a
// temporary sibling and renamed into place so a reader never observes a
// half-written entry, and each entry carries its own expiry timestamp so the
// on-disk format is self-describing.
package cache

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// ErrInvalidKey is returned for keys that sanitise to an empty filename.
var ErrInvalidKey = errors.New("cache: invalid key")

// Entry is the on-disk representation of one cached value.
type Entry struct {
	Key       string          `json:"key"`
	StoredAt  time.Time       `json:"stored_at"`
	ExpiresAt time.Time       `json:"expires_at"`
	Value     json.RawMessage `json:"value"`
}

// Stats summarises the cache directory.
type Stats struct {
	Dir     string `json:"dir"`
	Entries int    `json:"entries"`
	Bytes   int64  `json:"bytes"`
}

// Cache stores JSON values under a directory.
type Cache struct {
	dir string
	mu  sync.Mutex // serialises writes so temp-file naming cannot collide
}

// New creates (if needed) and returns a cache rooted at dir. Expiry sweeping is
// explicit (GC) so read-only commands such as `cache purge --dry-run` never
// delete anything.
func New(dir string) (*Cache, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, errors.New("cache: empty directory")
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("cache: resolve dir: %w", err)
	}
	if fi, err := os.Lstat(abs); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("cache: %s is a symlink; refusing to use it", abs)
	}
	if err := os.MkdirAll(abs, 0o700); err != nil {
		return nil, fmt.Errorf("cache: create dir: %w", err)
	}
	// Tighten a pre-existing directory too; the cache contains subscription
	// identifiers even though all content is public.
	_ = os.Chmod(abs, 0o700)
	c := &Cache{dir: abs}
	return c, nil
}

// managedPrefixes lists the file stems this package may own. It is a cheap
// filter for stats and candidates; deletion paths also verify the Entry.Key
// stored in the file so a shared cache directory cannot lose unrelated JSON
// that happens to share a prefix.
var managedPrefixes = []string{
	"profile_", "timeline_", "longtext_", "domain_", "hot_",
	"sspai_list", "sspai_detail_", "v2ex_latest", "v2ex_hot",
	"x_", "feed_",
}

// ManagedKey reports whether a cache file stem matches a feedmesh key prefix.
// It is intentionally cheap; callers that delete should use ListManaged or
// verify the entry with managedEntry.
func ManagedKey(key string) bool {
	for _, prefix := range managedPrefixes {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}

// managedEntry reports whether e is a feedmesh entry whose stored key maps to
// the same filename stem. Entry.Key is written by Cache.Set, so unrelated JSON
// files (including corrupt ones) are not mistaken for cache entries.
func managedEntry(e Entry, stem string) bool {
	safe := SafeKey(e.Key)
	return safe != "" && safe == stem && ManagedKey(safe)
}

func (c *Cache) readEntry(key string) (Entry, bool) {
	e, _, _, ok, err := c.readStored(key)
	if err != nil {
		return Entry{}, false
	}
	return e, ok
}

// readStored loads the entry for key without checking expiry. raw is the exact
// file contents, so a caller can remove the file only when it still matches.
// Corrupt JSON is reported as a miss with ok=false; raw is still returned so
// callers can distinguish it from a missing file without deleting it.
func (c *Cache) readStored(key string) (e Entry, raw []byte, p string, ok bool, err error) {
	p, err = c.path(key)
	if err != nil {
		return Entry{}, nil, "", false, err
	}
	raw, err = os.ReadFile(p)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Entry{}, nil, p, false, nil
		}
		return Entry{}, nil, p, false, err
	}
	if err := json.Unmarshal(raw, &e); err != nil {
		return Entry{}, raw, p, false, nil
	}
	return e, raw, p, true, nil
}

// entryMatches reports whether e was written by this cache for key. A shared
// cache directory can contain unrelated JSON files whose names happen to map
// to one of our keys; only trust a file whose Entry.Key maps back to the same
// filename.
func entryMatches(e Entry, key string) bool {
	safe := SafeKey(key)
	return safe != "" && SafeKey(e.Key) == safe
}

// ListManaged enumerates entries that verify as feedmesh cache entries. Files
// that merely share a managed filename prefix are ignored, so callers can use
// the result for deletion in a shared cache directory.
func (c *Cache) ListManaged() ([]KeyInfo, error) {
	list, err := c.List()
	if err != nil {
		return nil, err
	}
	out := make([]KeyInfo, 0, len(list))
	for _, ki := range list {
		if !ManagedKey(ki.Key) {
			continue
		}
		e, ok := c.readEntry(ki.Key)
		if !ok || !managedEntry(e, ki.Key) {
			continue
		}
		out = append(out, ki)
	}
	return out, nil
}

// GC removes expired entries that verify as feedmesh cache entries, plus stale
// temporary files left by an interrupted Set. Corrupt or foreign files are left
// alone: without a parseable Entry there is no reliable way to prove ownership.
// Cache.Get still removes a known key when it reads it.
func (c *Cache) GC() (int, error) {
	// Serialise with writers so an expired entry cannot be removed after a
	// concurrent Set has already renamed a fresh value into place.
	c.mu.Lock()
	defer c.mu.Unlock()
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return 0, nil
		}
		return 0, err
	}
	var removed int
	for _, de := range entries {
		if de.IsDir() {
			continue
		}
		name := de.Name()
		// Set writes to <key>.json.tmp<random> before rename. A process killed
		// mid-write leaves one behind; List and the normal expiry path ignore
		// them, so sweep stale ones here. The grace period protects a write
		// that is still in progress in another process.
		if i := strings.Index(name, ".json.tmp"); i > 0 && !strings.HasSuffix(name, ".json") {
			if !ManagedKey(name[:i]) {
				continue
			}
			info, err := de.Info()
			if err != nil || time.Since(info.ModTime()) < time.Hour {
				continue
			}
			if os.Remove(filepath.Join(c.dir, name)) == nil {
				removed++
			}
			continue
		}
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		stem := strings.TrimSuffix(name, ".json")
		if !ManagedKey(stem) {
			continue
		}
		e, raw, p, ok, rerr := c.readStored(stem)
		if rerr != nil || !ok || !managedEntry(e, stem) {
			continue
		}
		if !e.ExpiresAt.IsZero() && !time.Now().Before(e.ExpiresAt) {
			// Another process may have replaced the expired file since we read
			// it. Delete only the exact bytes we judged expired, otherwise a
			// concurrent Set would lose its fresh entry.
			if removeFileIfUnchanged(p, raw) {
				removed++
			}
		}
	}
	return removed, nil
}

// Dir returns the absolute cache directory.
func (c *Cache) Dir() string { return c.dir }

// SafeKey maps an arbitrary key onto a filesystem-safe name.
func SafeKey(key string) string {
	var b strings.Builder
	for _, r := range key {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	out := strings.Trim(b.String(), ".")
	return out
}

func (c *Cache) path(key string) (string, error) {
	name := SafeKey(key)
	if name == "" {
		return "", ErrInvalidKey
	}
	return filepath.Join(c.dir, name+".json"), nil
}

// removeFileIfUnchanged removes p only when its contents still match raw. A
// concurrent writer (even in another process) may have renamed a fresh entry
// into the same path in between; an unconditional remove would delete it.
func removeFileIfUnchanged(p string, raw []byte) bool {
	current, err := os.ReadFile(p)
	if err != nil || !bytes.Equal(current, raw) {
		return false
	}
	return os.Remove(p) == nil
}

// removeIfUnchanged is the in-process locked variant of removeFileIfUnchanged.
func (c *Cache) removeIfUnchanged(p string, raw []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	_ = removeFileIfUnchanged(p, raw)
}

// Get decodes a fresh entry into out. The boolean reports a hit; expired and
// unreadable entries are misses, and expired files are removed best-effort.
// Files that do not verify as an entry for key are left untouched.
func (c *Cache) Get(key string, out any) (bool, time.Time, error) {
	e, raw, p, ok, err := c.readStored(key)
	if err != nil {
		return false, time.Time{}, err
	}
	if !ok || !entryMatches(e, key) {
		return false, time.Time{}, nil
	}
	if !e.ExpiresAt.IsZero() && !time.Now().Before(e.ExpiresAt) {
		c.removeIfUnchanged(p, raw)
		return false, e.StoredAt, nil
	}
	if out != nil {
		if err := json.Unmarshal(e.Value, out); err != nil {
			c.removeIfUnchanged(p, raw)
			return false, e.StoredAt, nil
		}
	}
	return true, e.StoredAt, nil
}

// Peek decodes a fresh entry without deleting anything. It is used by
// read-only paths such as `cache purge --dry-run`, where even an expired file
// must remain untouched.
func (c *Cache) Peek(key string, out any) (bool, error) {
	return c.peek(key, out, false)
}

// PeekStored decodes an entry regardless of its expiry, without deleting it.
// Maintenance paths use it to inspect entries that are about to be purged even
// after they have expired. Ownership is still verified.
func (c *Cache) PeekStored(key string, out any) (bool, error) {
	return c.peek(key, out, true)
}

func (c *Cache) peek(key string, out any, includeExpired bool) (bool, error) {
	e, _, _, ok, err := c.readStored(key)
	if err != nil {
		return false, err
	}
	if !ok || !entryMatches(e, key) {
		return false, nil
	}
	if !includeExpired && !e.ExpiresAt.IsZero() && !time.Now().Before(e.ExpiresAt) {
		return false, nil
	}
	if out != nil {
		if err := json.Unmarshal(e.Value, out); err != nil {
			return false, nil
		}
	}
	return true, nil
}

// Set encodes v and writes it atomically with the given time-to-live.
func (c *Cache) Set(key string, v any, ttl time.Duration) error {
	p, err := c.path(key)
	if err != nil {
		return err
	}
	value, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("cache: encode %s: %w", key, err)
	}
	now := time.Now().UTC()
	e := Entry{Key: key, StoredAt: now, Value: value}
	if ttl > 0 {
		e.ExpiresAt = now.Add(ttl)
	}
	buf, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("cache: encode entry %s: %w", key, err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	tmp, err := os.CreateTemp(filepath.Dir(p), filepath.Base(p)+".tmp*")
	if err != nil {
		return fmt.Errorf("cache: temp file: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}
	if _, err := tmp.Write(buf); err != nil {
		cleanup()
		return fmt.Errorf("cache: write %s: %w", key, err)
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return fmt.Errorf("cache: sync %s: %w", key, err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("cache: close %s: %w", key, err)
	}
	if err := os.Rename(tmpName, p); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("cache: rename %s: %w", key, err)
	}
	return nil
}

// Delete removes one key. The boolean reports whether an entry actually
// existed, so callers can report accurate purge counts.
func (c *Cache) Delete(key string) (bool, error) {
	p, err := c.path(key)
	if err != nil {
		return false, err
	}
	if err := os.Remove(p); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// KeyInfo is one cached key with its on-disk size. Key is the sanitised file
// stem, which is what Delete accepts back.
type KeyInfo struct {
	Key   string `json:"key"`
	Bytes int64  `json:"bytes"`
}

// List enumerates every .json file, sorted by key. Callers that delete files
// should use ListManaged (or verify the Entry.Key) rather than trusting the
// filename prefix alone.
func (c *Cache) List() ([]KeyInfo, error) {
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	out := make([]KeyInfo, 0, len(entries))
	for _, de := range entries {
		if de.IsDir() || !strings.HasSuffix(de.Name(), ".json") {
			continue
		}
		info, err := de.Info()
		if err != nil {
			continue
		}
		out = append(out, KeyInfo{
			Key:   strings.TrimSuffix(de.Name(), ".json"),
			Bytes: info.Size(),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

// Stat walks the directory and reports entry count and byte size. It uses the
// cheap filename filter: files that do not match a feedmesh prefix are ignored,
// but a lookalike that shares a prefix may still be counted. Deletion paths use
// ListManaged, which verifies the Entry.Key in the file.
func (c *Cache) Stat() (Stats, error) {
	st := Stats{Dir: c.dir}
	list, err := c.List()
	if err != nil {
		return st, err
	}
	for _, ki := range list {
		if !ManagedKey(ki.Key) {
			continue
		}
		st.Entries++
		st.Bytes += ki.Bytes
	}
	return st, nil
}
