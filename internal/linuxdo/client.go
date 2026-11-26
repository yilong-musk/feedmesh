// Package linuxdo reads LINUX DO (linux.do) topics through the forum's
// official Discourse RSS feeds.
//
// The public site sits behind Cloudflare. RSS paths are readable anonymously,
// while the JSON and HTML paths answer a managed challenge; once a challenge
// fires it also applies to RSS for several minutes. Two rules keep this client
// usable: request only .rss paths, and keep a cookie jar so Cloudflare's
// visitor cookie survives between calls. A global read slot and a cooldown
// after a challenge keep a burst from extending the block.
package linuxdo

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/yilong-musk/feedmesh/internal/logx"
	"github.com/yilong-musk/feedmesh/internal/upstream"
)

// DefaultBaseURL is the public forum. Tests point BaseURL at an httptest server.
const DefaultBaseURL = "https://linux.do"

const (
	// DefaultLimit is the number of topics returned when the caller is silent.
	DefaultLimit = 30
	// MaxLimit is the widest answer this package will assemble: three pages.
	MaxLimit = 90

	// pageSize is the usual Discourse topic-feed page. Category feeds carry 25.
	pageSize         = 30
	categoryPageSize = 25
	maxPages         = 3

	// minInterval is the global spacing between upstream requests. LINUX DO is
	// a large forum behind Cloudflare; one request every five seconds keeps a
	// multi-feed refresh from looking like a scrape.
	minInterval = 5 * time.Second
	// challengeCooldown stops every feed after Cloudflare answers a challenge.
	// The observed block lasts about six to seven minutes.
	challengeCooldown = 10 * time.Minute
	userAgent         = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"
)

// Errors callers map onto HTTP responses.
var (
	// ErrInvalidRef means the feed reference is malformed.
	ErrInvalidRef = errors.New("invalid linux.do feed reference")
	// ErrNotFound means the forum has no such category, tag or feed.
	ErrNotFound = errors.New("linux.do feed not found")
	// ErrChallenge means Cloudflare answered a managed challenge (or the
	// cooldown that follows one is still active).
	ErrChallenge = errors.New("linux.do is challenging automated requests")
	// ErrNoItems means the feed parsed but carried no topic. Treating it as an
	// error keeps the last good list instead of caching an empty feed.
	ErrNoItems = errors.New("linux.do feed has no topics")
	// ErrUnexpectedPage means the response was not an RSS document.
	ErrUnexpectedPage = errors.New("linux.do response is not an RSS feed")
)

// Kind is the feed family.
type Kind string

const (
	KindLatest   Kind = "latest"
	KindTop      Kind = "top"
	KindHot      Kind = "hot"
	KindCategory Kind = "category"
	KindTag      Kind = "tag"
)

// Periods are the accepted values for KindTop.
var Periods = []string{"daily", "weekly", "monthly", "quarterly", "yearly", "all"}

var (
	periodSet = func() map[string]bool {
		m := make(map[string]bool, len(Periods))
		for _, p := range Periods {
			m[p] = true
		}
		return m
	}()

	slugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)
	idRe   = regexp.MustCompile(`^[0-9]{1,12}$`)
	tagRe  = regexp.MustCompile(`^[\p{L}\p{N}._-]{1,64}$`)
	imgRe  = regexp.MustCompile(`(?i)<img\b[^>]*\bsrc="([^"]+)"`)
)

// Ref identifies one feed.
type Ref struct {
	Kind   Kind
	Period string // Top only
	Order  string // Latest only: created (default) or activity
	Slug   string // Category only; empty means the redirect form /c/-/<id>
	ID     string // Category only
	Tag    string // Tag only
}

// Validate normalises a Ref and rejects values the forum cannot serve.
func (r Ref) Validate() (Ref, error) {
	r.Period = strings.ToLower(strings.TrimSpace(r.Period))
	r.Order = strings.ToLower(strings.TrimSpace(r.Order))
	r.Slug = strings.ToLower(strings.TrimSpace(r.Slug))
	r.ID = strings.TrimSpace(r.ID)
	r.Tag = strings.ToLower(strings.TrimSpace(r.Tag))

	switch r.Kind {
	case KindLatest:
		if r.Order == "" {
			r.Order = "created"
		}
		if r.Order != "created" && r.Order != "activity" {
			return r, ErrInvalidRef
		}
	case KindTop:
		if r.Period == "" {
			r.Period = "daily"
		}
		if !periodSet[r.Period] {
			return r, ErrInvalidRef
		}
	case KindHot:
	case KindCategory:
		if r.Slug != "" && !slugRe.MatchString(r.Slug) {
			return r, ErrInvalidRef
		}
		if !idRe.MatchString(r.ID) {
			return r, ErrInvalidRef
		}
	case KindTag:
		if !tagRe.MatchString(r.Tag) {
			return r, ErrInvalidRef
		}
	default:
		return r, ErrInvalidRef
	}
	return r, nil
}

// Key is the cache key and UID for the feed.
func (r Ref) Key() string {
	switch r.Kind {
	case KindLatest:
		return "latest:" + r.Order
	case KindTop:
		return "top:" + r.Period
	case KindHot:
		return "hot"
	case KindCategory:
		slug := r.Slug
		if slug == "" {
			slug = "-"
		}
		return "category:" + slug + ":" + r.ID
	case KindTag:
		return "tag:" + r.Tag
	}
	return string(r.Kind)
}

// Config configures a Client.
type Config struct {
	// BaseURL overrides the forum host for tests.
	BaseURL string
	// Timeout bounds one whole Fetch call.
	Timeout time.Duration
	// MinInterval is the global spacing between requests (default 5s).
	MinInterval time.Duration
	// Cooldown is how long every feed pauses after a challenge (default 10m).
	Cooldown time.Duration
	// Log reports lifecycle events; nil is silent.
	Log *logx.Logger
}

// Client reads LINUX DO RSS feeds. It is safe for concurrent use; requests
// serialize through a global read slot.
type Client struct {
	cfg  Config
	http *http.Client

	minInterval time.Duration
	cooldown    time.Duration

	mu            sync.Mutex
	lastRequest   time.Time
	cooldownUntil time.Time
	// requestSlot keeps one request in flight; a queued caller leaves when
	// its own deadline passes instead of waiting behind a slow page.
	requestSlot upstream.Slot
}

// New builds a Client.
func New(cfg Config) *Client {
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultBaseURL
	}
	if cfg.MinInterval <= 0 {
		cfg.MinInterval = minInterval
	}
	if cfg.Cooldown <= 0 {
		cfg.Cooldown = challengeCooldown
	}
	jar, _ := cookiejar.New(nil)
	return &Client{
		requestSlot: upstream.NewSlot(1),
		cfg:         cfg,
		minInterval: cfg.MinInterval,
		cooldown:    cfg.Cooldown,
		http: &http.Client{
			Jar:     jar,
			Timeout: cfg.Timeout,
		},
	}
}

// Budget reports the per-call deadline this client expects.
func (c *Client) Budget() time.Duration { return c.cfg.Timeout }

// Close releases idle connections.
func (c *Client) Close() { c.http.CloseIdleConnections() }

func (c *Client) logf(msg string, kv ...any) {
	if c.cfg.Log != nil {
		c.cfg.Log.Info(msg, kv...)
	}
}

// Topic is one forum topic as the RSS feed describes it. The body is the first
// post plus Discourse's "read the full topic" footer.
type Topic struct {
	ID        string
	Title     string
	URL       string
	Author    string
	Category  string
	HTML      string
	CreatedAt time.Time
	Pinned    bool
	Closed    bool
	Archived  bool
	Images    []string
}

// Feed is one parsed RSS document.
type Feed struct {
	Kind      Kind
	Channel   string
	Link      string
	Language  string
	FetchedAt time.Time
	Topics    []Topic
}

// Fetch reads a feed and returns up to limit topics. Topic feeds are paged at
// 25-30 items; the client follows pages until it has enough or the feed ends.
func (c *Client) Fetch(ctx context.Context, raw Ref, limit int) (*Feed, error) {
	ref, err := raw.Validate()
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = DefaultLimit
	}
	if limit > MaxLimit {
		limit = MaxLimit
	}

	perPage := pageSize
	if ref.Kind == KindCategory {
		perPage = categoryPageSize
	}
	topics := make([]Topic, 0, limit)
	seen := make(map[string]bool, limit)
	var first *Feed
	for page := 1; page <= maxPages && len(topics) < limit; page++ {
		doc, ferr := c.fetchPage(ctx, ref, page)
		if ferr != nil {
			return nil, ferr
		}
		if first == nil {
			first = doc
		}
		added := 0
		for _, t := range doc.Topics {
			if t.ID == "" || seen[t.ID] {
				continue
			}
			seen[t.ID] = true
			topics = append(topics, t)
			added++
			if len(topics) >= limit {
				break
			}
		}
		if added == 0 || len(doc.Topics) < perPage {
			break
		}
	}
	if len(topics) == 0 {
		return nil, ErrNoItems
	}
	// Top, hot and latest-by-activity are rankings and must keep the order the
	// forum returned. Latest-by-creation, category and tag feeds are time lists
	// and are sorted newest first.
	byTime := ref.Kind == KindCategory || ref.Kind == KindTag ||
		(ref.Kind == KindLatest && ref.Order == "created")
	if byTime {
		sort.SliceStable(topics, func(i, j int) bool {
			if topics[i].CreatedAt.Equal(topics[j].CreatedAt) {
				return topics[i].ID > topics[j].ID
			}
			return topics[i].CreatedAt.After(topics[j].CreatedAt)
		})
	}
	if len(topics) > limit {
		topics = topics[:limit]
	}
	first.Topics = topics
	first.FetchedAt = time.Now().UTC()
	c.logf("linux.do feed read", "feed", ref.Key(), "topics", len(topics))
	return first, nil
}

// fetchPage reads one RSS page.
func (c *Client) fetchPage(ctx context.Context, ref Ref, page int) (*Feed, error) {
	body, err := c.get(ctx, c.pageURL(ref, page))
	if err != nil {
		return nil, err
	}
	return parseFeed(ref.Kind, body)
}

// pageURL builds a feed URL. Only .rss paths are ever requested: JSON and HTML
// URLs answer a Cloudflare challenge and would take the RSS paths down with
// them.
func (c *Client) pageURL(ref Ref, page int) string {
	base := strings.TrimRight(c.cfg.BaseURL, "/")
	var path string
	switch ref.Kind {
	case KindLatest:
		path = "/latest.rss"
	case KindTop:
		path = "/top.rss"
	case KindHot:
		path = "/hot.rss"
	case KindCategory:
		slug := ref.Slug
		if slug == "" {
			slug = "-"
		}
		path = "/c/" + url.PathEscape(slug) + "/" + url.PathEscape(ref.ID) + ".rss"
	case KindTag:
		path = "/tag/" + url.PathEscape(ref.Tag) + ".rss"
	}
	q := url.Values{}
	if page > 1 {
		q.Set("page", strconv.Itoa(page))
	}
	switch ref.Kind {
	case KindLatest:
		if ref.Order != "" {
			q.Set("order", ref.Order)
		}
	case KindTop:
		q.Set("period", ref.Period)
	}
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	return base + path
}

// get performs one paced request. A challenge sets the global cooldown; no
// retry is attempted because retrying extends the block.
func (c *Client) get(ctx context.Context, target string) ([]byte, error) {
	// One request in flight and a gap measured from the previous completion;
	// this is stricter than spacing start times and keeps a slow page from
	// overlapping the next feed.
	if err := c.requestSlot.Acquire(ctx); err != nil {
		return nil, err
	}
	defer c.requestSlot.Release()
	if err := c.acquire(ctx); err != nil {
		return nil, err
	}
	defer c.markRequest()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/rss+xml, application/xml;q=0.9, text/xml;q=0.8, */*;q=0.5")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := upstream.ReadBody(resp.Body, 8<<20)
	if err != nil {
		return nil, err
	}
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return nil, ErrNotFound
	case http.StatusForbidden, http.StatusTooManyRequests:
		c.challenge("status", resp.StatusCode)
		return nil, ErrChallenge
	default:
		if resp.Header.Get("cf-mitigated") != "" {
			c.challenge("status", resp.StatusCode)
			return nil, ErrChallenge
		}
		return nil, fmt.Errorf("linux.do feed status %d", resp.StatusCode)
	}
	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	if strings.Contains(ct, "html") || !strings.Contains(string(body), "<rss") {
		if strings.Contains(strings.ToLower(string(body)), "just a moment") {
			c.challenge("content-type", ct)
			return nil, ErrChallenge
		}
		return nil, ErrUnexpectedPage
	}
	return body, nil
}

// acquire reserves the single global read slot, leaving minInterval between
// requests.
func (c *Client) acquire(ctx context.Context) error {
	c.mu.Lock()
	now := time.Now()
	if now.Before(c.cooldownUntil) {
		until := c.cooldownUntil
		c.mu.Unlock()
		return fmt.Errorf("%w until %s", ErrChallenge, until.Format(time.RFC3339))
	}
	wait := c.minInterval - now.Sub(c.lastRequest)
	if wait < 0 {
		wait = 0
	}
	c.mu.Unlock()
	if wait == 0 {
		return nil
	}
	t := time.NewTimer(wait)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// markRequest records the instant a request finished.
func (c *Client) markRequest() {
	c.mu.Lock()
	c.lastRequest = time.Now()
	c.mu.Unlock()
}

// challenge opens the cooldown window. The log line is the audit trail for a
// feed that suddenly serves stale content.
func (c *Client) challenge(kv ...any) {
	c.mu.Lock()
	c.cooldownUntil = time.Now().Add(c.cooldown)
	until := c.cooldownUntil
	c.mu.Unlock()
	if c.cfg.Log != nil {
		c.cfg.Log.Warn("linux.do challenge detected; pausing all feeds",
			append(kv, "until", until.Format(time.RFC3339))...)
	}
}

// rssDoc is the subset of the Discourse RSS document this client needs.
type rssDoc struct {
	Channel struct {
		Title       string    `xml:"title"`
		Links       []rssLink `xml:"link"`
		Description string    `xml:"description"`
		Language    string    `xml:"language"`
		Items       []rssItem `xml:"item"`
	} `xml:"channel"`
}

type rssLink struct {
	Rel  string `xml:"rel,attr"`
	Href string `xml:"href,attr"`
	Text string `xml:",chardata"`
}

type rssItem struct {
	Title       string    `xml:"title"`
	Creator     string    `xml:"http://purl.org/dc/elements/1.1/ creator"`
	Category    string    `xml:"category"`
	Description string    `xml:"description"`
	Links       []rssLink `xml:"link"`
	PubDate     string    `xml:"pubDate"`
	Pinned      string    `xml:"http://www.discourse.org/ topicPinned"`
	Closed      string    `xml:"http://www.discourse.org/ topicClosed"`
	Archived    string    `xml:"http://www.discourse.org/ topicArchived"`
}

// parseFeed converts one RSS document into a Feed. Exported for tests.
func parseFeed(kind Kind, body []byte) (*Feed, error) {
	var doc rssDoc
	if err := xml.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("linux.do rss parse: %w", err)
	}
	if doc.Channel.Title == "" && len(doc.Channel.Items) == 0 {
		return nil, ErrUnexpectedPage
	}
	out := &Feed{
		Kind:     kind,
		Channel:  strings.TrimSpace(doc.Channel.Title),
		Link:     feedLink(doc.Channel.Links),
		Language: strings.TrimSpace(doc.Channel.Language),
	}
	for _, it := range doc.Channel.Items {
		link := feedLink(it.Links)
		id := topicID(link)
		if link == "" && id != "" {
			link = "https://linux.do/t/topic/" + id
		}
		t := Topic{
			ID:       id,
			Title:    strings.TrimSpace(it.Title),
			URL:      link,
			Author:   strings.TrimSpace(it.Creator),
			Category: strings.TrimSpace(it.Category),
			HTML:     strings.TrimSpace(it.Description),
			Pinned:   yes(it.Pinned),
			Closed:   yes(it.Closed),
			Archived: yes(it.Archived),
		}
		t.CreatedAt = parsePubDate(it.PubDate)
		t.Images = feedImages(t.HTML)
		out.Topics = append(out.Topics, t)
	}
	return out, nil
}

func yes(s string) bool { return strings.EqualFold(strings.TrimSpace(s), "yes") }

// feedLink picks the human URL from a list of <link> elements. Discourse
// channel documents carry an atom:link (rel=self) before the plain link, so
// the first element is not always the permalink.
func feedLink(links []rssLink) string {
	// The plain <link>text</link> is the human page. Atom alternates often
	// point at a Markdown mirror (.md) and are only a fallback.
	for _, l := range links {
		if t := strings.TrimSpace(l.Text); t != "" {
			return t
		}
	}
	for _, l := range links {
		if l.Rel == "alternate" && strings.TrimSpace(l.Href) != "" {
			return strings.TrimSpace(l.Href)
		}
	}
	for _, l := range links {
		if strings.TrimSpace(l.Href) != "" {
			return strings.TrimSpace(l.Href)
		}
	}
	return ""
}

// topicID extracts the numeric topic id from a permalink such as
// https://linux.do/t/topic/3191226 or https://linux.do/t/slug/3191226?page=2.
func topicID(link string) string {
	u, err := url.Parse(strings.TrimSpace(link))
	if err != nil {
		return ""
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	for i := len(parts) - 1; i >= 0; i-- {
		if idRe.MatchString(parts[i]) {
			return parts[i]
		}
	}
	return ""
}

// parsePubDate accepts the three layouts Discourse uses across feeds.
func parsePubDate(s string) time.Time {
	s = strings.TrimSpace(s)
	for _, layout := range []string{time.RFC1123Z, time.RFC1123, time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

// feedImages returns the real photos of a post. Emoji are rendered as <img>
// tags too; they are not feed images.
func feedImages(html string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, m := range imgRe.FindAllStringSubmatch(html, -1) {
		u := strings.TrimSpace(m[1])
		if u == "" || strings.Contains(u, "/images/emoji/") {
			continue
		}
		if strings.HasPrefix(u, "//") {
			u = "https:" + u
		}
		if seen[u] {
			continue
		}
		seen[u] = true
		out = append(out, u)
	}
	return out
}
