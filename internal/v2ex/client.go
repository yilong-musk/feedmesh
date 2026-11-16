// Package v2ex reads V2EX's official Atom feed and its hot-tab page.
//
// V2EX has no "hottest" feed, so Hot() scrapes https://www.v2ex.com/?tab=hot
// and falls back to the legacy API when the page shape changes.
package v2ex

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultBaseURL is the production site.
	DefaultBaseURL = "https://www.v2ex.com"
	// LatestLimit is how many entries index.xml carries.
	LatestLimit = 50
	// MaxLimit caps one feed request.
	MaxLimit = 50
)

// Article is one topic.
type Article struct {
	ID        int
	Title     string
	URL       string
	Author    string
	Node      string
	Content   string
	Published time.Time
	Replies   int
}

// Config tunes a Client.
type Config struct {
	BaseURL   string
	Timeout   time.Duration
	UserAgent string
}

// Client reads V2EX over HTTP.
type Client struct {
	base string
	ua   string
	http *http.Client
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
		cfg.UserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0 Safari/537.36"
	}
	return &Client{base: strings.TrimRight(cfg.BaseURL, "/"), ua: cfg.UserAgent, http: &http.Client{Timeout: cfg.Timeout}}
}

var (
	hotID       = regexp.MustCompile(`<a href="/t/(\d+)`)
	hotTitle    = regexp.MustCompile(`(?s)<span class="item_title">\s*<a href="/t/\d+[^"]*"[^>]*>([^<]+)</a>`)
	hotNode     = regexp.MustCompile(`<a class="node" href="/go/[^"]*">([^<]+)</a>`)
	hotReplies  = regexp.MustCompile(`class="count_livid"[^>]*>(\d+)</a>`)
	replyAnchor = regexp.MustCompile(`#reply\d+$`)
	topicIDRe   = regexp.MustCompile(`/t/(\d+)`)
)

// Latest reads the official Atom feed (index.xml), newest first.
func (c *Client) Latest(ctx context.Context, limit int) ([]Article, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > MaxLimit {
		limit = MaxLimit
	}
	body, err := c.get(ctx, "/index.xml")
	if err != nil {
		return nil, err
	}
	var feed struct {
		Entries []struct {
			Title string `xml:"title"`
			Link  []struct {
				Rel  string `xml:"rel,attr"`
				Href string `xml:"href,attr"`
			} `xml:"link"`
			Published string `xml:"published"`
			Author    struct {
				Name string `xml:"name"`
			} `xml:"author"`
			Content struct {
				HTML string `xml:",chardata"`
			} `xml:"content"`
		} `xml:"entry"`
	}
	if err := xml.Unmarshal(body, &feed); err != nil {
		return nil, fmt.Errorf("v2ex atom parse: %w", err)
	}
	out := make([]Article, 0, len(feed.Entries))
	for _, e := range feed.Entries {
		url := ""
		for _, l := range e.Link {
			if l.Rel == "alternate" && l.Href != "" {
				url = l.Href
				break
			}
		}
		if url == "" {
			continue
		}
		id := topicID(url)
		if id == 0 {
			continue
		}
		pub, _ := time.Parse(time.RFC3339, strings.TrimSpace(e.Published))
		out = append(out, Article{
			ID:        id,
			Title:     strings.TrimSpace(e.Title),
			URL:       replyAnchor.ReplaceAllString(url, ""),
			Author:    strings.TrimSpace(e.Author.Name),
			Content:   strings.TrimSpace(e.Content.HTML),
			Published: pub,
		})
		if len(out) >= limit {
			break
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("v2ex atom feed returned no entries")
	}
	return out, nil
}

// Hot reads the hot tab, filters promoted nodes and sorts by reply count.
//
// The hot ranking exists only on that page; the legacy API endpoint returns a
// different list (today's most-discussed topics). A failed page read is
// reported as an error so the caller can serve its last good copy instead of
// silently substituting a list with different semantics.
func (c *Client) Hot(ctx context.Context, limit int) ([]Article, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > MaxLimit {
		limit = MaxLimit
	}
	body, err := c.get(ctx, "/?tab=hot")
	if err != nil {
		return nil, err
	}
	if bytes.Contains(body, []byte("Just a moment")) {
		return nil, fmt.Errorf("v2ex hot page is behind a Cloudflare challenge")
	}
	items, err := parseHotPage(body)
	if err != nil {
		return nil, err
	}
	if len(items) < 5 {
		return nil, fmt.Errorf("v2ex hot page returned only %d topics", len(items))
	}
	return finishHot(items, limit), nil
}

// promotedNode is the upstream node title that marks advertising posts. It is
// upstream data, not prose, so it stays in its original form.
const promotedNode = "推广" // i18n-ignore: upstream data

func finishHot(items []Article, limit int) []Article {
	kept := items[:0]
	for _, a := range items {
		if a.Node == promotedNode {
			continue
		}
		kept = append(kept, a)
	}
	sort.SliceStable(kept, func(i, j int) bool { return kept[i].Replies > kept[j].Replies })
	if len(kept) > limit {
		kept = kept[:limit]
	}
	return kept
}

func parseHotPage(page []byte) ([]Article, error) {
	parts := strings.Split(string(page), `<div class="cell item">`)
	out := make([]Article, 0, len(parts))
	for _, part := range parts[1:] {
		b := []byte(part)
		m := hotID.FindSubmatch(b)
		t := hotTitle.FindSubmatch(b)
		if m == nil || t == nil {
			continue
		}
		id, _ := strconv.Atoi(string(m[1]))
		if id == 0 {
			continue
		}
		node := ""
		if n := hotNode.FindSubmatch(b); n != nil {
			node = html.UnescapeString(string(n[1]))
		}
		replies := 0
		if r := hotReplies.FindSubmatch(b); r != nil {
			replies, _ = strconv.Atoi(string(r[1]))
		}
		out = append(out, Article{
			ID:      id,
			Title:   strings.TrimSpace(html.UnescapeString(string(t[1]))),
			URL:     fmt.Sprintf("%s/t/%d", DefaultBaseURL, id),
			Node:    strings.TrimSpace(node),
			Replies: replies,
		})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("v2ex hot page parsed empty")
	}
	return out, nil
}

func topicID(raw string) int {
	if m := topicIDRe.FindStringSubmatch(raw); m != nil {
		id, _ := strconv.Atoi(m[1])
		return id
	}
	return 0
}

func (c *Client) get(ctx context.Context, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.ua)
	// Accept-Language is deliberately not sent: since 2026-11 V2EX answers
	// HTML pages carrying a zh-CN preference with a Cloudflare JS challenge
	// (403, "Just a moment..."), while the same request without it gets the
	// page. The Atom feed is unaffected either way.
	req.Header.Set("Accept", "*/*")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("v2ex request failed: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("v2ex read failed: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("v2ex returned HTTP %d", resp.StatusCode)
	}
	return body, nil
}
