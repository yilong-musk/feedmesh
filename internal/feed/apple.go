package feed

import (
	"context"
	"errors"
	"html"
	"strings"
	"time"

	"github.com/yilong-musk/feedmesh/internal/apperr"
	"github.com/yilong-musk/feedmesh/internal/apple"
	"github.com/yilong-musk/feedmesh/internal/cachekey"
)

// Apple feed bounds. A publisher feed is the show's own RSS; thirty minutes
// is enough for a reader poll and keeps the anonymous Lookup calls well under
// Apple's published rate.
const (
	DefaultAppleLimit = apple.DefaultLimit
	MaxAppleLimit     = apple.MaxLimit

	appleTTL = 30 * time.Minute
)

// AppleTTL is the cache lifetime of one show or one chart.
func AppleTTL() time.Duration { return appleTTL }

// AppleExtra is the provider-specific item metadata for one episode.
type AppleExtra struct {
	Duration int `json:"duration,omitempty"`
}

// GetApple builds an episode feed for one Apple Podcasts show. id is a
// numeric show id or an Apple Podcasts URL.
func (s *Service) GetApple(ctx context.Context, id string, limit int, refresh bool) (*Feed, error) {
	if s.clients.Apple == nil {
		return nil, &apperr.Error{Code: apperr.CodeInternal, Message: "apple provider is not configured"}
	}
	parsed, err := apple.ParseID(id)
	if err != nil {
		return nil, &apperr.Error{Code: apperr.CodeBadRequest, Message: "invalid apple podcast id " + id, Err: err}
	}
	if limit <= 0 {
		limit = DefaultAppleLimit
	}
	if limit > MaxAppleLimit {
		limit = MaxAppleLimit
	}
	key := cachekey.Apple.Key("show", parsed)
	entry, cached, degraded, err := fetchCachedList(ctx, s, key, appleTTL, refresh, limit,
		func(ctx context.Context, n int) ([]apple.Episode, error) {
			show, cerr := s.clients.Apple.Episodes(ctx, parsed, n)
			if cerr != nil {
				return nil, cerr
			}
			if len(show.Episodes) == 0 {
				return nil, apple.ErrNoEpisodes
			}
			return show.Episodes, nil
		})
	if err != nil {
		return nil, appleError(err, parsed)
	}
	return appleShowFeed(parsed, entry, limit, cached, degraded), nil
}

// GetAppleChart builds a top-shows feed. country is a two-letter storefront;
// empty selects the US chart.
func (s *Service) GetAppleChart(ctx context.Context, country string, limit int, refresh bool) (*Feed, error) {
	if s.clients.Apple == nil {
		return nil, &apperr.Error{Code: apperr.CodeInternal, Message: "apple provider is not configured"}
	}
	parsed, err := apple.ParseCountry(country)
	if err != nil {
		return nil, &apperr.Error{Code: apperr.CodeBadRequest, Message: "invalid apple chart country " + country, Err: err}
	}
	if limit <= 0 {
		limit = DefaultAppleLimit
	}
	if limit > MaxAppleLimit {
		limit = MaxAppleLimit
	}
	key := cachekey.Apple.Key("chart", parsed)
	entry, cached, degraded, err := fetchCachedList(ctx, s, key, appleTTL, refresh, limit,
		func(ctx context.Context, n int) ([]apple.ChartEntry, error) {
			chart, cerr := s.clients.Apple.Chart(ctx, parsed, n)
			if cerr != nil {
				return nil, cerr
			}
			return chart.Entries, nil
		})
	if err != nil {
		return nil, appleError(err, parsed)
	}
	return appleChartFeed(parsed, entry, limit, cached, degraded), nil
}

func appleShowFeed(id string, c cachedList[apple.Episode], limit int, cached, degraded bool) *Feed {
	title, author, description, page, image, language := id, "", "", "", "", ""
	if len(c.Items) > 0 {
		ep := c.Items[0]
		title = firstNonEmpty(ep.ShowTitle, id)
		author = ep.ShowAuthor
		description = ep.ShowDescription
		page = ep.ShowURL
		image = ep.ShowImage
		language = ep.Language
	}
	if description == "" {
		description = "Apple Podcasts show " + title
	}
	if page == "" {
		page = "https://podcasts.apple.com/podcast/id" + id
	}
	n := len(c.Items)
	if limit < n {
		n = limit
	}
	items := make([]Item, 0, n)
	for _, ep := range c.Items[:n] {
		plain := HTMLToText(ep.Summary)
		item := Item{
			ID:        ep.ID,
			CreatedAt: ep.Published.UTC(),
			Text:      plain,
			HTML:      appleEpisodeHTML(ep, plain),
			URL:       firstNonEmpty(safeURLString(ep.URL), safeURLString(ep.EnclosureURL), page),
			Title:     firstNonEmpty(ep.Title, "Episode"),
			Author:    firstNonEmpty(ep.Author, author),
			Images:    []string{},
		}
		if img := safeURLString(firstNonEmpty(ep.Image, image)); img != "" {
			item.Images = append(item.Images, img)
		}
		if media := appleMedia(ep); media != nil {
			item.Video = media
		}
		if ep.Duration > 0 {
			item.Extra = &AppleExtra{Duration: int(ep.Duration / time.Second)}
		}
		items = append(items, item)
	}
	return &Feed{
		UID:         id,
		ScreenName:  title,
		FetchedAt:   c.FetchedAt,
		TitleText:   title + " on Apple Podcasts",
		LinkText:    page,
		Cached:      cached,
		Degraded:    degraded,
		Items:       items,
		Description: HTMLToText(description),
		Language:    language,
	}
}

func appleChartFeed(country string, c cachedList[apple.ChartEntry], limit int, cached, degraded bool) *Feed {
	n := len(c.Items)
	if limit < n {
		n = limit
	}
	items := make([]Item, 0, n)
	for _, row := range c.Items[:n] {
		link := safeURLString(row.URL)
		if link == "" {
			link = "https://podcasts.apple.com/podcast/id" + row.ID
		}
		text := row.Genre
		if text == "" {
			text = row.Artist
		}
		var b strings.Builder
		if row.Artist != "" {
			b.WriteString("<p>")
			b.WriteString(html.EscapeString(row.Artist))
			b.WriteString("</p>")
		}
		if img := safeURLString(row.Image); img != "" {
			writeImage(&b, img, "", "")
		}
		item := Item{
			ID:        row.ID,
			CreatedAt: c.FetchedAt,
			Text:      text,
			HTML:      b.String(),
			URL:       link,
			Title:     firstNonEmpty(row.Name, "Show"),
			Author:    row.Artist,
			Images:    []string{},
		}
		if img := safeURLString(row.Image); img != "" {
			item.Images = append(item.Images, img)
		}
		items = append(items, item)
	}
	return &Feed{
		UID:         country,
		ScreenName:  "Top podcasts",
		FetchedAt:   c.FetchedAt,
		TitleText:   "Top podcasts (" + strings.ToUpper(country) + ")",
		LinkText:    "https://podcasts.apple.com/" + country + "/charts",
		Cached:      cached,
		Degraded:    degraded,
		Items:       items,
		Description: "Apple Podcasts top shows in " + strings.ToUpper(country),
	}
}

func appleEpisodeHTML(ep apple.Episode, plain string) string {
	var b strings.Builder
	if h := NormalizeHTML(ep.Summary); h != "" {
		b.WriteString(h)
	}
	if img := safeURLString(firstNonEmpty(ep.Image, ep.ShowImage)); img != "" {
		writeImage(&b, img, "", "")
	}
	if audio := safeURLString(ep.EnclosureURL); audio != "" {
		b.WriteString(`<p><a href="`)
		b.WriteString(EscapeXML(audio))
		b.WriteString(`">Audio</a></p>`)
	}
	if b.Len() == 0 && plain != "" {
		b.WriteString("<p>")
		b.WriteString(html.EscapeString(plain))
		b.WriteString("</p>")
	}
	return b.String()
}

func appleMedia(ep apple.Episode) *Video {
	raw := safeURLString(ep.EnclosureURL)
	if raw == "" {
		return nil
	}
	mime := appleMIME(ep.EnclosureType, raw)
	if mime == "" {
		return nil
	}
	v := &Video{URL: raw, MIME: mime, Thumbnail: safeURLString(ep.Image)}
	if ep.EnclosureLength > 0 {
		v.Bytes = ep.EnclosureLength
	}
	if ep.Duration > 0 {
		v.DurationMS = int(ep.Duration / time.Millisecond)
	}
	return v
}

func appleMIME(typ, raw string) string {
	typ = strings.ToLower(strings.TrimSpace(typ))
	if i := strings.Index(typ, ";"); i >= 0 {
		typ = strings.TrimSpace(typ[:i])
	}
	if strings.HasPrefix(typ, "audio/") || strings.HasPrefix(typ, "video/") {
		return typ
	}
	u := strings.ToLower(raw)
	switch {
	case strings.Contains(u, ".mp3"):
		return "audio/mpeg"
	case strings.Contains(u, ".m4a"):
		return "audio/mp4"
	case strings.Contains(u, ".mp4"):
		return "video/mp4"
	default:
		return ""
	}
}

func appleError(err error, ref string) error {
	if err == nil {
		return nil
	}
	var we *apperr.Error
	if errors.As(err, &we) {
		return err
	}
	switch {
	case errors.Is(err, apple.ErrInvalidID):
		return &apperr.Error{Code: apperr.CodeBadRequest, Message: "invalid apple podcast id " + ref, Err: err}
	case errors.Is(err, apple.ErrInvalidCountry):
		return &apperr.Error{Code: apperr.CodeBadRequest, Message: "invalid apple chart country " + ref, Err: err}
	case errors.Is(err, apple.ErrNotFound):
		return &apperr.Error{Code: apperr.CodeUserNotFound, Message: "apple podcast " + ref + " not found", Err: err}
	case errors.Is(err, apple.ErrNoFeed):
		return &apperr.Error{Code: apperr.CodeUserNotFound, Message: "apple podcast " + ref + " has no rss feed", Err: err}
	case errors.Is(err, apple.ErrRateLimited):
		return &apperr.Error{Code: apperr.CodeRateLimited, Message: "apple podcasts rate limited", Err: err}
	case errors.Is(err, apple.ErrNoEpisodes), errors.Is(err, apple.ErrUnexpected):
		return &apperr.Error{Code: apperr.CodeUpstreamUnavailable, Message: "apple podcast " + ref + " could not be read", Err: err}
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		return &apperr.Error{Code: apperr.CodeTimeout, Message: "apple podcasts timed out", Err: err}
	default:
		return &apperr.Error{Code: apperr.CodeUpstreamUnavailable, Message: err.Error(), Err: err}
	}
}
