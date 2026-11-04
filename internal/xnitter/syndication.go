// Syndication enrichment.
//
// Nitter's RSS carries a video only as a poster image and never carries
// engagement counts or playable media. X's public embed endpoint
// (cdn.syndication.twimg.com) answers per post with the downloadable MP4
// variants, duration, view count, likes and the quoted post, without an
// account or an API key. It is an undocumented endpoint, so every call is
// best-effort: a failure leaves the RSS-derived item untouched.
package xnitter

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// SyndicationURL is X's public embed endpoint.
	SyndicationURL = "https://cdn.syndication.twimg.com/tweet-result"
	// enrichConcurrency keeps a 20-item page from opening 20 sockets at once.
	enrichConcurrency = 4
	// enrichTimeout bounds one embed read.
	enrichTimeout = 6 * time.Second
	// enrichBudget bounds the whole enrichment pass of one timeline page.
	enrichBudget = 15 * time.Second
	// syndicationToken is not validated by the endpoint (a single character is
	// accepted), but an empty value returns an empty document, so send one.
	syndicationToken = "a"
)

type syndicationDoc struct {
	IDStr             string             `json:"id_str"`
	Text              string             `json:"text"`
	FavoriteCount     int                `json:"favorite_count"`
	ConversationCount int                `json:"conversation_count"`
	User              syndicationUser    `json:"user"`
	Video             *syndicationVideo  `json:"video"`
	Photos            []syndicationPhoto `json:"photos"`
	Quoted            *syndicationDoc    `json:"quoted_tweet"`
}

type syndicationUser struct {
	IDStr      string `json:"id_str"`
	ScreenName string `json:"screen_name"`
	Name       string `json:"name"`
}

type syndicationVideo struct {
	DurationMS int                  `json:"durationMs"`
	ViewCount  int64                `json:"viewCount"`
	Poster     string               `json:"poster"`
	Variants   []syndicationVariant `json:"variants"`
}

type syndicationVariant struct {
	Type string `json:"type"`
	Src  string `json:"src"`
}

type syndicationPhoto struct {
	URL          string `json:"url"`
	OriginalInfo struct {
		Width  int `json:"width"`
		Height int `json:"height"`
	} `json:"original_info"`
}

var variantSizeRe = regexp.MustCompile(`/(\d+)x(\d+)/`)

// enrich fills playable media and engagement counts for the items that need
// them. Failures are per item and never change the post itself.
func (c *Client) enrich(ctx context.Context, handle string, tweets []Tweet) {
	if c.disableEnrich || c.syndicationURL == "" || len(tweets) == 0 {
		return
	}
	budgetCtx, cancel := context.WithTimeout(ctx, enrichBudget)
	defer cancel()

	sem := make(chan struct{}, enrichConcurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex
	failed := 0
	for i := range tweets {
		wg.Add(1)
		go func(t *Tweet) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-budgetCtx.Done():
				mu.Lock()
				failed++
				mu.Unlock()
				return
			}
			doc, err := c.fetchSyndication(budgetCtx, t.ID)
			if err != nil {
				mu.Lock()
				failed++
				mu.Unlock()
				c.log.Debug("x enrichment failed", "tweet", t.ID, "error", err.Error())
				return
			}
			applySyndication(t, doc)
		}(&tweets[i])
	}
	wg.Wait()
	// A silent total failure would look like "this account has no videos and
	// no engagement", so report it once per page.
	if failed == len(tweets) {
		c.log.Warn("x enrichment failed for every item", "handle", handle, "items", len(tweets))
	}
}

func (c *Client) fetchSyndication(ctx context.Context, id string) (*syndicationDoc, error) {
	u, err := url.Parse(c.syndicationURL)
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("id", id)
	q.Set("lang", "en")
	q.Set("token", syndicationToken)
	u.RawQuery = q.Encode()

	reqCtx, cancel := context.WithTimeout(ctx, enrichTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, u.String(), nil)
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
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var doc syndicationDoc
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("embed parse: %w", err)
	}
	if doc.IDStr == "" {
		return nil, fmt.Errorf("embed returned no post")
	}
	return &doc, nil
}

// applySyndication merges the embed document into an RSS-derived tweet. Values
// already read from the RSS are kept unless the embed has a better one.
func applySyndication(t *Tweet, doc *syndicationDoc) {
	if doc.User.IDStr != "" {
		t.AuthorID = doc.User.IDStr
	}
	t.Likes = doc.FavoriteCount
	t.Replies = doc.ConversationCount
	if v := doc.Video; v != nil {
		if t.Video == nil {
			t.Video = &Video{}
		}
		if src := bestMP4(v.Variants); src != "" {
			t.Video.URL = src
		}
		if poster := upgradeMediaURL(v.Poster); poster != "" {
			t.Video.Thumbnail = poster
		}
		t.Video.DurationMS = v.DurationMS
		t.Video.Views = v.ViewCount
	}
	if q := doc.Quoted; q != nil {
		if t.Quoted == nil {
			t.Quoted = &Quoted{}
		}
		qt := t.Quoted
		if qt.ID == "" {
			qt.ID = q.IDStr
		}
		if qt.Author == "" {
			qt.Author = q.User.ScreenName
		}
		if qt.AuthorName == "" {
			qt.AuthorName = q.User.Name
		}
		if qt.Text == "" {
			qt.Text = nonLinkText(q.Text)
		}
		if qt.URL == "" && qt.ID != "" {
			if qt.Author != "" {
				qt.URL = "https://x.com/" + qt.Author + "/status/" + qt.ID
			} else {
				qt.URL = "https://x.com/i/status/" + qt.ID
			}
		}
		for _, p := range q.Photos {
			if u := upgradeMediaURL(p.URL); u != "" {
				qt.Images = appendUnique(qt.Images, u)
			}
		}
		if qv := q.Video; qv != nil {
			if qt.Video == nil {
				qt.Video = &Video{}
			}
			if src := bestMP4(qv.Variants); src != "" {
				qt.Video.URL = src
			}
			if poster := upgradeMediaURL(qv.Poster); poster != "" {
				qt.Video.Thumbnail = poster
			}
			qt.Video.DurationMS = qv.DurationMS
			qt.Video.Views = qv.ViewCount
		}
	}
}

// nonLinkText keeps embed text only when it says something: a media-only post
// carries nothing but the t.co link to its own media.
func nonLinkText(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	for _, field := range strings.Fields(text) {
		if !strings.HasPrefix(field, "http://") && !strings.HasPrefix(field, "https://") {
			return text
		}
	}
	return ""
}

// bestMP4 picks the highest-resolution MP4 variant; HLS playlists are ignored
// because most readers cannot play them from an RSS enclosure.
func bestMP4(variants []syndicationVariant) string {
	best := ""
	bestArea := -1
	for _, v := range variants {
		if !strings.EqualFold(v.Type, "video/mp4") || v.Src == "" {
			continue
		}
		area := 0
		if m := variantSizeRe.FindStringSubmatch(v.Src); m != nil {
			w, _ := strconv.Atoi(m[1])
			h, _ := strconv.Atoi(m[2])
			area = w * h
		}
		if area >= bestArea {
			best, bestArea = v.Src, area
		}
	}
	return best
}

func appendUnique(list []string, item string) []string {
	for _, existing := range list {
		if existing == item {
			return list
		}
	}
	return append(list, item)
}
