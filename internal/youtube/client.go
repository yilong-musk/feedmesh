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
// gone", so the client retries before it reports a failure. When the feed
// keeps failing, the client reads the same list from the web client's browse
// endpoint (InnerTube) instead of returning nothing: that reply carries about
// 30 entries but only relative timestamps ("5 days ago"), so the Atom feed
// stays the primary source and the fallback exists to survive outage windows.
//
// The feed only accepts a channel id. Handles (@name) and channel URLs are
// resolved once against the public channel page; callers are expected to cache
// that mapping.
package youtube

import (
	"bytes"
	"context"
	"encoding/json"
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
	// MaxLimit is the widest answer the Atom feed can give.
	MaxLimit = 15
	// MaxAttempts bounds one request, including retries.
	MaxAttempts = 3

	// webAPIKey is the public key the YouTube web client ships with. It is not
	// a secret and needs no registration; it only identifies the client.
	webAPIKey = "AIzaSyAO_FJ2SlqU8Q4STEHLGCilw_Y9_11qcW8"
	// webClientVersion is sent with browse requests. YouTube accepts older
	// versions for a long time; bump it when the reply shape changes.
	webClientVersion = "2.20261026.01.00"
	// videosTabParams selects the Videos tab of a channel page.
	videosTabParams = "EgZ2aWRlb3PyBgQKAjoA"
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
	// DisableFallback keeps the client on the Atom feed only. Tests use it to
	// assert that a healthy feed never costs a browse request.
	DisableFallback bool
}

// Client reads YouTube over HTTP.
type Client struct {
	base       string
	ua         string
	http       *http.Client
	retryDelay time.Duration
	fallback   bool
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
		fallback:   !cfg.DisableFallback,
	}
}

var (
	// channelIDShape is the canonical form YouTube uses in feed URLs.
	channelIDShape = regexp.MustCompile(`^UC[A-Za-z0-9_-]{22}$`)
	// channelIDFromPage finds the channel id in the public channel page.
	channelIDFromPage = regexp.MustCompile(`"externalId":"(UC[A-Za-z0-9_-]{22})"`)
	// relativePattern matches the fallback's timestamps ("5 days ago").
	relativePattern = regexp.MustCompile(`(?i)(\d+|an?)\s+(second|minute|hour|day|week|month|year)s?\s+ago`)
	// viewsPattern matches "32K views" and "1,234 views".
	viewsPattern = regexp.MustCompile(`(?i)^([\d.,]+)\s*([KMB])?\s+views?$`)
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

// Videos reads the upload list for one channel id. The Atom feed is tried
// first; when it keeps failing, the browse endpoint answers instead so an
// outage window degrades to approximate timestamps rather than to no feed.
func (c *Client) Videos(ctx context.Context, channelID string) (*Feed, error) {
	channelID = strings.TrimSpace(channelID)
	if !channelIDShape.MatchString(channelID) {
		return nil, fmt.Errorf("youtube: invalid channel id %q", channelID)
	}
	body, feedErr := c.get(ctx, "/feeds/videos.xml?channel_id="+url.QueryEscape(channelID))
	if feedErr == nil {
		feed, err := parseAtomFeed(channelID, body)
		if err == nil {
			return feed, nil
		}
		feedErr = err
	}
	if c.fallback {
		if feed, err := c.browse(ctx, channelID); err == nil {
			return feed, nil
		}
	}
	return nil, feedErr
}

// parseAtomFeed turns one videos.xml body into a Feed.
func parseAtomFeed(channelID string, body []byte) (*Feed, error) {
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

// browse reads the channel's Videos tab through the web client's endpoint.
// Its timestamps are relative, so they become an approximate instant; the Atom
// feed remains the source of exact times.
func (c *Client) browse(ctx context.Context, channelID string) (*Feed, error) {
	payload, err := json.Marshal(map[string]any{
		"context": map[string]any{
			"client": map[string]any{
				"clientName":    "WEB",
				"clientVersion": webClientVersion,
				"hl":            "en",
				"gl":            "US",
			},
		},
		"browseId": channelID,
		"params":   videosTabParams,
	})
	if err != nil {
		return nil, err
	}
	body, err := c.post(ctx, "/youtubei/v1/browse?key="+webAPIKey+"&prettyPrint=false", payload)
	if err != nil {
		return nil, err
	}
	var doc browseDocument
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("youtube: browse parse: %w", err)
	}

	now := time.Now().UTC()
	out := &Feed{ChannelID: channelID, Title: strings.TrimSpace(doc.Metadata.Channel.Title)}
	for _, tab := range doc.Contents.Browse.Tabs {
		for _, row := range tab.Tab.Content.Grid.Contents {
			lk := row.Item.Content.Lockup
			if lk == nil || lk.ContentID == "" {
				continue
			}
			title := strings.TrimSpace(lk.Metadata.Lockup.Title.Content)
			if title == "" {
				continue
			}
			texts := lk.metadataTexts()
			out.Videos = append(out.Videos, Video{
				ID:          lk.ContentID,
				ChannelID:   channelID,
				Title:       title,
				Description: title,
				URL:         "https://www.youtube.com/watch?v=" + lk.ContentID,
				Thumbnail:   lk.bestThumbnail(),
				Published:   parseRelative(texts, now),
				Views:       parseViews(texts),
			})
		}
	}
	if len(out.Videos) == 0 {
		return nil, fmt.Errorf("youtube: browse returned no videos for %s", channelID)
	}
	return out, nil
}

// metadataTexts flattens the metadata rows of one lockup: typically the
// publisher, the view counter and the relative publish time.
func (l *browseLockup) metadataTexts() []string {
	var texts []string
	for _, row := range l.Metadata.Lockup.Metadata.Content.Rows {
		for _, part := range row.Parts {
			if text := strings.TrimSpace(part.Text.Content); text != "" {
				texts = append(texts, text)
			}
		}
	}
	return texts
}

// bestThumbnail returns the widest thumbnail source.
func (l *browseLockup) bestThumbnail() string {
	best, width := "", 0
	for _, src := range l.ContentImage.Thumb.Image.Sources {
		if src.URL != "" && src.Width >= width {
			best, width = src.URL, src.Width
		}
	}
	return best
}

// parseRelative turns "5 days ago" into an instant, using the first relative
// text in texts. Anything else (an empty list, "Upcoming") yields the zero
// time.
func parseRelative(texts []string, now time.Time) time.Time {
	for _, text := range texts {
		if t := parseRelativeText(text, now); !t.IsZero() {
			return t
		}
	}
	return time.Time{}
}

func parseRelativeText(text string, now time.Time) time.Time {
	text = strings.TrimSpace(text)
	for _, prefix := range []string{"Streamed ", "Premiered ", "Scheduled for "} {
		text = strings.TrimPrefix(text, prefix)
	}
	m := relativePattern.FindStringSubmatch(text)
	if m == nil {
		return time.Time{}
	}
	n := 1
	if m[1] != "a" && m[1] != "an" {
		parsed, err := strconv.Atoi(m[1])
		if err != nil || parsed <= 0 {
			return time.Time{}
		}
		n = parsed
	}
	var unit time.Duration
	switch strings.ToLower(m[2]) {
	case "second":
		unit = time.Second
	case "minute":
		unit = time.Minute
	case "hour":
		unit = time.Hour
	case "day":
		unit = 24 * time.Hour
	case "week":
		unit = 7 * 24 * time.Hour
	case "month":
		unit = 30 * 24 * time.Hour
	case "year":
		unit = 365 * 24 * time.Hour
	default:
		return time.Time{}
	}
	return now.Add(-time.Duration(n) * unit)
}

// parseViews reads a counter out of the metadata rows.
func parseViews(texts []string) int64 {
	for _, text := range texts {
		m := viewsPattern.FindStringSubmatch(text)
		if m == nil {
			continue
		}
		value, err := strconv.ParseFloat(strings.ReplaceAll(m[1], ",", ""), 64)
		if err != nil {
			return 0
		}
		switch strings.ToUpper(m[2]) {
		case "K":
			value *= 1e3
		case "M":
			value *= 1e6
		case "B":
			value *= 1e9
		}
		return int64(value)
	}
	return 0
}

// get performs a request with retries for the failure modes the feed is known
// for: 404/429/5xx and transport errors.
func (c *Client) get(ctx context.Context, path string) ([]byte, error) {
	return c.do(ctx, http.MethodGet, path, nil)
}

func (c *Client) post(ctx context.Context, path string, body []byte) ([]byte, error) {
	return c.do(ctx, http.MethodPost, path, body)
}

func (c *Client) do(ctx context.Context, method, path string, body []byte) ([]byte, error) {
	delay := c.retryDelay
	var lastErr error
	for attempt := 1; attempt <= MaxAttempts; attempt++ {
		out, err := c.doOnce(ctx, method, path, body)
		if err == nil {
			return out, nil
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

func (c *Client) doOnce(ctx context.Context, method, path string, body []byte) ([]byte, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.ua)
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-YouTube-Client-Name", "1")
		req.Header.Set("X-YouTube-Client-Version", webClientVersion)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
		return nil, &HTTPError{StatusCode: resp.StatusCode, URL: req.URL.String()}
	}
	out, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	return out, nil
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

// browseDocument mirrors the browse reply: the channel title, then one tab per
// channel section. Only the Videos tab carries a rich grid, and only its rows
// with a lockup are videos (the last row is a pagination marker).
type browseDocument struct {
	Contents struct {
		Browse struct {
			Tabs []browseTab `json:"tabs"`
		} `json:"twoColumnBrowseResultsRenderer"`
	} `json:"contents"`
	Metadata struct {
		Channel struct {
			Title string `json:"title"`
		} `json:"channelMetadataRenderer"`
	} `json:"metadata"`
}

type browseTab struct {
	Tab struct {
		Content struct {
			Grid struct {
				Contents []browseRow `json:"contents"`
			} `json:"richGridRenderer"`
		} `json:"content"`
	} `json:"tabRenderer"`
}

type browseRow struct {
	Item struct {
		Content struct {
			Lockup *browseLockup `json:"lockupViewModel"`
		} `json:"content"`
	} `json:"richItemRenderer"`
}

type browseLockup struct {
	ContentID string `json:"contentId"`
	Metadata  struct {
		Lockup struct {
			Title struct {
				Content string `json:"content"`
			} `json:"title"`
			Metadata struct {
				Content struct {
					Rows []struct {
						Parts []struct {
							Text struct {
								Content string `json:"content"`
							} `json:"text"`
						} `json:"metadataParts"`
					} `json:"metadataRows"`
				} `json:"contentMetadataViewModel"`
			} `json:"metadata"`
		} `json:"lockupMetadataViewModel"`
	} `json:"metadata"`
	ContentImage struct {
		Thumb struct {
			Image struct {
				Sources []struct {
					URL   string `json:"url"`
					Width int    `json:"width"`
				} `json:"sources"`
			} `json:"image"`
		} `json:"thumbnailViewModel"`
	} `json:"contentImage"`
}
