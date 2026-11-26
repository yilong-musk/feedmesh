package feed

import (
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/yilong-musk/feedmesh/internal/apperr"
	"github.com/yilong-musk/feedmesh/internal/cachekey"
	"github.com/yilong-musk/feedmesh/internal/xnitter"
)

// X feed bounds and cache lifetime. Nitter asks readers to poll no faster
// than its own <ttl> of 40 minutes, and the instances are volunteer-run.
const (
	DefaultXLimit = 20
	MaxXLimit     = xnitter.MaxLimit
	xTTL          = 40 * time.Minute
)

var retweetPrefix = regexp.MustCompile(`^RT by @\S+:\s*`)

// GetX builds a timeline for one X account through the Nitter pool.
func (s *Service) GetX(ctx context.Context, handle string, limit int, refresh bool) (*Feed, error) {
	if s.clients.X == nil {
		return nil, &apperr.Error{Code: apperr.CodeInternal, Message: "x provider is not configured"}
	}
	handle, err := xnitter.NormalizeHandle(handle)
	if err != nil {
		return nil, &apperr.Error{Code: apperr.CodeBadRequest, Message: err.Error()}
	}
	if limit <= 0 {
		limit = DefaultXLimit
	}
	if limit > MaxXLimit {
		limit = MaxXLimit
	}

	// One Nitter page is 20 tweets; cache the page and slice on the way out.
	entry, hit, degraded, err := fetchCachedList(ctx, s, cachekey.X.Key(strings.ToLower(handle)), xTTL, refresh, limit,
		func(ctx context.Context, _ int) ([]xnitter.Tweet, error) { return s.clients.X.Timeline(ctx, handle) })
	if err != nil {
		return nil, sspaiError(err)
	}
	items := entry.Items
	if len(items) > limit {
		items = items[:limit]
	}

	now := s.now().UTC()
	f := &Feed{
		UID:         handle,
		ScreenName:  "@" + handle,
		Description: "@" + handle + " latest posts (via Nitter instances)",
		FetchedAt:   now,
		Cached:      hit,
		Degraded:    degraded,
		TitleText:   handle + " / X",
		LinkText:    "https://x.com/" + handle,
		TTL:         40,
		Items:       make([]Item, 0, len(items)),
	}
	for _, t := range items {
		created := t.Published
		if created.IsZero() {
			created = now
		}
		title := strings.TrimSpace(t.Title)
		prefix := ""
		if t.IsRetweet {
			// Nitter prefixes retweets with "RT by @user: ". Keep the signal
			// but drop the duplicated prefix, then mark it with one glyph.
			title = retweetPrefix.ReplaceAllString(title, "")
			prefix = "🔁 "
		}
		if title == "" {
			title = truncateRunes(t.Text, 60)
		}
		images := make([]string, 0, len(t.Images))
		for _, img := range t.Images {
			if u := resolveSafeURL(img, "https://x.com"); u != "" {
				images = append(images, u)
			}
		}
		item := Item{
			ID:        t.ID,
			Title:     prefix + title,
			Text:      t.Text,
			HTML:      NormalizeHTMLBase(t.HTML, "https://x.com"),
			URL:       safeURLString(t.URL),
			Images:    images,
			Author:    t.Author,
			CreatedAt: created.UTC(),
		}
		extra := &XExtra{
			AuthorID: t.AuthorID,
			Likes:    t.Likes,
			Replies:  t.Replies,
		}
		if t.CardImage != "" {
			extra.CardImage = resolveSafeURL(t.CardImage, "https://x.com")
		}
		if t.Video != nil {
			v := &Video{
				URL:        resolveSafeURL(t.Video.URL, "https://video.twimg.com"),
				Thumbnail:  resolveSafeURL(t.Video.Thumbnail, "https://x.com"),
				DurationMS: t.Video.DurationMS,
				Views:      t.Video.Views,
			}
			if v.URL != "" || v.Thumbnail != "" || v.DurationMS > 0 || v.Views > 0 {
				item.Video = v
			}
		}
		if t.Quoted != nil {
			q := &QuotedItem{
				ID:         t.Quoted.ID,
				URL:        safeURLString(t.Quoted.URL),
				Author:     t.Quoted.Author,
				AuthorName: t.Quoted.AuthorName,
				Text:       t.Quoted.Text,
			}
			for _, img := range t.Quoted.Images {
				if u := resolveSafeURL(img, "https://x.com"); u != "" {
					q.Images = append(q.Images, u)
				}
			}
			if t.Quoted.Video != nil {
				v := &Video{
					URL:        resolveSafeURL(t.Quoted.Video.URL, "https://video.twimg.com"),
					Thumbnail:  resolveSafeURL(t.Quoted.Video.Thumbnail, "https://x.com"),
					DurationMS: t.Quoted.Video.DurationMS,
					Views:      t.Quoted.Video.Views,
				}
				if v.URL != "" || v.Thumbnail != "" || v.DurationMS > 0 || v.Views > 0 {
					q.Video = v
				}
			}
			extra.Quoted = q
		}
		item.Extra = extra
		f.Items = append(f.Items, item)
	}
	return f, nil
}

// QuotedItem is a quoted post carried inside an X item.
type QuotedItem struct {
	ID         string   `json:"id,omitempty"`
	URL        string   `json:"url,omitempty"`
	Author     string   `json:"author,omitempty"`
	AuthorName string   `json:"author_name,omitempty"`
	Text       string   `json:"text,omitempty"`
	Images     []string `json:"images,omitempty"`
	Video      *Video   `json:"video,omitempty"`
}

// XExtra carries X-specific engagement, card and quoted-post metadata.
type XExtra struct {
	AuthorID  string      `json:"author_id,omitempty"`
	CardImage string      `json:"card_image,omitempty"`
	Likes     int         `json:"likes"`
	Replies   int         `json:"replies"`
	Quoted    *QuotedItem `json:"quoted,omitempty"`
}

func truncateRunes(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= n {
		return string(r)
	}
	return string(r[:n]) + "…"
}
