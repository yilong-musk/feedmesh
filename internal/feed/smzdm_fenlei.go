package feed

import (
	"context"
	"errors"
	"fmt"
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
//   - a failed read spends the same budget: the failure is recorded and the
//     window's remaining requests answer with it instead of re-reading the
//     page (an outage must not turn a polling consumer into a page hammer);
//   - at most one category read at a time across all slugs;
//   - a challenge answer trips the slug's six-hour cooldown in the client.
const (
	// smzdmFenleiTTL is both the cache lifetime and the per-slug upstream
	// budget. A shorter TTL would allow two reads inside one hour.
	smzdmFenleiTTL = time.Hour

	// smzdmFenleiRetrySuffix marks the entry that records a failed read. It
	// is a separate key so the list entry keeps its own shape and the marker
	// never masquerades as a list.
	smzdmFenleiRetrySuffix = ":retry"
)

// SmzdmFenleiTTL is the cache lifetime of one category page.
func SmzdmFenleiTTL() time.Duration { return smzdmFenleiTTL }

// fenleiFailure records a read that failed, so the rest of the budget window
// answers with the same failure instead of paying for another page read.
// RetryAt is when a fresh attempt becomes allowed again.
type fenleiFailure struct {
	Slug    string    `json:"slug"`
	Message string    `json:"message"`
	RetryAt time.Time `json:"retry_at"`
}

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
	retryKey := key + smzdmFenleiRetrySuffix
	now := s.now().UTC()

	// A failed read spends the hour's budget exactly like a successful one.
	// Without this a polling consumer would re-read 600-720 KB on every poll
	// for as long as the upstream keeps failing.
	if f, ok := s.fenleiFailure(retryKey); ok && f.RetryAt.After(now) {
		s.log.Info("fenlei read suppressed: the hour's attempt already failed",
			"slug", slug, "retry_at", f.RetryAt.UTC().Format(time.RFC3339))
		return nil, fenleiFailureError(slug, f)
	}

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
	entry, hit, err := fetchCachedList(ctx, s, key, smzdmFenleiTTL, refresh, MaxSmzdmLimit,
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
		wrapped := smzdmFenleiError(err)
		s.recordFenleiFailure(retryKey, slug, err)
		return nil, wrapped
	}
	cards := entry.Items
	if len(cards) > limit {
		cards = cards[:limit]
	}

	readAt := s.fenleiReadTime(key, entry.FetchedAt, now)
	f := &Feed{
		TitleText:   "SMZDM Deals: " + slug,
		LinkText:    "https://www.smzdm.com/fenlei/" + slug + "/",
		Description: "Good-price cards from the SMZDM category " + slug,
		FetchedAt:   readAt,
		Cached:      hit,
		TTL:         int(smzdmFenleiTTL.Minutes()),
		Items:       make([]Item, 0, len(cards)),
	}
	for _, c := range cards {
		link := safeURLString(c.URL)
		if link == "" {
			continue
		}
		created, source := c.Published, "card"
		if created.IsZero() {
			// The page publishes no timestamp for a good-price card, so the
			// time the page was read is the only honest one. It is the same
			// instant for every card of one read, which date_source says.
			created, source = readAt, "fetched"
		} else {
			created = created.UTC()
		}
		images := []string{}
		if img := safeURLString(c.Image); img != "" {
			images = append(images, img)
		}
		price := c.Price
		f.Items = append(f.Items, Item{
			ID:         c.ID,
			Title:      c.Title,
			Text:       c.PriceText,
			HTML:       smzdmFenleiHTML(c, link),
			URL:        link,
			CreatedAt:  created,
			Images:     images,
			Price:      price,
			Slug:       slug,
			DateSource: source,
		})
	}
	return f, nil
}

// fenleiReadTime is when the page behind a list was read. A cache hit must
// report that instant rather than the request time: the cards carry no
// timestamp of their own, so created_at comes from here and every poll would
// otherwise re-stamp the same batch as new. An entry written before the read
// time was stored falls back to its write time, which is the same instant.
func (s *Service) fenleiReadTime(key string, fetchedAt, now time.Time) time.Time {
	if !fetchedAt.IsZero() {
		return fetchedAt.UTC()
	}
	if storedAt, ok := s.storedAt(key); ok {
		return storedAt.UTC()
	}
	return now
}

// recordFenleiFailure spends the rest of the budget window on a failed read.
// A challenge is left to the client's own six-hour breaker: that wait is
// already longer than one hour, and a second marker would only add noise.
func (s *Service) recordFenleiFailure(key, slug string, cause error) {
	if s.cache == nil || fenleiBreach(cause) {
		return
	}
	f := fenleiFailure{
		Slug:    slug,
		Message: cause.Error(),
		RetryAt: s.now().UTC().Add(smzdmFenleiTTL),
	}
	if err := s.cache.Set(key, f, smzdmFenleiTTL); err != nil {
		s.log.Warn("fenlei failure marker write failed", "key", key, "error", err.Error())
		return
	}
	s.log.Info("fenlei read failed; the hour's budget is spent",
		"slug", slug, "retry_at", f.RetryAt.Format(time.RFC3339))
}

// fenleiFailure reads the failure marker for key, expired or not: RetryAt is
// what decides the window, and it is compared against the service clock so the
// budget stays auditable in one place. The cache TTL only sweeps the file.
func (s *Service) fenleiFailure(key string) (fenleiFailure, bool) {
	if s.cache == nil {
		return fenleiFailure{}, false
	}
	var f fenleiFailure
	hit, err := s.cache.PeekStored(key, &f)
	if err != nil || !hit || f.RetryAt.IsZero() {
		return fenleiFailure{}, false
	}
	return f, true
}

// fenleiBreach reports the failures the client's breaker already covers.
func fenleiBreach(err error) bool {
	var blocked *smzdm.BlockedError
	var cooling *smzdm.CooldownError
	return errors.As(err, &blocked) || errors.As(err, &cooling)
}

// fenleiFailureError reports a suppressed read: the caller learns why it is
// not being retried and when a fresh attempt is allowed.
func fenleiFailureError(slug string, f fenleiFailure) error {
	return &weibo.Error{
		Code: weibo.CodeUpstreamUnavailable,
		Message: fmt.Sprintf("smzdm: category %q is not re-read until %s: the last read failed (%s)",
			slug, f.RetryAt.UTC().Format(time.RFC3339), f.Message),
	}
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
