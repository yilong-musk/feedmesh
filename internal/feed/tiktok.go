package feed

import (
	"context"
	"errors"
	"fmt"
	"html"
	"strings"
	"time"

	"github.com/yilong-musk/feedmesh/internal/apperr"
	"github.com/yilong-musk/feedmesh/internal/cachekey"
	"github.com/yilong-musk/feedmesh/internal/tiktok"
)

// TikTok feed bounds and cache lifetimes. The upstream page size is fixed at
// 15, so a ten-minute cache keeps polls cheap and the entry is stored for the
// whole last-good window: an upstream failure degrades to the previous list
// instead of an error.
const (
	// DefaultTikTokLimit is the number of videos returned when the caller is silent.
	DefaultTikTokLimit = tiktok.DefaultLimit
	// MaxTikTokLimit is the widest answer, three upstream pages.
	MaxTikTokLimit = tiktok.MaxLimit

	tiktokTTL = 10 * time.Minute
)

// TikTokTTL is the cache lifetime of one creator timeline.
func TikTokTTL() time.Duration { return tiktokTTL }

// GetTikTok builds a creator feed from the public timeline endpoint.
func (s *Service) GetTikTok(ctx context.Context, user string, limit int, refresh bool) (*Feed, error) {
	if s.clients.TikTok == nil {
		return nil, &apperr.Error{Code: apperr.CodeInternal, Message: "tiktok provider is not configured"}
	}
	user = strings.TrimSpace(strings.TrimPrefix(user, "@"))
	if user == "" {
		return nil, &apperr.Error{Code: apperr.CodeBadRequest, Message: "missing tiktok user"}
	}
	if limit <= 0 {
		limit = DefaultTikTokLimit
	}
	if limit > MaxTikTokLimit {
		limit = MaxTikTokLimit
	}

	// The item carries every field the feed needs, so the cache stores items
	// and can widen a narrow entry in place instead of letting one ?limit=1
	// request narrow every later read for the whole TTL.
	key := cachekey.TikTok.Key("user", strings.ToLower(user))
	entry, cached, degraded, err := fetchCachedList(ctx, s, key, tiktokTTL, refresh, limit,
		func(ctx context.Context, n int) ([]tiktok.Item, error) {
			tl, terr := s.clients.TikTok.Timeline(ctx, user, n)
			if terr != nil {
				return nil, terr
			}
			return tl.Items, nil
		})
	if err != nil {
		return nil, tiktokError(err, user)
	}
	return tiktokFeed(user, entry, limit, cached, degraded), nil
}

func tiktokFeed(user string, c cachedList[tiktok.Item], limit int, cached, degraded bool) *Feed {
	author, nickname := "", ""
	for _, v := range c.Items {
		if v.Author != "" {
			author = v.Author
		}
		if v.Nickname != "" {
			nickname = v.Nickname
		}
		if author != "" && nickname != "" {
			break
		}
	}
	if author == "" {
		author = user
	}
	display := nickname
	if display == "" {
		display = author
	}
	if display == "" {
		display = user
	}
	n := len(c.Items)
	if limit < n {
		n = limit
	}
	items := make([]Item, 0, n)
	for _, v := range c.Items[:n] {
		it := Item{
			ID:        v.ID,
			CreatedAt: v.CreatedAt,
			Text:      v.Desc,
			HTML:      tiktokHTML(v, display),
			URL:       "https://www.tiktok.com/@" + author + "/video/" + v.ID,
			Title:     tiktokTitle(v.Desc),
			Author:    author,
			Images:    []string{},
			Download: &DownloadMedia{
				PlayURL:    v.PlayURL,
				DurationMS: v.DurationMS,
				Width:      v.Width,
				Height:     v.Height,
				IsTop:      v.IsTop,
			},
		}
		if cover := safeURLString(v.CoverURL); cover != "" {
			it.Images = []string{cover}
		}
		items = append(items, it)
	}
	return &Feed{
		UID:         tiktokAuthor(c.Items, author),
		ScreenName:  display,
		FetchedAt:   c.FetchedAt,
		TitleText:   display + " on TikTok",
		LinkText:    "https://www.tiktok.com/@" + author,
		Cached:      cached,
		Degraded:    degraded,
		Items:       items,
		Description: "TikTok videos from " + display,
	}
}

// tiktokAuthor returns the account handle stored on the first item, which is
// the only identity a secUid-only request carries.
func tiktokAuthor(items []tiktok.Item, fallback string) string {
	for _, v := range items {
		if v.Author != "" {
			return v.Author
		}
	}
	return fallback
}

// tiktokHTML renders one video. Play addresses expire within hours, so the
// body carries the cover and the page link instead of a dead stream URL.
func tiktokHTML(v tiktok.Item, author string) string {
	var b strings.Builder
	if v.Desc != "" {
		b.WriteString("<p>")
		b.WriteString(html.EscapeString(v.Desc))
		b.WriteString("</p>")
	}
	if cover := safeURLString(v.CoverURL); cover != "" {
		b.WriteString(`<p><img src="`)
		b.WriteString(html.EscapeString(cover))
		b.WriteString(`" alt="" loading="lazy" style="max-width:100%"></p>`)
	}
	meta := make([]string, 0, 6)
	if v.IsImage {
		meta = append(meta, "Photo post")
	} else if v.DurationMS > 0 {
		meta = append(meta, "Duration "+tiktokDuration(v.DurationMS))
	}
	if v.Width > 0 && v.Height > 0 {
		meta = append(meta, fmt.Sprintf("%dx%d", v.Width, v.Height))
	}
	if v.PlayCount > 0 {
		meta = append(meta, "Play "+compactCount(v.PlayCount))
	}
	if v.DiggCount > 0 {
		meta = append(meta, "Like "+compactCount(v.DiggCount))
	}
	if v.CommentCnt > 0 {
		meta = append(meta, "Comment "+compactCount(v.CommentCnt))
	}
	if v.ShareCount > 0 {
		meta = append(meta, "Share "+compactCount(v.ShareCount))
	}
	if v.MusicTitle != "" {
		music := "Music " + v.MusicTitle
		if v.MusicBy != "" && v.MusicBy != v.MusicTitle {
			music += " — " + v.MusicBy
		}
		meta = append(meta, music)
	}
	if v.IsTop {
		meta = append(meta, "Pinned")
	}
	if len(meta) > 0 {
		b.WriteString("<p>")
		b.WriteString(html.EscapeString(strings.Join(meta, " · ")))
		b.WriteString("</p>")
	}
	return b.String()
}

// tiktokTitle keeps the whole caption: TikTok has no separate title field, and
// truncating here would be the only place where data gets shorter.
func tiktokTitle(desc string) string {
	desc = strings.TrimSpace(desc)
	if desc == "" {
		return "(no content)"
	}
	return desc
}

func tiktokDuration(ms int) string {
	if ms <= 0 {
		return "0:00"
	}
	total := ms / 1000
	h, m, s := total/3600, (total%3600)/60, total%60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

// compactCount renders 1.5M-style counters for the body text.
func compactCount(n int64) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.1fK", float64(n)/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
}

func tiktokError(err error, user string) error {
	if err == nil {
		return nil
	}
	var we *apperr.Error
	if errors.As(err, &we) {
		return err
	}
	switch {
	case errors.Is(err, tiktok.ErrInvalidUser):
		return &apperr.Error{Code: apperr.CodeBadRequest, Message: "invalid tiktok user " + user, Err: err}
	case errors.Is(err, tiktok.ErrNotFound):
		return &apperr.Error{Code: apperr.CodeUserNotFound, Message: "tiktok user " + user + " not found", Err: err}
	case errors.Is(err, tiktok.ErrPrivate):
		return &apperr.Error{Code: apperr.CodeUserNotFound, Message: "tiktok user " + user + " is private", Err: err}
	case errors.Is(err, context.DeadlineExceeded):
		return &apperr.Error{Code: apperr.CodeTimeout, Message: "tiktok timeline timed out", Err: err}
	case errors.Is(err, tiktok.ErrChallenge):
		return &apperr.Error{Code: apperr.CodeUpstreamUnavailable, Message: "tiktok challenge not solved", Err: err}
	default:
		return &apperr.Error{Code: apperr.CodeInternal, Message: err.Error(), Err: err}
	}
}
