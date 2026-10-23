// Package cache is a file-per-key JSON cache with atomic writes.
//
// One key maps to one file in the cache directory. Files are written to a
// temporary sibling and renamed into place so a reader never observes a
// half-written entry, and each entry carries its own expiry timestamp so the
// on-disk format is self-describing.
package cache

import (
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

// New creates (if needed) and returns a cache rooted at dir.
func New(dir string) (*Cache, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, errors.New("cache: empty directory")
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("cache: resolve dir: %w", err)
	}
	if err := os.MkdirAll(abs, 0o750); err != nil {
		return nil, fmt.Errorf("cache: create dir: %w", err)
	}
	return &Cache{dir: abs}, nil
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

// Get decodes a fresh entry into out. The boolean reports a hit; expired and
// unreadable entries are misses, and expired files are removed best-effort.
func (c *Cache) Get(key string, out any) (bool, time.Time, error) {
	p, err := c.path(key)
	if err != nil {
		return false, time.Time{}, err
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, time.Time{}, nil
		}
		return false, time.Time{}, err
	}
	var e Entry
	if err := json.Unmarshal(raw, &e); err != nil {
		// A corrupt entry is disposable: drop it and report a miss rather than
		// failing the caller's request.
		_ = os.Remove(p)
		return false, time.Time{}, nil
	}
	if !e.ExpiresAt.IsZero() && !time.Now().Before(e.ExpiresAt) {
		_ = os.Remove(p)
		return false, e.StoredAt, nil
	}
	if out != nil {
		if err := json.Unmarshal(e.Value, out); err != nil {
			_ = os.Remove(p)
			return false, e.StoredAt, nil
		}
	}
	return true, e.StoredAt, nil
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

// List enumerates cached entries, sorted by key.
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

// Stat walks the directory and reports entry count and byte size.
func (c *Cache) Stat() (Stats, error) {
	st := Stats{Dir: c.dir}
	list, err := c.List()
	if err != nil {
		return st, err
	}
	for _, ki := range list {
		st.Entries++
		st.Bytes += ki.Bytes
	}
	return st, nil
}
