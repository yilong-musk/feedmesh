package cache

import (
	"container/list"
	"os"
	"sync"
)

// defaultMemoryBudget bounds the in-process copy of recently read entries.
// The disk stays the source of truth (and what the CLI and the server share);
// this layer only saves the read and the outer JSON parse on a hot key.
const defaultMemoryBudget = 32 << 20

// memEntryOverhead approximates the bookkeeping cost of one entry.
const memEntryOverhead = 256

// memEntry is a parsed cache file together with the identity of the file it
// was read from. The identity is what lets a hit be trusted: every write
// renames a new file into place, so a changed (or purged) entry shows up as a
// different inode, size or modification time.
type memEntry struct {
	path  string
	info  os.FileInfo
	raw   []byte
	entry Entry
	size  int
}

// memory is a byte-bounded LRU of parsed entries keyed by file path.
type memory struct {
	mu    sync.Mutex
	max   int
	used  int
	ll    *list.List
	items map[string]*list.Element
}

func newMemory(max int) *memory {
	return &memory{max: max, ll: list.New(), items: make(map[string]*list.Element)}
}

func (m *memory) get(path string) (*memEntry, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	el, ok := m.items[path]
	if !ok {
		return nil, false
	}
	m.ll.MoveToFront(el)
	return el.Value.(*memEntry), true
}

func (m *memory) put(path string, info os.FileInfo, raw []byte, e Entry) {
	size := len(raw) + len(e.Value) + memEntryOverhead
	if info == nil || size > m.max/4 {
		m.drop(path)
		return
	}
	me := &memEntry{path: path, info: info, raw: raw, entry: e, size: size}
	m.mu.Lock()
	defer m.mu.Unlock()
	if el, ok := m.items[path]; ok {
		m.used -= el.Value.(*memEntry).size
		el.Value = me
		m.ll.MoveToFront(el)
	} else {
		m.items[path] = m.ll.PushFront(me)
	}
	m.used += size
	for m.used > m.max {
		last := m.ll.Back()
		if last == nil {
			break
		}
		old := last.Value.(*memEntry)
		m.ll.Remove(last)
		delete(m.items, old.path)
		m.used -= old.size
	}
}

func (m *memory) drop(path string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if el, ok := m.items[path]; ok {
		m.used -= el.Value.(*memEntry).size
		m.ll.Remove(el)
		delete(m.items, path)
	}
}

func (m *memory) len() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.ll.Len()
}

// sameFile reports whether two stats describe the same file contents as far
// as an atomically renamed cache file can change: same inode, size and
// modification time.
func sameFile(a, b os.FileInfo) bool {
	return os.SameFile(a, b) && a.Size() == b.Size() && a.ModTime().Equal(b.ModTime())
}
