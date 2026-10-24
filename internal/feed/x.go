package feed

import (
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/yilong-musk/feedmesh/internal/weibo"
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
		return nil, &weibo.Error{Code: weibo.CodeInternal, Message: "x provider is not configured"}
	}
	handle, err := xnitter.NormalizeHandle(handle)
	if err != nil {
		return nil, &weibo.Error{Code: weibo.CodeBadRequest, Message: err.Error()}
	}
	if limit <= 0 {
		limit = DefaultXLimit
	}
	if limit > MaxXLimit {
		limit = MaxXLimit
	}

	// One Nitter page is 20 tweets; cache the page and slice on the way out.
	items, hit, err := fetchCachedList(ctx, s, "x:"+strings.ToLower(handle), xTTL, refresh, limit,
		func(int) ([]xnitter.Tweet, error) { return s.clients.X.Timeline(ctx, handle) })
	if err != nil {
		return nil, sspaiError(err)
	}
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
		f.Items = append(f.Items, Item{
			ID:        t.ID,
			Title:     prefix + title,
			Text:      t.Text,
			HTML:      NormalizeHTMLBase(t.HTML, "https://x.com"),
			URL:       safeURLString(t.URL),
			Images:    images,
			CreatedAt: created.UTC(),
		})
	}
	return f, nil
}

func truncateRunes(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= n {
		return string(r)
	}
	return string(r[:n]) + "…"
}
