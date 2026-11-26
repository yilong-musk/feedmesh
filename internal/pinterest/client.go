// Package pinterest reads Pinterest's public RSS feeds.
//
// Two endpoints are served anonymously and need no account, cookie, token or
// browser:
//
//	/{user}/feed.rss       a profile's latest pins
//	/{user}/{board}.rss    one board's latest pins
//
// Each feed carries at most 25 pins and has no pagination. Item titles are
// often empty (a pin is an image first), and <description> holds a 236px
// thumbnail followed by the pin text, both as escaped HTML.
package pinterest

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/yilong-musk/feedmesh/internal/upstream"
)

// DefaultBaseURL is the public site. Tests point BaseURL at an httptest
// server; the /{user}/feed.rss and /{user}/{board}.rss paths are kept.
const DefaultBaseURL = "https://www.pinterest.com"

const (
	// DefaultLimit is the number of pins returned when the caller is silent.
	DefaultLimit = 20
	// MaxLimit is one feed slice. Pinterest serves 25 pins and has no
	// pagination, so a reader that wants older pins has to keep polling.
	MaxLimit = 25

	// userAgent is a desktop browser UA. The feeds answer without one today,
	// but the CDN in front of them varies by UA; keep the browser shape.
	userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
)

// Errors callers map onto HTTP responses.
var (
	// ErrInvalidUser means the profile name is malformed.
	ErrInvalidUser = errors.New("invalid pinterest user")
	// ErrInvalidBoard means the board slug is malformed.
	ErrInvalidBoard = errors.New("invalid pinterest board")
	// ErrUserNotFound means the profile feed answered 404.
	ErrUserNotFound = errors.New("pinterest user not found")
	// ErrBoardNotFound means the board feed answered 404.
	ErrBoardNotFound = errors.New("pinterest board not found")
	// ErrUnexpectedPage means the endpoint answered with an HTML shell
	// instead of an RSS document (Pinterest does this for unknown paths).
	ErrUnexpectedPage = errors.New("pinterest returned an HTML page instead of RSS")
	// ErrBadFeed means the document parsed as XML but carried no channel.
	ErrBadFeed = errors.New("pinterest feed could not be parsed")
	// ErrRateLimited means Pinterest asked us to slow down.
	ErrRateLimited = errors.New("pinterest rate limited")
	// ErrBlocked means the request was refused by the edge (403/418).
	ErrBlocked = errors.New("pinterest request blocked")
	// ErrUpstream means the endpoint answered with an unusable status.
	ErrUpstream = errors.New("pinterest upstream unavailable")
)

// Config tunes a Client.
type Config struct {
	// BaseURL replaces the public host; tests and mirrors use it.
	BaseURL   string
	Timeout   time.Duration
	UserAgent string
}

// Client reads Pinterest's public RSS feeds. It is safe for concurrent use.
type Client struct {
	base string
	ua   string
	http *http.Client
	now  func() time.Time
}

// New builds a Client with production defaults.
func New(cfg Config) *Client {
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultBaseURL
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 15 * time.Second
	}
	if cfg.UserAgent == "" {
		cfg.UserAgent = userAgent
	}
	return &Client{
		base: strings.TrimRight(cfg.BaseURL, "/"),
		ua:   cfg.UserAgent,
		http: &http.Client{Timeout: cfg.Timeout},
		now:  time.Now,
	}
}

// Pin is one feed entry.
type Pin struct {
	ID        string
	Title     string
	Text      string
	URL       string
	Image     string
	Published time.Time
}

// Result is one feed read: the channel metadata plus its pins.
type Result struct {
	User        string    `json:"user"`
	Board       string    `json:"board,omitempty"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Link        string    `json:"link"`
	FetchedAt   time.Time `json:"fetched_at"`
	Pins        []Pin     `json:"pins"`
}

// nameRE is the shape Pinterest uses for both profile names and board slugs.
// Letters, digits, dot, underscore and hyphen cover every live account and
// board seen; anything else is rejected before it reaches a URL path.
var nameRE = regexp.MustCompile(`^[A-Za-z0-9._-]{1,80}$`)

// NormalizeUser accepts a bare profile name, an @handle, or a profile URL and
// returns the bare name.
func NormalizeUser(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	name = strings.TrimPrefix(name, "https://")
	name = strings.TrimPrefix(name, "http://")
	name = strings.TrimPrefix(name, "www.")
	name = strings.TrimPrefix(name, "pinterest.com/")
	name = strings.Trim(name, "/")
	name = strings.TrimPrefix(name, "@")
	if !nameRE.MatchString(name) {
		return "", ErrInvalidUser
	}
	return name, nil
}

// NormalizeBoard accepts a bare board slug or an @-prefixed one and returns
// the bare slug. Board names are part of the feed URL, so the same character
// allowlist applies.
func NormalizeBoard(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	name = strings.Trim(name, "/")
	name = strings.TrimPrefix(name, "@")
	if !nameRE.MatchString(name) {
		return "", ErrInvalidBoard
	}
	return name, nil
}

// fetchURL is the feed address for one profile or board.
func (c *Client) fetchURL(user, board string) string {
	if board == "" {
		return c.base + "/" + url.PathEscape(user) + "/feed.rss"
	}
	return c.base + "/" + url.PathEscape(user) + "/" + url.PathEscape(board) + ".rss"
}

type rssDoc struct {
	Channel struct {
		Title       string `xml:"title"`
		Link        string `xml:"link"`
		Description string `xml:"description"`
		Items       []struct {
			Title       string `xml:"title"`
			Link        string `xml:"link"`
			Description string `xml:"description"`
			PubDate     string `xml:"pubDate"`
			GUID        string `xml:"guid"`
		} `xml:"item"`
	} `xml:"channel"`
}

// Fetch reads one profile feed (board empty) or one board feed.
func (c *Client) Fetch(ctx context.Context, user, board string, limit int) (*Result, error) {
	user, err := NormalizeUser(user)
	if err != nil {
		return nil, err
	}
	if board != "" {
		board, err = NormalizeBoard(board)
		if err != nil {
			return nil, err
		}
	}
	if limit <= 0 {
		limit = DefaultLimit
	}
	if limit > MaxLimit {
		limit = MaxLimit
	}

	body, err := c.get(ctx, user, board)
	if err != nil {
		return nil, err
	}

	var doc rssDoc
	if err := xml.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBadFeed, err)
	}
	if doc.Channel.Title == "" && len(doc.Channel.Items) == 0 {
		return nil, ErrUnexpectedPage
	}

	res := &Result{
		User:        user,
		Board:       board,
		Title:       cleanText(doc.Channel.Title),
		Description: cleanText(doc.Channel.Description),
		Link:        canonicalLink(user, board),
		FetchedAt:   c.now().UTC(),
		Pins:        make([]Pin, 0, len(doc.Channel.Items)),
	}
	for _, it := range doc.Channel.Items {
		link := strings.TrimSpace(it.Link)
		if link == "" {
			link = strings.TrimSpace(it.GUID)
		}
		if link == "" {
			continue
		}
		text := cleanText(it.Description)
		title := cleanText(it.Title)
		// A pin's title and its caption are often the same sentence; keeping
		// both is harmless and avoids dropping a caption when the title is a
		// site name.
		res.Pins = append(res.Pins, Pin{
			ID:        pinID(link),
			Title:     title,
			Text:      text,
			URL:       link,
			Image:     firstImage(it.Description),
			Published: parseTime(it.PubDate),
		})
	}
	// The feed order is Pinterest's own "recently pinned" order, which is what
	// a board subscriber wants; do not re-sort by pubDate. A pin added to a
	// board today can carry an old creation date, and sorting would bury it.
	if len(res.Pins) > limit {
		res.Pins = res.Pins[:limit]
	}
	return res, nil
}

func (c *Client) get(ctx context.Context, user, board string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.fetchURL(user, board), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.ua)
	req.Header.Set("Accept", "application/rss+xml, application/xml;q=0.9, */*;q=0.8")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("pinterest request failed: %w", err)
	}
	defer resp.Body.Close()
	body, err := upstream.ReadBody(resp.Body, 4<<20)
	if err != nil {
		return nil, fmt.Errorf("pinterest read failed: %w", err)
	}
	if resp.StatusCode == http.StatusOK {
		return body, nil
	}
	switch resp.StatusCode {
	case http.StatusNotFound:
		if board != "" {
			return nil, ErrBoardNotFound
		}
		return nil, ErrUserNotFound
	case http.StatusTooManyRequests:
		return nil, ErrRateLimited
	case http.StatusForbidden, http.StatusTeapot:
		return nil, ErrBlocked
	default:
		return nil, fmt.Errorf("%w: HTTP %d", ErrUpstream, resp.StatusCode)
	}
}

// canonicalLink is the human-facing profile or board URL.
func canonicalLink(user, board string) string {
	if board == "" {
		return DefaultBaseURL + "/" + user + "/"
	}
	return DefaultBaseURL + "/" + user + "/" + board + "/"
}

// pinID extracts the permalink identifier. Pinterest serves both numeric ids
// (/pin/424605071146418083/) and opaque tokens (/pin/e1_Afq.../), so the id is
// whatever the last path segment carries.
func pinID(link string) string {
	u, err := url.Parse(link)
	if err != nil {
		return ""
	}
	p := strings.Trim(u.Path, "/")
	if i := strings.LastIndex(p, "/"); i >= 0 {
		p = p[i+1:]
	}
	return p
}

var (
	tagRE = regexp.MustCompile(`(?s)<[^>]*>`)
	imgRE = regexp.MustCompile(`(?i)<img[^>]+src\s*=\s*["']([^"']+)["']`)
)

// cleanText turns the escaped description markup into plain text. Pinterest
// puts the thumbnail first and the caption after it; stripping tags leaves the
// caption in place.
func cleanText(raw string) string {
	s := tagRE.ReplaceAllString(raw, " ")
	s = html.UnescapeString(s)
	return strings.Join(strings.Fields(s), " ")
}

// firstImage returns the first image URL in one description, upgraded to the
// 736px variant. Pinterest stamps every thumbnail as /236x/; the originals
// path is not consistently available (four of six tested answered 403), while
// 736x answered for every pin tested.
func firstImage(description string) string {
	m := imgRE.FindStringSubmatch(description)
	if m == nil {
		return ""
	}
	return hiResThumb(strings.TrimSpace(m[1]))
}

func hiResThumb(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host != "i.pinimg.com" {
		return raw
	}
	if strings.HasPrefix(u.Path, "/236x/") {
		u.Path = "/736x/" + strings.TrimPrefix(u.Path, "/236x/")
	}
	return u.String()
}

// parseTime accepts the two stamps Pinterest emits: RFC 1123 with GMT and the
// RFC 1123Z offset form.
func parseTime(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	if t, err := http.ParseTime(s); err == nil {
		return t
	}
	return time.Time{}
}
