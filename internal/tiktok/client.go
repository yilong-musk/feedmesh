// Package tiktok reads a TikTok creator's public timeline.
//
// TikTok serves profile pages behind a Slardar WAF challenge: the first
// response is a ~1.4 KB "Please wait..." document that carries a small
// proof-of-work puzzle. Solving it is pure SHA-256 arithmetic — find the
// integer whose digest equals a target — so the standard library can do it
// without a browser or a JavaScript engine. The resulting challenge cookie
// unlocks the normal profile HTML, whose __UNIVERSAL_DATA_FOR_REHYDRATION__
// payload identifies the account, and the public JSON endpoint
// /api/creator/item_list/ then returns the videos.
//
// The package deliberately holds no third-party modules: the challenge solver,
// cookie handling and JSON parsing all use the standard library.
package tiktok

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/yilong-musk/feedmesh/internal/logx"
)

// DefaultBaseURL is the public site. Tests point BaseURL at an httptest server.
const DefaultBaseURL = "https://www.tiktok.com"

const (
	// DefaultLimit is the number of videos returned when the caller is silent.
	DefaultLimit = 20
	// MaxLimit is the widest answer this package will assemble. The upstream
	// page size is fixed at 15, so the cap is three pages.
	MaxLimit = 45

	pageSize = 15
	maxPages = 3

	// userAgent is a desktop Chrome UA. TikTok's WAF rejects some library
	// default user agents outright.
	userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"

	// challengeSearchLimit bounds the proof-of-work search. The published
	// solver checks a million candidates; every observed puzzle resolves in
	// well under one percent of that.
	challengeSearchLimit = 1_000_000
)

// Errors callers map onto HTTP responses.
var (
	// ErrInvalidUser means the handle or secUid is malformed.
	ErrInvalidUser = errors.New("invalid tiktok user")
	// ErrNotFound means TikTok reports no such account.
	ErrNotFound = errors.New("tiktok user not found")
	// ErrPrivate means the account is private or only visible when logged in.
	ErrPrivate = errors.New("tiktok account is private")
	// ErrChallenge means the WAF puzzle could not be solved or the retry was
	// still blocked; it is an upstream problem, not a caller problem.
	ErrChallenge = errors.New("tiktok challenge not solved")
)

var (
	universalDataRe = regexp.MustCompile(`(?s)<script[^>]*\bid="__UNIVERSAL_DATA_FOR_REHYDRATION__"[^>]*>(.*?)</script>`)
	challengeDataRe = regexp.MustCompile(`(?s)<p[^>]*\bid="cs"[^>]*\bclass="([^"]+)"`)
	challengeNameRe = regexp.MustCompile(`(?s)<p[^>]*\bid="wci"[^>]*\bclass="([^"]+)"`)
	origReIDRe      = regexp.MustCompile(`(?s)<p[^>]*\bid="rci"[^>]*\bclass="([^"]+)"`)
	origReValRe     = regexp.MustCompile(`(?s)<p[^>]*\bid="rs"[^>]*\bclass="([^"]+)"`)
	secUIDRe        = regexp.MustCompile(`^MS4wLjABAAAA[\w-]+$`)
	handleRe        = regexp.MustCompile(`^[A-Za-z0-9._]{1,64}$`)
)

// Config configures a Client.
type Config struct {
	// BaseURL overrides the site root for tests.
	BaseURL string
	// Timeout bounds one whole Timeline call.
	Timeout time.Duration
	// Log reports lifecycle events; nil is silent.
	Log *logx.Logger
}

// Client reads public TikTok timelines. It is safe for concurrent use; calls
// serialize because the cookie jar is shared and the upstream tolerates one
// request at a time.
type Client struct {
	cfg  Config
	http *http.Client

	mu   sync.Mutex
	warm bool
}

// New builds a Client.
func New(cfg Config) *Client {
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultBaseURL
	}
	jar, _ := cookiejar.New(nil)
	return &Client{
		cfg: cfg,
		http: &http.Client{
			Jar:     jar,
			Timeout: cfg.Timeout,
		},
	}
}

// Budget reports the per-call deadline this client expects.
func (c *Client) Budget() time.Duration { return c.cfg.Timeout }

// Close releases resources. The client holds none beyond the idle HTTP
// transport, so this exists for symmetry with other providers.
func (c *Client) Close() { c.http.CloseIdleConnections() }

func (c *Client) logf(msg string, kv ...any) {
	if c.cfg.Log != nil {
		c.cfg.Log.Info(msg, kv...)
	}
}

// Item is one video in a creator timeline.
type Item struct {
	ID         string
	Desc       string
	Author     string
	Nickname   string
	SecUID     string
	CreatedAt  time.Time
	DurationMS int
	Width      int
	Height     int
	IsTop      bool
	IsImage    bool
	PlayURL    string
	CoverURL   string
	PlayCount  int64
	DiggCount  int64
	CommentCnt int64
	ShareCount int64
	MusicTitle string
	MusicBy    string
}

// Timeline is one creator's page of videos, newest first.
type Timeline struct {
	SecUID    string
	Author    string
	Nickname  string
	Avatar    string
	Signature string
	FetchedAt time.Time
	Items     []Item
}

// Timeline fetches up to limit recent videos for a handle (@ omitted) or a
// secUid. The handle form costs one profile page fetch; the secUid form skips
// it but loses the display name when the account has no items.
func (c *Client) Timeline(ctx context.Context, user string, limit int) (*Timeline, error) {
	user = strings.TrimSpace(user)
	user = strings.TrimPrefix(user, "@")
	if limit <= 0 {
		limit = DefaultLimit
	}
	if limit > MaxLimit {
		limit = MaxLimit
	}
	if secUIDRe.MatchString(user) {
		return c.timelineBySecUID(ctx, user, limit)
	}
	if !handleRe.MatchString(user) {
		return nil, ErrInvalidUser
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	profile, err := c.profile(ctx, user)
	if err != nil {
		return nil, err
	}
	items, err := c.list(ctx, profile.UserInfo.User.SecUID, limit)
	if err != nil {
		return nil, err
	}
	tl := &Timeline{
		SecUID:    profile.UserInfo.User.SecUID,
		Author:    profile.UserInfo.User.UniqueID,
		Nickname:  profile.UserInfo.User.Nickname,
		Avatar:    profile.UserInfo.User.AvatarLarger,
		Signature: profile.UserInfo.User.Signature,
		FetchedAt: time.Now().UTC(),
		Items:     items,
	}
	if tl.Author == "" {
		tl.Author = user
	}
	return tl, nil
}

// timelineBySecUID serves the secUid form, which has no profile page to read.
func (c *Client) timelineBySecUID(ctx context.Context, secUID string, limit int) (*Timeline, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	items, err := c.list(ctx, secUID, limit)
	if err != nil {
		return nil, err
	}
	tl := &Timeline{SecUID: secUID, FetchedAt: time.Now().UTC(), Items: items}
	if len(items) > 0 {
		tl.Author = items[0].Author
		tl.Nickname = items[0].Nickname
	}
	return tl, nil
}

type apiUser struct {
	ID           string `json:"id"`
	UniqueID     string `json:"uniqueId"`
	Nickname     string `json:"nickname"`
	SecUID       string `json:"secUid"`
	Signature    string `json:"signature"`
	AvatarLarger string `json:"avatarLarger"`
}

type userDetail struct {
	UserInfo struct {
		User     apiUser           `json:"user"`
		ItemList []json.RawMessage `json:"itemList"`
	} `json:"userInfo"`
	StatusCode int `json:"statusCode"`
}

// profile resolves a handle to its secUid and display fields.
func (c *Client) profile(ctx context.Context, handle string) (*userDetail, error) {
	body, err := c.fetch(ctx, c.cfg.BaseURL+"/@"+url.PathEscape(handle))
	if err != nil {
		return nil, err
	}
	m := universalDataRe.FindStringSubmatch(body)
	if m == nil {
		return nil, ErrChallenge
	}
	var data struct {
		Scope map[string]json.RawMessage `json:"__DEFAULT_SCOPE__"`
	}
	if err := json.Unmarshal([]byte(m[1]), &data); err != nil {
		return nil, fmt.Errorf("tiktok profile json: %w", err)
	}
	raw, ok := data.Scope["webapp.user-detail"]
	if !ok {
		return nil, ErrChallenge
	}
	var detail userDetail
	if err := json.Unmarshal(raw, &detail); err != nil {
		return nil, fmt.Errorf("tiktok user-detail json: %w", err)
	}
	switch detail.StatusCode {
	case 0:
	case 10221:
		return nil, ErrNotFound
	case 10222:
		return nil, ErrPrivate
	default:
		return nil, fmt.Errorf("tiktok user-detail status %d", detail.StatusCode)
	}
	if detail.UserInfo.User.SecUID == "" {
		return nil, ErrNotFound
	}
	return &detail, nil
}

type apiItem struct {
	ID         string `json:"id"`
	Desc       string `json:"desc"`
	CreateTime int64  `json:"createTime"`
	IsTop      bool   `json:"isTop"`
	Stats      struct {
		PlayCount    int64 `json:"playCount"`
		DiggCount    int64 `json:"diggCount"`
		CommentCount int64 `json:"commentCount"`
		ShareCount   int64 `json:"shareCount"`
	} `json:"stats"`
	Video struct {
		Duration    int    `json:"duration"`
		Width       int    `json:"width"`
		Height      int    `json:"height"`
		PlayAddr    string `json:"playAddr"`
		Cover       string `json:"cover"`
		OriginCover string `json:"originCover"`
		BitrateInfo []struct {
			Bitrate  int64 `json:"Bitrate"`
			PlayAddr struct {
				URLList []string `json:"UrlList"`
			} `json:"PlayAddr"`
		} `json:"bitrateInfo"`
	} `json:"video"`
	Author struct {
		ID           string `json:"id"`
		UniqueID     string `json:"uniqueId"`
		Nickname     string `json:"nickname"`
		SecUID       string `json:"secUid"`
		AvatarLarger string `json:"avatarLarger"`
	} `json:"author"`
	Music struct {
		Title      string `json:"title"`
		AuthorName string `json:"authorName"`
	} `json:"music"`
	ImagePost *struct {
		Images []struct {
			URLList []string `json:"urlList"`
		} `json:"images"`
	} `json:"imagePost"`
}

type listPage struct {
	StatusCode      int       `json:"statusCode"`
	StatusMsg       string    `json:"statusMsg"`
	ItemList        []apiItem `json:"itemList"`
	HasMorePrevious bool      `json:"hasMorePrevious"`
}

// list walks the creator endpoint until limit items, the end of the account,
// or the page cap.
func (c *Client) list(ctx context.Context, secUID string, limit int) ([]Item, error) {
	cursor := time.Now().UnixMilli()
	items := make([]Item, 0, limit)
	seen := make(map[string]struct{}, limit)
	for page := 0; page < maxPages && len(items) < limit; page++ {
		body, err := c.fetch(ctx, c.listURL(secUID, cursor))
		if err != nil {
			return nil, err
		}
		var resp listPage
		if err := json.Unmarshal([]byte(body), &resp); err != nil {
			head := body
			if len(head) > 200 {
				head = head[:200]
			}
			return nil, fmt.Errorf("tiktok item list json: %w (%q)", err, head)
		}
		if resp.StatusCode != 0 {
			return nil, fmt.Errorf("tiktok item list status %d %s", resp.StatusCode, resp.StatusMsg)
		}
		last := int64(0)
		for _, raw := range resp.ItemList {
			if raw.ID == "" {
				continue
			}
			if _, dup := seen[raw.ID]; dup {
				continue
			}
			seen[raw.ID] = struct{}{}
			items = append(items, convertItem(raw))
			if raw.CreateTime > 0 && raw.CreateTime < last || last == 0 {
				last = raw.CreateTime
			}
		}
		if len(items) >= limit || !resp.HasMorePrevious || len(resp.ItemList) == 0 {
			break
		}
		if last <= 0 {
			break
		}
		cursor = last * 1000
	}
	if len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

// listURL builds one creator-endpoint request. The parameter set mirrors the
// public web client; count is fixed at 15 because TikTok rejects other values
// with "invalid count parameter".
func (c *Client) listURL(secUID string, cursor int64) string {
	q := url.Values{}
	for k, v := range map[string]string{
		"aid": "1988", "app_language": "en", "app_name": "tiktok_web", "browser_language": "en-US",
		"browser_name": "Mozilla", "browser_online": "true", "browser_platform": "Win32",
		"browser_version": "5.0 (Windows)", "channel": "tiktok_web", "cookie_enabled": "true",
		"device_platform": "web_pc", "focus_state": "true", "from_page": "user", "history_len": "2",
		"is_fullscreen": "false", "is_page_visible": "true", "language": "en", "os": "windows",
		"priority_region": "", "referer": "", "region": "US", "screen_height": "1080",
		"screen_width": "1920", "type": "1", "tz_name": "UTC", "webcast_language": "en",
	} {
		q.Set(k, v)
	}
	q.Set("count", strconv.Itoa(pageSize))
	q.Set("cursor", strconv.FormatInt(cursor, 10))
	q.Set("device_id", strconv.FormatInt(7250000000000000000+rand.Int63n(75098999999999457), 10))
	q.Set("secUid", secUID)
	q.Set("verifyFp", "verify_"+randomHex(7))
	return c.cfg.BaseURL + "/api/creator/item_list/?" + q.Encode()
}

func convertItem(raw apiItem) Item {
	it := Item{
		ID:         raw.ID,
		Desc:       raw.Desc,
		Author:     raw.Author.UniqueID,
		Nickname:   raw.Author.Nickname,
		SecUID:     raw.Author.SecUID,
		DurationMS: raw.Video.Duration * 1000,
		Width:      raw.Video.Width,
		Height:     raw.Video.Height,
		IsTop:      raw.IsTop,
		PlayCount:  raw.Stats.PlayCount,
		DiggCount:  raw.Stats.DiggCount,
		CommentCnt: raw.Stats.CommentCount,
		ShareCount: raw.Stats.ShareCount,
		MusicTitle: raw.Music.Title,
		MusicBy:    raw.Music.AuthorName,
	}
	if raw.CreateTime > 0 {
		it.CreatedAt = time.Unix(raw.CreateTime, 0).UTC()
	}
	it.PlayURL = bestPlayURL(raw)
	it.CoverURL = raw.Video.OriginCover
	if it.CoverURL == "" {
		it.CoverURL = raw.Video.Cover
	}
	if raw.ImagePost != nil {
		it.IsImage = true
		if it.CoverURL == "" && len(raw.ImagePost.Images) > 0 && len(raw.ImagePost.Images[0].URLList) > 0 {
			it.CoverURL = raw.ImagePost.Images[0].URLList[0]
		}
	}
	return it
}

// bestPlayURL picks the highest-bitrate rendition when the payload lists them,
// falling back to the default play address.
func bestPlayURL(raw apiItem) string {
	best := ""
	var bestBitrate int64
	for _, b := range raw.Video.BitrateInfo {
		if len(b.PlayAddr.URLList) == 0 || b.Bitrate <= bestBitrate {
			continue
		}
		best, bestBitrate = b.PlayAddr.URLList[0], b.Bitrate
	}
	if best != "" {
		return best
	}
	return raw.Video.PlayAddr
}

// fetch GETs a page and transparently solves the WAF challenge once per
// session. The challenge cookie is short-lived, so a failed retry reports
// ErrChallenge instead of looping.
func (c *Client) fetch(ctx context.Context, target string) (string, error) {
	body, status, err := c.get(ctx, target)
	if err != nil {
		return "", err
	}
	if universalDataRe.MatchString(body) || strings.HasPrefix(strings.TrimSpace(body), "{") {
		c.warm = true
		return body, nil
	}
	if !challengeDataRe.MatchString(body) {
		return "", fmt.Errorf("tiktok unexpected response status=%d len=%d", status, len(body))
	}
	name, value, err := solveChallenge(body)
	if err != nil {
		return "", err
	}
	c.setChallengeCookie(name, value, body)
	body, status, err = c.get(ctx, target)
	if err != nil {
		return "", err
	}
	if !universalDataRe.MatchString(body) && !strings.HasPrefix(strings.TrimSpace(body), "{") {
		return "", fmt.Errorf("%w: status=%d len=%d", ErrChallenge, status, len(body))
	}
	c.warm = true
	return body, nil
}

func (c *Client) get(ctx context.Context, target string) (string, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Referer", c.cfg.BaseURL+"/")
	resp, err := c.http.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return "", resp.StatusCode, err
	}
	return string(body), resp.StatusCode, nil
}

func (c *Client) setChallengeCookie(name, value, page string) {
	base, err := url.Parse(c.cfg.BaseURL)
	if err != nil {
		return
	}
	cookies := []*http.Cookie{{Name: name, Value: value, Path: "/", Domain: base.Hostname()}}
	if w := origReIDRe.FindStringSubmatch(page); w != nil {
		if s := origReValRe.FindStringSubmatch(page); s != nil {
			cookies = append(cookies, &http.Cookie{Name: w[1], Value: s[1], Path: "/", Domain: base.Hostname()})
		}
	}
	c.http.Jar.SetCookies(base, cookies)
}

// challengeData is the puzzle payload carried in the cs element.
type challengeData struct {
	V struct {
		A string `json:"a"`
		C string `json:"c"`
	} `json:"v"`
}

// solveChallenge returns the WAF cookie name and value for one challenge page.
// The proof of work is sha256(seed || decimal(i)) == target; the answer rides
// back inside the original JSON document as the "d" field, base64-wrapped.
func solveChallenge(page string) (string, string, error) {
	m := challengeDataRe.FindStringSubmatch(page)
	if m == nil {
		return "", "", ErrChallenge
	}
	raw := decodeBase64(m[1])
	var data challengeData
	if err := json.Unmarshal(raw, &data); err != nil {
		return "", "", fmt.Errorf("tiktok challenge json: %w", err)
	}
	seed := decodeBase64(data.V.A)
	want := decodeBase64(data.V.C)
	if len(seed) == 0 || len(want) != sha256.Size {
		return "", "", ErrChallenge
	}
	answer := -1
	for i := 0; i < challengeSearchLimit; i++ {
		h := sha256.New()
		h.Write(seed)
		h.Write([]byte(strconv.Itoa(i)))
		if string(h.Sum(nil)) == string(want) {
			answer = i
			break
		}
	}
	if answer < 0 {
		return "", "", ErrChallenge
	}
	// Splice the answer into the original document so the field order, and
	// therefore the cookie value, stay identical to the browser's.
	text := strings.TrimSuffix(strings.TrimSpace(string(raw)), "}")
	text += `,"d":"` + base64.StdEncoding.EncodeToString([]byte(strconv.Itoa(answer))) + `"}`
	nameM := challengeNameRe.FindStringSubmatch(page)
	if nameM == nil || nameM[1] == "" {
		return "", "", ErrChallenge
	}
	return nameM[1], base64.StdEncoding.EncodeToString([]byte(text)), nil
}

// decodeBase64 accepts padded and unpadded standard base64.
func decodeBase64(s string) []byte {
	s = strings.TrimRight(strings.TrimSpace(s), "=")
	if b, err := base64.RawStdEncoding.DecodeString(s); err == nil {
		return b
	}
	b, _ := base64.StdEncoding.DecodeString(s)
	return b
}

func randomHex(n int) string {
	const digits = "0123456789abcdef"
	b := make([]byte, n)
	for i := range b {
		b[i] = digits[rand.Intn(len(digits))]
	}
	return string(b)
}
