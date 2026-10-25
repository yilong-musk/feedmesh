// Package smzdm reads the official SMZDM (smzdm.com) RSS channels.
//
// The feed is the only channel this package touches. SMZDM's search and
// category pages sit behind a WAF that answers automated requests with a
// challenge page, while three of the five channels listed on the subscription
// page serve plain RSS anonymously:
//
//	deals   http://feed.smzdm.com/          selected deals (HTTPS answers 403)
//	post    https://post.smzdm.com/feed     community articles
//	haitao  https://haitao.smzdm.com/feed   overseas deals
//
// The "faxian" and "news" channels answer 302/202 with a WAF challenge and are
// deliberately not wired up.
package smzdm

import (
	"context"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	// MaxLimit is the width of one feed slice; every channel carries 30 entries.
	MaxLimit = 30
	// DefaultLimit is used when the caller asks for nothing.
	DefaultLimit = 20
)

// Channel names one of the official RSS channels.
type Channel string

// Supported channels.
const (
	ChannelDeals  Channel = "deals"
	ChannelPost   Channel = "post"
	ChannelHaitao Channel = "haitao"
)

// channelHost and channelPath split the feed URL so a test or mirror can
// replace the host without losing the channel's path.
var (
	channelHost = map[Channel]string{
		ChannelDeals:  "http://feed.smzdm.com",
		ChannelPost:   "https://post.smzdm.com",
		ChannelHaitao: "https://haitao.smzdm.com",
	}
	channelPath = map[Channel]string{
		ChannelDeals:  "/",
		ChannelPost:   "/feed",
		ChannelHaitao: "/feed",
	}
)

// ParseChannel normalises a channel name; an empty name means deals.
func ParseChannel(name string) (Channel, bool) {
	switch Channel(strings.ToLower(strings.TrimSpace(name))) {
	case "", ChannelDeals:
		return ChannelDeals, true
	case ChannelPost:
		return ChannelPost, true
	case ChannelHaitao:
		return ChannelHaitao, true
	}
	return "", false
}

// beijing is the zone SMZDM stamps when pubDate carries no zone suffix.
var beijing = time.FixedZone("CST", 8*60*60)

// Config tunes a Client.
type Config struct {
	// BaseURL replaces the host of every channel; the channel path is kept.
	// Tests and mirrors use it; production leaves it empty.
	BaseURL   string
	Timeout   time.Duration
	UserAgent string
}

// Client reads the SMZDM RSS channels. It is safe for concurrent use.
type Client struct {
	base string
	ua   string
	http *http.Client
}

// New builds a Client with production defaults.
func New(cfg Config) *Client {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 15 * time.Second
	}
	if cfg.UserAgent == "" {
		// The default Go agent is rejected by the CDN in front of the feed.
		cfg.UserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0 Safari/537.36"
	}
	return &Client{
		base: strings.TrimRight(cfg.BaseURL, "/"),
		ua:   cfg.UserAgent,
		http: &http.Client{Timeout: cfg.Timeout},
	}
}

// URL is the feed address for one channel.
func (c *Client) URL(ch Channel) string {
	host := channelHost[ch]
	if c.base != "" {
		host = c.base
	}
	return host + channelPath[ch]
}

// Deal is one feed entry.
type Deal struct {
	ID        string
	Title     string
	URL       string
	Author    string
	Summary   string
	Image     string
	Published time.Time
}

type rssDoc struct {
	Channel struct {
		Items []struct {
			Title       string `xml:"title"`
			Link        string `xml:"link"`
			PubDate     string `xml:"pubDate"`
			Description string `xml:"description"`
			FocusPic    string `xml:"focus_pic"`
			Author      string `xml:"author"`
			Content     string `xml:"encoded"`
		} `xml:"item"`
	} `xml:"channel"`
}

var (
	dealID   = regexp.MustCompile(`/p/([A-Za-z0-9]+)`)
	itemProp = regexp.MustCompile(`(?is)<p[^>]*itemprop="description"[^>]*>(.*?)</p>`)
	htmlTags = regexp.MustCompile(`(?s)<[^>]+>`)
	footer   = regexp.MustCompile(`本文来自\s*什么值得买网站\s*[（(]?\s*www\.smzdm\.com\s*[）)]?\s*。?`) // i18n-ignore: upstream footer text, matched literally
)

// List returns up to limit entries from one channel, newest first.
//
// The deal slice rotates: it is not a stable "latest 30" window, and entries
// inside one slice are not strictly time-ordered, so the client sorts before
// truncating.
func (c *Client) List(ctx context.Context, ch Channel, limit int) ([]Deal, error) {
	if _, ok := channelHost[ch]; !ok {
		return nil, fmt.Errorf("smzdm: unknown channel %q", ch)
	}
	if limit <= 0 {
		limit = DefaultLimit
	}
	if limit > MaxLimit {
		limit = MaxLimit
	}

	body, err := c.get(ctx, ch)
	if err != nil {
		return nil, err
	}

	var doc rssDoc
	if err := xml.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("smzdm %s feed parse: %w", ch, err)
	}
	out := make([]Deal, 0, len(doc.Channel.Items))
	for _, it := range doc.Channel.Items {
		link := strings.TrimSpace(it.Link)
		title := strings.TrimSpace(it.Title)
		m := dealID.FindStringSubmatch(link)
		if m == nil || title == "" {
			continue
		}
		author, summary := splitDescription(ch, strings.TrimSpace(it.Description), it.Content, strings.TrimSpace(it.Author))
		out = append(out, Deal{
			ID:        m[1],
			Title:     title,
			URL:       link,
			Author:    author,
			Summary:   summary,
			Image:     strings.TrimSpace(it.FocusPic),
			Published: parseTime(it.PubDate),
		})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("smzdm %s feed returned no entries", ch)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Published.After(out[j].Published)
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (c *Client) get(ctx context.Context, ch Channel) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.URL(ch), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.ua)
	req.Header.Set("Accept", "application/rss+xml, application/xml;q=0.9, */*;q=0.8")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("smzdm %s request failed: %w", ch, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, fmt.Errorf("smzdm %s read failed: %w", ch, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("smzdm %s returned HTTP %d", ch, resp.StatusCode)
	}
	return body, nil
}

// cleanContent extracts the human summary from content:encoded. Most deal
// entries carry only the "this article comes from smzdm" footer there; entries
// with a blurb put it in a paragraph marked itemprop="description".
func cleanContent(raw string) string {
	m := itemProp.FindStringSubmatch(raw)
	if m == nil {
		return ""
	}
	s := htmlTags.ReplaceAllString(m[1], " ")
	s = html.UnescapeString(s)
	s = footer.ReplaceAllString(s, " ")
	return collapse(s)
}

// cleanText strips markup from a text field, used by the article channel whose
// description is the blurb itself.
func cleanText(raw string) string {
	return collapse(html.UnescapeString(htmlTags.ReplaceAllString(raw, " ")))
}

// splitDescription maps one entry onto author and blurb.
//
// The deal channels pack both into <description> without a separator:
// "nickname + summary", or the nickname alone. The article channel separates
// them: <author> carries the name and <description> the blurb.
func splitDescription(ch Channel, desc, content, author string) (string, string) {
	if ch == ChannelPost {
		return strings.TrimSpace(author), cleanText(desc)
	}
	return splitDealDescription(desc, cleanContent(content))
}

// splitDealDescription splits the deal channels' "nickname + summary" pair.
func splitDealDescription(desc, summary string) (author, text string) {
	if summary != "" {
		d, s := collapse(desc), collapse(summary)
		if i := strings.Index(d, s); i > 0 {
			return strings.TrimSpace(d[:i]), summary
		}
		return "", summary
	}
	if desc == "" {
		return "", ""
	}
	// Without a clean summary, a short punctuation-free description is the
	// nickname; anything longer is treated as the blurb itself.
	if len([]rune(desc)) <= 14 && !strings.ContainsAny(desc, "，。！？；：、,.!?;:") {
		return desc, ""
	}
	return "", desc
}

// parseTime accepts both stamped formats SMZDM emits; a zone-less stamp is
// Beijing time.
func parseTime(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	if t, err := time.ParseInLocation("Mon, 02 Jan 2006 15:04:05", s, beijing); err == nil {
		return t
	}
	for _, layout := range []string{"Mon, 02 Jan 2006 15:04:05 -0700", time.RFC1123Z, time.RFC1123} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

func collapse(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
