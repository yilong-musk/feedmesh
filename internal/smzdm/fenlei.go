package smzdm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/yilong-musk/feedmesh/internal/guard"
)

// Category pages (https://www.smzdm.com/fenlei/<slug>/) are the only public
// source of SMZDM good-price cards outside the curated RSS slices. The page is
// server-rendered: the card list lives in the Nuxt payload script
// (<script type="application/json" id="__NUXT_DATA__">), which is a flattened
// value array where containers hold indices into that array.
//
// Search (search.smzdm.com) answers automated requests with a WAF challenge and
// is deliberately not used. A challenge page or an empty shell is reported as
// BlockedError and puts the slug on a cooldown: the client never retries it
// within the cooldown window.

const (
	// CategoryDefaultLimit is the width used when the caller asks for nothing.
	CategoryDefaultLimit = DefaultLimit
	// CategoryMaxLimit caps a category request the same way the RSS channels cap.
	CategoryMaxLimit = MaxLimit

	// categoryBreakerCooldown is how long one slug is skipped after the site
	// answers with a challenge, so a blocked slug is not hammered.
	categoryBreakerCooldown = 6 * time.Hour
	// categoryMinBody is the size below which a page without cards reads as an
	// empty shell (a real category page measured 600-720 KB).
	categoryMinBody = 20 << 10
	// categoryBodyLimit bounds how much of a page is read into memory.
	categoryBodyLimit = 8 << 20
)

// categoryHost is the category-page host; Config.BaseURL replaces it in tests.
const categoryHost = "https://www.smzdm.com"

var (
	// slugPattern keeps a caller-supplied slug from reaching the URL unfiltered.
	slugPattern = regexp.MustCompile(`^[a-z0-9]{2,32}$`)
	// categoryDealURL matches a good-price permalink. Article cards point at
	// post.smzdm.com and are discarded.
	categoryDealURL = regexp.MustCompile(`^https?://(?:www\.)?smzdm\.com/p/([0-9A-Za-z]+)/?$`)
	// nuxtPayload finds the Nuxt data script.
	nuxtPayload = regexp.MustCompile(`(?s)<script[^>]*id="__NUXT_DATA__"[^>]*>(.*?)</script>`)
	// cardPrice reads the leading amount off a display price such as
	// "86.44 ..." or "1,299 ..."; the trailing prose is kept as display text.
	cardPrice = regexp.MustCompile(`^([0-9][0-9,]*(?:\.[0-9]+)?)`)

	// robotsDisallowedSlugs are the category paths listed under "Disallow" in
	// https://www.smzdm.com/robots.txt. The client refuses them.
	robotsDisallowedSlugs = map[string]bool{
		"3cjiadian":       true,
		"jujiashenghuo":   true,
		"shishangyundong": true,
	}
)

// dealTypes are the card types that carry a price. Article cards use the site's
// article types and are filtered out.
var dealTypes = map[string]bool{"faxian": true, "youhui": true}

// InvalidSlugError reports a slug that is malformed or disallowed by robots.txt.
type InvalidSlugError struct {
	Slug   string
	Reason string
}

func (e *InvalidSlugError) Error() string {
	if e.Reason != "" {
		return fmt.Sprintf("smzdm: category slug %q is not allowed: %s", e.Slug, e.Reason)
	}
	return fmt.Sprintf("smzdm: category slug %q is not allowed", e.Slug)
}

// BlockedError reports a WAF challenge or an empty shell instead of a page.
// Until is the moment the slug's cooldown ends.
type BlockedError struct {
	Slug   string
	Reason string
	Until  time.Time
}

func (e *BlockedError) Error() string {
	msg := fmt.Sprintf("smzdm: category %q is blocked upstream (%s)", e.Slug, e.Reason)
	if !e.Until.IsZero() {
		msg += "; cooling down until " + e.Until.UTC().Format(time.RFC3339)
	}
	return msg
}

// CooldownError reports a slug that is inside its cooldown window.
type CooldownError struct {
	Slug  string
	Until time.Time
}

func (e *CooldownError) Error() string {
	return fmt.Sprintf("smzdm: category %q is cooling down until %s",
		e.Slug, e.Until.UTC().Format(time.RFC3339))
}

// Category is one good-price card from a category page.
type Category struct {
	ID    string
	Title string
	URL   string
	// Price is the leading amount of the card's display price, or nil when the
	// card carries none.
	Price *float64
	// PriceText is the display price as the page shows it (coupons, multi-buy
	// notes included), which is more useful than the number alone.
	PriceText string
	Image     string
	// Published is the card's own timestamp. Category pages only carry one for
	// article cards, so a good-price card usually leaves it zero and the feed
	// layer falls back to the fetch time.
	Published time.Time
}

// ValidateSlug reports whether slug is a usable category path.
func ValidateSlug(slug string) error {
	s := strings.TrimSpace(slug)
	if !slugPattern.MatchString(s) {
		return &InvalidSlugError{Slug: slug, Reason: "expected two to 32 lowercase letters or digits"}
	}
	if robotsDisallowedSlugs[s] {
		return &InvalidSlugError{Slug: s, Reason: "robots.txt disallows this category"}
	}
	return nil
}

// CategoryURL is the category page address for one slug.
func (c *Client) CategoryURL(slug string) string {
	host := categoryHost
	if c.base != "" {
		host = c.base
	}
	return host + "/fenlei/" + slug + "/"
}

// breakerFor returns the cooldown breaker for one slug.
func (c *Client) breakerFor(slug string) *guard.Breaker {
	c.breakerMu.Lock()
	defer c.breakerMu.Unlock()
	if c.breakers == nil {
		c.breakers = make(map[string]*guard.Breaker)
	}
	b := c.breakers[slug]
	if b == nil {
		b = guard.NewBreaker(categoryBreakerCooldown)
		b.SetClock(c.now)
		c.breakers[slug] = b
	}
	return b
}

// Category reads one category page and returns its good-price cards, newest
// first. Article, review and repost cards are dropped.
//
// One call is one upstream request: there is no retry. A challenge page trips
// the slug's breaker for six hours and reports BlockedError; a slug inside that
// window reports CooldownError without touching the network.
func (c *Client) Category(ctx context.Context, slug string, limit int) ([]Category, error) {
	slug = strings.TrimSpace(strings.ToLower(slug))
	if err := ValidateSlug(slug); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = CategoryDefaultLimit
	}
	if limit > CategoryMaxLimit {
		limit = CategoryMaxLimit
	}

	breaker := c.breakerFor(slug)
	allowed, probe := breaker.AllowRequest()
	if !allowed {
		state := breaker.State()
		until := time.Time{}
		if state.Until != nil {
			until = *state.Until
		}
		return nil, &CooldownError{Slug: slug, Until: until}
	}

	body, status, err := c.getCategory(ctx, slug)
	if err != nil {
		if probe {
			breaker.AbortProbe()
		}
		return nil, err
	}

	var reason string
	switch {
	case status == http.StatusAccepted:
		reason = "HTTP 202 challenge"
	case bytes.Contains(body, []byte("TCaptcha")):
		reason = "captcha challenge in the page"
	}
	cards := parseCategoryCards(body, c.now())
	if reason == "" && len(cards) == 0 && len(body) < categoryMinBody {
		reason = fmt.Sprintf("empty shell (%d bytes, no cards)", len(body))
	}
	if reason != "" {
		blocked := &BlockedError{Slug: slug, Reason: reason}
		blocked.Until = breaker.Trip()
		return nil, blocked
	}
	if probe {
		breaker.Success()
	}
	if len(cards) == 0 {
		return nil, fmt.Errorf("smzdm: category %q returned no good-price cards", slug)
	}
	if len(cards) > limit {
		cards = cards[:limit]
	}
	return cards, nil
}

// getCategory performs the single upstream read. Non-200 statuses other than
// 202 are reported as errors; 202 is returned with its body so the caller can
// classify it with the rest of the challenge signals.
func (c *Client) getCategory(ctx context.Context, slug string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.CategoryURL(slug), nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("User-Agent", c.ua)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	resp, err := c.pages.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("smzdm: category %q request failed: %w", slug, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, categoryBodyLimit))
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("smzdm: category %q read failed: %w", slug, err)
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return nil, resp.StatusCode, fmt.Errorf("smzdm: category %q returned HTTP %d", slug, resp.StatusCode)
	}
	return body, resp.StatusCode, nil
}

// parseCategoryCards extracts the good-price cards from a category page.
//
// Only cards with a deal type, a /p/<id>/ permalink on www.smzdm.com and a
// readable price are kept, which is what drops the article and repost cards the
// same page mixes in.
func parseCategoryCards(body []byte, now time.Time) []Category {
	m := nuxtPayload.FindSubmatch(body)
	if m == nil {
		return nil
	}
	var nodes []any
	if err := json.Unmarshal(m[1], &nodes); err != nil {
		return nil
	}
	dec := &nodeReader{nodes: nodes}

	out := make([]Category, 0, 20)
	seen := make(map[string]bool, 20)
	for _, node := range nodes {
		card, ok := node.(map[string]any)
		if !ok {
			continue
		}
		articleType := strings.ToLower(dec.str(card["article_type"]))
		if !dealTypes[articleType] {
			continue
		}
		link := dec.str(card["article_url"])
		u := categoryDealURL.FindStringSubmatch(link)
		if u == nil || seen[u[1]] {
			continue
		}
		title := dec.str(card["title"])
		if title == "" {
			continue
		}
		priceText := dec.str(card["subtitle"])
		price, ok := parsePrice(priceText)
		if !ok {
			continue
		}
		seen[u[1]] = true
		out = append(out, Category{
			ID:        u[1],
			Title:     title,
			URL:       link,
			Price:     price,
			PriceText: priceText,
			Image:     normalizeImageURL(dec.str(card["focus_pic_url"])),
			Published: parseCardDate(dec.str(card["article_no_format_date"]), dec.str(card["article_format_date"]), now),
		})
	}
	// Cards inside one payload are not ordered, and a card may carry no
	// timestamp at all: sort the dated ones newest first and keep the rest in
	// page order behind them.
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Published.After(out[j].Published)
	})
	return out
}

// nodeReader resolves flattened Nuxt payload values. Container members hold
// indices into the flat array; scalars are stored at their own index.
type nodeReader struct {
	nodes []any
}

// value resolves one member of a container.
func (r *nodeReader) value(v any) any {
	n, ok := toIndex(v)
	if !ok || n < 0 || n >= len(r.nodes) {
		return nil
	}
	scalar := r.nodes[n]
	switch scalar.(type) {
	case map[string]any, []any:
		return nil // nested containers are not needed by the card reader
	}
	return scalar
}

func (r *nodeReader) str(v any) string {
	s, _ := r.value(v).(string)
	return strings.TrimSpace(s)
}

// toIndex reports whether v is a payload reference.
func toIndex(v any) (int, bool) {
	f, ok := v.(float64)
	if !ok {
		return 0, false
	}
	n := int(f)
	if float64(n) != f {
		return 0, false
	}
	return n, true
}

// parsePrice reads the leading amount of a display price.
func parsePrice(text string) (*float64, bool) {
	m := cardPrice.FindStringSubmatch(strings.TrimSpace(text))
	if m == nil {
		return nil, false
	}
	v, err := strconv.ParseFloat(strings.ReplaceAll(m[1], ",", ""), 64)
	if err != nil {
		return nil, false
	}
	return &v, true
}

// normalizeImageURL turns a protocol-relative image reference into https.
func normalizeImageURL(raw string) string {
	u := strings.TrimSpace(raw)
	if strings.HasPrefix(u, "//") {
		return "https:" + u
	}
	return u
}

// parseCardDate reads a card timestamp. The full field ("2006-01-02 15:04:05",
// Beijing time) wins; the short field ("01-02 15:04", or "06-01-02" with a year)
// falls back to the fetch time's year. A short date that would land more than a
// day in the future belongs to the previous year, which is the December to
// January case.
func parseCardDate(full, short string, now time.Time) time.Time {
	if s := strings.TrimSpace(full); s != "" {
		if t, err := time.ParseInLocation("2006-01-02 15:04:05", s, beijing); err == nil {
			return t
		}
	}
	s := strings.TrimSpace(short)
	if s == "" {
		return time.Time{}
	}
	if t, err := time.ParseInLocation("06-01-02", s, beijing); err == nil {
		return t
	}
	t, err := time.ParseInLocation("01-02 15:04", s, beijing)
	if err != nil {
		return time.Time{}
	}
	ref := now.In(beijing)
	t = time.Date(ref.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), 0, 0, beijing)
	if t.After(ref.Add(24 * time.Hour)) {
		t = time.Date(ref.Year()-1, t.Month(), t.Day(), t.Hour(), t.Minute(), 0, 0, beijing)
	}
	return t
}
