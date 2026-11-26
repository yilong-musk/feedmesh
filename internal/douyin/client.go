package douyin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/yilong-musk/feedmesh/internal/logx"
)

const (
	// DefaultLimit is the number of videos a request keeps.
	DefaultLimit = 20
	// MaxLimit caps one call; the API's first page returns at most 44 items.
	MaxLimit = 44
	// DefaultTimeout bounds one Timeline call, including the browser warm-up.
	DefaultTimeout = 45 * time.Second
	// idleTimeout closes Chromium when no feed asked for it recently.
	idleTimeout = 2 * time.Minute
	// userAgent keeps the page on the desktop layout.
	userAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/156.0.0.0 Safari/537.36"
	// homeURL is where the visitor cookies and the request signer live.
	homeURL = "https://www.douyin.com/"
)

// Errors reported to the caller.
var (
	ErrInvalidUser = errors.New("invalid douyin user id")
	ErrNotReady    = errors.New("douyin page did not become ready")
)

// Config configures the client.
type Config struct {
	// ChromePath is an explicit Chromium binary; empty means auto-detect.
	ChromePath string
	// ProfileDir keeps the visitor identity between runs; empty means a
	// throwaway profile inside the OS temp directory.
	ProfileDir string
	// Timeout bounds one Timeline call.
	Timeout time.Duration
	// Log reports lifecycle events; nil is silent.
	Log *logx.Logger
}

// Item is one video from a creator's timeline.
type Item struct {
	ID         string
	Desc       string
	Author     string
	SecUID     string
	CreatedAt  time.Time
	DurationMS int
	Width      int
	Height     int
	IsTop      bool
	IsImage    bool
	PlayURL    string
	CoverURL   string
}

// Timeline is one creator's page of videos, newest first.
type Timeline struct {
	SecUID    string
	Author    string
	FetchedAt time.Time
	Items     []Item
}

// Client fetches timelines through a headless browser.
type Client struct {
	cfg Config

	mu     sync.Mutex
	br     *browser
	closed bool
	reap   bool

	// slot serialises Timeline calls. Every call drives the same page, and a
	// navigation started by one call destroys the execution context of
	// another call's in-flight fetch. A channel instead of a mutex lets a
	// queued caller leave when its own deadline passes.
	slot chan struct{}
}

// New returns a client. The browser starts lazily on the first Timeline call.
func New(cfg Config) *Client {
	if cfg.Timeout <= 0 {
		cfg.Timeout = DefaultTimeout
	}
	return &Client{cfg: cfg, slot: make(chan struct{}, 1)}
}

// Budget is the maximum time one call may take.
func (c *Client) Budget() time.Duration { return c.cfg.Timeout }

// Close stops the browser.
func (c *Client) Close() {
	c.mu.Lock()
	c.closed = true
	br := c.br
	c.br = nil
	c.mu.Unlock()
	if br != nil {
		br.Close()
	}
}

// browser returns the shared browser, starting it when needed.
func (c *Client) browser() (*browser, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil, errors.New("douyin client is closed")
	}
	if c.br != nil {
		if !c.br.isDead() {
			return c.br, nil
		}
		// A browser whose protocol stream broke stays dead; without this the
		// client would answer "browser is closed" until the process restarts
		// (the idle reaper only exists after a first successful call).
		dead := c.br
		c.br = nil
		go dead.Close()
		c.logf("douyin browser stopped answering, starting a new one")
	}
	br, err := startBrowser(c.cfg.ChromePath, c.cfg.ProfileDir, c.logf)
	if err != nil {
		return nil, err
	}
	c.br = br
	return br, nil
}

func (c *Client) logf(msg string, kv ...any) {
	if c.cfg.Log != nil {
		c.cfg.Log.Info(msg, kv...)
	}
}

// startReaper makes sure one idle watcher exists for the current browser.
func (c *Client) startReaper(br *browser) {
	c.mu.Lock()
	if c.reap {
		c.mu.Unlock()
		return
	}
	c.reap = true
	c.mu.Unlock()
	go c.reapIdle(br)
}

// reapIdle closes the browser once it has been unused for a while.
func (c *Client) reapIdle(br *browser) {
	for {
		time.Sleep(30 * time.Second)
		c.mu.Lock()
		if c.closed || c.br != br {
			c.reap = false
			c.mu.Unlock()
			return
		}
		lastUsed := time.Unix(0, br.lastUsed.Load())
		if time.Since(lastUsed) < idleTimeout {
			c.mu.Unlock()
			continue
		}
		c.br = nil
		c.reap = false
		c.mu.Unlock()
		br.Close()
		c.logf("douyin browser closed after idle timeout")
		return
	}
}

// Timeline reads the creator's first page, newest first. Pinned videos keep
// their marker but are ordered by publish time like every other item.
func (c *Client) Timeline(ctx context.Context, secUID string, limit int) (*Timeline, error) {
	secUID = strings.TrimSpace(secUID)
	if !validSecUID(secUID) {
		return nil, ErrInvalidUser
	}
	if limit <= 0 {
		limit = DefaultLimit
	}
	if limit > MaxLimit {
		limit = MaxLimit
	}

	ctx, cancel := context.WithTimeout(ctx, c.cfg.Timeout)
	defer cancel()

	select {
	case c.slot <- struct{}{}:
	case <-ctx.Done():
		return nil, fmt.Errorf("wait for the douyin browser: %w", ctx.Err())
	}
	defer func() { <-c.slot }()

	br, err := c.browser()
	if err != nil {
		return nil, err
	}

	// A fresh page needs about fifteen seconds before its signer is ready;
	// every attempt before that answers "Sign Invalid" or an empty body.
	if !br.warm.Load() {
		if err := br.navigate(homeURL); err != nil {
			return nil, fmt.Errorf("open douyin: %w", err)
		}
	}

	var body string
	var lastErr error
	err = waitFor(ctx, 2*time.Second, func() (bool, error) {
		raw, ferr := br.fetchPosts(secUID, limit)
		if ferr != nil {
			lastErr = ferr
			return false, nil
		}
		var probe struct {
			StatusCode int               `json:"status_code"`
			AwemeList  []json.RawMessage `json:"aweme_list"`
		}
		if jerr := json.Unmarshal([]byte(raw), &probe); jerr == nil && probe.AwemeList != nil {
			body = raw
			return true, nil
		}
		if len(raw) > 0 {
			lastErr = fmt.Errorf("upstream rejected the request: %s", clip(raw, 120))
		}
		return false, nil
	})
	if err != nil {
		if lastErr != nil {
			return nil, fmt.Errorf("%w (%v)", ErrNotReady, lastErr)
		}
		return nil, fmt.Errorf("%w: %v", ErrNotReady, err)
	}

	out, err := parseTimeline(body, secUID, limit)
	if err != nil {
		return nil, err
	}

	br.warm.Store(true)
	br.lastUsed.Store(time.Now().UnixNano())
	c.startReaper(br)
	return out, nil
}

// parseTimeline turns one upstream page into a timeline. It is separate from
// Timeline so the mapping can be tested without a browser.
func parseTimeline(body, secUID string, limit int) (*Timeline, error) {
	var resp struct {
		AwemeList []apiAweme `json:"aweme_list"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		return nil, fmt.Errorf("parse douyin response: %w", err)
	}

	out := &Timeline{SecUID: secUID, FetchedAt: time.Now().UTC()}
	seen := make(map[string]bool, len(resp.AwemeList))
	for i := range resp.AwemeList {
		a := &resp.AwemeList[i]
		if a.AwemeID == "" || seen[a.AwemeID] {
			continue
		}
		seen[a.AwemeID] = true
		it := Item{
			ID:         a.AwemeID,
			Desc:       strings.TrimSpace(a.Desc),
			Author:     a.Author.Nickname,
			SecUID:     a.Author.SecUID,
			CreatedAt:  time.Unix(a.CreateTime, 0).UTC(),
			DurationMS: a.Video.Duration,
			Width:      a.Video.Width,
			Height:     a.Video.Height,
			IsTop:      a.IsTop == 1,
			IsImage:    len(a.Images) > 0,
			PlayURL:    a.Video.playURL(),
			CoverURL:   a.Video.coverURL(),
		}
		if out.Author == "" {
			out.Author = a.Author.Nickname
		}
		if it.SecUID == "" {
			it.SecUID = secUID
		}
		out.Items = append(out.Items, it)
	}
	if len(out.Items) == 0 {
		return nil, fmt.Errorf("%w: empty timeline", ErrNotReady)
	}
	// Pinned videos arrive first in upstream order; readers want the real
	// timeline, so sort every item by publish time.
	sort.SliceStable(out.Items, func(i, j int) bool { return out.Items[i].CreatedAt.After(out.Items[j].CreatedAt) })
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
	}
	return out, nil
}

func validSecUID(s string) bool {
	if len(s) < 20 || len(s) > 200 {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
		default:
			return false
		}
	}
	return true
}

func clip(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= n {
		return string(r)
	}
	return string(r[:n]) + "..."
}

// videoInfo is the media block of one video.
type videoInfo struct {
	Duration int `json:"duration"`
	Width    int `json:"width"`
	Height   int `json:"height"`
	PlayAddr struct {
		URLList []string `json:"url_list"`
	} `json:"play_addr"`
	Cover struct {
		URLList []string `json:"url_list"`
	} `json:"cover"`
	Poster string `json:"poster"`
}

// playURL picks a clean stream. Douyin's play_addr is the watermark-free one.
func (v *videoInfo) playURL() string {
	for _, u := range v.PlayAddr.URLList {
		if strings.Contains(u, "zjcdn") || strings.Contains(u, "douyinvod") {
			return u
		}
	}
	if len(v.PlayAddr.URLList) > 0 {
		return v.PlayAddr.URLList[0]
	}
	return ""
}

// coverURL prefers the poster image.
func (v *videoInfo) coverURL() string {
	if v.Poster != "" {
		return v.Poster
	}
	if len(v.Cover.URLList) > 0 {
		return v.Cover.URLList[0]
	}
	return ""
}

// apiAweme is the subset of the upstream object this service uses.
type apiAweme struct {
	AwemeID    string `json:"aweme_id"`
	Desc       string `json:"desc"`
	CreateTime int64  `json:"create_time"`
	IsTop      int    `json:"is_top"`
	Author     struct {
		Nickname string `json:"nickname"`
		SecUID   string `json:"sec_uid"`
	} `json:"author"`
	Video  videoInfo `json:"video"`
	Images []struct {
		URLList []string `json:"url_list"`
	} `json:"images"`
}

// fetchPosts asks the page to call the post list endpoint; the page signs it.
// The fetch aborts itself after 15 seconds: Runtime.evaluate's own timeout does
// not bound an awaited promise, and a reply that misses the 30-second protocol
// read deadline would cost the whole browser instead of one attempt.
func (b *browser) fetchPosts(secUID string, count int) (string, error) {
	u := fmt.Sprintf("/aweme/v1/web/aweme/post/?device_platform=webapp&aid=6383&channel=channel_pc_web"+
		"&publish_video_strategy_type=2&locate_query=false&show_live_replay_strategy=1"+
		"&need_time_list=1&time_list_query=0&max_cursor=0&sec_user_id=%s&count=%d", secUID, count)
	expr := fmt.Sprintf("fetch(%q, {credentials:'include', signal: AbortSignal.timeout(15000)}).then(r => r.text())", u)
	return b.evaluate(expr, true, 20000)
}
