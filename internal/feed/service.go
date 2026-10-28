package feed

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/yilong-musk/feedmesh/internal/cache"
	"github.com/yilong-musk/feedmesh/internal/guard"
	"github.com/yilong-musk/feedmesh/internal/logx"
	"github.com/yilong-musk/feedmesh/internal/smzdm"
	"github.com/yilong-musk/feedmesh/internal/sspai"
	"github.com/yilong-musk/feedmesh/internal/v2ex"
	"github.com/yilong-musk/feedmesh/internal/weibo"
	"github.com/yilong-musk/feedmesh/internal/xnitter"
	"github.com/yilong-musk/feedmesh/internal/youtube"
)

// Limits and cache lifetimes.
const (
	// DefaultLimit is the number of posts returned when the caller is silent.
	DefaultLimit = 10
	// MaxLimit is the weibo container page size: one getIndex call cannot
	// return more than ten posts.
	MaxLimit = 10
)

// TTLs configures per-kind cache lifetimes.
type TTLs struct {
	Profile  time.Duration
	Timeline time.Duration
	LongText time.Duration
	Domain   time.Duration
}

// DefaultTTLs returns the shipped cache policy.
func DefaultTTLs() TTLs {
	return TTLs{
		Profile:  time.Hour,
		Timeline: 5 * time.Minute,
		LongText: 24 * time.Hour,
		Domain:   24 * time.Hour,
	}
}

// Service coordinates the upstream client and the on-disk cache.
// Clients bundles the upstream providers a Service can serve.
type Clients struct {
	Weibo   *weibo.Client
	Sspai   *sspai.Client
	V2ex    *v2ex.Client
	X       *xnitter.Client
	Smzdm   *smzdm.Client
	Youtube *youtube.Client
}

type Service struct {
	clients Clients
	cache   *cache.Cache
	log     *logx.Logger
	group   *guard.Group
	ttls    TTLs
	now     func() time.Time

	listMu    sync.Mutex
	listLocks map[string]*keyedMutex

	// fenleiMu serialises category-page reads across slugs: the upstream budget
	// is one page per hour per slug and the site only tolerates one connection
	// at a time, so two slugs never run concurrently.
	fenleiMu sync.Mutex
}

// keyedMutex serialises cache reads and writes for one list key. Locks are
// reference counted so the map does not grow with every account ever requested.
type keyedMutex struct {
	mu   sync.Mutex
	refs int
}

// lockListKey blocks until the caller owns the lock for key and returns the
// matching unlock function.
func (s *Service) lockListKey(key string) func() {
	s.listMu.Lock()
	if s.listLocks == nil {
		s.listLocks = make(map[string]*keyedMutex)
	}
	lock := s.listLocks[key]
	if lock == nil {
		lock = &keyedMutex{}
		s.listLocks[key] = lock
	}
	lock.refs++
	s.listMu.Unlock()

	lock.mu.Lock()
	return func() {
		lock.mu.Unlock()
		s.listMu.Lock()
		lock.refs--
		if lock.refs == 0 && s.listLocks[key] == lock {
			delete(s.listLocks, key)
		}
		s.listMu.Unlock()
	}
}

// NewService wires a Service.
func NewService(clients Clients, c *cache.Cache, log *logx.Logger, ttls TTLs) *Service {
	if log == nil {
		log = logx.Discard()
	}
	if ttls == (TTLs{}) {
		ttls = DefaultTTLs()
	}
	return &Service{
		clients: clients,
		cache:   c,
		log:     log,
		group:   guard.NewGroup(),
		ttls:    ttls,
		now:     time.Now,
	}
}

// ResolveUID accepts either a numeric uid or a vanity domain and returns a uid.
// refresh bypasses the domain->uid cache so "--refresh" is honest end to end.
func (s *Service) ResolveUID(ctx context.Context, target string, refresh bool) (string, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return "", &weibo.Error{Code: weibo.CodeBadRequest, Message: "uid or domain is required"}
	}
	if err := weibo.ValidateUID(target); err == nil {
		return target, nil
	}
	if err := weibo.ValidateDomain(target); err != nil {
		return "", err
	}
	uid, err := s.resolveDomain(ctx, target, refresh)
	if err != nil {
		return "", err
	}
	return uid, nil
}

func (s *Service) resolveDomain(ctx context.Context, domain string, refresh bool) (string, error) {
	uid, _, err := fetchCached(ctx, s, "domain:"+strings.ToLower(domain), s.ttls.Domain, refresh,
		func() (string, error) { return s.clients.Weibo.ResolveDomain(ctx, domain) })
	return uid, err
}

// GetFeed builds a feed. refresh bypasses the cache (and overwrites it).
func (s *Service) GetFeed(ctx context.Context, uid string, limit int, refresh bool) (*Feed, error) {
	uid = strings.TrimSpace(uid)
	if err := weibo.ValidateUID(uid); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = DefaultLimit
	}
	if limit > MaxLimit {
		limit = MaxLimit
	}

	profile, profileHit, err := fetchCached(ctx, s, "profile:"+uid, s.ttls.Profile, refresh,
		func() (*weibo.Profile, error) { return s.clients.Weibo.Profile(ctx, uid) })
	if err != nil {
		return nil, err
	}

	timeline, timelineHit, err := fetchCached(ctx, s, "timeline:"+uid, s.ttls.Timeline, refresh,
		func() (*weibo.Timeline, error) {
			return s.clients.Weibo.Timeline(ctx, uid, profile.ContainerID, 0)
		})
	if err != nil {
		return nil, err
	}

	posts := timeline.Items
	if len(posts) > limit {
		posts = posts[:limit]
	}
	// A timeline returned by single-flight is shared with every waiter. Hydrate
	// mutates MBlog.Text, so copy the selected posts before filling in long text.
	posts = cloneTimeline(posts)

	f := &Feed{
		UID:         uid,
		ScreenName:  profile.ScreenName,
		Description: profile.Description,
		FetchedAt:   s.now().UTC(),
		Cached:      profileHit && timelineHit,
		Items:       make([]Item, 0, len(posts)),
	}
	for _, m := range posts {
		s.hydrate(ctx, uid, m, refresh, 0)
		f.Items = append(f.Items, s.toItem(uid, m))
	}
	return f, nil
}

// cloneTimeline copies the posts and the quoted-retweet chain so a caller can
// mutate the result (long-text hydration) without touching a timeline shared
// by concurrent single-flight waiters.
func cloneTimeline(items []*weibo.MBlog) []*weibo.MBlog {
	out := make([]*weibo.MBlog, len(items))
	for i, m := range items {
		out[i] = cloneMBlog(m)
	}
	return out
}

func cloneMBlog(m *weibo.MBlog) *weibo.MBlog {
	if m == nil {
		return nil
	}
	c := *m
	if m.Pics != nil {
		c.Pics = append([]weibo.Pic(nil), m.Pics...)
	}
	if m.User != nil {
		u := *m.User
		c.User = &u
	}
	if m.PageInfo != nil {
		p := *m.PageInfo
		c.PageInfo = &p
	}
	c.Retweeted = cloneMBlog(m.Retweeted)
	return &c
}

// hydrate fills in full text for long posts, recursing into the quoted retweet.
// Long-text failures are tolerated: the truncated text is still usable.
func (s *Service) hydrate(ctx context.Context, uid string, m *weibo.MBlog, refresh bool, depth int) {
	if m == nil || depth > 1 {
		return
	}
	if m.IsLongText {
		id := m.IDString()
		if id != "" {
			// Include the timeline owner so `cache purge --uid` can remove
			// long-text bodies even after the timeline entry has expired.
			key := "longtext:" + uid + ":" + id
			text, _, err := fetchCached(ctx, s, key, s.ttls.LongText, refresh,
				func() (string, error) { return s.clients.Weibo.LongText(ctx, id) })
			switch {
			case err == nil && strings.TrimSpace(text) != "":
				m.Text = text
			case err != nil:
				fallback, fbErr := s.clients.Weibo.StatusText(ctx, id)
				if fbErr == nil && strings.TrimSpace(fallback) != "" {
					m.Text = fallback
				} else {
					s.log.Warn("long text unavailable, keeping truncated text",
						"id", id, "error", err.Error(), "fallback_error", errorString(fbErr))
				}
			}
		}
	}
	s.hydrate(ctx, uid, m.Retweeted, refresh, depth+1)
}

func (s *Service) toItem(uid string, m *weibo.MBlog) Item {
	created, ok := ParseCreatedAt(m.CreatedAt)
	if !ok {
		created = s.now().UTC()
		s.log.Debug("unparsable created_at", "id", m.IDString(), "raw", m.CreatedAt)
	}
	it := Item{
		ID:        m.IDString(),
		BID:       m.BID,
		CreatedAt: created.UTC(),
		Text:      HTMLToText(m.Text),
		HTML:      BuildDescription(m),
		URL:       safeURLString(ItemURL(uid, m)),
	}
	for _, p := range m.Pics {
		if u := safeURLString(p.BestURL()); u != "" {
			it.Images = append(it.Images, u)
		}
	}
	if it.Images == nil {
		it.Images = []string{}
	}
	if pi := m.PageInfo; pi != nil {
		if pi.IsVideo() {
			it.VideoURL = safeURLString(pi.BestVideoURL())
		}
		it.PageURL = safeURLString(pi.Link())
	}
	it.Title = ItemTitle(m.StatusTitle, it.Text)
	return it
}

// ItemURL builds the canonical permalink for a post. weibo.com permalinks need
// the base-62 bid; without one we fall back to the detail page.
func ItemURL(uid string, m *weibo.MBlog) string {
	if m == nil {
		return ""
	}
	if bid := m.BID; bid != "" && !strings.ContainsAny(bid, "/ ") {
		return fmt.Sprintf("https://weibo.com/%s/%s", uid, bid)
	}
	if id := m.IDString(); id != "" {
		return "https://m.weibo.cn/detail/" + id
	}
	return ""
}

// createdLayouts covers weibo's "Fri Oct 23 14:30:04 +0800 2026" timestamp.
var createdLayouts = []string{
	"Mon Jan _2 15:04:05 -0700 2006",
	"Mon Jan 02 15:04:05 -0700 2006",
	time.RFC1123Z,
}

// ParseCreatedAt parses a weibo created_at string.
func ParseCreatedAt(raw string) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, false
	}
	for _, layout := range createdLayouts {
		if t, err := time.Parse(layout, raw); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// fetchCached reads through the cache, collapsing concurrent identical fetches.
func fetchCached[T any](ctx context.Context, s *Service, key string, ttl time.Duration, refresh bool,
	fn func() (T, error)) (T, bool, error) {

	if !refresh && s.cache != nil {
		var v T
		if hit, _, err := s.cache.Get(key, &v); err == nil && hit {
			return v, true, nil
		}
	}

	var zero T
	value, err, _ := guard.Do(s.group, key, func() (T, error) {
		if !refresh && s.cache != nil {
			var v T
			if hit, _, err := s.cache.Get(key, &v); err == nil && hit {
				return v, nil
			}
		}
		val, ferr := fn()
		if ferr != nil {
			return zero, ferr
		}
		if s.cache != nil {
			if cerr := s.cache.Set(key, val, ttl); cerr != nil {
				s.log.Warn("cache write failed", "key", key, "error", cerr.Error())
			}
		}
		return val, nil
	})
	if err != nil {
		return zero, false, err
	}
	return value, false, nil
}

// cachedList is the on-disk shape of a paginated list. Want records how many
// entries the fetch asked upstream for, so a short list reads as "upstream had
// nothing more" instead of "somebody asked for fewer".
type cachedList[T any] struct {
	Items []T `json:"items"`
	Want  int `json:"want"`
}

// maxListFetchWidth is the largest list width any provider exposes today
// (V2EX). It caps the width carried over from an old cache entry so a forged
// or legacy value cannot make a refresh fetch an unbounded list.
const maxListFetchWidth = 50

// covers reports whether the entry can answer a request for n entries.
func (c cachedList[T]) covers(n int) bool {
	return c.Want >= n || len(c.Items) >= n
}

// readCachedEntry returns the entry for key without checking its width.
func readCachedEntry[T any](s *Service, key string) (cachedList[T], bool) {
	if s.cache == nil {
		return cachedList[T]{}, false
	}
	var e cachedList[T]
	if hit, _, err := s.cache.Get(key, &e); err == nil && hit {
		return e, true
	}
	return cachedList[T]{}, false
}

// readCachedList returns the entry for key when it can satisfy want.
func readCachedList[T any](s *Service, key string, want int) (cachedList[T], bool) {
	e, ok := readCachedEntry[T](s, key)
	if !ok || !e.covers(want) {
		return cachedList[T]{}, false
	}
	return e, true
}

// fetchCachedList reads a paginated list through the cache and returns at
// least want entries, or as many as upstream has.
//
// A plain read-through entry lets the first caller size the cache: one
// ?limit=3 request would then truncate every larger request for the whole TTL.
// So the entry remembers the width it was fetched at (Want) and only answers
// requests it can actually satisfy; a wider request fetches again and widens
// the entry. The common small-limit path keeps its single upstream page while
// a large limit still gets everything available.
func fetchCachedList[T any](ctx context.Context, s *Service, key string, ttl time.Duration, refresh bool, want int,
	fn func(n int) ([]T, error)) ([]T, bool, error) {

	if want < 1 {
		want = 1
	}
	// Cache read/widen/write must be atomic per key. With only single-flight
	// keyed by width, a narrow and a wide request could both miss, then write
	// in the wrong order and truncate the wider entry (refresh used to skip the
	// wider-entry merge entirely).
	unlock := s.lockListKey(key)
	defer unlock()

	if refresh {
		// Refresh deliberately bypasses the old values, but keep the cache at
		// least as wide as the entry being replaced. Otherwise a small-limit
		// refresh would truncate a wider cache that another reader just warmed.
		// The caller still only sees its requested limit; every item in the
		// refreshed range comes from upstream.
		if prev, ok := readCachedEntry[T](s, key); ok {
			width := prev.Want
			if len(prev.Items) > width {
				width = len(prev.Items)
			}
			if width > want {
				want = width
				if want > maxListFetchWidth {
					want = maxListFetchWidth
				}
			}
		}
	}
	if !refresh {
		if e, ok := readCachedList[T](s, key, want); ok {
			return e.Items, true, nil
		}
	}

	// Different widths get their own flight: a narrow request must never
	// answer (and so silently truncate) a wide one that joins it.
	flight := key + "#" + strconv.Itoa(want)
	e, err, _ := guard.Do(s.group, flight, func() (cachedList[T], error) {
		if !refresh {
			if cached, ok := readCachedList[T](s, key, want); ok {
				return cached, nil
			}
		}
		items, ferr := fn(want)
		if ferr != nil {
			return cachedList[T]{}, ferr
		}
		cur := cachedList[T]{Items: items, Want: want}
		// Another request may have just stored a wider list; keep the widest.
		// An explicit refresh must not be discarded in favour of the old entry:
		// the caller asked for fresh upstream data and we just fetched it.
		if !refresh {
			if prev, ok := readCachedList[T](s, key, want); ok && len(prev.Items) > len(cur.Items) {
				return prev, nil
			}
		}
		if s.cache != nil {
			if cerr := s.cache.Set(key, cur, ttl); cerr != nil {
				s.log.Warn("cache write failed", "key", key, "error", cerr.Error())
			}
		}
		return cur, nil
	})
	if err != nil {
		return nil, false, err
	}
	return e.Items, false, nil
}
