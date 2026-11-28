package feed

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/yilong-musk/feedmesh/internal/apperr"
	"github.com/yilong-musk/feedmesh/internal/cachekey"
	"github.com/yilong-musk/feedmesh/internal/trumpstruth"
)

// Trump's Truth feed bounds. The archive is one fixed timeline (Donald Trump),
// not an arbitrary Truth Social account. A reader wants the latest posts, so
// the list stays fresh for ten minutes.
const (
	// DefaultTrumpTruthLimit is the number of posts returned when the caller is silent.
	DefaultTrumpTruthLimit = trumpstruth.DefaultLimit
	// MaxTrumpTruthLimit is the widest answer this service will ask the archive for.
	MaxTrumpTruthLimit = trumpstruth.MaxLimit

	trumpstruthTTL = 10 * time.Minute
)

// TrumpTruthTTL is the cache lifetime of the archive feed.
func TrumpTruthTTL() time.Duration { return trumpstruthTTL }

// TrumpTruthFeed is the provider the archive feed needs; tests substitute
// their own implementation.
type TrumpTruthFeed interface {
	Latest(ctx context.Context, limit int) (*trumpstruth.Result, error)
}

// TrumpTruthExtra is the provider-specific item metadata for one archived post.
type TrumpTruthExtra struct {
	PostType    string `json:"post_type,omitempty"`
	Removed     bool   `json:"removed,omitempty"`
	OriginalURL string `json:"original_url,omitempty"`
}

// GetTrumpTruth builds the latest-posts feed from trumpstruth.org.
func (s *Service) GetTrumpTruth(ctx context.Context, limit int, refresh bool) (*Feed, error) {
	if s.clients.TrumpTruth == nil {
		return nil, &apperr.Error{Code: apperr.CodeInternal, Message: "trumpstruth provider is not configured"}
	}
	if limit <= 0 {
		limit = DefaultTrumpTruthLimit
	}
	if limit > MaxTrumpTruthLimit {
		limit = MaxTrumpTruthLimit
	}
	key := cachekey.TrumpTruth.Key("latest")
	// The provider always reads the full slice and the cache stores the whole
	// result, so a narrow request warms the same entry a wide one uses.
	res, cached, degraded, err := fetchCached(ctx, s, key, trumpstruthTTL, refresh,
		func(ctx context.Context) (*trumpstruth.Result, error) {
			return s.clients.TrumpTruth.Latest(ctx, trumpstruth.MaxLimit)
		})
	if err != nil {
		return nil, trumpTruthError(err)
	}
	if res == nil {
		return nil, &apperr.Error{Code: apperr.CodeInternal, Message: "trumpstruth provider returned no result"}
	}
	return trumpTruthFeed(res, limit, cached, degraded), nil
}

func trumpTruthFeed(res *trumpstruth.Result, limit int, cached, degraded bool) *Feed {
	title := strings.TrimSpace(res.Title)
	if title == "" {
		title = "Trump's Truth"
	}
	description := strings.TrimSpace(res.Description)
	if description == "" {
		description = "Archive of Donald Trump's Truth Social posts"
	}
	link := safeURLString(res.Link)
	if link == "" {
		link = "https://www.trumpstruth.org/"
	}
	n := len(res.Posts)
	if limit < n {
		n = limit
	}
	items := make([]Item, 0, n)
	for _, p := range res.Posts[:n] {
		permalink := safeURLString(p.URL)
		if permalink == "" {
			continue
		}
		text := strings.TrimSpace(p.Text)
		itemTitle := strings.TrimSpace(p.Title)
		if itemTitle == "" {
			itemTitle = text
		}
		if itemTitle == "" {
			itemTitle = "(no content)"
		}
		image := safeURLString(p.Image)
		images := []string{}
		if image != "" {
			images = append(images, image)
		}
		created := p.Published
		if created.IsZero() {
			created = res.FetchedAt
		} else {
			created = created.UTC()
		}
		id := p.ID
		if id == "" {
			id = permalink
		}
		original := safeURLString(p.OriginalURL)
		var extra any
		if p.PostType != "" || p.Removed || original != "" {
			extra = &TrumpTruthExtra{PostType: p.PostType, Removed: p.Removed, OriginalURL: original}
		}
		items = append(items, Item{
			ID:        id,
			Title:     itemTitle,
			Author:    strings.TrimSpace(p.Author),
			Text:      text,
			HTML:      trumpTruthHTML(permalink, image, text),
			URL:       permalink,
			Images:    images,
			CreatedAt: created,
			Extra:     extra,
		})
	}
	return &Feed{
		UID:         "latest",
		ScreenName:  "Donald J. Trump",
		Description: description,
		FetchedAt:   res.FetchedAt.UTC(),
		Cached:      cached,
		Degraded:    degraded,
		TitleText:   title,
		LinkText:    link,
		TTL:         int(trumpstruthTTL.Minutes()),
		Items:       items,
	}
}

func trumpTruthHTML(permalink, image, text string) string {
	var b strings.Builder
	if image != "" {
		writeImage(&b, image, "", "")
	}
	if text != "" {
		b.WriteString("<p>")
		b.WriteString(EscapeXML(text))
		b.WriteString("</p>")
	}
	b.WriteString(`<p><a href="`)
	b.WriteString(EscapeXML(permalink))
	b.WriteString(`">View on Trump's Truth</a></p>`)
	return b.String()
}

func trumpTruthError(err error) error {
	if err == nil {
		return nil
	}
	var we *apperr.Error
	if errors.As(err, &we) {
		return err
	}
	switch {
	case errors.Is(err, trumpstruth.ErrRateLimited):
		return &apperr.Error{Code: apperr.CodeRateLimited, Message: "trumpstruth rate limited", Err: err}
	case errors.Is(err, trumpstruth.ErrBlocked):
		return &apperr.Error{Code: apperr.CodeUpstreamBlocked, Message: "trumpstruth blocked the request", Err: err}
	case errors.Is(err, trumpstruth.ErrUnexpectedPage), errors.Is(err, trumpstruth.ErrBadFeed):
		return &apperr.Error{Code: apperr.CodeUpstreamUnavailable, Message: "trumpstruth feed could not be parsed", Err: err}
	case errors.Is(err, context.DeadlineExceeded):
		return &apperr.Error{Code: apperr.CodeTimeout, Message: "trumpstruth feed timed out", Err: err}
	case errors.Is(err, trumpstruth.ErrUpstream):
		return &apperr.Error{Code: apperr.CodeUpstreamUnavailable, Message: "trumpstruth feed is unavailable", Err: err}
	default:
		return &apperr.Error{Code: apperr.CodeNetwork, Message: err.Error(), Err: err}
	}
}
