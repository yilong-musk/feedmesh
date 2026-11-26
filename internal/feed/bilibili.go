package feed

import (
	"context"
	"errors"
	"fmt"
	"html"
	"strings"
	"time"

	"github.com/yilong-musk/feedmesh/internal/apperr"
	"github.com/yilong-musk/feedmesh/internal/bilibili"
	"github.com/yilong-musk/feedmesh/internal/cachekey"
)

// Bilibili feed bounds and cache lifetimes. One page carries up to 50 videos,
// and the signed endpoint rate-limits aggressively, so a ten-minute cache also
// keeps the upstream calm. The entry is stored for the whole last-good window.
const (
	// DefaultBilibiliLimit is the number of videos returned when the caller is silent.
	DefaultBilibiliLimit = bilibili.DefaultLimit
	// MaxBilibiliLimit is the widest single-page answer.
	MaxBilibiliLimit = bilibili.MaxLimit

	bilibiliTTL = 10 * time.Minute
)

// BilibiliTTL is the cache lifetime of one creator timeline.
func BilibiliTTL() time.Duration { return bilibiliTTL }

// GetBilibili builds a creator feed from the public space endpoint.
func (s *Service) GetBilibili(ctx context.Context, mid string, limit int, refresh bool) (*Feed, error) {
	if s.clients.Bilibili == nil {
		return nil, &apperr.Error{Code: apperr.CodeInternal, Message: "bilibili provider is not configured"}
	}
	mid = strings.TrimSpace(mid)
	if mid == "" {
		return nil, &apperr.Error{Code: apperr.CodeBadRequest, Message: "missing bilibili user"}
	}
	if limit <= 0 {
		limit = DefaultBilibiliLimit
	}
	if limit > MaxBilibiliLimit {
		limit = MaxBilibiliLimit
	}

	key := cachekey.Bilibili.Key("user", mid)
	entry, cached, degraded, err := fetchCachedList(ctx, s, key, bilibiliTTL, refresh, limit,
		func(ctx context.Context, n int) ([]bilibili.Video, error) {
			tl, terr := s.clients.Bilibili.Timeline(ctx, mid, n)
			if terr != nil {
				return nil, terr
			}
			return tl.Videos, nil
		})
	if err != nil {
		return nil, bilibiliError(err, mid)
	}
	return bilibiliFeed(mid, entry, limit, cached, degraded), nil
}

func bilibiliFeed(mid string, c cachedList[bilibili.Video], limit int, cached, degraded bool) *Feed {
	author := ""
	for _, v := range c.Items {
		if v.Author != "" {
			author = v.Author
			break
		}
	}
	if author == "" {
		author = mid
	}
	n := len(c.Items)
	if limit < n {
		n = limit
	}
	items := make([]Item, 0, n)
	for _, v := range c.Items[:n] {
		it := Item{
			ID:        v.BVID,
			CreatedAt: v.CreatedAt,
			Text:      v.Desc,
			HTML:      bilibiliHTML(v),
			URL:       "https://www.bilibili.com/video/" + v.BVID,
			Title:     v.Title,
			Author:    v.Author,
			Views:     v.Play,
		}
		if cover := safeURLString(v.Cover); cover != "" {
			it.Images = []string{cover}
		}
		items = append(items, it)
	}
	return &Feed{
		Language:    langZhCN,
		UID:         mid,
		ScreenName:  author,
		FetchedAt:   c.FetchedAt,
		TitleText:   author + " on Bilibili",
		LinkText:    "https://space.bilibili.com/" + mid,
		Cached:      cached,
		Degraded:    degraded,
		Items:       items,
		Description: "Bilibili videos from " + author,
	}
}

// bilibiliHTML renders one upload: cover, description, then the counters.
func bilibiliHTML(v bilibili.Video) string {
	var b strings.Builder
	if cover := safeURLString(v.Cover); cover != "" {
		b.WriteString(`<p><img src="`)
		b.WriteString(html.EscapeString(cover))
		b.WriteString(`" alt="" loading="lazy" style="max-width:100%"></p>`)
	}
	if v.Desc != "" {
		b.WriteString("<p>")
		b.WriteString(html.EscapeString(v.Desc))
		b.WriteString("</p>")
	}
	meta := make([]string, 0, 6)
	if v.Duration > 0 {
		meta = append(meta, "Duration "+bilibiliDuration(v.Duration))
	}
	if v.Play > 0 {
		meta = append(meta, "Play "+compactCount(v.Play))
	}
	if v.Danmaku > 0 {
		meta = append(meta, "Danmaku "+compactCount(v.Danmaku))
	}
	if v.Comment > 0 {
		meta = append(meta, "Comment "+compactCount(v.Comment))
	}
	if v.TypeName != "" {
		meta = append(meta, v.TypeName)
	}
	if len(meta) > 0 {
		b.WriteString("<p>")
		b.WriteString(html.EscapeString(strings.Join(meta, " · ")))
		b.WriteString("</p>")
	}
	return b.String()
}

func bilibiliDuration(d time.Duration) string {
	total := int(d.Seconds())
	h, m, s := total/3600, (total%3600)/60, total%60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

func bilibiliError(err error, mid string) error {
	if err == nil {
		return nil
	}
	var we *apperr.Error
	if errors.As(err, &we) {
		return err
	}
	switch {
	case errors.Is(err, bilibili.ErrInvalidUser):
		return &apperr.Error{Code: apperr.CodeBadRequest, Message: "invalid bilibili user " + mid, Err: err}
	case errors.Is(err, bilibili.ErrNotFound):
		return &apperr.Error{Code: apperr.CodeUserNotFound, Message: "bilibili user " + mid + " not found", Err: err}
	case errors.Is(err, context.DeadlineExceeded):
		return &apperr.Error{Code: apperr.CodeTimeout, Message: "bilibili timeline timed out", Err: err}
	case errors.Is(err, bilibili.ErrRateLimited):
		return &apperr.Error{Code: apperr.CodeUpstreamBlocked, Message: "bilibili rate limited the request", Err: err}
	default:
		return &apperr.Error{Code: apperr.CodeInternal, Message: err.Error(), Err: err}
	}
}
