// Package polymarket reads Polymarket's public market and activity APIs.
//
// Two anonymous, documented HTTP APIs are involved:
//
//	https://gamma-api.polymarket.com   events, markets, odds
//	https://data-api.polymarket.com/v2 wallet activity
//
// Neither needs an account, API key, cookie or browser. The Gamma API serves
// hundreds of kilobytes per event page, so this package trims what it keeps:
// list calls keep only the few markets a feed summary needs, and the event
// call keeps the top markets by volume.
package polymarket

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/yilong-musk/feedmesh/internal/upstream"
)

const (
	// DefaultBaseURL is the Gamma API host.
	DefaultBaseURL = "https://gamma-api.polymarket.com"
	// DefaultDataURL is the Data API v2 host.
	DefaultDataURL = "https://data-api.polymarket.com/v2"

	// DefaultLimit is the number of items returned when the caller is silent.
	DefaultLimit = 20
	// MaxLimit is the widest event list or event detail answer.
	MaxLimit = 50
	// DefaultActivityLimit is the number of wallet actions returned by default.
	DefaultActivityLimit = 30
	// MaxActivityLimit caps one wallet activity page.
	MaxActivityLimit = 100
)

// hiddenFromNewTag is Polymarket's own "Hide From New" tag. Without it the
// newest-events feed is 80% five-minute crypto Up or Down markets.
const hiddenFromNewTag = "102169"

// userAgent is a desktop browser UA; both APIs answer to plain clients, but a
// browser shape keeps the edge behaviour consistent.
const userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"

// Errors callers map onto HTTP responses.
var (
	// ErrInvalidTag means the tag slug is malformed.
	ErrInvalidTag = errors.New("invalid polymarket tag")
	// ErrInvalidSlug means the event slug is malformed.
	ErrInvalidSlug = errors.New("invalid polymarket event slug")
	// ErrInvalidAddress means the wallet address is malformed.
	ErrInvalidAddress = errors.New("invalid polymarket wallet address")
	// ErrNotFound means the requested event does not exist.
	ErrNotFound = errors.New("polymarket event not found")
	// ErrBadRequest means the API rejected the request as invalid.
	ErrBadRequest = errors.New("polymarket request rejected")
	// ErrRateLimited means the API asked us to slow down.
	ErrRateLimited = errors.New("polymarket rate limited")
	// ErrBlocked means the request was refused by the edge.
	ErrBlocked = errors.New("polymarket request blocked")
	// ErrUpstream means the API answered with an unusable status.
	ErrUpstream = errors.New("polymarket upstream unavailable")
	// ErrBadPayload means the response was not the documented JSON shape.
	ErrBadPayload = errors.New("polymarket response could not be parsed")
)

// Config tunes a Client.
type Config struct {
	// BaseURL replaces the Gamma API host; tests use it.
	BaseURL string
	// DataURL replaces the Data API host; tests use it.
	DataURL   string
	Timeout   time.Duration
	UserAgent string
}

// Client talks to both Polymarket APIs. It is safe for concurrent use.
type Client struct {
	base string
	data string
	ua   string
	http *http.Client
	now  func() time.Time
}

// New builds a Client with production defaults.
func New(cfg Config) *Client {
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultBaseURL
	}
	if cfg.DataURL == "" {
		cfg.DataURL = DefaultDataURL
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 15 * time.Second
	}
	if cfg.UserAgent == "" {
		cfg.UserAgent = userAgent
	}
	return &Client{
		base: strings.TrimRight(cfg.BaseURL, "/"),
		data: strings.TrimRight(cfg.DataURL, "/"),
		ua:   cfg.UserAgent,
		http: &http.Client{Timeout: cfg.Timeout},
		now:  time.Now,
	}
}

// Tag is one event tag.
type Tag struct {
	Slug  string `json:"slug"`
	Label string `json:"label"`
}

// Market is one market inside an event.
type Market struct {
	ID                string
	Question          string
	GroupItemTitle    string
	Slug              string
	Outcomes          []string
	Prices            []float64
	Volume            float64
	Volume24h         float64
	Liquidity         float64
	OneDayPriceChange float64
	StartDate         time.Time
	EndDate           time.Time
	Image             string
}

// Event is one Polymarket event with its markets.
type Event struct {
	ID          string
	Slug        string
	Title       string
	Description string
	Image       string
	CreatedAt   time.Time
	StartDate   time.Time
	EndDate     time.Time
	Volume      float64
	Volume24h   float64
	Liquidity   float64
	Tags        []Tag
	Markets     []Market
	FetchedAt   time.Time
}

// Activity is one wallet action from Data API v2.
type Activity struct {
	Wallet          string
	Timestamp       time.Time
	Type            string
	Side            string
	Outcome         string
	OutcomeIndex    int
	Title           string
	Slug            string
	EventSlug       string
	Icon            string
	Size            float64
	USDCSize        float64
	Price           float64
	TransactionHash string
	TokenID         string
}

// flexFloat accepts a JSON number, a numeric string, or null.
type flexFloat float64

func (f *flexFloat) UnmarshalJSON(b []byte) error {
	s := strings.TrimSpace(string(b))
	if s == "null" || s == `""` || s == "" {
		*f = 0
		return nil
	}
	s = strings.Trim(s, `"`)
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return err
	}
	*f = flexFloat(v)
	return nil
}

func (f flexFloat) Float() float64 { return float64(f) }

// stringList accepts a JSON array of strings or the string-encoded array the
// Gamma API uses for outcomes and prices.
type stringList []string

func (l *stringList) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		if strings.TrimSpace(s) == "" {
			*l = nil
			return nil
		}
		var out []string
		if err := json.Unmarshal([]byte(s), &out); err != nil {
			return err
		}
		*l = out
		return nil
	}
	var out []string
	if err := json.Unmarshal(b, &out); err != nil {
		return err
	}
	*l = out
	return nil
}

type apiMarket struct {
	ID                string     `json:"id"`
	Question          string     `json:"question"`
	GroupItemTitle    string     `json:"groupItemTitle"`
	Slug              string     `json:"slug"`
	Outcomes          stringList `json:"outcomes"`
	OutcomePrices     stringList `json:"outcomePrices"`
	Volume            flexFloat  `json:"volume"`
	Volume24h         flexFloat  `json:"volume24hr"`
	Liquidity         flexFloat  `json:"liquidityNum"`
	OneDayPriceChange flexFloat  `json:"oneDayPriceChange"`
	StartDate         string     `json:"startDate"`
	EndDate           string     `json:"endDate"`
	Image             string     `json:"image"`
	Active            bool       `json:"active"`
	Closed            bool       `json:"closed"`
}

type apiEvent struct {
	ID          string      `json:"id"`
	Slug        string      `json:"slug"`
	Title       string      `json:"title"`
	Description string      `json:"description"`
	Image       string      `json:"image"`
	Icon        string      `json:"icon"`
	CreatedAt   string      `json:"createdAt"`
	Creation    string      `json:"creationDate"`
	StartDate   string      `json:"startDate"`
	EndDate     string      `json:"endDate"`
	Volume      flexFloat   `json:"volume"`
	Volume24h   flexFloat   `json:"volume24hr"`
	Liquidity   flexFloat   `json:"liquidity"`
	Tags        []Tag       `json:"tags"`
	Markets     []apiMarket `json:"markets"`
}

type apiActivity struct {
	ProxyWallet     string    `json:"proxy_wallet"`
	Timestamp       int64     `json:"timestamp"`
	Type            string    `json:"type"`
	Size            flexFloat `json:"size"`
	USDCSize        flexFloat `json:"usdc_size"`
	Price           flexFloat `json:"price"`
	TransactionHash string    `json:"transaction_hash"`
	TokenID         string    `json:"token_id"`
	Side            string    `json:"side"`
	OutcomeIndex    int       `json:"outcome_index"`
	Title           string    `json:"title"`
	Slug            string    `json:"slug"`
	EventSlug       string    `json:"event_slug"`
	Outcome         string    `json:"outcome"`
	Icon            string    `json:"icon"`
}

type activityDoc struct {
	Data []apiActivity `json:"data"`
}

// Order values accepted by ListEvents.
const (
	OrderNew = "createdAt"
	OrderHot = "volume24hr"
)

var (
	tagRE     = regexp.MustCompile(`^[a-z0-9-]{1,60}$`)
	slugRE    = regexp.MustCompile(`^[A-Za-z0-9._-]{1,200}$`)
	addressRE = regexp.MustCompile(`^0x[0-9a-fA-F]{40}$`)
)

// NormalizeTag lowercases and validates a tag slug.
func NormalizeTag(raw string) (string, error) {
	s := strings.ToLower(strings.TrimSpace(raw))
	if s == "" {
		return "", nil
	}
	if !tagRE.MatchString(s) {
		return "", ErrInvalidTag
	}
	return s, nil
}

// NormalizeSlug validates an event slug.
func NormalizeSlug(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if !slugRE.MatchString(s) {
		return "", ErrInvalidSlug
	}
	return s, nil
}

// NormalizeAddress validates and lowercases a wallet address.
func NormalizeAddress(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if !addressRE.MatchString(s) {
		return "", ErrInvalidAddress
	}
	return strings.ToLower(s), nil
}

// ListEvents returns events ordered by creation time (OrderNew) or 24-hour
// volume (OrderHot). tag="" means every tag. The newest-events list always
// excludes Polymarket's "Hide From New" tag so five-minute crypto markets do
// not flood the feed.
func (c *Client) ListEvents(ctx context.Context, order, tag string, limit int) ([]Event, error) {
	tag, err := NormalizeTag(tag)
	if err != nil {
		return nil, err
	}
	switch order {
	case OrderNew, OrderHot:
	default:
		return nil, fmt.Errorf("polymarket: unknown order %q", order)
	}
	if limit <= 0 {
		limit = DefaultLimit
	}
	if limit > MaxLimit {
		limit = MaxLimit
	}

	q := url.Values{}
	q.Set("active", "true")
	q.Set("closed", "false")
	q.Set("exclude_tag_id", hiddenFromNewTag)
	q.Set("order", order)
	q.Set("ascending", "false")
	q.Set("limit", strconv.Itoa(limit))
	if tag != "" {
		q.Set("tag_slug", tag)
	}
	body, err := c.get(ctx, c.base+"/events?"+q.Encode())
	if err != nil {
		return nil, err
	}
	var raw []apiEvent
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBadPayload, err)
	}
	out := make([]Event, 0, len(raw))
	now := c.now().UTC()
	for _, e := range raw {
		ev := slimEvent(e, 3)
		ev.FetchedAt = now
		out = append(out, ev)
	}
	return out, nil
}

// Event returns one event with its markets sorted by volume, capped at limit.
func (c *Client) Event(ctx context.Context, slug string, limit int) (*Event, error) {
	slug, err := NormalizeSlug(slug)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = DefaultLimit
	}
	if limit > MaxLimit {
		limit = MaxLimit
	}
	body, err := c.get(ctx, c.base+"/events/slug/"+url.PathEscape(slug))
	if err != nil {
		return nil, err
	}
	var raw apiEvent
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBadPayload, err)
	}
	if raw.Slug == "" && raw.Title == "" {
		return nil, ErrNotFound
	}
	e := slimEvent(raw, 0)
	e.FetchedAt = c.now().UTC()
	sort.SliceStable(e.Markets, func(i, j int) bool {
		return e.Markets[i].Volume > e.Markets[j].Volume
	})
	if len(e.Markets) > limit {
		e.Markets = e.Markets[:limit]
	}
	return &e, nil
}

// Activity returns the newest actions of one wallet.
func (c *Client) Activity(ctx context.Context, address string, limit int) ([]Activity, error) {
	address, err := NormalizeAddress(address)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = DefaultActivityLimit
	}
	if limit > MaxActivityLimit {
		limit = MaxActivityLimit
	}
	q := url.Values{}
	q.Set("user", address)
	q.Set("limit", strconv.Itoa(limit))
	q.Set("sort_by", "TIMESTAMP")
	q.Set("sort_direction", "DESC")
	body, err := c.get(ctx, c.data+"/activity?"+q.Encode())
	if err != nil {
		return nil, err
	}
	var doc activityDoc
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBadPayload, err)
	}
	out := make([]Activity, 0, len(doc.Data))
	for _, a := range doc.Data {
		out = append(out, Activity{
			Wallet:          strings.ToLower(a.ProxyWallet),
			Timestamp:       time.Unix(a.Timestamp, 0).UTC(),
			Type:            a.Type,
			Side:            a.Side,
			Outcome:         a.Outcome,
			OutcomeIndex:    a.OutcomeIndex,
			Title:           a.Title,
			Slug:            a.Slug,
			EventSlug:       a.EventSlug,
			Icon:            a.Icon,
			Size:            a.Size.Float(),
			USDCSize:        a.USDCSize.Float(),
			Price:           a.Price.Float(),
			TransactionHash: a.TransactionHash,
			TokenID:         a.TokenID,
		})
	}
	return out, nil
}

// get performs one GET and classifies the answer.
func (c *Client) get(ctx context.Context, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.ua)
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("polymarket request failed: %w", err)
	}
	defer resp.Body.Close()
	body, err := upstream.ReadBody(resp.Body, 16<<20)
	if err != nil {
		return nil, fmt.Errorf("polymarket read failed: %w", err)
	}
	if resp.StatusCode == http.StatusOK {
		return body, nil
	}
	switch resp.StatusCode {
	case http.StatusNotFound:
		return nil, ErrNotFound
	case http.StatusBadRequest:
		return nil, fmt.Errorf("%w: %s", ErrBadRequest, strings.TrimSpace(string(body)))
	case http.StatusTooManyRequests:
		return nil, ErrRateLimited
	case http.StatusForbidden, http.StatusUnauthorized:
		return nil, ErrBlocked
	default:
		return nil, fmt.Errorf("%w: HTTP %d", ErrUpstream, resp.StatusCode)
	}
}

// slimEvent keeps the fields a feed needs. keep=0 keeps every market; a
// positive keep retains that many markets by volume.
func slimEvent(e apiEvent, keep int) Event {
	out := Event{
		ID:          e.ID,
		Slug:        e.Slug,
		Title:       strings.TrimSpace(e.Title),
		Description: strings.TrimSpace(e.Description),
		Image:       firstNonEmpty(e.Image, e.Icon),
		CreatedAt:   parseTime(firstNonEmpty(e.CreatedAt, e.Creation)),
		StartDate:   parseTime(e.StartDate),
		EndDate:     parseTime(e.EndDate),
		Volume:      e.Volume.Float(),
		Volume24h:   e.Volume24h.Float(),
		Liquidity:   e.Liquidity.Float(),
		Tags:        e.Tags,
	}
	markets := make([]Market, 0, len(e.Markets))
	for _, m := range e.Markets {
		if m.Closed || !m.Active {
			continue
		}
		markets = append(markets, Market{
			ID:                m.ID,
			Question:          strings.TrimSpace(m.Question),
			GroupItemTitle:    strings.TrimSpace(m.GroupItemTitle),
			Slug:              m.Slug,
			Outcomes:          m.Outcomes,
			Prices:            parsePrices(m.OutcomePrices),
			Volume:            m.Volume.Float(),
			Volume24h:         m.Volume24h.Float(),
			Liquidity:         m.Liquidity.Float(),
			OneDayPriceChange: m.OneDayPriceChange.Float(),
			StartDate:         parseTime(m.StartDate),
			EndDate:           parseTime(m.EndDate),
			Image:             firstNonEmpty(m.Image, e.Image, e.Icon),
		})
	}
	if keep > 0 && len(markets) > keep {
		sort.SliceStable(markets, func(i, j int) bool { return markets[i].Volume > markets[j].Volume })
		markets = markets[:keep]
	}
	out.Markets = markets
	return out
}

func parsePrices(raw stringList) []float64 {
	out := make([]float64, 0, len(raw))
	for _, s := range raw {
		v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
		if err != nil {
			return nil
		}
		out = append(out, v)
	}
	return out
}

func parseTime(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t.UTC()
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC()
	}
	if t, err := time.Parse("2006-01-02 15:04:05.999999999Z07:00", s); err == nil {
		return t.UTC()
	}
	return time.Time{}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
