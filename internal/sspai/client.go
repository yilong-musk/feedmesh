// Package sspai talks to the public sspai.com JSON API.
//
// The official RSS feed only exposes a short summary, so feedmesh reads the
// article API instead: the list endpoint supplies metadata (cover, author,
// release time) and the info endpoint supplies the full HTML body.
package sspai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultBaseURL is the production site.
	DefaultBaseURL = "https://sspai.com"
	// ImageBaseURL serves article banners referenced by the list API.
	ImageBaseURL = "https://rssfile.sspai.com/"
	// PageSize is the largest page the list endpoint honours.
	PageSize = 10
	// MaxLimit caps how many articles one feed may request.
	MaxLimit = 30
)

// Config tunes a Client.
type Config struct {
	BaseURL   string
	Timeout   time.Duration
	UserAgent string
}

// Client reads the sspai article API. It is safe for concurrent use.
type Client struct {
	base string
	ua   string
	http *http.Client
}

// New builds a Client with production defaults.
func New(cfg Config) *Client {
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultBaseURL
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 10 * time.Second
	}
	if cfg.UserAgent == "" {
		cfg.UserAgent = "feedmesh/1.0 (+https://sspai.com)"
	}
	return &Client{
		base: strings.TrimRight(cfg.BaseURL, "/"),
		ua:   cfg.UserAgent,
		http: &http.Client{Timeout: cfg.Timeout},
	}
}

// Article is one list entry, optionally enriched by Detail.
type Article struct {
	ID           int    `json:"id"`
	Title        string `json:"title"`
	Summary      string `json:"summary"`
	Banner       string `json:"banner"`
	ReleasedTime int64  `json:"released_time"`
	PostType     int    `json:"post_type"`
	Author       struct {
		Nickname string `json:"nickname"`
	} `json:"author"`

	// Filled by Detail.
	Body     string `json:"-"`
	Keywords string `json:"-"`
	IsPay    bool   `json:"-"`
}

// URL is the canonical article permalink.
func (a Article) URL() string { return DefaultBaseURL + "/post/" + strconv.Itoa(a.ID) }

// CoverURL turns the relative banner path into an absolute image URL.
func (a Article) CoverURL() string {
	b := strings.TrimSpace(a.Banner)
	if b == "" {
		return ""
	}
	if strings.HasPrefix(b, "http://") || strings.HasPrefix(b, "https://") {
		return b
	}
	return ImageBaseURL + strings.TrimLeft(b, "/")
}

type listResponse struct {
	Error int       `json:"error"`
	Msg   string    `json:"msg"`
	Data  []Article `json:"data"`
}

type detailResponse struct {
	Error int    `json:"error"`
	Msg   string `json:"msg"`
	Data  *struct {
		Body     string `json:"body"`
		Keywords string `json:"keywords"`
		IsPay    bool   `json:"is_pay"`
		IsFree   bool   `json:"is_free"`
	} `json:"data"`
}

// List returns up to limit articles, newest first, paging through the API.
func (c *Client) List(ctx context.Context, limit int) ([]Article, error) {
	if limit <= 0 {
		limit = PageSize
	}
	if limit > MaxLimit {
		limit = MaxLimit
	}
	out := make([]Article, 0, limit)
	for offset := 0; len(out) < limit; offset += PageSize {
		page, err := c.listPage(ctx, offset, PageSize)
		if err != nil {
			return nil, err
		}
		for _, a := range page {
			if a.ID <= 0 || strings.TrimSpace(a.Title) == "" {
				continue
			}
			out = append(out, a)
		}
		if len(page) < PageSize {
			break
		}
	}
	if len(out) > limit {
		out = out[:limit]
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("sspai list returned no articles")
	}
	return out, nil
}

func (c *Client) listPage(ctx context.Context, offset, limit int) ([]Article, error) {
	q := url.Values{}
	q.Set("limit", strconv.Itoa(limit))
	q.Set("offset", strconv.Itoa(offset))
	var lr listResponse
	if err := c.getJSON(ctx, "/api/v1/article/index/page/get?"+q.Encode(), &lr); err != nil {
		return nil, err
	}
	if lr.Error != 0 {
		return nil, fmt.Errorf("sspai list error %d: %s", lr.Error, strings.TrimSpace(lr.Msg))
	}
	return lr.Data, nil
}

// Detail fetches the full HTML body and paywall/keyword metadata for one article.
func (c *Client) Detail(ctx context.Context, id int) (Article, error) {
	q := url.Values{}
	q.Set("id", strconv.Itoa(id))
	var dr detailResponse
	if err := c.getJSON(ctx, "/api/v1/article/info/get?"+q.Encode(), &dr); err != nil {
		return Article{}, err
	}
	if dr.Error != 0 || dr.Data == nil {
		return Article{}, fmt.Errorf("sspai detail error %d: %s", dr.Error, strings.TrimSpace(dr.Msg))
	}
	return Article{
		ID:       id,
		Body:     dr.Data.Body,
		Keywords: dr.Data.Keywords,
		IsPay:    dr.Data.IsPay && !dr.Data.IsFree,
	}, nil
}

func (c *Client) getJSON(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", c.ua)
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Referer", DefaultBaseURL+"/")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("sspai request failed: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return fmt.Errorf("sspai read failed: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("sspai returned HTTP %d", resp.StatusCode)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("sspai returned non-JSON payload: %w", err)
	}
	return nil
}
