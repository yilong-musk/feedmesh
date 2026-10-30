// Package reddit reads a subreddit's public Atom listing.
//
// Reddit publishes one Atom document per listing:
//
//	https://www.reddit.com/r/{sub}/{sort}/.rss?limit=N[&t={window}]
//
// No credentials, no OAuth application and no request signature are involved.
// The anonymous JSON endpoints (…/hot.json) answer 403 to datacenter addresses,
// so the Atom document is the only anonymous source; in exchange it carries
// fewer fields: an entry's content holds the outbound link and the comment
// page, and nothing else. Score, comment counts, thumbnails and self text are
// JSON-only and are therefore absent here.
//
// The document is aggressively rate limited for anonymous clients. Every reply
// carries x-ratelimit-remaining and x-ratelimit-reset, and once the window is
// spent the next request is answered with 429 and an empty body (observed on a
// datacenter address: one read per 60 s window). The client therefore keeps a
// single global slot shared by every subreddit and sort: at most one upstream
// request in flight, at least MinInterval between two reads, and no request at
// all while a reset window is open. A caller that cannot read right now gets
// ErrBusy immediately, so the feed layer can serve its last good list instead
// of holding the reader for a minute.
package reddit

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/yilong-musk/feedmesh/internal/logx"
)

const (
	// DefaultBaseURL is the production site.
	DefaultBaseURL = "https://www.reddit.com"
	// DefaultLimit is the shipped page width.
	DefaultLimit = 20
	// MaxLimit is the widest answer one Atom page can give.
	MaxLimit = 25
	// DefaultMinInterval is the floor between two upstream reads. It is the
	// hard constraint of docs/PLATFORMS.md: one read at a time, never faster
	// than this, whatever the caller asks for.
	DefaultMinInterval = 8 * time.Second
	// DefaultUserAgent identifies the service. The default Go user agent is
	// refused by the site's edge.
	DefaultUserAgent = "feedmesh/1.0 (+https://github.com/yilong-musk/feedmesh)"
	// DefaultTimeout bounds one read, including the response body.
	DefaultTimeout = 15 * time.Second
	// maxBody is the largest listing the client accepts.
	maxBody = 4 << 20
)

// Sort is a listing order. Hot is the site's ranking, new is chronological and
// top needs a Window.
type Sort string

// Sort values.
const (
	SortHot Sort = "hot"
	SortNew Sort = "new"
	SortTop Sort = "top"
)

// Window is the period a top listing covers. It is ignored by the other sorts.
type Window string

// Window values.
const (
	WindowDay   Window = "day"
	WindowWeek  Window = "week"
	WindowMonth Window = "month"
	WindowYear  Window = "year"
	WindowAll   Window = "all"
)

// ParseSort returns the sort a query value names. An empty value means hot.
func ParseSort(raw string) (Sort, bool) {
	switch Sort(strings.ToLower(strings.TrimSpace(raw))) {
	case "", SortHot:
		return SortHot, true
	case SortNew:
		return SortNew, true
	case SortTop:
		return SortTop, true
	}
	return "", false
}

// ParseWindow returns the window a query value names. An empty value means day.
func ParseWindow(raw string) (Window, bool) {
	switch Window(strings.ToLower(strings.TrimSpace(raw))) {
	case "", WindowDay:
		return WindowDay, true
	case WindowWeek:
		return WindowWeek, true
	case WindowMonth:
		return WindowMonth, true
	case WindowYear:
		return WindowYear, true
	case WindowAll:
		return WindowAll, true
	}
	return "", false
}

// Post is one listing entry.
type Post struct {
	ID string `json:"id"`
	// Title is the submission title.
	Title string `json:"title"`
	// Author is the submitter name without the /u/ prefix.
	Author string `json:"author"`
	// Permalink is the reddit comment page for the submission.
	Permalink string `json:"permalink"`
	// ExternalURL is the outbound link of a link submission. It is empty for
	// a self post: the Atom document carries no post body.
	ExternalURL string    `json:"external_url,omitempty"`
	Published   time.Time `json:"published"`
}

// Listing is one page of a subreddit.
type Listing struct {
	Sub string `json:"sub"`
	// Title is the subreddit name as upstream spells it.
	Title string `json:"title"`
	// Description is the subreddit public description (the feed subtitle).
	Description string `json:"description"`
	Sort        Sort   `json:"sort"`
	Window      Window `json:"window"`
	Posts       []Post `json:"posts"`
}

// Config tunes a Client.
type Config struct {
	// BaseURL overrides the site root. Tests and mirrors use it; the listing
	// path itself stays the production one.
	BaseURL string
	Timeout time.Duration
	// UserAgent is sent on every request. The production value names the
	// service so an operator at the other end can see who is reading.
	UserAgent string
	// MinInterval is the floor between two upstream reads. Zero uses
	// DefaultMinInterval; tests set a millisecond.
	MinInterval time.Duration
	// Log receives one info line per upstream read. Nil discards them.
	Log *logx.Logger
	// Now is the clock the rate-limit window is measured with. Nil uses
	// time.Now.
	Now func() time.Time
}

// Client reads Reddit over HTTP through one global rate-limited slot.
type Client struct {
	base        string
	ua          string
	http        *http.Client
	minInterval time.Duration
	log         *logx.Logger
	now         func() time.Time

	mu        sync.Mutex
	inFlight  bool
	lastRead  time.Time
	notBefore time.Time
}

// New builds a Client with production defaults.
func New(cfg Config) *Client {
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultBaseURL
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = DefaultTimeout
	}
	if cfg.UserAgent == "" {
		cfg.UserAgent = DefaultUserAgent
	}
	if cfg.MinInterval == 0 {
		cfg.MinInterval = DefaultMinInterval
	}
	if cfg.MinInterval < 0 {
		// A negative value disables the floor. Tests that want two reads back
		// to back use it; production never does.
		cfg.MinInterval = 0
	}
	if cfg.Log == nil {
		cfg.Log = logx.Discard()
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Client{
		base: strings.TrimRight(cfg.BaseURL, "/"),
		ua:   cfg.UserAgent,
		http: &http.Client{
			Timeout: cfg.Timeout,
			// Every hop of a redirect is an upstream request, and the site
			// answers an unknown subreddit with a 302 to its search page. The
			// client reads the status and the Location itself; following it
			// would spend a second request inside the same rate-limit window.
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		minInterval: cfg.MinInterval,
		log:         cfg.Log,
		now:         cfg.Now,
	}
}

// Errors the feed layer maps onto the API envelope.
var (
	// ErrBusy reports that the single upstream slot is not available yet:
	// another read is in flight, or the minimum interval has not elapsed.
	ErrBusy = errors.New("reddit: upstream read not allowed yet")
	// ErrRateLimited reports that the site's own quota window is open. No
	// request is made while it lasts.
	ErrRateLimited = errors.New("reddit: upstream rate limit window is open")
	// ErrSubredditNotFound reports a name the site does not serve a listing
	// for. The site answers with a redirect to its subreddit search rather
	// than a 404, so the client reads the Location.
	ErrSubredditNotFound = errors.New("reddit: subreddit not found")
	// ErrBadRequest reports a name, sort or window the client refuses to send.
	ErrBadRequest = errors.New("reddit: invalid request")
)

// HTTPError is a non-2xx answer from the site.
type HTTPError struct {
	StatusCode int
	URL        string
}

// Error implements error.
func (e *HTTPError) Error() string {
	return fmt.Sprintf("reddit: %s answered %d", e.URL, e.StatusCode)
}

// subShape is the accepted subreddit name: letters, digits, underscore and
// hyphen, at most 64 characters.
var subShape = regexp.MustCompile(`^[a-z0-9_][a-z0-9_-]{0,63}$`)

// NormalizeSub reduces a subreddit reference to the lower-case name the site
// serves. "programming", "r/programming" and "/r/programming/" are the same
// subreddit, and the site is case-insensitive, so one form is used for both the
// request path and the cache key.
func NormalizeSub(raw string) (string, error) {
	name := strings.ToLower(strings.TrimSpace(raw))
	name = strings.TrimPrefix(name, "/")
	name = strings.TrimPrefix(name, "r/")
	name = strings.Trim(name, "/")
	name = strings.TrimSpace(name)
	if !subShape.MatchString(name) {
		return "", fmt.Errorf("%w: %q is not a subreddit name", ErrBadRequest, raw)
	}
	return name, nil
}

// MinInterval reports the floor between two upstream reads.
func (c *Client) MinInterval() time.Duration { return c.minInterval }

// acquire reserves the one upstream slot and reports why it could not. It never
// waits: a caller that cannot read now degrades instead of queueing.
func (c *Client) acquire() (func(), error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	if c.inFlight {
		return nil, fmt.Errorf("%w: another upstream read is in flight", ErrBusy)
	}
	if !c.notBefore.IsZero() && now.Before(c.notBefore) {
		return nil, fmt.Errorf("%w: retry in %s", ErrRateLimited, c.notBefore.Sub(now).Round(time.Second))
	}
	if !c.lastRead.IsZero() && now.Sub(c.lastRead) < c.minInterval {
		return nil, fmt.Errorf("%w: last read %s ago, minimum interval is %s",
			ErrBusy, now.Sub(c.lastRead).Round(time.Millisecond), c.minInterval)
	}
	c.inFlight = true
	c.lastRead = now
	return c.release, nil
}

func (c *Client) release() {
	c.mu.Lock()
	c.inFlight = false
	c.mu.Unlock()
}

// noteResponse records the quota state the reply reports. The site sends the
// numbers on 200s as well as on 429s, so honouring only the 429 would turn
// every second read into a wasted request.
func (c *Client) noteResponse(h http.Header, status int) {
	reset, ok := secondsHeader(h, "X-Ratelimit-Reset")
	if !ok || reset <= 0 {
		return
	}
	remaining, hasRemaining := secondsHeader(h, "X-Ratelimit-Remaining")
	if status != http.StatusTooManyRequests && (!hasRemaining || remaining > 0) {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if until := c.now().Add(reset); until.After(c.notBefore) {
		c.notBefore = until
	}
}

// secondsHeader reads a header that carries a number of seconds, which Reddit
// spells either as an integer or with a fractional part.
func secondsHeader(h http.Header, name string) (time.Duration, bool) {
	raw := strings.TrimSpace(h.Get(name))
	if raw == "" {
		return 0, false
	}
	seconds, err := strconv.ParseFloat(raw, 64)
	if err != nil || seconds < 0 {
		return 0, false
	}
	return time.Duration(seconds * float64(time.Second)), true
}

// List reads one page of a subreddit listing. limit is clamped to the widest
// page the site serves, so a caller cannot ask for a width the document cannot
// carry.
func (c *Client) List(ctx context.Context, sub string, sort Sort, window Window, limit int) (*Listing, error) {
	name, err := NormalizeSub(sub)
	if err != nil {
		return nil, err
	}
	if sort == "" {
		sort = SortHot
	}
	if window == "" {
		window = WindowDay
	}
	if _, ok := ParseSort(string(sort)); !ok {
		return nil, fmt.Errorf("%w: unknown sort %q", ErrBadRequest, sort)
	}
	if _, ok := ParseWindow(string(window)); !ok {
		return nil, fmt.Errorf("%w: unknown window %q", ErrBadRequest, window)
	}
	if limit <= 0 {
		limit = DefaultLimit
	}
	if limit > MaxLimit {
		limit = MaxLimit
	}

	release, err := c.acquire()
	if err != nil {
		c.log.Debug("reddit upstream read skipped", "sub", name, "sort", string(sort), "reason", err.Error())
		return nil, err
	}
	defer release()

	ctx, cancel := context.WithTimeout(ctx, c.http.Timeout)
	defer cancel()

	q := url.Values{}
	q.Set("limit", strconv.Itoa(limit))
	if sort == SortTop {
		q.Set("t", string(window))
	}
	path := "/r/" + name + "/" + string(sort) + "/.rss?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.ua)
	req.Header.Set("Accept", "application/atom+xml")

	start := c.now()
	resp, err := c.http.Do(req)
	if err != nil {
		c.log.Info("reddit upstream read",
			"sub", name, "sort", string(sort), "window", string(window),
			"status", 0, "error", err.Error(),
			"ms", c.now().Sub(start).Milliseconds())
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	c.noteResponse(resp.Header, resp.StatusCode)

	body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	c.log.Info("reddit upstream read",
		"sub", name, "sort", string(sort), "window", string(window),
		"status", resp.StatusCode, "bytes", len(body),
		"ms", c.now().Sub(start).Milliseconds())

	switch {
	case resp.StatusCode == http.StatusOK:
		if readErr != nil {
			return nil, readErr
		}
		return parseListing(name, sort, window, limit, body)
	case resp.StatusCode == http.StatusTooManyRequests:
		return nil, fmt.Errorf("reddit: %s answered 429: %w", path, ErrRateLimited)
	case resp.StatusCode >= 300 && resp.StatusCode < 400:
		// An unknown subreddit is redirected to the subreddit search page.
		location := resp.Header.Get("Location")
		if strings.Contains(location, "/subreddits/search") || strings.Contains(location, "/search.rss") {
			return nil, fmt.Errorf("%w: %q", ErrSubredditNotFound, name)
		}
		return nil, &HTTPError{StatusCode: resp.StatusCode, URL: req.URL.String()}
	case resp.StatusCode == http.StatusNotFound:
		return nil, fmt.Errorf("%w: %q", ErrSubredditNotFound, name)
	default:
		return nil, &HTTPError{StatusCode: resp.StatusCode, URL: req.URL.String()}
	}
}

// parseListing turns one Atom document into a Listing. Entries keep the order
// the document gives them: a hot listing is a ranking, and re-sorting it by
// time would destroy the ranking the reader asked for.
func parseListing(sub string, sort Sort, window Window, limit int, body []byte) (*Listing, error) {
	var doc atomDocument
	if err := xml.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("reddit: listing parse: %w", err)
	}
	out := &Listing{
		Sub:         sub,
		Title:       strings.TrimSpace(doc.Title),
		Description: strings.TrimSpace(doc.Subtitle),
		Sort:        sort,
		Window:      window,
	}
	for _, e := range doc.Entries {
		title := strings.TrimSpace(e.Title)
		id := strings.TrimSpace(e.ID)
		if title == "" || id == "" {
			continue
		}
		permalink := ""
		if e.Link != nil {
			permalink = strings.TrimSpace(e.Link.Href)
		}
		external, comments := anchors(e.Content.Body, permalink)
		if permalink == "" {
			permalink = comments
		}
		p := Post{
			ID:          id,
			Title:       title,
			Author:      authorName(e.Author.Name),
			Permalink:   permalink,
			ExternalURL: external,
			Published:   parseTime(e.Published, e.Updated),
		}
		out.Posts = append(out.Posts, p)
	}
	if len(out.Posts) > limit {
		out.Posts = out.Posts[:limit]
	}
	return out, nil
}

// atomDocument mirrors the parts of the listing this package reads.
type atomDocument struct {
	XMLName  xml.Name    `xml:"feed"`
	Title    string      `xml:"title"`
	Subtitle string      `xml:"subtitle"`
	Entries  []atomEntry `xml:"entry"`
}

type atomEntry struct {
	Author    atomAuthor  `xml:"author"`
	Content   atomContent `xml:"content"`
	ID        string      `xml:"id"`
	Link      *atomLink   `xml:"link"`
	Published string      `xml:"published"`
	Updated   string      `xml:"updated"`
	Title     string      `xml:"title"`
}

type atomAuthor struct {
	Name string `xml:"name"`
}

type atomContent struct {
	Type string `xml:"type,attr"`
	Body string `xml:",chardata"`
}

type atomLink struct {
	Rel  string `xml:"rel,attr"`
	Href string `xml:"href,attr"`
}

// anchorPattern finds the anchors of an entry body. After the XML decode the
// body is HTML whose ampersands are still entity-escaped (the document carries
// "&amp;#32; submitted by … &lt;a href=&quot;…&quot;&gt;"), so the markup
// itself is real and only the href values need unescaping.
var anchorPattern = regexp.MustCompile(`(?is)<a\s+[^>]*href="([^"]*)"[^>]*>(.*?)</a>`)

// anchors reads the two links a listing entry carries: the outbound link of a
// link submission and the comment page. The document marks them with the
// "[link]" and "[comments]" anchor texts; the submitter's profile is a third
// anchor and is not a submission target.
func anchors(body, permalink string) (external, comments string) {
	for _, m := range anchorPattern.FindAllStringSubmatch(body, -1) {
		href := html.UnescapeString(strings.TrimSpace(m[1]))
		label := strings.ToLower(strings.TrimSpace(html.UnescapeString(m[2])))
		switch label {
		case "[link]":
			if external == "" {
				external = href
			}
		case "[comments]":
			if comments == "" {
				comments = href
			}
		}
	}
	if comments == "" && strings.Contains(permalink, "/comments/") {
		comments = permalink
	}
	if external == "" {
		external = fallbackExternal(body, comments)
	}
	return external, comments
}

// fallbackExternal reads the outbound link of an entry that does not mark it
// with the "[link]" anchor text: the first anchor that is neither the comment
// page nor a profile page is the submission target.
func fallbackExternal(body, comments string) string {
	for _, m := range anchorPattern.FindAllStringSubmatch(body, -1) {
		href := html.UnescapeString(strings.TrimSpace(m[1]))
		if href == "" || href == comments {
			continue
		}
		u, err := url.Parse(href)
		if err != nil || u.Host == "" {
			continue
		}
		switch {
		case strings.Contains(u.Path, "/user/"), strings.Contains(u.Path, "/comments/"):
			continue
		}
		return href
	}
	return ""
}

// authorName strips the "/u/" prefix the document uses.
func authorName(raw string) string {
	name := strings.TrimSpace(raw)
	for _, prefix := range []string{"/u/", "u/", "/user/"} {
		if strings.HasPrefix(name, prefix) {
			return strings.TrimPrefix(name, prefix)
		}
	}
	return name
}

// parseTime reads the entry timestamp, falling back to the updated one.
func parseTime(values ...string) time.Time {
	for _, raw := range values {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if t, err := time.Parse(time.RFC3339, raw); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}
