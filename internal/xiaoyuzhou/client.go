// Package xiaoyuzhou reads a public Xiaoyuzhou show page.
//
// The anonymous JSON API answers 401. The show page is a Next.js document
// whose __NEXT_DATA__ already lists the newest episodes, including the audio
// address, so one HTML read is the whole provider. That list is one page
// (about fifteen episodes); older episodes are not in the document.
package xiaoyuzhou

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/yilong-musk/feedmesh/internal/upstream"
)

const (
	// DefaultBaseURL is the public site.
	DefaultBaseURL = "https://www.xiaoyuzhoufm.com"
	// DefaultLimit is the width when the caller is silent.
	DefaultLimit = 15
	// MaxLimit is the widest list the show page carries.
	MaxLimit = 15

	maxPage   = 2 << 20
	userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"
	nextData  = `<script id="__NEXT_DATA__" type="application/json">`
)

// Errors callers map onto HTTP responses.
var (
	// ErrInvalidID means the reference is not a Xiaoyuzhou show id or show URL.
	ErrInvalidID = errors.New("xiaoyuzhou: invalid show id")
	// ErrNotFound means the site has no show for that id.
	ErrNotFound = errors.New("xiaoyuzhou: show not found")
	// ErrNoEpisodes means the page parsed but listed no episodes.
	ErrNoEpisodes = errors.New("xiaoyuzhou: show has no episodes")
	// ErrRateLimited means the site asked us to slow down.
	ErrRateLimited = errors.New("xiaoyuzhou: rate limited")
	// ErrUnexpected means the body was not a show page.
	ErrUnexpected = errors.New("xiaoyuzhou: unexpected response")
)

var (
	bareID  = regexp.MustCompile(`(?i)^[0-9a-f]{24}$`)
	idInURL = regexp.MustCompile(`(?i)(?:^|/)podcast/([0-9a-f]{24})(?:[/?#.]|$)`)
)

// HTTPError is a non-2xx answer from the show page.
type HTTPError struct {
	Status int
	URL    string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("xiaoyuzhou: %s answered %d", e.URL, e.Status)
}

// Episode is one item from the show page.
type Episode struct {
	ID        string
	Title     string
	Summary   string
	URL       string
	AudioURL  string
	Image     string
	Published time.Time
	Duration  time.Duration
	Plays     int64

	ShowTitle       string
	ShowAuthor      string
	ShowDescription string
	ShowImage       string
	ShowURL         string
}

// Show is one podcast and the newest episodes the page lists.
type Show struct {
	ID          string
	Title       string
	Author      string
	Description string
	URL         string
	Image       string
	Episodes    []Episode
}

// Config tunes a Client. Tests point BaseURL at an httptest server.
type Config struct {
	BaseURL string
	Timeout time.Duration
}

// Client reads Xiaoyuzhou over HTTP.
type Client struct {
	base string
	http *http.Client
}

// New builds a Client with production defaults.
func New(cfg Config) *Client {
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultBaseURL
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 15 * time.Second
	}
	return &Client{
		base: strings.TrimRight(cfg.BaseURL, "/"),
		http: &http.Client{Timeout: cfg.Timeout},
	}
}

// ParseID accepts a 24-character show id or a xiaoyuzhoufm.com/podcast URL.
func ParseID(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	s = strings.TrimSuffix(s, ".xml")
	if bareID.MatchString(s) {
		return strings.ToLower(s), nil
	}
	if m := idInURL.FindStringSubmatch(s); m != nil {
		return strings.ToLower(m[1]), nil
	}
	return "", ErrInvalidID
}

// Episodes reads one show page and returns the newest episodes it lists.
func (c *Client) Episodes(ctx context.Context, id string, limit int) (*Show, error) {
	id, err := ParseID(id)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = DefaultLimit
	}
	if limit > MaxLimit {
		limit = MaxLimit
	}
	pageURL := c.base + "/podcast/" + id
	body, status, err := c.get(ctx, pageURL)
	if err != nil {
		if status == http.StatusNotFound {
			return nil, ErrNotFound
		}
		return nil, err
	}
	show, err := parsePage(body, limit)
	if err != nil {
		return nil, err
	}
	if show.Title == "" && len(show.Episodes) == 0 {
		return nil, ErrNotFound
	}
	if len(show.Episodes) == 0 {
		return nil, ErrNoEpisodes
	}
	show.ID = id
	if show.URL == "" {
		show.URL = "https://www.xiaoyuzhoufm.com/podcast/" + id
	}
	for i := range show.Episodes {
		ep := &show.Episodes[i]
		ep.ShowTitle = show.Title
		ep.ShowAuthor = show.Author
		ep.ShowDescription = show.Description
		ep.ShowImage = show.Image
		ep.ShowURL = show.URL
		if ep.URL == "" && ep.ID != "" {
			ep.URL = "https://www.xiaoyuzhoufm.com/episode/" + ep.ID
		}
		if ep.Image == "" {
			ep.Image = show.Image
		}
	}
	return show, nil
}

func (c *Client) get(ctx context.Context, rawURL string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests {
		upstream.Drain(resp.Body)
		return nil, resp.StatusCode, ErrRateLimited
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		upstream.Drain(resp.Body)
		return nil, resp.StatusCode, &HTTPError{Status: resp.StatusCode, URL: rawURL}
	}
	body, err := upstream.ReadBody(resp.Body, maxPage)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return body, resp.StatusCode, nil
}

type pageDoc struct {
	Props struct {
		PageProps struct {
			Podcast podcastDoc `json:"podcast"`
		} `json:"pageProps"`
	} `json:"props"`
}

type podcastDoc struct {
	Title       string       `json:"title"`
	Author      string       `json:"author"`
	Description string       `json:"description"`
	Brief       string       `json:"brief"`
	Image       imageDoc     `json:"image"`
	Episodes    []episodeDoc `json:"episodes"`
}

type episodeDoc struct {
	ID          string    `json:"eid"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Shownotes   string    `json:"shownotes"`
	PubDate     string    `json:"pubDate"`
	Duration    int       `json:"duration"`
	PlayCount   int64     `json:"playCount"`
	Image       *imageDoc `json:"image"`
	Enclosure   struct {
		URL string `json:"url"`
	} `json:"enclosure"`
}

type imageDoc struct {
	PicURL    string `json:"picUrl"`
	Large     string `json:"largePicUrl"`
	Middle    string `json:"middlePicUrl"`
	Small     string `json:"smallPicUrl"`
	Thumbnail string `json:"thumbnailUrl"`
}

func (i imageDoc) URL() string {
	return firstNonEmpty(i.Large, i.Middle, i.PicURL, i.Small, i.Thumbnail)
}

func parsePage(body []byte, limit int) (*Show, error) {
	raw := string(body)
	i := strings.Index(raw, nextData)
	if i < 0 {
		return nil, ErrUnexpected
	}
	raw = raw[i+len(nextData):]
	j := strings.Index(raw, "</script>")
	if j < 0 {
		return nil, ErrUnexpected
	}
	var doc pageDoc
	if err := json.Unmarshal([]byte(raw[:j]), &doc); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnexpected, err)
	}
	pod := doc.Props.PageProps.Podcast
	show := &Show{
		Title:       strings.TrimSpace(pod.Title),
		Author:      strings.TrimSpace(pod.Author),
		Description: firstNonEmpty(pod.Description, pod.Brief),
		Image:       pod.Image.URL(),
	}
	episodes := make([]Episode, 0, len(pod.Episodes))
	for _, item := range pod.Episodes {
		if strings.TrimSpace(item.ID) == "" && strings.TrimSpace(item.Title) == "" {
			continue
		}
		ep := Episode{
			ID:        item.ID,
			Title:     strings.TrimSpace(item.Title),
			Summary:   firstNonEmpty(item.Shownotes, item.Description),
			AudioURL:  strings.TrimSpace(item.Enclosure.URL),
			Published: parseTime(item.PubDate),
			Plays:     item.PlayCount,
		}
		if item.Duration > 0 {
			ep.Duration = time.Duration(item.Duration) * time.Second
		}
		if item.Image != nil {
			ep.Image = item.Image.URL()
		}
		episodes = append(episodes, ep)
	}
	sort.SliceStable(episodes, func(a, b int) bool {
		if episodes[a].Published.IsZero() {
			return false
		}
		if episodes[b].Published.IsZero() {
			return true
		}
		return episodes[a].Published.After(episodes[b].Published)
	})
	if limit > 0 && len(episodes) > limit {
		episodes = episodes[:limit]
	}
	show.Episodes = episodes
	return show, nil
}

func parseTime(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC()
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
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
