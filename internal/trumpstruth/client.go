// Package trumpstruth reads the public archive of Donald Trump's Truth Social
// posts at trumpstruth.org.
//
// truthsocial.com itself is behind Cloudflare on this network and has no
// anonymous read API. The archive publishes a normal RSS document at /feed,
// refreshed every few minutes, and that is the only source this package uses.
package trumpstruth

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/yilong-musk/feedmesh/internal/upstream"
)

// DefaultBaseURL is the public archive. Tests point BaseURL at an httptest
// server; the /feed path is kept.
const DefaultBaseURL = "https://www.trumpstruth.org"

const (
	// DefaultLimit is the number of posts returned when the caller is silent.
	DefaultLimit = 20
	// MaxLimit is the widest slice this client asks for. The archive accepts
	// up to 500, but a reader polling the latest posts does not need that,
	// and a 100-item document is already the archive's own default window.
	MaxLimit = 100

	userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
)

// Errors callers map onto HTTP responses.
var (
	// ErrUnexpectedPage means the endpoint answered with an HTML shell
	// instead of an RSS document (a challenge page does this).
	ErrUnexpectedPage = errors.New("trumpstruth returned an HTML page instead of RSS")
	// ErrBadFeed means the document parsed as XML but carried no channel.
	ErrBadFeed = errors.New("trumpstruth feed could not be parsed")
	// ErrRateLimited means the archive asked us to slow down.
	ErrRateLimited = errors.New("trumpstruth rate limited")
	// ErrBlocked means the request was refused by the edge (403/418).
	ErrBlocked = errors.New("trumpstruth request blocked")
	// ErrUpstream means the endpoint answered with an unusable status.
	ErrUpstream = errors.New("trumpstruth upstream unavailable")
)

// Config tunes a Client.
type Config struct {
	BaseURL   string
	Timeout   time.Duration
	UserAgent string
}

// Client reads the archive feed. It is safe for concurrent use.
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

// Post is one archived status.
type Post struct {
	ID          string
	Title       string
	Text        string
	URL         string
	OriginalURL string
	Image       string
	Published   time.Time
	PostType    string
	Removed     bool
	Author      string
}

// Result is one feed read.
type Result struct {
	Title       string
	Description string
	Link        string
	FetchedAt   time.Time
	Posts       []Post
}

// Latest reads the newest posts. limit is clamped to MaxLimit.
func (c *Client) Latest(ctx context.Context, limit int) (*Result, error) {
	if limit <= 0 {
		limit = DefaultLimit
	}
	if limit > MaxLimit {
		limit = MaxLimit
	}
	body, err := c.get(ctx, limit)
	if err != nil {
		return nil, err
	}
	if looksLikeHTML(body) {
		return nil, ErrUnexpectedPage
	}
	var doc rssDoc
	if err := xml.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBadFeed, err)
	}
	if strings.TrimSpace(doc.Channel.Title) == "" && len(doc.Channel.Items) == 0 {
		return nil, ErrBadFeed
	}
	res := &Result{
		Title:       plain(doc.Channel.Title),
		Description: plain(doc.Channel.Description),
		Link:        DefaultBaseURL + "/",
		FetchedAt:   c.now().UTC(),
		Posts:       make([]Post, 0, len(doc.Channel.Items)),
	}
	for _, it := range doc.Channel.Items {
		link := strings.TrimSpace(it.Link)
		if link == "" {
			link = strings.TrimSpace(it.GUID)
		}
		if link == "" {
			continue
		}
		text := plain(it.Description)
		if isMediaCaption(text) {
			text = ""
		}
		res.Posts = append(res.Posts, Post{
			ID:          statusID(link),
			Title:       plain(it.Title),
			Text:        text,
			URL:         link,
			OriginalURL: strings.TrimSpace(it.OriginalURL),
			Image:       firstImage(it.Media),
			Published:   parseTime(it.PubDate),
			PostType:    strings.TrimSpace(it.PostType),
			Removed:     strings.EqualFold(strings.TrimSpace(it.Removed), "true"),
			Author:      plain(it.Creator),
		})
	}
	if len(res.Posts) > limit {
		res.Posts = res.Posts[:limit]
	}
	return res, nil
}

func (c *Client) get(ctx context.Context, limit int) ([]byte, error) {
	u := c.base + "/feed?limit=" + strconv.Itoa(limit)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.ua)
	req.Header.Set("Accept", "application/rss+xml, application/xml;q=0.9, */*;q=0.8")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("trumpstruth request failed: %w", err)
	}
	defer resp.Body.Close()
	body, err := upstream.ReadBody(resp.Body, 8<<20)
	if err != nil {
		return nil, fmt.Errorf("trumpstruth read failed: %w", err)
	}
	if resp.StatusCode == http.StatusOK {
		return body, nil
	}
	switch resp.StatusCode {
	case http.StatusTooManyRequests:
		return nil, ErrRateLimited
	case http.StatusForbidden, http.StatusTeapot:
		return nil, ErrBlocked
	default:
		return nil, fmt.Errorf("%w: HTTP %d", ErrUpstream, resp.StatusCode)
	}
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
			Creator     string `xml:"http://purl.org/dc/elements/1.1/ creator"`
			OriginalURL string `xml:"https://truthsocial.com/ns originalUrl"`
			PostType    string `xml:"https://www.trumpstruth.org/rss-extensions postType"`
			Removed     string `xml:"https://www.trumpstruth.org/rss-extensions removed"`
			Media       []struct {
				URL    string `xml:"url,attr"`
				Medium string `xml:"medium,attr"`
			} `xml:"http://search.yahoo.com/mrss/ content"`
		} `xml:"item"`
	} `xml:"channel"`
}

func looksLikeHTML(body []byte) bool {
	s := strings.ToLower(string(body[:min(len(body), 512)]))
	if strings.Contains(s, "<rss") {
		return false
	}
	return strings.Contains(s, "<html") || strings.Contains(s, "just a moment") || strings.Contains(s, "attention required")
}

func firstImage(media []struct {
	URL    string `xml:"url,attr"`
	Medium string `xml:"medium,attr"`
}) string {
	for _, m := range media {
		if m.Medium == "" || m.Medium == "image" {
			if u := strings.TrimSpace(m.URL); u != "" {
				return u
			}
		}
	}
	return ""
}

var tagRE = regexp.MustCompile(`(?s)<[^>]*>`)

func plain(raw string) string {
	s := tagRE.ReplaceAllString(raw, " ")
	s = html.UnescapeString(s)
	return strings.Join(strings.Fields(s), " ")
}

func isMediaCaption(s string) bool {
	switch strings.ToLower(s) {
	case "view image", "view video":
		return true
	default:
		return false
	}
}

var statusRE = regexp.MustCompile(`/statuses/(\d+)`)

func statusID(link string) string {
	if m := statusRE.FindStringSubmatch(link); m != nil {
		return m[1]
	}
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

func parseTime(raw string) time.Time {
	raw = strings.TrimSpace(raw)
	for _, layout := range []string{time.RFC1123Z, time.RFC1123} {
		if t, err := time.Parse(layout, raw); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}
