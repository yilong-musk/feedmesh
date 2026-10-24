package weibo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/yilong-musk/feedmesh/internal/guard"
	"github.com/yilong-musk/feedmesh/internal/logx"
)

// Endpoints and defaults.
const (
	baseURL    = "https://m.weibo.cn"
	visitorURL = "https://visitor.passport.weibo.cn/visitor/genvisitor2"

	// DefaultUserAgent is the mobile Chrome UA the upstream Node client uses.
	DefaultUserAgent = "Mozilla/5.0 (Linux; Android 6.0; Nexus 5 Build/MRA58N) " +
		"AppleWebKit/537.36 (KHTML, like Gecko) Chrome/143.0.0.0 Mobile Safari/537.36"

	// DefaultTimeout is 9s; slower providers use a larger per-service default.
	DefaultTimeout = 9 * time.Second

	maxBodyBytes = 8 << 20
	cookieTTL    = time.Hour
)

type requestMode int

const (
	modeJSON requestMode = iota
	modeDocument
)

// Config tunes a Client.
type Config struct {
	Timeout   time.Duration
	UserAgent string
	Breaker   *guard.Breaker
	Log       *logx.Logger
	Retries   int
	Backoff   time.Duration

	// BaseURL and VisitorURL exist so tests can point the client at a fake
	// upstream. Production always leaves them empty.
	BaseURL    string
	VisitorURL string
	HotURL     string
}

// Client talks to weibo. All upstream traffic funnels through one semaphore, so
// the service never issues more than one concurrent request to weibo.
type Client struct {
	cfg  Config
	http *http.Client
	sem  chan struct{}

	cookieMu sync.Mutex
	cookie   string
	cookieAt time.Time

	fetchMu sync.Mutex
}

// New builds a Client, applying defaults for unset Config fields.
func New(cfg Config) *Client {
	if cfg.Timeout <= 0 {
		cfg.Timeout = DefaultTimeout
	}
	if cfg.UserAgent == "" {
		cfg.UserAgent = DefaultUserAgent
	}
	if cfg.Log == nil {
		cfg.Log = logx.Discard()
	}
	if cfg.Retries <= 0 {
		cfg.Retries = 3
	}
	if cfg.Backoff <= 0 {
		cfg.Backoff = 300 * time.Millisecond
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = baseURL
	}
	if cfg.VisitorURL == "" {
		cfg.VisitorURL = visitorURL
	}
	if cfg.HotURL == "" {
		cfg.HotURL = hotSearchURL
	}
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		MaxIdleConns:          16,
		MaxIdleConnsPerHost:   4,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   cfg.Timeout,
		ResponseHeaderTimeout: cfg.Timeout,
		ForceAttemptHTTP2:     true,
	}
	hc := &http.Client{
		Transport: transport,
		Timeout:   cfg.Timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 8 {
				return errors.New("too many redirects")
			}
			return nil
		},
	}
	return &Client{cfg: cfg, http: hc, sem: make(chan struct{}, 1)}
}

// Breaker exposes the shared breaker for health reporting.
func (c *Client) Breaker() *guard.Breaker { return c.cfg.Breaker }

// base returns the API root (m.weibo.cn in production).
func (c *Client) base() string { return strings.TrimRight(c.cfg.BaseURL, "/") }

// visitorEndpoint returns the visitor cookie endpoint.
func (c *Client) visitorEndpoint() string { return c.cfg.VisitorURL }

// Timeout returns the configured per-request timeout.
func (c *Client) Timeout() time.Duration { return c.cfg.Timeout }

// Visitor refreshes and returns the visitor cookie header value. The cookie is
// required: without it m.weibo.cn answers 418/403.
func (c *Client) Visitor(ctx context.Context) (string, error) {
	return c.visitor(ctx, true)
}

func (c *Client) visitor(ctx context.Context, force bool) (string, error) {
	c.cookieMu.Lock()
	if !force && c.cookie != "" && time.Since(c.cookieAt) < cookieTTL {
		ck := c.cookie
		c.cookieMu.Unlock()
		return ck, nil
	}
	c.cookieMu.Unlock()

	c.fetchMu.Lock()
	defer c.fetchMu.Unlock()

	c.cookieMu.Lock()
	if !force && c.cookie != "" && time.Since(c.cookieAt) < cookieTTL {
		ck := c.cookie
		c.cookieMu.Unlock()
		return ck, nil
	}
	c.cookieMu.Unlock()

	ck, err := c.fetchVisitor(ctx)
	if err != nil {
		return "", err
	}
	c.cookieMu.Lock()
	c.cookie = ck
	c.cookieAt = time.Now()
	c.cookieMu.Unlock()
	return ck, nil
}

func (c *Client) invalidateCookie() {
	c.cookieMu.Lock()
	c.cookie = ""
	c.cookieAt = time.Time{}
	c.cookieMu.Unlock()
}

func (c *Client) fetchVisitor(ctx context.Context) (string, error) {
	form := url.Values{}
	form.Set("cb", "visitor_gray_callback")
	form.Set("tid", "")
	form.Set("from", "weibo")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.visitorEndpoint(), strings.NewReader(form.Encode()))
	if err != nil {
		return "", &Error{Code: CodeInternal, Message: "build visitor request", Err: err}
	}
	req.Header.Set("User-Agent", c.cfg.UserAgent)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "*/*")

	if err := c.acquire(ctx); err != nil {
		return "", err
	}
	defer c.release()

	start := time.Now()
	resp, err := c.http.Do(req)
	if err != nil {
		return "", networkError(ctx, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return "", networkError(ctx, err)
	}
	c.cfg.Log.Debug("visitor response", "status", resp.StatusCode, "bytes", len(body), "ms", time.Since(start).Milliseconds())

	if resp.StatusCode == http.StatusTeapot || resp.StatusCode == http.StatusForbidden {
		if c.cfg.Breaker != nil {
			c.cfg.Breaker.Trip()
		}
		return "", &Error{Code: CodeUpstreamBlocked, Status: resp.StatusCode,
			Message: fmt.Sprintf("visitor endpoint returned HTTP %d", resp.StatusCode)}
	}
	if resp.StatusCode != http.StatusOK {
		return "", &Error{Code: CodeUpstreamUnavailable, Status: resp.StatusCode,
			Message: fmt.Sprintf("visitor endpoint returned HTTP %d", resp.StatusCode)}
	}

	var pairs []string
	for _, ck := range resp.Cookies() {
		if ck.Name == "" || ck.Value == "" {
			continue
		}
		pairs = append(pairs, ck.Name+"="+ck.Value)
	}
	if len(pairs) > 0 {
		return strings.Join(pairs, "; "), nil
	}

	// Fallback: the JSON body carries sub/subp directly.
	payload := extractJSONObject(body)
	if payload != nil {
		var vr visitorResponse
		if err := json.Unmarshal(payload, &vr); err == nil && vr.Data != nil && vr.Data.Sub != "" {
			header := "SUB=" + vr.Data.Sub
			if vr.Data.Subp != "" {
				header += "; SUBP=" + vr.Data.Subp
			}
			return header, nil
		}
	}
	return "", &Error{Code: CodeUpstreamBlocked, Message: "visitor endpoint returned no cookies"}
}

// extractJSONObject digs the JSON payload out of the JS callback wrapper.
func extractJSONObject(body []byte) []byte {
	start := -1
	for i, b := range body {
		if b == '{' {
			start = i
			break
		}
	}
	if start < 0 {
		return nil
	}
	end := -1
	for i := len(body) - 1; i >= start; i-- {
		if body[i] == '}' {
			end = i
			break
		}
	}
	if end <= start {
		return nil
	}
	return body[start : end+1]
}

func (c *Client) acquire(ctx context.Context) error {
	select {
	case c.sem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return timeoutError(ctx, "waiting for upstream slot")
	}
}

func (c *Client) release() { <-c.sem }

// fetchJSON performs a GET and decodes the JSON body into out.
func (c *Client) fetchJSON(ctx context.Context, rawURL, referer string, out any) (int, error) {
	body, status, _, err := c.do(ctx, rawURL, referer, modeJSON)
	if err != nil {
		return status, err
	}
	if out != nil {
		if err := json.Unmarshal(body, out); err != nil {
			return status, &Error{Code: CodeUpstreamUnavailable, Status: status,
				Message: "upstream returned a non-JSON payload", Err: err}
		}
	}
	return status, nil
}

// do performs a GET with breaker checks, one cookie-refresh retry on a block,
// and bounded exponential backoff on transport errors and 5xx responses.
func (c *Client) do(ctx context.Context, rawURL, referer string, mode requestMode) ([]byte, int, string, error) {
	probe := false
	if c.cfg.Breaker != nil {
		allowed, admittedProbe := c.cfg.Breaker.AllowRequest()
		if !allowed {
			return nil, 0, "", breakerOpenError()
		}
		probe = admittedProbe
	}
	if c.cfg.Breaker != nil {
		// The probe may also be admitted during a retry, so decide at return
		// time whether it still needs releasing.
		defer func() {
			if probe {
				c.cfg.Breaker.AbortProbe()
			}
		}()
	}

	attempts := c.cfg.Retries + 1
	var lastErr error
	cookieRetried := false

	// checkBreaker is called before each retry. A visitor 418/403 opens the
	// breaker from inside send; without this check the remaining attempts would
	// keep hammering both the visitor and the timeline endpoints.
	checkBreaker := func() error {
		if c.cfg.Breaker == nil {
			return nil
		}
		if c.cfg.Breaker.State().Open {
			return breakerOpenError()
		}
		if probe {
			return nil
		}
		allowed, admittedProbe := c.cfg.Breaker.AllowRequest()
		if !allowed {
			return breakerOpenError()
		}
		probe = admittedProbe
		return nil
	}

	for i := 0; i < attempts; i++ {
		if i > 0 {
			if err := checkBreaker(); err != nil {
				return nil, 0, "", err
			}
			delay := c.cfg.Backoff
			for j := 1; j < i; j++ {
				delay *= 2
			}
			if err := sleepCtx(ctx, delay); err != nil {
				// The overall deadline expired mid-backoff: report the real
				// cause (usually a timeout) rather than the backoff itself.
				var we *Error
				if lastErr != nil && errors.As(lastErr, &we) && we.Code == CodeTimeout {
					return nil, we.Status, "", lastErr
				}
				return nil, 0, "", timeoutError(ctx, "during retry backoff")
			}
			// The breaker may have opened while we were sleeping.
			if err := checkBreaker(); err != nil {
				return nil, 0, "", err
			}
		}

		body, status, finalURL, err := c.send(ctx, rawURL, referer, mode)
		if err != nil {
			if ctx.Err() != nil {
				return nil, 0, "", timeoutError(ctx, "request cancelled")
			}
			if code := CodeOf(err); code == CodeUpstreamBlocked || code == CodeBreakerOpen {
				// fetchVisitor trips the breaker itself; never retry a block.
				return nil, StatusOf(err), "", err
			}
			lastErr = err
			c.cfg.Log.Debug("upstream transport error", "url", rawURL, "attempt", i+1, "error", err.Error())
			continue
		}

		switch {
		case status == http.StatusTeapot || status == http.StatusForbidden:
			if !cookieRetried && i+1 < attempts {
				cookieRetried = true
				c.cfg.Log.Warn("upstream blocked, refreshing visitor cookie", "status", status, "url", rawURL)
				c.invalidateCookie()
				continue
			}
			if c.cfg.Breaker != nil {
				c.cfg.Breaker.Trip()
			}
			return body, status, finalURL, &Error{Code: CodeUpstreamBlocked, Status: status,
				Message: fmt.Sprintf("upstream returned HTTP %d (blocked)", status)}
		case status >= 500:
			lastErr = &Error{Code: CodeUpstreamUnavailable, Status: status,
				Message: fmt.Sprintf("upstream returned HTTP %d", status)}
			continue
		case status != http.StatusOK:
			return body, status, finalURL, &Error{Code: CodeUpstreamUnavailable, Status: status,
				Message: fmt.Sprintf("upstream returned HTTP %d", status)}
		}
		if probe {
			c.cfg.Breaker.Success()
		}
		return body, status, finalURL, nil
	}

	if lastErr != nil {
		var we *Error
		if errors.As(lastErr, &we) {
			return nil, we.Status, "", lastErr
		}
		return nil, 0, "", networkError(ctx, lastErr)
	}
	return nil, 0, "", &Error{Code: CodeNetwork, Message: "upstream request failed"}
}

func breakerOpenError() *Error {
	return &Error{Code: CodeBreakerOpen,
		Message: "upstream circuit breaker is open after a block; retry later"}
}

func (c *Client) send(ctx context.Context, rawURL, referer string, mode requestMode) ([]byte, int, string, error) {
	cookie, err := c.visitor(ctx, false)
	if err != nil {
		return nil, 0, "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, 0, "", &Error{Code: CodeInternal, Message: "build request", Err: err}
	}
	req.Header.Set("User-Agent", c.cfg.UserAgent)
	req.Header.Set("Cookie", cookie)
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	if mode == modeJSON {
		req.Header.Set("Mweibo-Pwa", "1")
		req.Header.Set("X-Requested-With", "XMLHttpRequest")
		req.Header.Set("Accept", "application/json, text/plain, */*")
	} else {
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	}
	if referer != "" {
		req.Header.Set("Referer", referer)
	}

	if err := c.acquire(ctx); err != nil {
		return nil, 0, "", err
	}
	defer c.release()

	start := time.Now()
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, "", networkError(ctx, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return nil, 0, "", networkError(ctx, err)
	}
	finalURL := rawURL
	if resp.Request != nil && resp.Request.URL != nil {
		finalURL = resp.Request.URL.String()
	}
	c.cfg.Log.Debug("upstream request", "status", resp.StatusCode, "bytes", len(body),
		"ms", time.Since(start).Milliseconds(), "url", rawURL)
	return body, resp.StatusCode, finalURL, nil
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func networkError(ctx context.Context, err error) error {
	if ctx.Err() != nil || errors.Is(err, context.DeadlineExceeded) {
		return timeoutError(ctx, err.Error())
	}
	var ne interface{ Timeout() bool }
	if errors.As(err, &ne) && ne.Timeout() {
		return &Error{Code: CodeTimeout, Message: "upstream request timed out", Err: err}
	}
	return &Error{Code: CodeNetwork, Message: "upstream request failed: " + err.Error(), Err: err}
}

func timeoutError(ctx context.Context, detail string) error {
	msg := "upstream request timed out"
	if detail != "" {
		msg += ": " + detail
	}
	err := ctx.Err()
	return &Error{Code: CodeTimeout, Message: msg, Err: err}
}
