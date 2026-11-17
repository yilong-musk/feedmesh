// Package telegram reads public Telegram channels through the t.me web
// preview. No account, bot token or API key is involved: a channel that has
// the public preview enabled serves an ordinary HTML page at /s/<name> that
// lists recent posts together with their text, photos, video thumbnails, link
// cards, view counters and timestamps.
//
// The package deliberately holds no third-party modules. The preview page is
// a flat HTML document with one widget per post, so parsing is a small
// regexp and balanced-<div> scan instead of a DOM implementation.
package telegram

import (
	"context"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/yilong-musk/feedmesh/internal/logx"
)

// DefaultBaseURL is the public preview host. Tests point BaseURL at an
// httptest server; the /s/<channel> path is kept either way.
const DefaultBaseURL = "https://t.me"

const (
	// DefaultLimit is the number of posts returned when the caller is silent.
	DefaultLimit = 20
	// MaxLimit is one preview page. Older posts need ?before=<id> pagination;
	// the provider deliberately stays on the newest page so a reader polling
	// every few minutes does not spend several requests on history it already
	// has.
	MaxLimit = 20

	// userAgent is a desktop browser UA. t.me serves the same page to other
	// clients today, but a browser UA keeps the preview markup stable.
	userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"
)

// Errors callers map onto HTTP responses.
var (
	// ErrInvalidChannel means the channel name is malformed.
	ErrInvalidChannel = errors.New("invalid telegram channel")
	// ErrNotFound means t.me has no such channel.
	ErrNotFound = errors.New("telegram channel not found")
	// ErrNoPreview means the channel exists but does not serve the public web
	// preview (private channels and groups are the common cases), so an
	// anonymous client cannot read it.
	ErrNoPreview = errors.New("telegram channel has no public web preview")
	// ErrRateLimited means t.me asked us to slow down.
	ErrRateLimited = errors.New("telegram rate limited")
	// ErrNoPosts means the page parsed but carried no visible post. Treating
	// it as an error keeps the last good list in place instead of caching an
	// empty feed.
	ErrNoPosts = errors.New("telegram channel has no visible posts")
	// ErrUnexpectedPage means the response was not a preview page at all.
	ErrUnexpectedPage = errors.New("telegram preview page could not be parsed")
)

var (
	channelRe = regexp.MustCompile(`^[a-z][a-z0-9_]{2,63}$`)

	channelTitleRe      = regexp.MustCompile(`(?s)<div class="tgme_channel_info_header_title"><span[^>]*>(.*?)</span></div>`)
	channelDescRe       = regexp.MustCompile(`(?s)<div class="tgme_channel_info_description">(.*?)</div>`)
	channelAvatarRe     = regexp.MustCompile(`(?s)<div class="tgme_channel_info_header">.*?<i class="tgme_page_photo_image[^"]*"[^>]*>\s*<img src="([^"]+)"`)
	channelSubscriberRe = regexp.MustCompile(`(?s)<span class="counter_value">([^<]*)</span>\s*<span class="counter_type">subscribers</span>`)

	messageOpenRe = regexp.MustCompile(`(?s)<div class="tgme_widget_message\b[^"]*"[^>]*data-post="([^"]+)"[^>]*>`)
	divTagRe      = regexp.MustCompile(`(?is)<(/?)div\b[^>]*>`)
	timeRe        = regexp.MustCompile(`<time datetime="([^"]+)"`)
	textOpenRe    = regexp.MustCompile(`(?s)<div class="tgme_widget_message_text\b[^"]*"[^>]*>`)

	ownerNameRe   = regexp.MustCompile(`(?s)<a class="tgme_widget_message_owner_name"[^>]*>(.*?)</a>`)
	fromAuthorRe  = regexp.MustCompile(`(?s)<span class="tgme_widget_message_from_author"[^>]*>(.*?)</span>`)
	viewsRe       = regexp.MustCompile(`<span class="tgme_widget_message_views">([^<]*)</span>`)
	forwardedRe   = regexp.MustCompile(`(?s)<div class="tgme_widget_message_forwarded_from\b[^"]*"[^>]*>`)
	photoAnchorRe = regexp.MustCompile(`(?s)<a class="tgme_widget_message_photo_wrap\b[^"]*"[^>]*>`)
	videoThumbRe  = regexp.MustCompile(`(?s)<i class="tgme_widget_message_video_thumb"[^>]*>`)
	backgroundRe  = regexp.MustCompile(`background-image:url\('([^']+)'\)`)

	linkPreviewRe = regexp.MustCompile(`(?s)<a class="tgme_widget_message_link_preview"[^>]*href="([^"]*)"[^>]*>`)
	linkSiteRe    = regexp.MustCompile(`(?s)<div class="link_preview_site_name[^"]*"[^>]*>(.*?)</div>`)
	linkTitleRe   = regexp.MustCompile(`(?s)<div class="link_preview_title"[^>]*>(.*?)</div>`)
	linkDescRe    = regexp.MustCompile(`(?s)<div class="link_preview_description"[^>]*>(.*?)</div>`)
	linkImageRe   = regexp.MustCompile(`(?s)<i class="link_preview_image"[^>]*style="[^"]*background-image:url\('([^']+)'\)`)

	anyTag   = regexp.MustCompile(`(?s)<[^>]*>`)
	brTag    = regexp.MustCompile(`(?i)<br\s*/?>`)
	spaceRun = regexp.MustCompile(`[ \t\x{00a0}]+`)
)

// Config configures a Client.
type Config struct {
	// BaseURL overrides the preview host for tests.
	BaseURL string
	// Timeout bounds one whole Fetch call.
	Timeout time.Duration
	// Log reports lifecycle events; nil is silent.
	Log *logx.Logger
}

// Client reads public Telegram channel previews. It is safe for concurrent
// use; the upstream tolerates a page request every few minutes, and the feed
// layer's cache keeps the steady-state rate well below that.
type Client struct {
	cfg  Config
	http *http.Client
}

// New builds a Client.
func New(cfg Config) *Client {
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultBaseURL
	}
	return &Client{
		cfg: cfg,
		http: &http.Client{
			Timeout: cfg.Timeout,
			// The preview path redirects when a channel has no public preview.
			// Reading the redirect instead of following it separates "private
			// or preview disabled" from a normal 200 landing page.
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
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

// Post is one message in a channel preview.
type Post struct {
	Channel   string
	ID        int64
	CreatedAt time.Time
	// TextHTML is the inner HTML of the message body, exactly as Telegram
	// rendered it. The feed layer sanitises it before it reaches RSS.
	TextHTML string
	// Author is the channel's display name as printed above the post.
	Author string
	// Signature is the per-post author signature some channels enable.
	Signature string
	Views     int64
	// Images are photo URLs (one per photo in a grouped album).
	Images []string
	// PosterURL is the still frame of a video post.
	PosterURL string
	// Media names the dominant attachment: photo, video, document, audio,
	// sticker, poll, location, contact or media. Empty for text-only posts.
	Media string
	// LinkPreview describes an attached link card, when present.
	LinkURL   string
	LinkSite  string
	LinkTitle string
	LinkText  string
	LinkImage string
	// ForwardedFrom is the source label of a forwarded post.
	ForwardedFrom string

	// Channel metadata is repeated on each post so a cached list answers the
	// feed header without a second cache key. Entries are tiny and the list
	// helper already stores the post list.
	ChannelTitle       string
	ChannelDescription string
	ChannelAvatar      string
	ChannelSubscribers string
}

// Channel is one preview page.
type Channel struct {
	Name        string
	Title       string
	Description string
	Avatar      string
	Subscribers string
	FetchedAt   time.Time
	Posts       []Post
}

// NormalizeChannel accepts @name, name, t.me/name, t.me/s/name and full
// https://t.me/s/name URLs, and returns the bare lowercase channel name.
func NormalizeChannel(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	s = strings.TrimSpace(strings.TrimPrefix(s, "@"))
	if s == "" {
		return "", ErrInvalidChannel
	}
	low := strings.ToLower(s)
	for _, prefix := range []string{"https://", "http://"} {
		if strings.HasPrefix(low, prefix) {
			s = s[len(prefix):]
			low = strings.ToLower(s)
			break
		}
	}
	for _, prefix := range []string{"t.me/", "telegram.me/", "www.t.me/"} {
		if strings.HasPrefix(low, prefix) {
			s = s[len(prefix):]
			low = strings.ToLower(s)
			break
		}
	}
	if strings.HasPrefix(low, "s/") {
		s = s[2:]
	}
	// A trailing post id or query never identifies another channel; drop it.
	if i := strings.IndexAny(s, "/?#"); i >= 0 {
		s = s[:i]
	}
	s = strings.ToLower(strings.TrimSpace(s))
	if !channelRe.MatchString(s) {
		return "", ErrInvalidChannel
	}
	return s, nil
}

// Fetch reads the newest page of a channel. The returned posts are sorted
// newest first (Telegram prints them oldest first) and cut to limit.
func (c *Client) Fetch(ctx context.Context, raw string, limit int) (*Channel, error) {
	name, err := NormalizeChannel(raw)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = DefaultLimit
	}
	if limit > MaxLimit {
		limit = MaxLimit
	}

	body, status, err := c.get(ctx, c.cfg.BaseURL+"/s/"+url.PathEscape(name))
	if err != nil {
		return nil, err
	}
	switch status {
	case http.StatusOK:
	case http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther,
		http.StatusTemporaryRedirect, http.StatusPermanentRedirect:
		return nil, ErrNoPreview
	case http.StatusNotFound:
		return nil, ErrNotFound
	case http.StatusTooManyRequests:
		return nil, ErrRateLimited
	default:
		return nil, fmt.Errorf("telegram preview status %d", status)
	}

	ch, err := ParsePage(name, body)
	if err != nil {
		return nil, err
	}
	if len(ch.Posts) == 0 {
		return nil, ErrNoPosts
	}
	sort.SliceStable(ch.Posts, func(i, j int) bool { return ch.Posts[i].ID > ch.Posts[j].ID })
	if len(ch.Posts) > limit {
		ch.Posts = ch.Posts[:limit]
	}
	ch.FetchedAt = time.Now().UTC()
	if ch.Title == "" {
		ch.Title = name
	}
	for i := range ch.Posts {
		p := &ch.Posts[i]
		p.Channel = name
		p.ChannelTitle = ch.Title
		p.ChannelDescription = ch.Description
		p.ChannelAvatar = ch.Avatar
		p.ChannelSubscribers = ch.Subscribers
		if p.Author == "" {
			p.Author = ch.Title
		}
	}
	c.logf("telegram preview read", "channel", name, "posts", len(ch.Posts))
	return ch, nil
}

// ParsePage turns one preview document into a Channel. It is exported so the
// feed-level tests can reuse real fixtures without an HTTP server.
func ParsePage(name, body string) (*Channel, error) {
	if !strings.Contains(body, "tgme_channel_info") && !strings.Contains(body, "tgme_widget_message") {
		return nil, ErrUnexpectedPage
	}
	ch := &Channel{Name: name}
	if m := channelTitleRe.FindStringSubmatch(body); m != nil {
		ch.Title = cleanText(m[1])
	}
	if m := channelDescRe.FindStringSubmatch(body); m != nil {
		ch.Description = cleanText(m[1])
	}
	if m := channelAvatarRe.FindStringSubmatch(body); m != nil {
		ch.Avatar = html.UnescapeString(m[1])
	}
	if m := channelSubscriberRe.FindStringSubmatch(body); m != nil {
		ch.Subscribers = strings.TrimSpace(m[1])
	}

	matches := messageOpenRe.FindAllStringSubmatchIndex(body, -1)
	for i, idx := range matches {
		postRef := body[idx[2]:idx[3]]
		openEnd := idx[1]
		closeAt := findDivEnd(body, openEnd)
		// A malformed or truncated block must never swallow the next post: the
		// preview markup is upstream HTML, so the next widget is the hard
		// boundary.
		if i+1 < len(matches) && matches[i+1][0] < closeAt {
			closeAt = matches[i+1][0]
		}
		seg := body[openEnd:closeAt]
		if p, ok := parsePost(name, postRef, seg); ok {
			ch.Posts = append(ch.Posts, p)
		}
	}
	return ch, nil
}

// findDivEnd returns the index of the </div> that closes the element whose
// content starts at start. The opening tag has already been consumed, so the
// scan starts at depth one.
func findDivEnd(s string, start int) int {
	depth := 1
	for _, m := range divTagRe.FindAllStringSubmatchIndex(s[start:], -1) {
		if m[3] > m[2] {
			depth--
		} else {
			depth++
		}
		if depth == 0 {
			return start + m[0]
		}
	}
	return len(s)
}

func parsePost(channel, ref, seg string) (Post, bool) {
	idText := ref
	if i := strings.LastIndexByte(ref, '/'); i >= 0 {
		idText = ref[i+1:]
	}
	id, err := strconv.ParseInt(idText, 10, 64)
	if err != nil || id <= 0 {
		return Post{}, false
	}
	p := Post{Channel: channel, ID: id}
	if m := timeRe.FindStringSubmatch(seg); m != nil {
		if t, err := time.Parse(time.RFC3339, m[1]); err == nil {
			p.CreatedAt = t.UTC()
		}
	}
	if m := textOpenRe.FindStringSubmatchIndex(seg); m != nil {
		start := m[1]
		p.TextHTML = strings.TrimSpace(seg[start:findDivEnd(seg, start)])
	}
	if m := ownerNameRe.FindStringSubmatch(seg); m != nil {
		p.Author = cleanText(m[1])
	}
	if m := fromAuthorRe.FindStringSubmatch(seg); m != nil {
		p.Signature = cleanText(m[1])
	}
	if m := viewsRe.FindStringSubmatch(seg); m != nil {
		p.Views = parseCompactCount(m[1])
	}
	for _, m := range photoAnchorRe.FindAllString(seg, -1) {
		if u := backgroundURL(m); u != "" {
			p.Images = append(p.Images, u)
		}
	}
	if m := videoThumbRe.FindString(seg); m != "" {
		p.PosterURL = backgroundURL(m)
	}
	p.Media = mediaKind(seg, len(p.Images) > 0, p.PosterURL != "")
	p.LinkURL, p.LinkSite, p.LinkTitle, p.LinkText, p.LinkImage = parseLinkPreview(seg)
	if m := forwardedRe.FindStringSubmatchIndex(seg); m != nil {
		p.ForwardedFrom = cleanText(seg[m[1]:findDivEnd(seg, m[1])])
	}

	if p.TextHTML == "" && len(p.Images) == 0 && p.PosterURL == "" &&
		p.LinkURL == "" && p.Media == "" {
		// Service messages (joins, pins, title changes) carry no content.
		return Post{}, false
	}
	return p, true
}

// mediaKind names the dominant attachment in one message segment.
func mediaKind(seg string, hasPhoto, hasVideoPoster bool) string {
	switch {
	case strings.Contains(seg, "tgme_widget_message_video_player"),
		strings.Contains(seg, "tgme_widget_message_video_wrap"):
		return "video"
	case hasVideoPoster:
		return "video"
	case hasPhoto:
		return "photo"
	case strings.Contains(seg, "tgme_widget_message_document"):
		return "document"
	case strings.Contains(seg, "tgme_widget_message_audio_player"):
		return "audio"
	case strings.Contains(seg, "tgme_widget_message_sticker"):
		return "sticker"
	case strings.Contains(seg, "tgme_widget_message_poll"):
		return "poll"
	case strings.Contains(seg, "tgme_widget_message_location"):
		return "location"
	case strings.Contains(seg, "tgme_widget_message_contact"):
		return "contact"
	case strings.Contains(seg, "tgme_widget_message_roundvideo"):
		return "video"
	}
	return ""
}

func parseLinkPreview(seg string) (link, site, title, text, image string) {
	m := linkPreviewRe.FindStringSubmatchIndex(seg)
	if m == nil {
		return "", "", "", "", ""
	}
	link = html.UnescapeString(seg[m[2]:m[3]])
	end := strings.Index(seg[m[1]:], "</a>")
	if end < 0 {
		end = len(seg)
	} else {
		end += m[1]
	}
	block := seg[m[1]:end]
	if mm := linkSiteRe.FindStringSubmatch(block); mm != nil {
		site = cleanText(mm[1])
	}
	if mm := linkTitleRe.FindStringSubmatch(block); mm != nil {
		title = cleanText(mm[1])
	}
	if mm := linkDescRe.FindStringSubmatch(block); mm != nil {
		text = cleanText(mm[1])
	}
	if mm := linkImageRe.FindStringSubmatch(block); mm != nil {
		image = html.UnescapeString(mm[1])
	}
	return link, site, title, text, image
}

// backgroundURL extracts the URL from an inline background-image style. The
// callers pass a complete opening tag, so the match can only come from that
// element's own style attribute.
func backgroundURL(tag string) string {
	m := backgroundRe.FindStringSubmatch(tag)
	if m == nil {
		return ""
	}
	u := html.UnescapeString(m[1])
	if strings.HasPrefix(u, "//") {
		u = "https:" + u
	}
	return u
}

// cleanText flattens a small HTML fragment to one line of plain text.
func cleanText(s string) string {
	s = brTag.ReplaceAllString(s, " ")
	s = anyTag.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	s = spaceRun.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

// parseCompactCount reads counters such as 2.8K, 1.2M and 17,300.
func parseCompactCount(s string) int64 {
	s = strings.TrimSpace(strings.ReplaceAll(s, ",", ""))
	if s == "" {
		return 0
	}
	mult := float64(1)
	switch s[len(s)-1] {
	case 'K', 'k':
		mult, s = 1_000, s[:len(s)-1]
	case 'M', 'm':
		mult, s = 1_000_000, s[:len(s)-1]
	case 'B', 'b':
		mult, s = 1_000_000_000, s[:len(s)-1]
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0
	}
	return int64(f * mult)
}

func (c *Client) get(ctx context.Context, target string) (string, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	resp, err := c.http.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return "", resp.StatusCode, err
	}
	return string(body), resp.StatusCode, nil
}
