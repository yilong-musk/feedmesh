// Package bilibili reads a Bilibili creator's public video timeline.
//
// Bilibili's space API requires a WBI signature: the mixin key is derived from
// two rotating image names published by /x/web-interface/nav, and every query
// is signed with md5(sorted query + mixin key). The algorithm is pure Go
// (md5 + a fixed 64-entry permutation table), so no browser and no third-party
// module is needed. The API also wants a visitor cookie (buvid3/buvid4) and a
// few browser-fingerprint parameters; with those in place the endpoint answers
// normally, and a 412/-352 answer is a rate limit that a short backoff clears.
package bilibili

import (
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/yilong-musk/feedmesh/internal/logx"
)

// DefaultBaseURL is the API host. Tests point BaseURL at an httptest server.
const DefaultBaseURL = "https://api.bilibili.com"

const (
	// DefaultLimit is the number of videos returned when the caller is silent.
	DefaultLimit = 20
	// MaxLimit is the widest single-page answer; the upstream accepts ps=50.
	MaxLimit = 50

	// wbiKeyTTL keeps the rotating WBI keys for an hour.
	wbiKeyTTL = time.Hour
	// userAgent is a desktop Chrome UA; the API rejects some library defaults.
	userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"
)

// Errors callers map onto HTTP responses.
var (
	// ErrInvalidUser means the mid is not a positive integer.
	ErrInvalidUser = errors.New("invalid bilibili user")
	// ErrNotFound means the account does not exist or is closed.
	ErrNotFound = errors.New("bilibili user not found")
	// ErrRateLimited means the upstream kept answering 412/-352 after backoff.
	ErrRateLimited = errors.New("bilibili rate limited")
)

var midRe = regexp.MustCompile(`^\d{1,20}$`)

// mixinKeyEncTab is the fixed permutation published in Bilibili's web client.
var mixinKeyEncTab = []int{
	46, 47, 18, 2, 53, 8, 23, 32, 15, 50, 10, 31, 58, 3, 45, 35, 27, 43, 5, 49,
	33, 9, 42, 19, 29, 28, 14, 39, 12, 38, 41, 13, 37, 48, 7, 16, 24, 55, 40,
	61, 26, 17, 0, 1, 60, 51, 30, 4, 22, 25, 54, 21, 56, 59, 6, 63, 57, 62, 11,
	36, 20, 34, 44, 52,
}

// Config configures a Client.
type Config struct {
	// BaseURL overrides the API host for tests.
	BaseURL string
	// Timeout bounds one whole Timeline call.
	Timeout time.Duration
	// Log reports lifecycle events; nil is silent.
	Log *logx.Logger
}

// Client reads public Bilibili timelines. It is safe for concurrent use; calls
// serialize because the visitor cookie and WBI keys are shared.
type Client struct {
	cfg  Config
	http *http.Client

	mu       sync.Mutex
	warm     bool
	buvid3   string
	buvid4   string
	keys     wbiKeys
	keysTime time.Time
	// backoff is the retry schedule for rate-limited calls; tests shorten it.
	backoff []time.Duration
}

type wbiKeys struct {
	img string
	sub string
}

// New builds a Client.
func New(cfg Config) *Client {
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultBaseURL
	}
	return &Client{
		cfg:     cfg,
		http:    &http.Client{Timeout: cfg.Timeout},
		backoff: []time.Duration{0, time.Second, time.Second, 2 * time.Second, 2 * time.Second, 3 * time.Second},
	}
}

// Budget reports the per-call deadline this client expects.
func (c *Client) Budget() time.Duration { return c.cfg.Timeout }

// Close releases idle connections.
func (c *Client) Close() { c.http.CloseIdleConnections() }

func (c *Client) logf(msg string, kv ...any) {
	if c.cfg.Log != nil {
		c.cfg.Log.Info(msg, kv...)
	}
}

// Video is one upload.
type Video struct {
	BVID      string
	Title     string
	Desc      string
	Author    string
	AuthorID  string
	CreatedAt time.Time
	Duration  time.Duration
	Cover     string
	Play      int64
	Danmaku   int64
	Comment   int64
	TypeName  string
}

// Timeline is one creator's page of videos, newest first.
type Timeline struct {
	Mid       string
	Author    string
	Avatar    string
	Signature string
	Fans      int64
	Total     int
	FetchedAt time.Time
	Videos    []Video
}

// Timeline fetches up to limit recent videos for a numeric mid.
func (c *Client) Timeline(ctx context.Context, mid string, limit int) (*Timeline, error) {
	mid = strings.TrimSpace(mid)
	if !midRe.MatchString(mid) {
		return nil, ErrInvalidUser
	}
	if limit <= 0 {
		limit = DefaultLimit
	}
	if limit > MaxLimit {
		limit = MaxLimit
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	card, err := c.userCard(ctx, mid)
	if err != nil {
		return nil, err
	}
	keys, err := c.wbiKeys(ctx)
	if err != nil {
		return nil, err
	}
	videos, total, err := c.search(ctx, mid, keys, limit)
	if err != nil {
		return nil, err
	}
	return &Timeline{
		Mid:       mid,
		Author:    card.Name,
		Avatar:    card.Face,
		Signature: card.Sign,
		Fans:      card.Fans,
		Total:     total,
		FetchedAt: time.Now().UTC(),
		Videos:    videos,
	}, nil
}

type cardInfo struct {
	Name string
	Face string
	Sign string
	Fans int64
}

// userCard reads the public profile card; it does not need a WBI signature.
func (c *Client) userCard(ctx context.Context, mid string) (cardInfo, error) {
	var out cardInfo
	body, status, err := c.get(ctx, c.cfg.BaseURL+"/x/web-interface/card?mid="+url.QueryEscape(mid), "https://space.bilibili.com/"+mid)
	if err != nil {
		return out, err
	}
	if status == http.StatusPreconditionFailed {
		return out, fmt.Errorf("%w: card status %d", ErrRateLimited, status)
	}
	var resp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Card struct {
				Mid  string `json:"mid"`
				Name string `json:"name"`
				Face string `json:"face"`
				Sign string `json:"sign"`
				Fans int64  `json:"fans"`
			} `json:"card"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		return out, fmt.Errorf("bilibili card json: %w", err)
	}
	switch resp.Code {
	case 0:
	case -404:
		return out, ErrNotFound
	default:
		return out, fmt.Errorf("bilibili card status %d %s", resp.Code, resp.Message)
	}
	card := resp.Data.Card
	if card.Mid == "" && card.Name == "" {
		return out, ErrNotFound
	}
	out.Name = card.Name
	out.Face = card.Face
	out.Sign = card.Sign
	out.Fans = card.Fans
	return out, nil
}

// wbiKeys fetches and caches the rotating image/sub keys.
func (c *Client) wbiKeys(ctx context.Context) (wbiKeys, error) {
	if c.keys.img != "" && time.Since(c.keysTime) < wbiKeyTTL {
		return c.keys, nil
	}
	body, status, err := c.get(ctx, c.cfg.BaseURL+"/x/web-interface/nav", "https://www.bilibili.com/")
	if err != nil {
		return wbiKeys{}, err
	}
	if status == http.StatusPreconditionFailed {
		return wbiKeys{}, fmt.Errorf("%w: nav status %d", ErrRateLimited, status)
	}
	var resp struct {
		Code int `json:"code"`
		Data struct {
			WbiImg struct {
				ImgURL string `json:"img_url"`
				SubURL string `json:"sub_url"`
			} `json:"wbi_img"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		return wbiKeys{}, fmt.Errorf("bilibili nav json: %w", err)
	}
	img := keyFromURL(resp.Data.WbiImg.ImgURL)
	sub := keyFromURL(resp.Data.WbiImg.SubURL)
	if img == "" || sub == "" {
		return wbiKeys{}, fmt.Errorf("bilibili nav returned no wbi keys")
	}
	c.keys = wbiKeys{img: img, sub: sub}
	c.keysTime = time.Now()
	return c.keys, nil
}

func keyFromURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	raw = raw[strings.LastIndex(raw, "/")+1:]
	if i := strings.Index(raw, "."); i >= 0 {
		raw = raw[:i]
	}
	return raw
}

// search calls the signed space endpoint, retrying a rate limit a couple of
// times with a short backoff.
func (c *Client) search(ctx context.Context, mid string, keys wbiKeys, limit int) ([]Video, int, error) {
	params := url.Values{}
	params.Set("keyword", "")
	params.Set("mid", mid)
	params.Set("order", "pubdate")
	params.Set("order_avoided", "true")
	params.Set("platform", "web")
	params.Set("pn", "1")
	params.Set("ps", strconv.Itoa(limit))
	params.Set("tid", "0")
	params.Set("web_location", "333.1387")
	params.Set("special_type", "")
	params.Set("index", "0")
	for k, v := range dmParams() {
		params.Set(k, v)
	}

	var lastErr error
	for attempt, wait := range c.backoff {
		if wait > 0 {
			select {
			case <-ctx.Done():
				return nil, 0, ctx.Err()
			case <-time.After(wait):
			}
		}
		c.ensureVisitor(ctx)
		target := c.cfg.BaseURL + "/x/space/wbi/arc/search?" + signParams(params, mixinKey(keys))
		body, status, err := c.get(ctx, target, "https://space.bilibili.com/"+mid+"/video")
		if err != nil {
			lastErr = err
			continue
		}
		if status == http.StatusPreconditionFailed {
			lastErr = fmt.Errorf("%w: search status %d", ErrRateLimited, status)
			c.logf("bilibili search rate limited", "mid", mid, "attempt", attempt)
			continue
		}
		var resp struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
			Data    struct {
				Page struct {
					Count int `json:"count"`
				} `json:"page"`
				List struct {
					VList []rawVideo `json:"vlist"`
				} `json:"list"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(body), &resp); err != nil {
			return nil, 0, fmt.Errorf("bilibili search json: %w", err)
		}
		switch resp.Code {
		case 0:
		case -352, -401:
			lastErr = fmt.Errorf("%w: status %d", ErrRateLimited, resp.Code)
			c.logf("bilibili search rejected", "mid", mid, "code", resp.Code, "attempt", attempt)
			continue
		case -404:
			return nil, 0, ErrNotFound
		default:
			return nil, 0, fmt.Errorf("bilibili search status %d %s", resp.Code, resp.Message)
		}
		videos := make([]Video, 0, len(resp.Data.List.VList))
		for _, raw := range resp.Data.List.VList {
			videos = append(videos, raw.convert())
		}
		return videos, resp.Data.Page.Count, nil
	}
	if lastErr == nil {
		lastErr = ErrRateLimited
	}
	return nil, 0, lastErr
}

type rawVideo struct {
	BVID     string `json:"bvid"`
	Title    string `json:"title"`
	Desc     string `json:"description"`
	Author   string `json:"author"`
	Mid      int64  `json:"mid"`
	Created  int64  `json:"created"`
	Length   string `json:"length"`
	Pic      string `json:"pic"`
	Play     int64  `json:"play"`
	VideoRev int64  `json:"video_review"`
	Comment  int64  `json:"comment"`
	TypeName string `json:"typename"`
}

func (r rawVideo) convert() Video {
	v := Video{
		BVID:     r.BVID,
		Title:    strings.TrimSpace(r.Title),
		Desc:     strings.TrimSpace(r.Desc),
		Author:   r.Author,
		AuthorID: strconv.FormatInt(r.Mid, 10),
		Cover:    strings.Replace(r.Pic, "http://", "https://", 1),
		Play:     r.Play,
		Danmaku:  r.VideoRev,
		Comment:  r.Comment,
		TypeName: r.TypeName,
	}
	if r.Created > 0 {
		v.CreatedAt = time.Unix(r.Created, 0).UTC()
	}
	v.Duration = parseLength(r.Length)
	return v
}

// parseLength reads the upstream "MM:SS" / "HH:MM:SS" display length.
func parseLength(s string) time.Duration {
	parts := strings.Split(strings.TrimSpace(s), ":")
	if len(parts) < 2 {
		return 0
	}
	var total int
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return 0
		}
		total = total*60 + n
	}
	return time.Duration(total) * time.Second
}

// ensureVisitor obtains the anonymous visitor cookies once per process.
func (c *Client) ensureVisitor(ctx context.Context) {
	if c.warm {
		return
	}
	body, status, err := c.get(ctx, c.cfg.BaseURL+"/x/frontend/finger/spi", "https://www.bilibili.com/")
	if err != nil || status != http.StatusOK {
		return
	}
	var resp struct {
		Code int `json:"code"`
		Data struct {
			B3 string `json:"b_3"`
			B4 string `json:"b_4"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil || resp.Data.B3 == "" {
		return
	}
	c.buvid3, c.buvid4 = resp.Data.B3, resp.Data.B4
	c.warm = true
}

func (c *Client) get(ctx context.Context, target, referer string) (string, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Accept-Language", "en,zh-CN;q=0.9,zh;q=0.8")
	if referer != "" {
		req.Header.Set("Referer", referer)
		req.Header.Set("Origin", "https://space.bilibili.com")
	}
	if c.buvid3 != "" {
		cookie := "buvid3=" + c.buvid3
		if c.buvid4 != "" {
			cookie += "; buvid4=" + c.buvid4
		}
		req.Header.Set("Cookie", cookie)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return "", resp.StatusCode, err
	}
	return string(data), resp.StatusCode, nil
}

// mixinKey derives the 32-byte key from the two published image names.
func mixinKey(keys wbiKeys) string {
	src := keys.img + keys.sub
	out := make([]byte, 0, len(mixinKeyEncTab))
	for _, i := range mixinKeyEncTab {
		if i < len(src) {
			out = append(out, src[i])
		}
	}
	if len(out) > 32 {
		out = out[:32]
	}
	return string(out)
}

// signParams sorts the query, strips the characters Bilibili drops, appends
// wts, and returns the encoded query with a w_rid signature.
func signParams(params url.Values, mixin string) string {
	signed := url.Values{}
	for k, values := range params {
		for _, v := range values {
			signed.Add(k, filterSignatureChars(v))
		}
	}
	return signParamsAt(signed, mixin, time.Now().Unix())
}

// signParamsAt is signParams with an explicit timestamp so tests can pin it.
func signParamsAt(signed url.Values, mixin string, wts int64) string {
	signed.Set("wts", strconv.FormatInt(wts, 10))
	query := signed.Encode()
	sum := md5.Sum([]byte(query + mixin))
	signed.Set("w_rid", hex.EncodeToString(sum[:]))
	return signed.Encode()
}

// filterSignatureChars drops the punctuation Bilibili removes before signing.
func filterSignatureChars(s string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case '!', '\'', '(', ')', '*':
			return -1
		}
		return r
	}, s)
}

// dmParams mimics the browser fingerprint parameters the page sends.
func dmParams() map[string]string {
	return map[string]string{
		"dm_img_list":      "[]",
		"dm_img_str":       randomB64(16 + rand.Intn(48)),
		"dm_cover_img_str": randomB64(32 + rand.Intn(96)),
		"dm_img_inter":     `{"ds":[],"wh":[` + strconv.Itoa(2*1920+2*1080+3*50) + `,` + strconv.Itoa(4*1920-1080+50) + `,50],"of":[` + strconv.Itoa(3*10+2*10+200) + `,` + strconv.Itoa(400) + `,200]}`,
	}
}

func randomB64(n int) string {
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789+/"
	b := make([]byte, n)
	for i := range b {
		b[i] = alphabet[rand.Intn(len(alphabet))]
	}
	// Bilibili strips the base64 padding.
	return strings.TrimRight(base64.StdEncoding.EncodeToString(b), "=")
}
