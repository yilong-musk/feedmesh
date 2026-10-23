// Package xnitter reads X (Twitter) timelines through public Nitter
// instances.
//
// Nitter is a community-run front end for X: it needs no API key and no
// account, and it serves a plain RSS feed per profile. Two things make this
// workable in practice:
//
//   - Instance health is not stable. Timelines are tried against a pool that
//     is refreshed from status.d420.de and falls back to a baked-in list.
//   - Some instances sit behind Anubis, a proof-of-work gate. The client
//     solves the SHA-256 challenge and keeps the cookie in a standard jar.
//
// The feed is a real chronological timeline, unlike X's anonymous GraphQL
// endpoint, which returns a "highlights" mix and misses recent posts.
package xnitter

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// StatusURL lists public instance health.
	StatusURL = "https://status.d420.de/api/v1/instances"
	// MaxLimit is one Nitter RSS page.
	MaxLimit = 20
	// poolTTL is how long a fetched instance list is reused.
	poolTTL = 30 * time.Minute

	maxBody  = 4 << 20
	maxNonce = 5_000_000
)

// defaultInstances are used when the status directory is unreachable.
var defaultInstances = []string{
	"https://nitter.kareem.one",
	"https://nitter.meowing.monster",
	"https://nitter.netbub.com",
}

// Tweet is one timeline entry.
type Tweet struct {
	ID        string
	URL       string
	Author    string
	Title     string
	HTML      string
	Text      string
	Images    []string
	Published time.Time
	IsRetweet bool
}

// Config tunes a Client.
type Config struct {
	Instances []string
	StatusURL string
	Timeout   time.Duration
	UserAgent string
	Now       func() time.Time
}

// Client fetches timelines from a pool of Nitter instances.
type Client struct {
	http      *http.Client
	ua        string
	fallback  []string
	statusURL string
	now       func() time.Time

	mu       sync.Mutex
	pool     []string
	pooledAt time.Time
}

// New builds a Client with production defaults.
func New(cfg Config) *Client {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 20 * time.Second
	}
	if cfg.UserAgent == "" {
		cfg.UserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0 Safari/537.36"
	}
	if cfg.StatusURL == "" {
		cfg.StatusURL = StatusURL
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	fallback := cfg.Instances
	if len(fallback) == 0 {
		fallback = defaultInstances
	}
	// A real jar so Anubis session cookies survive to the retry.
	jar, _ := cookiejar.New(nil)
	return &Client{
		http:      &http.Client{Timeout: cfg.Timeout, Jar: jar},
		ua:        cfg.UserAgent,
		fallback:  append([]string(nil), fallback...),
		statusURL: cfg.StatusURL,
		now:       cfg.Now,
	}
}

var handleRe = regexp.MustCompile(`^[A-Za-z0-9_]{1,15}$`)

// NormalizeHandle strips a leading @ and validates the name.
func NormalizeHandle(raw string) (string, error) {
	h := strings.TrimPrefix(strings.TrimSpace(raw), "@")
	if !handleRe.MatchString(h) {
		return "", fmt.Errorf("invalid X handle %q (letters, digits and _, 1-15 chars)", raw)
	}
	return h, nil
}

// Timeline returns the newest tweets for handle, newest first.
func (c *Client) Timeline(ctx context.Context, handle string) ([]Tweet, error) {
	handle, err := NormalizeHandle(handle)
	if err != nil {
		return nil, err
	}
	var failures []string
	for _, base := range c.instances(ctx) {
		if ctx.Err() != nil {
			break
		}
		tweets, ferr := c.timelineFrom(ctx, base, handle)
		if ferr == nil && len(tweets) > 0 {
			return tweets, nil
		}
		if ferr == nil {
			ferr = fmt.Errorf("returned no tweets")
		}
		failures = append(failures, base+": "+ferr.Error())
	}
	if len(failures) == 0 {
		return nil, fmt.Errorf("xnitter: no instance available")
	}
	return nil, fmt.Errorf("xnitter: all instances failed (%s)", strings.Join(failures, "; "))
}

func (c *Client) timelineFrom(ctx context.Context, base, handle string) ([]Tweet, error) {
	target := strings.TrimRight(base, "/") + "/" + handle + "/rss"
	body, err := c.get(ctx, target)
	if err != nil {
		return nil, err
	}
	if ch := parseChallenge(body); ch != nil {
		if err := c.solve(ctx, base, ch, target); err != nil {
			return nil, fmt.Errorf("anubis: %w", err)
		}
		if body, err = c.get(ctx, target); err != nil {
			return nil, err
		}
	}
	return parseRSS(body, base, handle)
}

// instances returns the current candidate list, best first.
func (c *Client) instances(ctx context.Context) []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.pool) > 0 && c.now().Sub(c.pooledAt) < poolTTL {
		return append([]string(nil), c.pool...)
	}
	if pool, err := c.fetchPool(ctx); err == nil && len(pool) > 0 {
		c.pool, c.pooledAt = pool, c.now()
		return append([]string(nil), pool...)
	}
	// The directory is optional: keep serving from the baked-in list.
	return append([]string(nil), c.fallback...)
}

type statusDoc struct {
	Hosts []struct {
		URL      string  `json:"url"`
		Points   float64 `json:"points"`
		RSS      bool    `json:"rss"`
		BadHost  bool    `json:"is_bad_host"`
		Healthy  bool    `json:"healthy"`
		Upstream bool    `json:"is_upstream"`
	} `json:"hosts"`
}

func (c *Client) fetchPool(ctx context.Context) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.statusURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.ua)
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status directory returned HTTP %d", resp.StatusCode)
	}
	var doc statusDoc
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&doc); err != nil {
		return nil, err
	}
	type cand struct {
		url    string
		points float64
	}
	var cands []cand
	for _, h := range doc.Hosts {
		if !h.RSS || h.BadHost || h.URL == "" || !strings.HasPrefix(h.URL, "https://") {
			continue
		}
		if h.Points < 60 {
			continue
		}
		cands = append(cands, cand{url: strings.TrimRight(h.URL, "/"), points: h.Points})
	}
	sort.SliceStable(cands, func(i, j int) bool { return cands[i].points > cands[j].points })
	out := make([]string, 0, len(cands))
	for _, cd := range cands {
		out = append(out, cd.url)
	}
	return out, nil
}

func (c *Client) get(ctx context.Context, target string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.ua)
	req.Header.Set("Accept", "application/rss+xml, application/xml;q=0.9, */*;q=0.8")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return body, nil
}

// ---- Anubis proof-of-work gate -------------------------------------------

type challenge struct {
	kind       string // "fast" or "preact"
	randomData string
	diff       int
	id         string
	redir      string
	result     string // preact: sha256(challenge)
}

var (
	anonChallenge = regexp.MustCompile(`(?s)<script id="anubis_challenge"[^>]*>(.*?)</script>`)
	preactInfo    = regexp.MustCompile(`(?s)<script id="preact_info"[^>]*>(.*?)</script>`)
)

func parseChallenge(body []byte) *challenge {
	if m := anonChallenge.FindSubmatch(body); m != nil {
		var doc struct {
			Challenge struct {
				RandomData string `json:"randomData"`
				Difficulty int    `json:"difficulty"`
				ID         string `json:"id"`
			} `json:"challenge"`
		}
		if err := json.Unmarshal(m[1], &doc); err == nil && doc.Challenge.RandomData != "" {
			return &challenge{kind: "fast", randomData: doc.Challenge.RandomData,
				diff: doc.Challenge.Difficulty, id: doc.Challenge.ID}
		}
	}
	if m := preactInfo.FindSubmatch(body); m != nil {
		var doc struct {
			Challenge string `json:"challenge"`
			Redir     string `json:"redir"`
		}
		if err := json.Unmarshal(m[1], &doc); err == nil && doc.Challenge != "" {
			sum := sha256.Sum256([]byte(doc.Challenge))
			return &challenge{kind: "preact", redir: doc.Redir, result: hex.EncodeToString(sum[:])}
		}
	}
	return nil
}

func (c *Client) solve(ctx context.Context, base string, ch *challenge, path string) error {
	var target string
	switch ch.kind {
	case "fast":
		nonce, digest, err := solvePoW(ctx, ch.randomData, ch.diff)
		if err != nil {
			return err
		}
		target = fmt.Sprintf("%s/.within.website/x/cmd/anubis/api/pass-challenge?id=%s&response=%s&nonce=%d&redir=%s&elapsedTime=3000",
			strings.TrimRight(base, "/"), url.QueryEscape(ch.id), digest, nonce,
			url.QueryEscape(path))
	case "preact":
		sep := "?"
		if strings.Contains(ch.redir, "?") {
			sep = "&"
		}
		target = strings.TrimRight(base, "/") + ch.redir + sep + "result=" + ch.result
	default:
		return fmt.Errorf("unknown challenge kind %q", ch.kind)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", c.ua)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	// The gate answers 200 (or a redirect); 429/403 still often sets the
	// session cookie, so treat those as solved and let the retry decide.
	if resp.StatusCode >= 500 {
		return fmt.Errorf("gate returned HTTP %d", resp.StatusCode)
	}
	return nil
}

func solvePoW(ctx context.Context, data string, diff int) (int, string, error) {
	if diff < 1 {
		diff = 1
	}
	prefix := strings.Repeat("0", diff)
	for nonce := 0; nonce < maxNonce; nonce++ {
		if nonce%4096 == 0 {
			if err := ctx.Err(); err != nil {
				return 0, "", err
			}
		}
		sum := sha256.Sum256([]byte(data + strconv.Itoa(nonce)))
		h := hex.EncodeToString(sum[:])
		if strings.HasPrefix(h, prefix) {
			return nonce, h, nil
		}
	}
	return 0, "", fmt.Errorf("no solution after %d attempts", maxNonce)
}

// ---- RSS parsing ----------------------------------------------------------

type rssDoc struct {
	Channel struct {
		Title string    `xml:"title"`
		Items []rssItem `xml:"item"`
	} `xml:"channel"`
}

type rssItem struct {
	Title       string `xml:"title"`
	Creator     string `xml:"creator"`
	Description string `xml:"description"`
	PubDate     string `xml:"pubDate"`
	GUID        string `xml:"guid"`
	Link        string `xml:"link"`
}

func parseRSS(body []byte, base, handle string) ([]Tweet, error) {
	var doc rssDoc
	if err := xml.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("nitter rss parse: %w", err)
	}
	if len(doc.Channel.Items) == 0 {
		return nil, fmt.Errorf("nitter rss carried no items")
	}
	out := make([]Tweet, 0, len(doc.Channel.Items))
	for _, it := range doc.Channel.Items {
		id := strings.TrimSpace(it.GUID)
		if id == "" {
			id = statusID(it.Link)
		}
		if id == "" {
			continue
		}
		author := strings.TrimPrefix(strings.TrimSpace(it.Creator), "@")
		html := rewriteLinks(strings.TrimSpace(it.Description), base)
		out = append(out, Tweet{
			ID:        id,
			URL:       "https://x.com/" + handle + "/status/" + id,
			Author:    author,
			Title:     strings.TrimSpace(it.Title),
			HTML:      html,
			Text:      htmlToText(html),
			Images:    imageURLs(html),
			Published: parseTime(it.PubDate),
			IsRetweet: strings.HasPrefix(strings.TrimSpace(it.Title), "RT by @"),
		})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("nitter rss had no usable items")
	}
	return out, nil
}

var statusRe = regexp.MustCompile(`/status/(\d+)`)

func statusID(link string) string {
	if m := statusRe.FindStringSubmatch(link); m != nil {
		return m[1]
	}
	return ""
}

var (
	imgRe     = regexp.MustCompile(`(?i)<img[^>]+src="([^"]+)"`)
	tagRe     = regexp.MustCompile(`(?s)<[^>]+>`)
	spaceRe   = regexp.MustCompile(`\s+`)
	paragraph = regexp.MustCompile(`(?i)</p>|<br\s*/?>|<hr\s*/?>`)
)

func imageURLs(html string) []string {
	var out []string
	seen := map[string]bool{}
	for _, m := range imgRe.FindAllStringSubmatch(html, -1) {
		src := strings.TrimSpace(m[1])
		if src == "" || strings.HasPrefix(src, "data:") || seen[src] {
			continue
		}
		seen[src] = true
		out = append(out, src)
	}
	return out
}

func htmlToText(html string) string {
	s := paragraph.ReplaceAllString(html, "\n")
	s = tagRe.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "&amp;", "&")
	s = strings.ReplaceAll(s, "&lt;", "<")
	s = strings.ReplaceAll(s, "&gt;", ">")
	s = strings.ReplaceAll(s, "&quot;", `"`)
	s = strings.ReplaceAll(s, "&#39;", "'")
	s = strings.ReplaceAll(s, "&nbsp;", " ")
	var lines []string
	for _, line := range strings.Split(s, "\n") {
		if line = spaceRe.ReplaceAllString(strings.TrimSpace(line), " "); line != "" {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

func parseTime(raw string) time.Time {
	raw = strings.TrimSpace(raw)
	for _, layout := range []string{time.RFC1123Z, time.RFC1123, time.RFC822Z, time.RFC822} {
		if t, err := time.Parse(layout, raw); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

// rewriteLinks turns instance-local links into x.com links so the feed does
// not leak the mirror that happened to answer. Image hosts are untouched.
func rewriteLinks(html, base string) string {
	u, err := url.Parse(base)
	if err != nil || u.Host == "" {
		return html
	}
	re := regexp.MustCompile(`https?://` + regexp.QuoteMeta(u.Host) + `[^"'\s<>]*`)
	return re.ReplaceAllStringFunc(html, func(raw string) string {
		p, perr := url.Parse(raw)
		if perr != nil || p.Path == "" {
			return raw
		}
		// Path only: drops the "#m" anchor Nitter appends to status links.
		return "https://x.com" + p.Path
	})
}
