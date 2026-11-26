package feed

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/yilong-musk/feedmesh/internal/polymarket"
	"github.com/yilong-musk/feedmesh/internal/weibo"
)

// Polymarket bounds and cache lifetimes. Event lists move with new markets and
// 24-hour volume, so ten minutes is enough; single-event odds and wallet
// activity are shorter lived.
const (
	// DefaultPolymarketLimit is the number of events or markets returned when the caller is silent.
	DefaultPolymarketLimit = polymarket.DefaultLimit
	// MaxPolymarketLimit is the widest event or market answer.
	MaxPolymarketLimit = polymarket.MaxLimit
	// DefaultPolymarketActivityLimit is the wallet action count when the caller is silent.
	DefaultPolymarketActivityLimit = polymarket.DefaultActivityLimit
	// MaxPolymarketActivityLimit caps one wallet activity page.
	MaxPolymarketActivityLimit = polymarket.MaxActivityLimit

	polymarketListTTL     = 10 * time.Minute
	polymarketEventTTL    = 5 * time.Minute
	polymarketActivityTTL = 5 * time.Minute
)

// PolymarketTTL is the cache lifetime of one Polymarket feed kind.
func PolymarketTTL(kind string) time.Duration {
	switch kind {
	case "event":
		return polymarketEventTTL
	case "activity":
		return polymarketActivityTTL
	default:
		return polymarketListTTL
	}
}

// GetPolymarketEvents builds the newest-events or trending-events feed.
func (s *Service) GetPolymarketEvents(ctx context.Context, order, tag string, limit int, refresh bool) (*Feed, error) {
	if s.clients.Polymarket == nil {
		return nil, &weibo.Error{Code: weibo.CodeInternal, Message: "polymarket provider is not configured"}
	}
	rawTag := tag
	tag, err := polymarket.NormalizeTag(tag)
	if err != nil {
		return nil, polymarketError(err, "tag", rawTag)
	}
	if limit <= 0 {
		limit = DefaultPolymarketLimit
	}
	if limit > MaxPolymarketLimit {
		limit = MaxPolymarketLimit
	}
	if order != polymarket.OrderNew && order != polymarket.OrderHot {
		return nil, &weibo.Error{Code: weibo.CodeBadRequest, Message: "unknown polymarket order " + order}
	}

	key := "polymarket:events:" + order + ":" + firstNonEmpty(tag, "all")
	entry, cached, degraded, err := fetchCachedList(ctx, s, key, polymarketListTTL, refresh, limit,
		func(ctx context.Context, n int) ([]polymarket.Event, error) {
			return s.clients.Polymarket.ListEvents(ctx, order, tag, n)
		})
	if err != nil {
		return nil, polymarketError(err, "events", tag)
	}
	return polymarketEventsFeed(entry, order, tag, limit, cached, degraded), nil
}

// GetPolymarketEvent builds one event's market list, newest odds first.
func (s *Service) GetPolymarketEvent(ctx context.Context, slug string, limit int, refresh bool) (*Feed, error) {
	if s.clients.Polymarket == nil {
		return nil, &weibo.Error{Code: weibo.CodeInternal, Message: "polymarket provider is not configured"}
	}
	rawSlug := slug
	slug, err := polymarket.NormalizeSlug(slug)
	if err != nil {
		return nil, polymarketError(err, "event", rawSlug)
	}
	if limit <= 0 {
		limit = DefaultPolymarketLimit
	}
	if limit > MaxPolymarketLimit {
		limit = MaxPolymarketLimit
	}

	key := "polymarket:event:" + slug
	// Always read the full slice: the cache stores it whole, then each caller
	// gets its own limit. Fetching with the caller's limit would let a narrow
	// first request truncate the cached event for everyone else.
	ev, cached, degraded, err := fetchCached(ctx, s, key, polymarketEventTTL, refresh,
		func(ctx context.Context) (*polymarket.Event, error) {
			return s.clients.Polymarket.Event(ctx, slug, MaxPolymarketLimit)
		})
	if err != nil {
		return nil, polymarketError(err, "event", slug)
	}
	if ev == nil {
		return nil, &weibo.Error{Code: weibo.CodeInternal, Message: "polymarket provider returned no event"}
	}
	return polymarketEventFeed(ev, limit, cached, degraded), nil
}

// GetPolymarketActivity builds a wallet activity feed.
func (s *Service) GetPolymarketActivity(ctx context.Context, address string, limit int, refresh bool) (*Feed, error) {
	if s.clients.Polymarket == nil {
		return nil, &weibo.Error{Code: weibo.CodeInternal, Message: "polymarket provider is not configured"}
	}
	rawAddress := address
	address, err := polymarket.NormalizeAddress(address)
	if err != nil {
		return nil, polymarketError(err, "address", rawAddress)
	}
	if limit <= 0 {
		limit = DefaultPolymarketActivityLimit
	}
	if limit > MaxPolymarketActivityLimit {
		limit = MaxPolymarketActivityLimit
	}

	key := "polymarket:activity:" + address
	entry, cached, degraded, err := fetchCachedList(ctx, s, key, polymarketActivityTTL, refresh, limit,
		func(ctx context.Context, n int) ([]polymarket.Activity, error) {
			return s.clients.Polymarket.Activity(ctx, address, n)
		})
	if err != nil {
		return nil, polymarketError(err, "activity", address)
	}
	return polymarketActivityFeed(address, entry, limit, cached, degraded), nil
}

func polymarketEventsFeed(c cachedList[polymarket.Event], order, tag string, limit int, cached, degraded bool) *Feed {
	n := len(c.Items)
	if limit < n {
		n = limit
	}
	title := "Polymarket New Markets"
	description := "The newest markets on Polymarket"
	link := "https://polymarket.com"
	if order == polymarket.OrderHot {
		title = "Polymarket Trending"
		description = "Polymarket events ranked by 24-hour volume"
	}
	if tag != "" {
		title += " · " + tag
		description += " (tag " + tag + ")"
		link += "/" + tag
	}
	items := make([]Item, 0, n)
	for _, ev := range c.Items[:n] {
		created := ev.CreatedAt
		if order == polymarket.OrderHot {
			created = firstTime(ev.StartDate, ev.CreatedAt, c.FetchedAt)
		} else {
			created = firstTime(ev.CreatedAt, ev.StartDate, c.FetchedAt)
		}
		items = append(items, polymarketEventItem(ev, created, 3))
	}
	return &Feed{
		UID:         "events:" + order + ":" + tag,
		TitleText:   title,
		LinkText:    link,
		Description: description,
		FetchedAt:   c.FetchedAt,
		Cached:      cached,
		Degraded:    degraded,
		TTL:         int(polymarketListTTL.Minutes()),
		Items:       items,
	}
}

func polymarketEventFeed(ev *polymarket.Event, limit int, cached, degraded bool) *Feed {
	link := polymarketEventURL(ev.Slug)
	n := len(ev.Markets)
	if limit < n {
		n = limit
	}
	items := make([]Item, 0, n)
	for _, m := range ev.Markets[:n] {
		items = append(items, polymarketMarketItem(*ev, m))
	}
	desc := strings.TrimSpace(ev.Description)
	if desc == "" {
		desc = "Polymarket event " + ev.Slug
	}
	return &Feed{
		UID:         ev.Slug,
		ScreenName:  ev.Title,
		TitleText:   ev.Title + " on Polymarket",
		LinkText:    link,
		Description: desc,
		FetchedAt:   ev.FetchedAt,
		Cached:      cached,
		Degraded:    degraded,
		TTL:         int(polymarketEventTTL.Minutes()),
		Items:       items,
	}
}

func polymarketActivityFeed(address string, c cachedList[polymarket.Activity], limit int, cached, degraded bool) *Feed {
	n := len(c.Items)
	if limit < n {
		n = limit
	}
	items := make([]Item, 0, n)
	for _, a := range c.Items[:n] {
		items = append(items, polymarketActivityItem(a))
	}
	short := address
	if len(short) > 10 {
		short = short[:6] + "…" + short[len(short)-4:]
	}
	return &Feed{
		UID:         address,
		ScreenName:  short,
		TitleText:   "Polymarket wallet " + short,
		LinkText:    "https://polymarket.com/profile/" + address,
		Description: "Recent Polymarket activity for " + address,
		FetchedAt:   c.FetchedAt,
		Cached:      cached,
		Degraded:    degraded,
		TTL:         int(polymarketActivityTTL.Minutes()),
		Items:       items,
	}
}

// polymarketEventItem renders one event row. keep is the number of markets
// summarised in the body.
func polymarketEventItem(ev polymarket.Event, created time.Time, keep int) Item {
	link := safeURLString(polymarketEventURL(ev.Slug))
	image := safeURLString(ev.Image)
	images := []string{}
	if image != "" {
		images = append(images, image)
	}
	markets := ev.Markets
	if keep > 0 && len(markets) > keep {
		markets = markets[:keep]
	}
	var b strings.Builder
	if image != "" {
		writeImage(&b, image, "", "")
	}
	if desc := strings.TrimSpace(ev.Description); desc != "" {
		b.WriteString("<p>")
		b.WriteString(EscapeXML(truncateRunes(desc, 400)))
		b.WriteString("</p>")
	}
	for _, m := range markets {
		b.WriteString("<p><strong>")
		b.WriteString(EscapeXML(polymarketMarketLabel(m)))
		b.WriteString("</strong>: ")
		b.WriteString(EscapeXML(polymarketOdds(m)))
		b.WriteString("</p>")
	}
	b.WriteString("<p>")
	b.WriteString(EscapeXML(polymarketMeta(ev)))
	b.WriteString("</p><p><a href=\"")
	b.WriteString(EscapeXML(link))
	b.WriteString("\">View event on Polymarket</a></p>")

	title := strings.TrimSpace(ev.Title)
	if title == "" {
		title = "(no content)"
	}
	return Item{
		ID:        firstNonEmpty(ev.ID, ev.Slug),
		Title:     title,
		Text:      polymarketMeta(ev),
		HTML:      b.String(),
		URL:       link,
		Images:    images,
		CreatedAt: created.UTC(),
		Volume24H: ev.Volume24h,
		Liquidity: ev.Liquidity,
		EndsAt:    ev.EndDate,
	}
}

// polymarketMarketItem renders one market inside a single-event feed.
func polymarketMarketItem(ev polymarket.Event, m polymarket.Market) Item {
	label := polymarketMarketLabel(m)
	odds := polymarketOdds(m)
	link := safeURLString(polymarketMarketURL(ev.Slug, m.Slug))
	image := safeURLString(firstNonEmpty(m.Image, ev.Image))
	images := []string{}
	if image != "" {
		images = append(images, image)
	}
	var b strings.Builder
	if image != "" {
		writeImage(&b, image, "", "")
	}
	b.WriteString("<p>")
	b.WriteString(EscapeXML(odds))
	b.WriteString("</p>")
	if change := m.OneDayPriceChange; change != 0 {
		b.WriteString("<p>24h change: ")
		b.WriteString(fmt.Sprintf("%+.1f pp", change*100))
		b.WriteString("</p>")
	}
	b.WriteString("<p>")
	b.WriteString(EscapeXML(polymarketMarketMeta(m)))
	b.WriteString("</p><p><a href=\"")
	b.WriteString(EscapeXML(link))
	b.WriteString("\">View market on Polymarket</a></p>")

	title := label
	if title == "" {
		title = "(no content)"
	}
	return Item{
		ID:        firstNonEmpty(m.ID, m.Slug),
		Title:     title,
		Text:      odds + " · " + polymarketMarketMeta(m),
		HTML:      b.String(),
		URL:       link,
		Images:    images,
		CreatedAt: firstTime(m.StartDate, ev.StartDate, ev.FetchedAt),
		Volume24H: m.Volume24h,
		Liquidity: m.Liquidity,
		EndsAt:    m.EndDate,
	}
}

func polymarketActivityItem(a polymarket.Activity) Item {
	label := strings.TrimSpace(a.Title)
	if label == "" {
		label = a.Type
	}
	if strings.EqualFold(a.Type, "TRADE") {
		label = strings.ToUpper(a.Side) + " " + firstNonEmpty(a.Outcome, a.Type) + " · " + label
	} else if a.Type != "" {
		label = a.Type + " · " + label
	}
	link := "https://polymarket.com"
	if a.EventSlug != "" {
		link = "https://polymarket.com/event/" + a.EventSlug
	} else if a.Slug != "" {
		link = "https://polymarket.com/event/" + a.Slug
	}
	link = safeURLString(link)
	image := safeURLString(a.Icon)
	images := []string{}
	if image != "" {
		images = append(images, image)
	}
	var b strings.Builder
	if image != "" {
		writeImage(&b, image, "", "")
	}
	b.WriteString("<p>")
	b.WriteString(EscapeXML(polymarketActivityMeta(a)))
	b.WriteString("</p><p><a href=\"")
	b.WriteString(EscapeXML(link))
	b.WriteString("\">View on Polymarket</a></p>")

	// A transaction can carry several fills, so the guid also includes the token,
	// side, price and size: two fills in the same second must not collide.
	id := fmt.Sprintf("%s:%s:%s:%s:%s:%.6f:%.6f",
		a.TransactionHash, a.TokenID, a.Type, a.Side,
		a.Timestamp.Format(time.RFC3339Nano), a.Price, a.Size)
	return Item{
		ID:        id,
		Title:     label,
		Text:      polymarketActivityMeta(a),
		HTML:      b.String(),
		URL:       link,
		Images:    images,
		CreatedAt: a.Timestamp,
	}
}

func polymarketEventURL(slug string) string {
	return "https://polymarket.com/event/" + slug
}

func polymarketMarketURL(eventSlug, marketSlug string) string {
	if marketSlug == "" || marketSlug == eventSlug {
		return polymarketEventURL(eventSlug)
	}
	return "https://polymarket.com/event/" + eventSlug + "/" + marketSlug
}

func polymarketMarketLabel(m polymarket.Market) string {
	if s := strings.TrimSpace(m.GroupItemTitle); s != "" {
		return s
	}
	return strings.TrimSpace(m.Question)
}

// polymarketOdds renders the outcome prices as percentages: "Yes 47.5% · No 52.5%".
func polymarketOdds(m polymarket.Market) string {
	if len(m.Outcomes) == 0 || len(m.Prices) != len(m.Outcomes) {
		return ""
	}
	parts := make([]string, 0, len(m.Outcomes))
	for i, name := range m.Outcomes {
		parts = append(parts, fmt.Sprintf("%s %.1f%%", name, m.Prices[i]*100))
	}
	return strings.Join(parts, " · ")
}

func polymarketMeta(ev polymarket.Event) string {
	parts := []string{}
	if ev.Volume24h > 0 {
		parts = append(parts, "24h volume "+formatUSD(ev.Volume24h))
	}
	if ev.Liquidity > 0 {
		parts = append(parts, "liquidity "+formatUSD(ev.Liquidity))
	}
	if !ev.EndDate.IsZero() {
		parts = append(parts, "ends "+ev.EndDate.Format("2006-01-02"))
	}
	return strings.Join(parts, " · ")
}

func polymarketMarketMeta(m polymarket.Market) string {
	parts := []string{}
	if m.Volume24h > 0 {
		parts = append(parts, "24h volume "+formatUSD(m.Volume24h))
	} else if m.Volume > 0 {
		parts = append(parts, "volume "+formatUSD(m.Volume))
	}
	if m.Liquidity > 0 {
		parts = append(parts, "liquidity "+formatUSD(m.Liquidity))
	}
	if !m.EndDate.IsZero() {
		parts = append(parts, "ends "+m.EndDate.Format("2006-01-02"))
	}
	return strings.Join(parts, " · ")
}

func polymarketActivityMeta(a polymarket.Activity) string {
	parts := []string{}
	if a.USDCSize > 0 {
		parts = append(parts, formatUSD(a.USDCSize))
	}
	if a.Size > 0 {
		parts = append(parts, fmt.Sprintf("%.4g shares", a.Size))
	}
	if a.Price > 0 {
		parts = append(parts, fmt.Sprintf("at %.3f", a.Price))
	}
	if a.Outcome != "" {
		parts = append(parts, "outcome "+a.Outcome)
	}
	parts = append(parts, a.Timestamp.Format(time.RFC3339))
	return strings.Join(parts, " · ")
}

// formatUSD prints 1.3M / 292K / 45.6, matching the rest of the service.
func formatUSD(v float64) string {
	switch {
	case v >= 1_000_000_000:
		return fmt.Sprintf("$%.1fB", v/1_000_000_000)
	case v >= 1_000_000:
		return fmt.Sprintf("$%.1fM", v/1_000_000)
	case v >= 1_000:
		return fmt.Sprintf("$%.0fK", v/1_000)
	default:
		return fmt.Sprintf("$%.0f", v)
	}
}

func firstTime(values ...time.Time) time.Time {
	for _, v := range values {
		if !v.IsZero() {
			return v.UTC()
		}
	}
	return time.Time{}
}

// polymarketError classifies a provider failure for the HTTP layer.
func polymarketError(err error, kind, ref string) error {
	if err == nil {
		return nil
	}
	var we *weibo.Error
	if errors.As(err, &we) {
		return err
	}
	switch {
	case errors.Is(err, polymarket.ErrInvalidTag),
		errors.Is(err, polymarket.ErrInvalidSlug),
		errors.Is(err, polymarket.ErrInvalidAddress),
		errors.Is(err, polymarket.ErrBadRequest):
		return &weibo.Error{Code: weibo.CodeBadRequest, Message: "invalid polymarket " + kind + " " + ref, Err: err}
	case errors.Is(err, polymarket.ErrNotFound):
		return &weibo.Error{Code: weibo.CodeUserNotFound, Message: "polymarket " + kind + " " + ref + " not found", Err: err}
	case errors.Is(err, polymarket.ErrRateLimited):
		return &weibo.Error{Code: weibo.CodeRateLimited, Message: "polymarket rate limited", Err: err}
	case errors.Is(err, polymarket.ErrBlocked):
		return &weibo.Error{Code: weibo.CodeUpstreamBlocked, Message: "polymarket blocked the request", Err: err}
	case errors.Is(err, polymarket.ErrBadPayload):
		return &weibo.Error{Code: weibo.CodeUpstreamUnavailable, Message: "polymarket response could not be parsed", Err: err}
	case errors.Is(err, context.DeadlineExceeded):
		return &weibo.Error{Code: weibo.CodeTimeout, Message: "polymarket request timed out", Err: err}
	case errors.Is(err, polymarket.ErrUpstream):
		return &weibo.Error{Code: weibo.CodeUpstreamUnavailable, Message: "polymarket upstream unavailable", Err: err}
	default:
		return &weibo.Error{Code: weibo.CodeNetwork, Message: err.Error(), Err: err}
	}
}
