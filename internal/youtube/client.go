// Package youtube reads YouTube's official per-channel upload feed.
//
// Every public channel exposes its latest uploads as Atom at
//
//	https://www.youtube.com/feeds/videos.xml?channel_id=UC...
//
// with no credentials, no API key and no request signature. One feed carries
// 15 entries and cannot be paginated, so a channel that publishes more than
// that between two polls will drop items; readers that want completeness need
// a shorter interval, not a wider page.
//
// The endpoint is not always available: it answers 404 or 500 for short
// windows even for channels that exist (reported by RSS-Bridge, newsboat and
// Google's developer forum). A single 404 therefore does not mean "channel
// gone", so the client retries before it reports a failure and the feed layer
// keeps serving the last good list when the retries are exhausted.
//
// The feed only accepts a channel id. Handles (@name) and channel URLs are
// resolved once against the public channel page; callers are expected to cache
// that mapping.
package youtube

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultBaseURL is the production site.
	DefaultBaseURL = "https://www.youtube.com"
	// DefaultLimit is the width of one feed.
	DefaultLimit = 15
	// MaxLimit is the widest answer the endpoint can give.
	MaxLimit = 15
	// MaxAttempts bounds one request, including retries.
	MaxAttempts = 3
)

// Video is one upload.
type Video struct {
	ID          string    `json:"id"`
	ChannelID   string    `json:"channel_id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	URL         string    `json:"url"`
	Thumbnail   string    `json:"thumbnail"`
	Published   time.Time `json:"published"`
	Updated     time.Time `json:"updated"`
	Views       int64     `json:"views"`
	Likes       int64     `json:"likes"`
}

// Feed is one channel's upload list.
type Feed struct {
	ChannelID string  `json:"channel_id"`
	Title     string  `json:"title"`
	Videos    []Video `json:"videos"`
}

// Config tunes a Client.
type Config struct {
	BaseURL   string
	Timeout   time.Duration
	UserAgent string
	// RetryDelay is the pause before the second attempt; every later attempt
	// doubles it. Tests set a millisecond.
	RetryDelay time.Duration
}

// Client reads YouTube over HTTP.
type Client struct {
	base       string
	ua         string
	http       *http.Client
	retryDelay time.Duration
}

// New builds a Client with production defaults.
func New(cfg Config) *Client {
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultBaseURL
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 20 * time.Second
	}
	if cfg.UserAgent == "" {
		cfg.UserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0 Safari/537.36"
	}
	if cfg.RetryDelay <= 0 {
		cfg.RetryDelay = 2 * time.Second
	}
	return &Client{
		base:       strings.TrimRight(cfg.BaseURL, "/"),
		ua:         cfg.UserAgent,
		http:       &http.Client{Timeout: cfg.Timeout},
		retryDelay: cfg.RetryDelay,
	}
}

var (
	// channelIDShape is the canonical form YouTube uses in feed URLs.
	channelIDShape = regexp.MustCompile(`^UC[A-Za-z0-9_-]{22}$`)
	// channelIDFromPage finds the channel id in the public channel page.
	channelIDFromPage = regexp.MustCompile(`"externalId":"(UC[A-Za-z0-9_-]{22})"`)
)

// HTTPError is a non-2xx answer from YouTube.
type HTTPError struct {
	StatusCode int
	URL        string
}

// ErrChannelNotFound reports a reference that YouTube does not resolve to a
// channel. A 404 from the upload feed itself is not this error: that endpoint
// is known to answer 404 for channels that exist.
var ErrChannelNotFound = errors.New("youtube: channel not found")

// Error implements error.
func (e *HTTPError) Error() string {
	return fmt.Sprintf("youtube: %s answered %d", e.URL, e.StatusCode)
}

// ChannelID resolves ref to a channel id. ref may be a UC... id, an @handle,
// a bare handle, or a channel URL. Only the last form costs a request.
func (c *Client) ChannelID(ctx context.Context, ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", fmt.Errorf("youtube: empty channel reference")
	}
	if i := strings.Index(ref, "youtube.com/"); i >= 0 {
		ref = ref[i+len("youtube.com/"):]
	}
	ref = normalizeRef(ref)
	if channelIDShape.MatchString(ref) {
		return ref, nil
	}
	handle := strings.TrimPrefix(ref, "@")
	if handle == "" || strings.ContainsAny(handle, "/ ?#") {
		return "", fmt.Errorf("youtube: invalid channel reference %q", ref)
	}

	body, err := c.get(ctx, "/@"+url.PathEscape(handle))
	if err != nil {
		var he *HTTPError
		if errors.As(err, &he) && he.StatusCode == http.StatusNotFound {
			return "", fmt.Errorf("youtube: channel %q not found: %w", handle, ErrChannelNotFound)
		}
		return "", err
	}
	m := channelIDFromPage.FindSubmatch(body)
	if m == nil {
		return "", fmt.Errorf("youtube: no channel id on the page for %q: %w", handle, ErrChannelNotFound)
	}
	return string(m[1]), nil
}

// Videos reads the upload feed for one channel id.
func (c *Client) Videos(ctx context.Context, channelID string) (*Feed, error) {
	channelID = strings.TrimSpace(channelID)
	if !channelIDShape.MatchString(channelID) {
		return nil, fmt.Errorf("youtube: invalid channel id %q", channelID)
	}
	body, err := c.get(ctx, "/feeds/videos.xml?channel_id="+url.QueryEscape(channelID))
	if err != nil {
		return nil, err
	}
	var doc atomDocument
	if err := xml.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("youtube: feed parse: %w", err)
	}
	out := &Feed{ChannelID: channelID, Title: strings.TrimSpace(doc.Title)}
	for _, e := range doc.Entries {
		title := strings.TrimSpace(e.Title)
		id := strings.TrimSpace(e.VideoID)
		if id == "" || title == "" {
			continue
		}
		v := Video{
			ID:          id,
			ChannelID:   channelID,
			Title:       title,
			Description: strings.TrimSpace(e.Media.Description),
			URL:         "https://www.youtube.com/watch?v=" + id,
			Thumbnail:   pickThumbnail(e.Media.Thumbnails),
			Published:   parseTime(e.Published),
			Updated:     parseTime(e.Updated),
			Views:       parseInt(e.Media.Community.Statistics.Views),
			Likes:       parseInt(e.Media.Community.Rating.Count),
		}
		if v.Description == "" {
			v.Description = title
		}
		out.Videos = append(out.Videos, v)
	}
	return out, nil
}

// get performs one request with retries for the failure modes the feed is
// known for: 404/429/5xx and transport errors.
func (c *Client) get(ctx context.Context, path string) ([]byte, error) {
	delay := c.retryDelay
	var lastErr error
	for attempt := 1; attempt <= MaxAttempts; attempt++ {
		body, err := c.getOnce(ctx, path)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !retryable(err) || ctx.Err() != nil || attempt == MaxAttempts {
			break
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
		}
		delay *= 2
	}
	return nil, lastErr
}

func (c *Client) getOnce(ctx context.Context, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.ua)
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
		return nil, &HTTPError{StatusCode: resp.StatusCode, URL: req.URL.String()}
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	return body, nil
}

// retryable reports whether another attempt can plausibly succeed.
func retryable(err error) bool {
	var he *HTTPError
	if errors.As(err, &he) {
		return he.StatusCode == http.StatusNotFound ||
			he.StatusCode == http.StatusTooManyRequests ||
			he.StatusCode >= 500
	}
	return true
}

// tabSegments are the channel sub-pages a pasted URL may carry. They select a
// tab, not a different channel, so they are dropped from the reference.
var tabSegments = map[string]bool{
	"videos":    true,
	"shorts":    true,
	"streams":   true,
	"playlists": true,
	"about":     true,
	"featured":  true,
	"community": true,
}

// normalizeRef reduces a channel URL tail to the channel id or handle it
// names: trailing slashes, query strings, fragments and tab segments go away.
func normalizeRef(ref string) string {
	for _, cut := range []string{"?", "#"} {
		if i := strings.Index(ref, cut); i >= 0 {
			ref = ref[:i]
		}
	}
	ref = strings.Trim(ref, "/")
	parts := strings.Split(ref, "/")
	if n := len(parts); n > 1 && tabSegments[strings.ToLower(parts[n-1])] {
		parts = parts[:n-1]
	}
	ref = strings.Join(parts, "/")
	return strings.TrimPrefix(ref, "channel/")
}

// pickThumbnail prefers the widest thumbnail the entry carries.
func pickThumbnail(list []atomThumbnail) string {
	best, width := "", 0
	for _, t := range list {
		u := strings.TrimSpace(t.URL)
		if u == "" {
			continue
		}
		w, _ := strconv.Atoi(t.Width)
		if w >= width {
			best, width = u, w
		}
	}
	return best
}

func parseTime(raw string) time.Time {
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(raw))
	if err != nil {
		return time.Time{}
	}
	return t.UTC()
}

func parseInt(raw string) int64 {
	n, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil {
		return 0
	}
	return n
}

// atomDocument mirrors the parts of the feed this package reads. The namespaces
// are spelled out on every element because encoding/xml matches them exactly.
type atomDocument struct {
	XMLName xml.Name    `xml:"feed"`
	Title   string      `xml:"title"`
	Entries []atomEntry `xml:"entry"`
}

type atomEntry struct {
	VideoID   string    `xml:"http://www.youtube.com/xml/schemas/2015 videoId"`
	Title     string    `xml:"title"`
	Published string    `xml:"published"`
	Updated   string    `xml:"updated"`
	Media     atomMedia `xml:"http://search.yahoo.com/mrss/ group"`
}

type atomMedia struct {
	Description string          `xml:"http://search.yahoo.com/mrss/ description"`
	Thumbnails  []atomThumbnail `xml:"http://search.yahoo.com/mrss/ thumbnail"`
	// Community carries the counters; YouTube nests them one level below the
	// group since the 2021 feed revision.
	Community atomCommunity `xml:"http://search.yahoo.com/mrss/ community"`
}

type atomCommunity struct {
	Statistics atomStatistics `xml:"http://search.yahoo.com/mrss/ statistics"`
	Rating     atomRating     `xml:"http://search.yahoo.com/mrss/ starRating"`
}

type atomThumbnail struct {
	URL    string `xml:"url,attr"`
	Width  string `xml:"width,attr"`
	Height string `xml:"height,attr"`
}

type atomStatistics struct {
	Views string `xml:"views,attr"`
}

type atomRating struct {
	Count string `xml:"count,attr"`
}
