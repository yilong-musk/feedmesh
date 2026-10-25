package feed

import (
	"context"
	"strings"
	"time"

	"github.com/yilong-musk/feedmesh/internal/smzdm"
	"github.com/yilong-musk/feedmesh/internal/weibo"
)

// SMZDM feed bounds and cache lifetime. The upstream slice rotates every few
// minutes, so a short TTL keeps the feed moving without hammering the CDN.
const (
	DefaultSmzdmLimit = 20
	MaxSmzdmLimit     = smzdm.MaxLimit
	smzdmTTL          = 5 * time.Minute
)

// GetSmzdm builds the SMZDM deals feed from the official RSS channel.
func (s *Service) GetSmzdm(ctx context.Context, limit int, refresh bool) (*Feed, error) {
	if s.clients.Smzdm == nil {
		return nil, &weibo.Error{Code: weibo.CodeInternal, Message: "smzdm provider is not configured"}
	}
	if limit <= 0 {
		limit = DefaultSmzdmLimit
	}
	if limit > MaxSmzdmLimit {
		limit = MaxSmzdmLimit
	}

	deals, hit, err := fetchCachedList(ctx, s, "smzdm:deals", smzdmTTL, refresh, limit,
		func(n int) ([]smzdm.Deal, error) { return s.clients.Smzdm.List(ctx, n) })
	if err != nil {
		return nil, sspaiError(err)
	}
	if len(deals) > limit {
		deals = deals[:limit]
	}

	now := s.now().UTC()
	f := &Feed{
		TitleText:   "SMZDM Deals",
		LinkText:    "https://www.smzdm.com",
		Description: "Selected deals from SMZDM",
		FetchedAt:   now,
		Cached:      hit,
		TTL:         15,
		Items:       make([]Item, 0, len(deals)),
	}
	for _, d := range deals {
		created := d.Published
		if created.IsZero() {
			created = now
		} else {
			// API timestamps are UTC; the feed stamps Beijing time.
			created = created.UTC()
		}
		images := []string{}
		if img := safeURLString(d.Image); img != "" {
			images = append(images, img)
		}
		f.Items = append(f.Items, Item{
			ID:        d.ID,
			Title:     d.Title,
			Text:      d.Summary,
			HTML:      smzdmHTML(d),
			URL:       d.URL,
			CreatedAt: created,
			Images:    images,
		})
	}
	return f, nil
}

// smzdmHTML renders one deal: cover, blurb (when the feed carries one), the
// poster nickname, and a link back to the deal page.
func smzdmHTML(d smzdm.Deal) string {
	var b strings.Builder
	if img := safeURLString(d.Image); img != "" {
		writeImage(&b, img, "", "")
	}
	if sum := strings.TrimSpace(d.Summary); sum != "" {
		b.WriteString("<p>")
		b.WriteString(EscapeXML(sum))
		b.WriteString("</p>")
	}
	if who := strings.TrimSpace(d.Author); who != "" {
		b.WriteString("<p><em>Posted by ")
		b.WriteString(EscapeXML(who))
		b.WriteString("</em></p>")
	}
	b.WriteString(`<p><a href="`)
	b.WriteString(EscapeXML(d.URL))
	b.WriteString(`">View deal on SMZDM</a></p>`)
	return b.String()
}
