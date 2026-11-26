package feed

import (
	"context"
	"errors"
	"fmt"
	"html"
	"strings"
	"time"

	"github.com/yilong-musk/feedmesh/internal/douyin"
	"github.com/yilong-musk/feedmesh/internal/weibo"
)

// Douyin feed bounds and cache lifetimes. One page carries at most 44 videos;
// the page itself needs a headless browser, so a ten-minute cache keeps polls
// cheap while staying close to a creator's publishing rhythm. The entry is
// stored for the whole last-good window, so an upstream failure degrades to the
// previous list instead of an error.
const (
	// DefaultDouyinLimit is the number of videos returned when the caller is silent.
	DefaultDouyinLimit = douyin.DefaultLimit
	// MaxDouyinLimit is the widest single-page answer.
	MaxDouyinLimit = douyin.MaxLimit

	douyinTTL = 10 * time.Minute
)

// DouyinTTL is the cache lifetime of one creator timeline.
func DouyinTTL() time.Duration { return douyinTTL }

// GetDouyin builds a creator feed from the public timeline page.
func (s *Service) GetDouyin(ctx context.Context, secUID string, limit int, refresh bool) (*Feed, error) {
	if s.clients.Douyin == nil {
		return nil, &weibo.Error{Code: weibo.CodeInternal, Message: "douyin provider is not configured"}
	}
	secUID = strings.TrimSpace(secUID)
	if secUID == "" {
		return nil, &weibo.Error{Code: weibo.CodeBadRequest, Message: "missing douyin user"}
	}
	if limit <= 0 {
		limit = DefaultDouyinLimit
	}
	if limit > MaxDouyinLimit {
		limit = MaxDouyinLimit
	}

	// Everything the feed needs — caption, author, identifiers, media — lives
	// on the item, so the cache stores items and can widen a narrow entry in
	// place. A plain read-through entry would let one ?limit=1 request narrow
	// every later read for the whole TTL, which is exactly what used to happen.
	key := "douyin:user:" + secUID
	entry, cached, degraded, err := fetchCachedList(ctx, s, key, douyinTTL, refresh, limit,
		func(n int) ([]douyin.Item, error) {
			tl, terr := s.clients.Douyin.Timeline(ctx, secUID, n)
			if terr != nil {
				return nil, terr
			}
			return tl.Items, nil
		})
	if err != nil {
		return nil, douyinError(err, secUID)
	}
	return douyinFeed(secUID, entry, limit, cached, degraded), nil
}

func douyinFeed(secUID string, c cachedList[douyin.Item], limit int, cached, degraded bool) *Feed {
	author := ""
	for _, v := range c.Items {
		if v.Author != "" {
			author = v.Author
			break
		}
	}
	if author == "" {
		author = secUID
	}
	n := len(c.Items)
	if limit < n {
		n = limit
	}
	items := make([]Item, 0, n)
	for _, v := range c.Items[:n] {
		it := Item{
			ID:         v.ID,
			CreatedAt:  v.CreatedAt,
			Text:       v.Desc,
			HTML:       douyinHTML(v),
			URL:        "https://www.douyin.com/video/" + v.ID,
			Title:      douyinTitle(v.Desc),
			Author:     v.Author,
			PlayURL:    v.PlayURL,
			DurationMS: v.DurationMS,
			Width:      v.Width,
			Height:     v.Height,
			IsTop:      v.IsTop,
		}
		if v.CoverURL != "" {
			it.Images = []string{v.CoverURL}
		}
		items = append(items, it)
	}
	return &Feed{
		Language:    langZhCN,
		UID:         secUID,
		ScreenName:  author,
		FetchedAt:   c.FetchedAt,
		TitleText:   author + " on Douyin",
		LinkText:    "https://www.douyin.com/user/" + secUID,
		Cached:      cached,
		Degraded:    degraded,
		Items:       items,
		Description: "Douyin videos from " + author,
	}
}

// douyinHTML renders one video. The play address expires within hours, so the
// body carries the cover and the page link instead of a dead stream URL.
func douyinHTML(v douyin.Item) string {
	var b strings.Builder
	if v.Desc != "" {
		b.WriteString("<p>")
		b.WriteString(html.EscapeString(v.Desc))
		b.WriteString("</p>")
	}
	if v.CoverURL != "" {
		b.WriteString(`<p><img src="`)
		b.WriteString(html.EscapeString(v.CoverURL))
		b.WriteString(`" alt="" loading="lazy" style="max-width:100%"></p>`)
	}
	meta := "Duration " + douyinDuration(v.DurationMS)
	if v.Width > 0 && v.Height > 0 {
		meta += fmt.Sprintf(" · %dx%d", v.Width, v.Height)
	}
	if v.IsTop {
		meta += " · Pinned"
	}
	b.WriteString("<p>")
	b.WriteString(html.EscapeString(meta))
	b.WriteString("</p>")
	return b.String()
}

func douyinDuration(ms int) string {
	if ms <= 0 {
		return "unknown"
	}
	sec := ms / 1000
	return fmt.Sprintf("%d:%02d", sec/60, sec%60)
}

// douyinTitle passes the caption through as-is. Douyin has no separate title
// field, and other providers keep their titles whole, so shortening it here
// would be the only place that drops source data. A client that wants a
// one-line label can shorten the text itself.
func douyinTitle(desc string) string {
	line := strings.TrimSpace(desc)
	if line == "" {
		return "(no content)"
	}
	return line
}

func douyinError(err error, secUID string) error {
	if err == nil {
		return nil
	}
	var we *weibo.Error
	if errors.As(err, &we) {
		return err
	}
	switch {
	case errors.Is(err, douyin.ErrInvalidUser):
		return &weibo.Error{Code: weibo.CodeBadRequest, Message: "invalid douyin user " + secUID, Err: err}
	case errors.Is(err, context.DeadlineExceeded):
		return &weibo.Error{Code: weibo.CodeTimeout, Message: "douyin timeline timed out", Err: err}
	case errors.Is(err, douyin.ErrNotReady):
		return &weibo.Error{Code: weibo.CodeUpstreamUnavailable, Message: "douyin timeline unavailable", Err: err}
	default:
		return &weibo.Error{Code: weibo.CodeInternal, Message: err.Error(), Err: err}
	}
}
