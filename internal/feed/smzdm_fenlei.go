package feed

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/yilong-musk/feedmesh/internal/smzdm"
	"github.com/yilong-musk/feedmesh/internal/weibo"
)

// SMZDM category-page (fenlei) support.
//
// The RSS channels carry the curated slice only. A category page lists the good
// prices of one interest category, which is where deals outside that slice show
// up. The page is expensive (600-720 KB, measured 8-50s) and the site blocks
// automation that reads too often, so:
//
//   - one upstream read per slug per hour: the cache lifetime below is exactly
//     that budget, and a refresh inside the window is served from the cache;
//   - at most one category read at a time across all slugs;
//   - a challenge answer trips the slug's six-hour cooldown in the client.
const (
	// smzdmFenleiTTL is both the cache lifetime and the per-slug upstream
	// budget. A shorter TTL would allow two reads inside one hour.
	smzdmFenleiTTL = time.Hour
)

// SmzdmFenleiTTL is the cache lifetime of one category page.
func SmzdmFenleiTTL() time.Duration { return smzdmFenleiTTL }

// GetSmzdmFenlei builds one category page from https://www.smzdm.com/fenlei/<slug>/.
func (s *Service) GetSmzdmFenlei(ctx context.Context, slug string, limit int, refresh bool) (*Feed, error) {
	slug = strings.TrimSpace(strings.ToLower(slug))
	if err := smzdm.ValidateSlug(slug); err != nil {
		return nil, &weibo.Error{Code: weibo.CodeBadRequest, Message: err.Error(), Err: err}
	}
	if s.clients.Smzdm == nil {
		return nil, &weibo.Error{Code: weibo.CodeInternal, Message: "smzdm provider is not configured"}
	}
	if limit <= 0 {
		limit = DefaultSmzdmLimit
	}
	if limit > MaxSmzdmLimit {
		limit = MaxSmzdmLimit
	}

	key := "smzdm:fenlei:" + slug
	// refresh asks for new upstream data, but the hourly budget is a hard
	// constraint: inside the window the cached list is the newest list that
	// exists, and re-reading upstream would spend a request for nothing.
	if refresh {
		if storedAt, ok := s.storedAt(key); ok && s.now().Sub(storedAt) < smzdmFenleiTTL {
			s.log.Info("fenlei refresh served from cache: hourly upstream budget spent",
				"slug", slug, "stored_at", storedAt.UTC().Format(time.RFC3339))
			refresh = false
		}
	}

	// The cache is always filled at full width: one page holds every card the
	// slug offers, so a wider request must never pay for a second read.
	cards, hit, err := fetchCachedList(ctx, s, key, smzdmFenleiTTL, refresh, MaxSmzdmLimit,
		func(n int) ([]smzdm.Category, error) {
			s.fenleiMu.Lock()
			defer s.fenleiMu.Unlock()
			// Every upstream read is logged: the hourly budget is a hard
			// constraint, so an operator needs a count to audit it.
			start := s.now()
			cards, err := s.clients.Smzdm.Category(ctx, slug, n)
			s.log.Info("fenlei page read",
				"slug", slug, "cards", len(cards), "ms", s.now().Sub(start).Milliseconds(),
				"error", errorString(err))
			return cards, err
		})
	if err != nil {
		return nil, smzdmFenleiError(err)
	}
	if len(cards) > limit {
		cards = cards[:limit]
	}

	now := s.now().UTC()
	f := &Feed{
		TitleText:   "SMZDM Deals: " + slug,
		LinkText:    "https://www.smzdm.com/fenlei/" + slug + "/",
		Description: "Good-price cards from the SMZDM category " + slug,
		FetchedAt:   now,
		Cached:      hit,
		TTL:         int(smzdmFenleiTTL.Minutes()),
		Items:       make([]Item, 0, len(cards)),
	}
	for _, c := range cards {
		link := safeURLString(c.URL)
		if link == "" {
			continue
		}
		created := c.Published
		if created.IsZero() {
			// The page publishes no timestamp for a good-price card, so the
			// read time is the only honest one.
			created = now
		} else {
			created = created.UTC()
		}
		images := []string{}
		if img := safeURLString(c.Image); img != "" {
			images = append(images, img)
		}
		price := c.Price
		f.Items = append(f.Items, Item{
			ID:        c.ID,
			Title:     c.Title,
			Text:      c.PriceText,
			HTML:      smzdmFenleiHTML(c, link),
			URL:       link,
			CreatedAt: created,
			Images:    images,
			Price:     price,
			Slug:      slug,
		})
	}
	return f, nil
}

// smzdmFenleiHTML renders one category card: cover image, display price and a
// link back to the deal.
func smzdmFenleiHTML(c smzdm.Category, link string) string {
	var b strings.Builder
	if img := safeURLString(c.Image); img != "" {
		writeImage(&b, img, "", "")
	}
	if text := strings.TrimSpace(c.PriceText); text != "" {
		b.WriteString("<p>")
		b.WriteString(EscapeXML(text))
		b.WriteString("</p>")
	}
	b.WriteString(`<p><a href="`)
	b.WriteString(EscapeXML(link))
	b.WriteString(`">View deal on SMZDM</a></p>`)
	return b.String()
}

// smzdmFenleiError keeps the provider's error codes: a challenge is a blocked
// upstream, a cooling slug is an open breaker, everything else is unavailable.
func smzdmFenleiError(err error) error {
	if err == nil {
		return nil
	}
	var blocked *smzdm.BlockedError
	if errors.As(err, &blocked) {
		return &weibo.Error{Code: weibo.CodeUpstreamBlocked, Message: blocked.Error(), Err: err}
	}
	var cooling *smzdm.CooldownError
	if errors.As(err, &cooling) {
		return &weibo.Error{Code: weibo.CodeBreakerOpen, Message: cooling.Error(), Err: err}
	}
	var slug *smzdm.InvalidSlugError
	if errors.As(err, &slug) {
		return &weibo.Error{Code: weibo.CodeBadRequest, Message: slug.Error(), Err: err}
	}
	return sspaiError(err)
}

// storedAt reports when the entry for key was written.
func (s *Service) storedAt(key string) (time.Time, bool) {
	if s.cache == nil {
		return time.Time{}, false
	}
	return s.cache.StoredAt(key)
}
