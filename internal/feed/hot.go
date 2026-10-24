package feed

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/yilong-musk/feedmesh/internal/weibo"
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
		func() ([]weibo.HotItem, error) { return s.clients.Weibo.HotSearch(ctx) })
	if err != nil {
		return nil, err
	}
	// realpos is the authoritative rank; do not rely on the order of the
	// upstream array, and keep the limit applied to the ranked order. Copy the
	// slice first because a value returned by single-flight may be shared with
	// other callers and sorting it in place would race with them.
	items = append([]weibo.HotItem(nil), items...)
	sort.SliceStable(items, func(i, j int) bool { return items[i].Rank < items[j].Rank })
	if len(items) > limit {
		items = items[:limit]
	}

	now := s.now().UTC()
	f := &Feed{
		TitleText:   "Weibo Hot Search",
		LinkText:    "https://s.weibo.com/top/summary",
		Description: "Realtime Weibo hot search ranking",
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
			Rank:      h.Rank,
			Heat:      h.Heat,
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
	b.WriteString(`<p>Heat `)
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
	b.WriteString(`">Search on Weibo</a></p>`)
	return b.String()
}

// FormatHeat renders a heat counter in compact units: K for thousands and M
// for millions.
func FormatHeat(n int64) string {
	switch {
	case n >= 1000000:
		return fmt.Sprintf("%.1fM", float64(n)/1000000)
	case n >= 1000:
		return fmt.Sprintf("%.1fK", float64(n)/1000)
	default:
		return fmt.Sprintf("%d", n)
	}
}
