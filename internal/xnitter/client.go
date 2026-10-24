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
	stdhtml "html"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
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
	// poolFailureTTL stops a dead status directory from adding a timeout to
	// every X request.
	poolFailureTTL = 30 * time.Second

	maxBody          = 4 << 20
	maxNonce         = 2_000_000
	maxDifficulty    = 12
	maxRandomData    = 2 << 10
	maxPoolAttempts  = 3
	maxPoolInstances = 20
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
	http           *http.Client
	ua             string
	fallback       []string
	statusURL      string
	now            func() time.Time
	attemptTimeout time.Duration
	trusted        map[string]struct{}
	dialer         *net.Dialer

	mu           sync.Mutex
	pool         []string
	pooledAt     time.Time
	poolFailedAt time.Time
	fetchMu      sync.Mutex
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
	fallback := append([]string(nil), cfg.Instances...)
	if len(fallback) == 0 {
		fallback = append([]string(nil), defaultInstances...)
	}
	// A real jar so Anubis session cookies survive to the retry.
	jar, _ := cookiejar.New(nil)
	c := &Client{
		ua:             cfg.UserAgent,
		fallback:       fallback,
		statusURL:      cfg.StatusURL,
		now:            cfg.Now,
		attemptTimeout: cfg.Timeout,
		trusted:        trustedHosts(cfg.StatusURL, fallback),
		dialer:         &net.Dialer{Timeout: cfg.Timeout, KeepAlive: 30 * time.Second},
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = c.dialContext
	c.http = &http.Client{
		Transport: transport,
		Timeout:   cfg.Timeout,
		Jar:       jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects")
			}
			// Nitter instances are untrusted; never let one redirect us to a
			// different host (which would turn the fetcher into an SSRF proxy).
			if len(via) > 0 && !strings.EqualFold(via[0].URL.Host, req.URL.Host) {
				return fmt.Errorf("refusing cross-host redirect to %s", req.URL.Host)
			}
			return nil
		},
	}
	return c
}

// trustedHosts lists hosts whose operator explicitly configured them. Directory
// results are not included, so their resolved addresses are checked at dial
// time. Proxies come from the environment and are trusted infrastructure.
func trustedHosts(statusURL string, instances []string) map[string]struct{} {
	out := make(map[string]struct{}, len(instances)+2)
	add := func(raw string) {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return
		}
		if !strings.Contains(raw, "://") {
			raw = "http://" + raw
		}
		u, err := url.Parse(raw)
		if err != nil || u.Hostname() == "" {
			return
		}
		out[normalizeHost(u.Hostname())] = struct{}{}
	}
	add(statusURL)
	for _, inst := range instances {
		add(inst)
	}
	for _, name := range []string{"HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy"} {
		add(os.Getenv(name))
	}
	return out
}

func normalizeHost(host string) string {
	return strings.ToLower(strings.TrimRight(strings.TrimSpace(host), "."))
}

// dialContext blocks directory-provided hosts from resolving to loopback,
// private, link-local or other non-public addresses. Checking the resolved IP
// at dial time also covers DNS names that point at internal services and DNS
// rebinding between validation and use. When an egress proxy is configured,
// the proxy resolves the origin, so the same policy must be enforced there.
func (c *Client) dialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	host = normalizeHost(host)
	if _, ok := c.trusted[host]; ok {
		return c.dialer.DialContext(ctx, network, addr)
	}
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("resolve %s: %w", host, err)
	}
	var lastErr error
	for _, ip := range ips {
		if !isPublicIP(ip.IP) {
			continue
		}
		conn, derr := c.dialer.DialContext(ctx, network, net.JoinHostPort(ip.IP.String(), port))
		if derr == nil {
			return conn, nil
		}
		lastErr = derr
	}
	if lastErr != nil {
		return nil, fmt.Errorf("connect to %s: %w", host, lastErr)
	}
	return nil, fmt.Errorf("refusing to connect to non-public host %q", host)
}

// isPublicIP rejects addresses that a directory-poisoning attacker could use to
// reach the host or its private network.
func isPublicIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() ||
		ip.IsMulticast() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return false
	}
	if ip4 := ip.To4(); ip4 != nil {
		switch {
		case ip4[0] == 0:
			return false // 0.0.0.0/8
		case ip4[0] == 100 && ip4[1] >= 64 && ip4[1] <= 127:
			return false // 100.64.0.0/10, carrier-grade NAT
		case ip4[0] == 192 && ip4[1] == 0 && ip4[2] == 0:
			return false // 192.0.0.0/24, IETF protocol assignments
		case ip4[0] == 192 && ip4[1] == 0 && ip4[2] == 2:
			return false // 192.0.2.0/24, TEST-NET-1
		case ip4[0] == 198 && (ip4[1] == 18 || ip4[1] == 19):
			return false // 198.18.0.0/15, benchmarking
		case ip4[0] == 198 && ip4[1] == 51 && ip4[2] == 100:
			return false // 198.51.100.0/24, TEST-NET-2
		case ip4[0] == 203 && ip4[1] == 0 && ip4[2] == 113:
			return false // 203.0.113.0/24, TEST-NET-3
		case ip4[0] >= 240:
			return false // 240.0.0.0/4, reserved and broadcast
		}
	}
	return true
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
	try := func(base string) ([]Tweet, bool) {
		attemptCtx, cancel := context.WithTimeout(ctx, c.attemptTimeout)
		defer cancel()
		tweets, ferr := c.timelineFrom(attemptCtx, base, handle)
		if ferr == nil && len(tweets) > 0 {
			return tweets, true
		}
		if ferr == nil {
			ferr = fmt.Errorf("returned no tweets")
		}
		failures = append(failures, base+": "+ferr.Error())
		return nil, false
	}

	tried := make(map[string]bool)
	for i, base := range c.instances(ctx) {
		if i >= maxPoolAttempts || ctx.Err() != nil {
			break
		}
		if tried[base] {
			continue
		}
		tried[base] = true
		if tweets, ok := try(base); ok {
			return tweets, nil
		}
	}
	// The directory attempt cap must not hide the baked-in fallback: when the
	// whole pool fails for this server, try the operator-trusted hosts too.
	for _, base := range c.fallback {
		if ctx.Err() != nil {
			break
		}
		if tried[base] {
			continue
		}
		tried[base] = true
		if tweets, ok := try(base); ok {
			return tweets, nil
		}
	}
	if len(failures) == 0 {
		return nil, fmt.Errorf("xnitter: no instance available")
	}
	// Do not echo every mirror URL into an HTTP error response.
	return nil, fmt.Errorf("xnitter: all instances failed (%d tried)", len(failures))
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
	if pool, ok := c.cachedPool(); ok {
		return pool
	}

	// Only one goroutine refreshes the directory. Other callers wait here and
	// then re-check the cache, so a slow status endpoint cannot create a
	// stampede against every X request.
	c.fetchMu.Lock()
	defer c.fetchMu.Unlock()
	if pool, ok := c.cachedPool(); ok {
		return pool
	}

	c.mu.Lock()
	failedRecently := !c.poolFailedAt.IsZero() && c.now().Sub(c.poolFailedAt) < poolFailureTTL
	c.mu.Unlock()
	if failedRecently {
		return c.fallbackWithPool()
	}

	statusCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	pool, err := c.fetchPool(statusCtx)
	cancel()
	c.mu.Lock()
	if err == nil && len(pool) > 0 {
		c.pool, c.pooledAt = pool, c.now()
		c.poolFailedAt = time.Time{}
	} else {
		c.poolFailedAt = c.now()
	}
	c.mu.Unlock()
	return c.fallbackWithPool()
}

func (c *Client) cachedPool() ([]string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.pool) > 0 && c.now().Sub(c.pooledAt) < poolTTL {
		return append([]string(nil), c.pool...), true
	}
	return nil, false
}

// fallbackWithPool merges the baked-in hosts after the directory results, so a
// directory full of dead nodes still leaves a working fallback.
func (c *Client) fallbackWithPool() []string {
	c.mu.Lock()
	pool := append([]string(nil), c.pool...)
	c.mu.Unlock()
	seen := make(map[string]bool, len(pool)+len(c.fallback))
	out := make([]string, 0, len(pool)+len(c.fallback))
	for _, u := range append(pool, c.fallback...) {
		if u == "" || seen[u] {
			continue
		}
		seen[u] = true
		out = append(out, u)
	}
	return out
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
		if !h.RSS || h.BadHost || h.Points < 60 {
			continue
		}
		host, err := publicInstanceURL(h.URL)
		if err != nil {
			continue
		}
		cands = append(cands, cand{url: host, points: h.Points})
	}
	sort.SliceStable(cands, func(i, j int) bool { return cands[i].points > cands[j].points })
	out := make([]string, 0, len(cands))
	for _, cd := range cands {
		if len(out) >= maxPoolInstances {
			break
		}
		out = append(out, cd.url)
	}
	return out, nil
}

// publicInstanceURL accepts only https URLs with a public-looking hostname.
// IP literals and obvious local names are rejected here; the client also
// verifies resolved addresses at dial time so DNS names and alternate IP
// spellings cannot point the fetcher at loopback or private services.
func publicInstanceURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("invalid instance URL")
	}
	if !strings.EqualFold(u.Scheme, "https") || u.User != nil || u.Host == "" {
		return "", fmt.Errorf("instance must be an https URL without userinfo")
	}
	host := normalizeHost(u.Hostname())
	if host == "" || host == "localhost" || strings.ContainsAny(host, " \t\r\n") {
		return "", fmt.Errorf("invalid instance host")
	}
	if net.ParseIP(host) != nil {
		return "", fmt.Errorf("instance host must not be an IP literal")
	}
	if !strings.Contains(host, ".") {
		return "", fmt.Errorf("instance host must be a qualified name")
	}
	for _, suffix := range []string{
		".local", ".localhost", ".internal", ".lan", ".home.arpa",
		".nip.io", ".sslip.io", ".xip.io", ".localtest.me", ".lvh.me", ".vcap.me", ".traefik.me",
	} {
		if strings.HasSuffix(host, suffix) {
			return "", fmt.Errorf("instance host looks private")
		}
	}
	// Refuse hostnames whose first label parses as a number in any base; this
	// catches decimal, octal and hexadecimal spellings of IPv4 addresses.
	if first := strings.Split(host, ".")[0]; first != "" {
		if _, err := strconv.ParseUint(first, 0, 64); err == nil {
			return "", fmt.Errorf("instance host looks like an address")
		}
	}
	if port := u.Port(); port != "" {
		u.Host = net.JoinHostPort(host, port)
	} else {
		u.Host = host
	}
	u.Scheme = "https"
	u.Path = ""
	u.RawPath = ""
	u.RawQuery = ""
	u.ForceQuery = false
	u.Fragment = ""
	u.RawFragment = ""
	return strings.TrimRight(u.String(), "/"), nil
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
	baseURL, err := url.Parse(base)
	if err != nil || baseURL.Host == "" {
		return fmt.Errorf("invalid Nitter base URL")
	}
	var target *url.URL
	switch ch.kind {
	case "fast":
		nonce, digest, err := solvePoW(ctx, ch.randomData, ch.diff)
		if err != nil {
			return err
		}
		target = baseURL.ResolveReference(&url.URL{Path: "/.within.website/x/cmd/anubis/api/pass-challenge"})
		q := target.Query()
		q.Set("id", ch.id)
		q.Set("response", digest)
		q.Set("nonce", strconv.Itoa(nonce))
		q.Set("redir", path)
		q.Set("elapsedTime", "3000")
		target.RawQuery = q.Encode()
	case "preact":
		redir, err := url.Parse(ch.redir)
		if err != nil || redir.IsAbs() || redir.Host != "" ||
			!strings.HasPrefix(ch.redir, "/") || strings.HasPrefix(ch.redir, "//") {
			return fmt.Errorf("refusing unsafe Anubis redirect")
		}
		target = baseURL.ResolveReference(redir)
		q := target.Query()
		q.Set("result", ch.result)
		target.RawQuery = q.Encode()
	default:
		return fmt.Errorf("unknown challenge kind %q", ch.kind)
	}
	if !strings.EqualFold(target.Host, baseURL.Host) {
		return fmt.Errorf("refusing cross-host challenge URL")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
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
	if diff > maxDifficulty {
		return 0, "", fmt.Errorf("challenge difficulty %d exceeds limit %d", diff, maxDifficulty)
	}
	if len(data) > maxRandomData {
		return 0, "", fmt.Errorf("challenge data too large")
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
		if len(out) >= MaxLimit {
			break
		}
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
// not leak the mirror that happened to answer. Query strings are preserved,
// relative links are absolutised, and Nitter's proxied /pic/ URLs are mapped
// back to the public pbs.twimg.com host.
func rewriteLinks(html, base string) string {
	u, err := url.Parse(base)
	if err != nil || u.Host == "" {
		return html
	}
	re := regexp.MustCompile(`(?i)(href|src)="([^"]*)"`)
	html = re.ReplaceAllStringFunc(html, func(attr string) string {
		m := re.FindStringSubmatch(attr)
		if len(m) != 3 {
			return attr
		}
		raw := strings.TrimSpace(stdhtml.UnescapeString(m[2]))
		if raw == "" || strings.HasPrefix(raw, "#") {
			return attr
		}
		p, perr := url.Parse(raw)
		if perr != nil {
			return attr
		}
		if p.Scheme != "" && p.Scheme != "http" && p.Scheme != "https" {
			return attr
		}
		mirror := p.Host == "" || strings.EqualFold(p.Host, u.Host)
		if !mirror {
			return attr
		}
		if strings.HasPrefix(p.Path, "/pic/") {
			if rewritten := nitterMediaURL(p.Path); rewritten != "" {
				return m[1] + `="` + stdhtml.EscapeString(rewritten) + `"`
			}
		}
		p.Scheme = "https"
		p.Host = "x.com"
		p.Fragment = ""
		return m[1] + `="` + stdhtml.EscapeString(p.String()) + `"`
	})
	// Visible URL text in Nitter's <cite> blocks also names the mirror.
	visible := regexp.MustCompile(`https?://` + regexp.QuoteMeta(u.Host) + `([^"'\s<>]*)`)
	return visible.ReplaceAllString(html, "https://x.com$1")
}

// nitterMediaURL decodes Nitter's /pic/ proxy path back to pbs.twimg.com.
func nitterMediaURL(path string) string {
	rest := strings.TrimPrefix(path, "/pic/")
	if rest == "" {
		return ""
	}
	if decoded, err := url.PathUnescape(rest); err == nil {
		rest = decoded
	}
	if u, err := url.Parse(rest); err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" {
		return u.String()
	}
	return "https://pbs.twimg.com/" + strings.TrimLeft(rest, "/")
}
