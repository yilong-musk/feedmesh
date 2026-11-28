// Package apple reads Apple Podcasts without an account or API key.
//
// A show is not hosted by Apple. The public iTunes Lookup API maps a show id
// to the publisher's RSS feed, and this package parses that feed. The top
// chart comes from Apple's public marketing-tools JSON.
package apple

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
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
	// DefaultLookupURL is the public iTunes Lookup host.
	DefaultLookupURL = "https://itunes.apple.com"
	// DefaultChartURL is the public top-shows chart. The older
	// rss.applemarketingtools.com name only redirects here.
	DefaultChartURL = "https://rss.marketingtools.apple.com"

	// DefaultLimit is the number of episodes or chart rows when the caller is silent.
	DefaultLimit = 20
	// MaxLimit is the widest list this provider returns. The chart endpoint
	// publishes 50 rows, and a show feed is trimmed to the same width after
	// the newest entries are selected.
	MaxLimit = 50

	maxScan      = 300
	maxLookup    = 1 << 20
	maxChart     = 2 << 20
	maxFeedBytes = 8 << 20
	chartWidth   = 50

	userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"
)

// Errors callers map onto HTTP responses.
var (
	// ErrInvalidID means the reference is not an Apple show id or show URL.
	ErrInvalidID = errors.New("apple: invalid show id")
	// ErrInvalidCountry means the chart storefront is not a two-letter code.
	ErrInvalidCountry = errors.New("apple: invalid country")
	// ErrNotFound means Lookup has no show for that id.
	ErrNotFound = errors.New("apple: show not found")
	// ErrNoFeed means the show exists but Apple has no publisher feed for it.
	ErrNoFeed = errors.New("apple: show has no rss feed")
	// ErrNoEpisodes means the publisher feed parsed but listed no episodes.
	ErrNoEpisodes = errors.New("apple: show has no episodes")
	// ErrRateLimited means Apple asked us to slow down.
	ErrRateLimited = errors.New("apple: rate limited")
	// ErrUnexpected means the body was not the document we asked for.
	ErrUnexpected = errors.New("apple: unexpected response")
)

var (
	digits    = regexp.MustCompile(`^[0-9]{4,12}$`)
	idInURL   = regexp.MustCompile(`(?i)(?:^|/)id([0-9]{4,12})(?:[/?#.]|$)`)
	countryRe = regexp.MustCompile(`^[a-z]{2}$`)
)

// HTTPError is a non-2xx answer from Apple or from a publisher feed.
type HTTPError struct {
	Status int
	URL    string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("apple: %s answered %d", e.URL, e.Status)
}

// Episode is one item from a publisher feed.
type Episode struct {
	ID              string
	Title           string
	Summary         string
	URL             string
	EnclosureURL    string
	EnclosureType   string
	EnclosureLength int64
	Image           string
	Published       time.Time
	Duration        time.Duration
	Author          string

	// Show fields are copied onto every episode so a cached list still knows
	// which show it came from.
	ShowTitle       string
	ShowAuthor      string
	ShowDescription string
	ShowImage       string
	ShowURL         string
	Language        string
	FeedURL         string
}

// Show is one podcast and its newest episodes.
type Show struct {
	ID          string
	Title       string
	Author      string
	Description string
	URL         string
	Image       string
	Language    string
	FeedURL     string
	Episodes    []Episode
}

// ChartEntry is one row of a top-shows chart.
type ChartEntry struct {
	ID     string
	Name   string
	Artist string
	URL    string
	Image  string
	Genre  string
}

// Chart is one storefront's top-shows list.
type Chart struct {
	Country string
	Title   string
	Entries []ChartEntry
}

// Config tunes a Client. Tests point the base URLs at an httptest server and
// set AllowPrivate so that server's loopback address is accepted as a
// publisher feed.
type Config struct {
	LookupURL    string
	ChartURL     string
	Timeout      time.Duration
	AllowPrivate bool
}

// Client reads Apple Podcasts over HTTP.
type Client struct {
	lookup       string
	chart        string
	allowPrivate bool
	http         *http.Client
	budget       time.Duration
}

// New builds a Client with production defaults.
func New(cfg Config) *Client {
	if cfg.LookupURL == "" {
		cfg.LookupURL = DefaultLookupURL
	}
	if cfg.ChartURL == "" {
		cfg.ChartURL = DefaultChartURL
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 20 * time.Second
	}
	c := &Client{
		lookup:       strings.TrimRight(cfg.LookupURL, "/"),
		chart:        strings.TrimRight(cfg.ChartURL, "/"),
		allowPrivate: cfg.AllowPrivate,
		budget:       cfg.Timeout,
	}
	c.http = &http.Client{
		Timeout:       cfg.Timeout,
		CheckRedirect: c.checkRedirect,
	}
	return c
}

// ParseID accepts a numeric Apple show id or a podcasts.apple.com / iTunes URL
// that contains one (/id123 or a bare id123).
func ParseID(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	s = strings.TrimSuffix(s, ".xml")
	if digits.MatchString(s) {
		return s, nil
	}
	if m := idInURL.FindStringSubmatch(s); m != nil {
		return m[1], nil
	}
	return "", ErrInvalidID
}

// ParseCountry accepts a two-letter storefront. Empty means the US chart.
func ParseCountry(raw string) (string, error) {
	s := strings.ToLower(strings.TrimSpace(raw))
	if s == "" {
		return "us", nil
	}
	if !countryRe.MatchString(s) {
		return "", ErrInvalidCountry
	}
	return s, nil
}

// Episodes resolves id and returns the newest episodes from the publisher feed.
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
	ctx, cancel := context.WithTimeout(ctx, c.budget)
	defer cancel()

	body, _, err := c.get(ctx, c.lookup+"/lookup?id="+url.QueryEscape(id)+"&entity=podcast", maxLookup)
	if err != nil {
		return nil, err
	}
	var lookup lookupResponse
	if err := json.Unmarshal(body, &lookup); err != nil {
		return nil, fmt.Errorf("%w: lookup", ErrUnexpected)
	}
	if lookup.ResultCount < 1 || len(lookup.Results) == 0 {
		return nil, ErrNotFound
	}
	row := lookup.Results[0]
	feedURL := strings.TrimSpace(row.FeedURL)
	if feedURL == "" {
		return nil, ErrNoFeed
	}
	if err := c.publicFeed(feedURL); err != nil {
		return nil, err
	}
	feedBody, _, err := c.get(ctx, feedURL, maxFeedBytes)
	if err != nil {
		return nil, err
	}
	show, err := parseFeed(strings.NewReader(string(feedBody)), limit)
	if err != nil {
		return nil, err
	}
	if len(show.Episodes) == 0 {
		return nil, ErrNoEpisodes
	}
	show.ID = id
	if show.Title == "" {
		show.Title = firstNonEmpty(row.CollectionName, row.TrackName, id)
	}
	if show.Author == "" {
		show.Author = row.ArtistName
	}
	if show.Image == "" {
		show.Image = firstNonEmpty(row.Artwork600, row.Artwork100)
	}
	show.URL = firstNonEmpty(row.PageURL, "https://podcasts.apple.com/podcast/id"+id)
	show.FeedURL = feedURL
	for i := range show.Episodes {
		ep := &show.Episodes[i]
		ep.ShowTitle = show.Title
		ep.ShowAuthor = show.Author
		ep.ShowDescription = show.Description
		ep.ShowImage = show.Image
		ep.ShowURL = show.URL
		ep.Language = show.Language
		ep.FeedURL = show.FeedURL
		if ep.Author == "" {
			ep.Author = show.Author
		}
		if ep.Image == "" {
			ep.Image = show.Image
		}
	}
	return show, nil
}

// Chart returns one storefront's top shows. country is a two-letter code;
// empty selects the US chart.
func (c *Client) Chart(ctx context.Context, country string, limit int) (*Chart, error) {
	country, err := ParseCountry(country)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = DefaultLimit
	}
	if limit > MaxLimit {
		limit = MaxLimit
	}
	ctx, cancel := context.WithTimeout(ctx, c.budget)
	defer cancel()

	raw := c.chart + "/api/v2/" + country + "/podcasts/top/" + strconv.Itoa(chartWidth) + "/podcasts.json"
	body, status, err := c.get(ctx, raw, maxChart)
	if err != nil {
		if status == http.StatusNotFound {
			return nil, ErrNotFound
		}
		return nil, err
	}
	var parsed chartResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("%w: chart", ErrUnexpected)
	}
	out := &Chart{Country: country, Title: parsed.Feed.Title}
	if out.Title == "" {
		out.Title = "Top podcasts"
	}
	for _, row := range parsed.Feed.Results {
		if strings.TrimSpace(row.ID) == "" {
			continue
		}
		genre := ""
		if len(row.Genres) > 0 {
			genre = row.Genres[0].Name
		}
		out.Entries = append(out.Entries, ChartEntry{
			ID:     row.ID,
			Name:   row.Name,
			Artist: row.ArtistName,
			URL:    row.URL,
			Image:  row.Artwork,
			Genre:  genre,
		})
		if len(out.Entries) >= limit {
			break
		}
	}
	if len(out.Entries) == 0 {
		return nil, ErrNotFound
	}
	return out, nil
}

func (c *Client) get(ctx context.Context, rawURL string, limit int64) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "*/*")
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
	body, err := upstream.ReadBody(resp.Body, limit)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return body, resp.StatusCode, nil
}

func (c *Client) checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 5 {
		return errors.New("apple: too many redirects")
	}
	if c.allowPrivate {
		return nil
	}
	return c.publicURL(req.URL)
}

func (c *Client) publicFeed(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("%w: feed url", ErrUnexpected)
	}
	return c.publicURL(u)
}

func (c *Client) publicURL(u *url.URL) error {
	if u == nil || u.Hostname() == "" {
		return fmt.Errorf("%w: feed url", ErrUnexpected)
	}
	if u.User != nil {
		return fmt.Errorf("%w: feed url", ErrUnexpected)
	}
	if u.Scheme != "https" && !(c.allowPrivate && u.Scheme == "http") {
		return fmt.Errorf("%w: feed url", ErrUnexpected)
	}
	host := u.Hostname()
	if ip := net.ParseIP(host); ip != nil {
		if c.blocked(ip) {
			return fmt.Errorf("%w: feed url", ErrUnexpected)
		}
		return nil
	}
	if c.allowPrivate {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return err
	}
	if len(ips) == 0 {
		return fmt.Errorf("%w: feed url", ErrUnexpected)
	}
	for _, ip := range ips {
		if c.blocked(ip.IP) {
			return fmt.Errorf("%w: feed url", ErrUnexpected)
		}
	}
	return nil
}

func (c *Client) blocked(ip net.IP) bool {
	if c.allowPrivate {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast()
}

type lookupResponse struct {
	ResultCount int `json:"resultCount"`
	Results     []struct {
		CollectionName string `json:"collectionName"`
		TrackName      string `json:"trackName"`
		ArtistName     string `json:"artistName"`
		FeedURL        string `json:"feedUrl"`
		PageURL        string `json:"trackViewUrl"`
		Artwork600     string `json:"artworkUrl600"`
		Artwork100     string `json:"artworkUrl100"`
	} `json:"results"`
}

type chartResponse struct {
	Feed struct {
		Title   string `json:"title"`
		Results []struct {
			ID         string `json:"id"`
			Name       string `json:"name"`
			ArtistName string `json:"artistName"`
			URL        string `json:"url"`
			Artwork    string `json:"artworkUrl100"`
			Genres     []struct {
				Name string `json:"name"`
			} `json:"genres"`
		} `json:"results"`
	} `json:"feed"`
}

func parseFeed(r io.Reader, limit int) (*Show, error) {
	dec := xml.NewDecoder(io.LimitReader(r, maxFeedBytes))
	dec.CharsetReader = func(string, io.Reader) (io.Reader, error) {
		return nil, fmt.Errorf("%w: non-utf8 feed", ErrUnexpected)
	}
	show := &Show{}
	var episodes []Episode
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			if len(episodes) > 0 {
				break
			}
			return nil, fmt.Errorf("%w: %v", ErrUnexpected, err)
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		switch se.Name.Local {
		case "rss", "channel":
			continue
		case "item":
			ep, err := parseItem(dec)
			if err != nil {
				if len(episodes) > 0 {
					goto done
				}
				return nil, err
			}
			episodes = append(episodes, ep)
			if len(episodes) >= maxScan {
				goto done
			}
		case "title":
			text, err := readText(dec, se)
			if err != nil {
				return nil, err
			}
			if show.Title == "" {
				show.Title = text
			}
		case "description", "summary":
			text, err := readText(dec, se)
			if err != nil {
				return nil, err
			}
			if show.Description == "" {
				show.Description = text
			}
		case "author":
			text, err := readText(dec, se)
			if err != nil {
				return nil, err
			}
			if show.Author == "" {
				show.Author = text
			}
		case "language":
			text, err := readText(dec, se)
			if err != nil {
				return nil, err
			}
			if show.Language == "" {
				show.Language = text
			}
		case "image":
			if href := attr(se, "href"); href != "" {
				show.Image = href
				if err := dec.Skip(); err != nil {
					return nil, err
				}
				continue
			}
			inner, err := imageURL(dec)
			if err != nil {
				return nil, err
			}
			if show.Image == "" {
				show.Image = inner
			}
		default:
			if err := dec.Skip(); err != nil {
				return nil, err
			}
		}
	}
done:
	if len(episodes) == 0 {
		return show, nil
	}
	sort.SliceStable(episodes, func(i, j int) bool {
		if episodes[i].Published.IsZero() {
			return false
		}
		if episodes[j].Published.IsZero() {
			return true
		}
		return episodes[i].Published.After(episodes[j].Published)
	})
	if limit > 0 && len(episodes) > limit {
		episodes = episodes[:limit]
	}
	show.Episodes = episodes
	return show, nil
}

func parseItem(dec *xml.Decoder) (Episode, error) {
	var ep Episode
	for {
		tok, err := dec.Token()
		if err != nil {
			return Episode{}, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "title":
				text, err := readText(dec, t)
				if err != nil {
					return Episode{}, err
				}
				if ep.Title == "" {
					ep.Title = text
				}
			case "description", "summary", "encoded":
				text, err := readText(dec, t)
				if err != nil {
					return Episode{}, err
				}
				if t.Name.Local == "encoded" || ep.Summary == "" {
					ep.Summary = text
				}
			case "link":
				text, err := readText(dec, t)
				if err != nil {
					return Episode{}, err
				}
				if ep.URL == "" {
					ep.URL = text
				}
			case "guid":
				text, err := readText(dec, t)
				if err != nil {
					return Episode{}, err
				}
				if ep.ID == "" {
					ep.ID = text
				}
			case "pubDate":
				text, err := readText(dec, t)
				if err != nil {
					return Episode{}, err
				}
				ep.Published = parseTime(text)
			case "duration":
				text, err := readText(dec, t)
				if err != nil {
					return Episode{}, err
				}
				if ep.Duration == 0 {
					ep.Duration = parseDuration(text)
				}
			case "author", "creator":
				text, err := readText(dec, t)
				if err != nil {
					return Episode{}, err
				}
				if ep.Author == "" {
					ep.Author = text
				}
			case "enclosure":
				ep.EnclosureURL = attr(t, "url")
				ep.EnclosureType = attr(t, "type")
				ep.EnclosureLength, _ = strconv.ParseInt(attr(t, "length"), 10, 64)
				if err := dec.Skip(); err != nil {
					return Episode{}, err
				}
			case "image":
				if href := attr(t, "href"); href != "" {
					ep.Image = href
				}
				if err := dec.Skip(); err != nil {
					return Episode{}, err
				}
			default:
				if err := dec.Skip(); err != nil {
					return Episode{}, err
				}
			}
		case xml.EndElement:
			if t.Name.Local == "item" {
				if ep.ID == "" {
					ep.ID = firstNonEmpty(ep.EnclosureURL, ep.URL, ep.Title)
				}
				return ep, nil
			}
		}
	}
}

func readText(dec *xml.Decoder, start xml.StartElement) (string, error) {
	var b strings.Builder
	for {
		tok, err := dec.Token()
		if err != nil {
			return "", err
		}
		switch t := tok.(type) {
		case xml.CharData:
			b.Write(t)
		case xml.StartElement:
			if err := dec.Skip(); err != nil {
				return "", err
			}
		case xml.EndElement:
			if t.Name.Local == start.Name.Local {
				return strings.TrimSpace(b.String()), nil
			}
		}
	}
}

func imageURL(dec *xml.Decoder) (string, error) {
	var found string
	for {
		tok, err := dec.Token()
		if err != nil {
			return "", err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "url" && found == "" {
				found, err = readText(dec, t)
				if err != nil {
					return "", err
				}
				continue
			}
			if err := dec.Skip(); err != nil {
				return "", err
			}
		case xml.EndElement:
			if t.Name.Local == "image" {
				return strings.TrimSpace(found), nil
			}
		}
	}
}

func attr(se xml.StartElement, name string) string {
	for _, a := range se.Attr {
		if a.Name.Local == name {
			return strings.TrimSpace(a.Value)
		}
	}
	return ""
}

func parseTime(s string) time.Time {
	s = strings.TrimSpace(s)
	for _, layout := range []string{time.RFC1123Z, time.RFC1123, time.RFC3339, "Mon, 2 Jan 2006 15:04:05 -0700", "Mon, 2 Jan 2006 15:04:05 MST"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

func parseDuration(s string) time.Duration {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	if n, err := strconv.Atoi(s); err == nil && n >= 0 {
		return time.Duration(n) * time.Second
	}
	parts := strings.Split(s, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0
	}
	nums := make([]int, len(parts))
	for i, p := range parts {
		n, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil || n < 0 {
			return 0
		}
		nums[i] = n
	}
	var sec int
	switch len(nums) {
	case 2:
		sec = nums[0]*60 + nums[1]
	case 3:
		sec = nums[0]*3600 + nums[1]*60 + nums[2]
	}
	return time.Duration(sec) * time.Second
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
