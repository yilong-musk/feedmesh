package feed

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/yilong-musk/weibo-rss-pro/internal/weibo"
)

// Hot list bounds and cache lifetime.
const (
	DefaultHotLimit = 20
	MaxHotLimit     = 50
	hotTTL          = 5 * time.Minute
)

// GetHot builds the realtime hot search feed. Items share one pubDate; their
// guid is the search URL, so a topic keeps the same identity while its rank
// moves, and readers do not re-alert on every refresh.
func (s *Service) GetHot(ctx context.Context, limit int, refresh bool) (*Feed, error) {
	if limit <= 0 {
		limit = DefaultHotLimit
	}
	if limit > MaxHotLimit {
		limit = MaxHotLimit
	}

	items, hit, err := fetchCached(ctx, s, "hot:top", hotTTL, refresh,
		func() ([]weibo.HotItem, error) { return s.client.HotSearch(ctx) })
	if err != nil {
		return nil, err
	}
	if len(items) > limit {
		items = items[:limit]
	}

	now := s.now().UTC()
	f := &Feed{
		TitleText:   "微博热搜",
		LinkText:    "https://s.weibo.com/top/summary",
		Description: "微博实时热搜榜",
		FetchedAt:   now,
		Cached:      hit,
		TTL:         5,
		Items:       make([]Item, 0, len(items)),
	}
	for _, h := range items {
		link := HotSearchLink(h)
		f.Items = append(f.Items, Item{
			ID:        fmt.Sprintf("hot-%d", h.Rank),
			Title:     fmt.Sprintf("%d. %s", h.Rank, h.Word),
			Text:      h.Word,
			HTML:      hotHTML(h, link),
			URL:       link,
			CreatedAt: now,
			Images:    []string{},
		})
	}
	return f, nil
}

// HotSearchLink builds the weibo search URL for a hot entry. Topics keep their
// hash marks; plain keywords are searched verbatim.
func HotSearchLink(h weibo.HotItem) string {
	q := h.Word
	if h.TopicFlag == 1 || strings.Contains(h.WordScheme, "#") {
		q = "#" + strings.Trim(h.Word, "#") + "#"
	}
	return "https://s.weibo.com/weibo?q=" + url.QueryEscape(q)
}

func hotHTML(h weibo.HotItem, link string) string {
	var b strings.Builder
	b.WriteString(`<p>热度 `)
	b.WriteString(FormatHeat(h.Heat))
	if h.Label != "" {
		b.WriteString(` · `)
		b.WriteString(EscapeXML(h.Label))
	}
	if h.Category != "" {
		b.WriteString(` · `)
		b.WriteString(EscapeXML(h.Category))
	}
	b.WriteString(`</p><p><a href="`)
	b.WriteString(EscapeXML(link))
	b.WriteString(`">在微博搜索</a></p>`)
	return b.String()
}

// FormatHeat renders the heat counter the way weibo does: 万 above ten thousand.
func FormatHeat(n int64) string {
	switch {
	case n >= 100000000:
		return fmt.Sprintf("%.1f 亿", float64(n)/100000000)
	case n >= 10000:
		return fmt.Sprintf("%.1f 万", float64(n)/10000)
	default:
		return fmt.Sprintf("%d", n)
	}
}
