package feed

import (
	"context"
	"errors"
	"html"
	"path"
	"strings"
	"time"

	"github.com/yilong-musk/feedmesh/internal/apperr"
	"github.com/yilong-musk/feedmesh/internal/cachekey"
	"github.com/yilong-musk/feedmesh/internal/xiaoyuzhou"
)

// Xiaoyuzhou feed bounds. The show page lists about fifteen episodes and
// does not page, so the cache only has to cover a reader poll.
const (
	DefaultXiaoyuzhouLimit = xiaoyuzhou.DefaultLimit
	MaxXiaoyuzhouLimit     = xiaoyuzhou.MaxLimit

	xiaoyuzhouTTL = 30 * time.Minute
)

// XiaoyuzhouTTL is the cache lifetime of one show.
func XiaoyuzhouTTL() time.Duration { return xiaoyuzhouTTL }

// XiaoyuzhouExtra is the provider-specific item metadata for one episode.
type XiaoyuzhouExtra struct {
	Duration int   `json:"duration,omitempty"`
	Views    int64 `json:"views,omitempty"`
}

// GetXiaoyuzhou builds an episode feed for one Xiaoyuzhou show. id is the
// show id from a /podcast/ URL.
func (s *Service) GetXiaoyuzhou(ctx context.Context, id string, limit int, refresh bool) (*Feed, error) {
	if s.clients.Xiaoyuzhou == nil {
		return nil, &apperr.Error{Code: apperr.CodeInternal, Message: "xiaoyuzhou provider is not configured"}
	}
	parsed, err := xiaoyuzhou.ParseID(id)
	if err != nil {
		return nil, &apperr.Error{Code: apperr.CodeBadRequest, Message: "invalid xiaoyuzhou show id " + id, Err: err}
	}
	if limit <= 0 {
		limit = DefaultXiaoyuzhouLimit
	}
	if limit > MaxXiaoyuzhouLimit {
		limit = MaxXiaoyuzhouLimit
	}
	key := cachekey.Xiaoyuzhou.Key("show", parsed)
	entry, cached, degraded, err := fetchCachedList(ctx, s, key, xiaoyuzhouTTL, refresh, limit,
		func(ctx context.Context, n int) ([]xiaoyuzhou.Episode, error) {
			show, cerr := s.clients.Xiaoyuzhou.Episodes(ctx, parsed, n)
			if cerr != nil {
				return nil, cerr
			}
			if len(show.Episodes) == 0 {
				return nil, xiaoyuzhou.ErrNoEpisodes
			}
			return show.Episodes, nil
		})
	if err != nil {
		return nil, xiaoyuzhouError(err, parsed)
	}
	return xiaoyuzhouFeed(parsed, entry, limit, cached, degraded), nil
}

func xiaoyuzhouFeed(id string, c cachedList[xiaoyuzhou.Episode], limit int, cached, degraded bool) *Feed {
	title, author, description, page, image := id, "", "", "", ""
	if len(c.Items) > 0 {
		ep := c.Items[0]
		title = firstNonEmpty(ep.ShowTitle, id)
		author = ep.ShowAuthor
		description = ep.ShowDescription
		page = ep.ShowURL
		image = ep.ShowImage
	}
	if description == "" {
		description = "Xiaoyuzhou show " + title
	}
	if page == "" {
		page = "https://www.xiaoyuzhoufm.com/podcast/" + id
	}
	n := len(c.Items)
	if limit < n {
		n = limit
	}
	items := make([]Item, 0, n)
	for _, ep := range c.Items[:n] {
		plain := HTMLToText(ep.Summary)
		link := safeURLString(ep.URL)
		if link == "" {
			link = page
		}
		item := Item{
			ID:        ep.ID,
			CreatedAt: ep.Published.UTC(),
			Text:      plain,
			HTML:      xiaoyuzhouHTML(ep, plain),
			URL:       link,
			Title:     firstNonEmpty(ep.Title, "Episode"),
			Author:    author,
			Images:    []string{},
		}
		if img := safeURLString(firstNonEmpty(ep.Image, image)); img != "" {
			item.Images = append(item.Images, img)
		}
		if media := xiaoyuzhouMedia(ep); media != nil {
			item.Video = media
		}
		extra := &XiaoyuzhouExtra{}
		if ep.Duration > 0 {
			extra.Duration = int(ep.Duration / time.Second)
		}
		if ep.Plays > 0 {
			extra.Views = ep.Plays
		}
		if extra.Duration > 0 || extra.Views > 0 {
			item.Extra = extra
		}
		items = append(items, item)
	}
	return &Feed{
		UID:         id,
		ScreenName:  title,
		FetchedAt:   c.FetchedAt,
		TitleText:   title + " on Xiaoyuzhou",
		LinkText:    page,
		Cached:      cached,
		Degraded:    degraded,
		Items:       items,
		Description: HTMLToText(description),
		Language:    langZhCN,
	}
}

func xiaoyuzhouHTML(ep xiaoyuzhou.Episode, plain string) string {
	var b strings.Builder
	if h := NormalizeHTML(ep.Summary); h != "" {
		b.WriteString(h)
	}
	if img := safeURLString(firstNonEmpty(ep.Image, ep.ShowImage)); img != "" {
		writeImage(&b, img, "", "")
	}
	if audio := safeURLString(ep.AudioURL); audio != "" {
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

func xiaoyuzhouMedia(ep xiaoyuzhou.Episode) *Video {
	raw := safeURLString(ep.AudioURL)
	if raw == "" {
		return nil
	}
	mime := xiaoyuzhouMIME(raw)
	v := &Video{URL: raw, MIME: mime, Thumbnail: safeURLString(ep.Image)}
	if ep.Duration > 0 {
		v.DurationMS = int(ep.Duration / time.Millisecond)
	}
	return v
}

func xiaoyuzhouMIME(raw string) string {
	switch strings.ToLower(path.Ext(strings.Split(raw, "?")[0])) {
	case ".m4a", ".mp4", ".aac":
		return "audio/mp4"
	case ".ogg", ".opus":
		return "audio/ogg"
	default:
		return "audio/mpeg"
	}
}

func xiaoyuzhouError(err error, ref string) error {
	if err == nil {
		return nil
	}
	var we *apperr.Error
	if errors.As(err, &we) {
		return err
	}
	switch {
	case errors.Is(err, xiaoyuzhou.ErrInvalidID):
		return &apperr.Error{Code: apperr.CodeBadRequest, Message: "invalid xiaoyuzhou show id " + ref, Err: err}
	case errors.Is(err, xiaoyuzhou.ErrNotFound):
		return &apperr.Error{Code: apperr.CodeUserNotFound, Message: "xiaoyuzhou show " + ref + " not found", Err: err}
	case errors.Is(err, xiaoyuzhou.ErrRateLimited):
		return &apperr.Error{Code: apperr.CodeRateLimited, Message: "xiaoyuzhou rate limited", Err: err}
	case errors.Is(err, xiaoyuzhou.ErrNoEpisodes), errors.Is(err, xiaoyuzhou.ErrUnexpected):
		return &apperr.Error{Code: apperr.CodeUpstreamUnavailable, Message: "xiaoyuzhou show " + ref + " could not be read", Err: err}
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		return &apperr.Error{Code: apperr.CodeTimeout, Message: "xiaoyuzhou timed out", Err: err}
	default:
		return &apperr.Error{Code: apperr.CodeUpstreamUnavailable, Message: err.Error(), Err: err}
	}
}
