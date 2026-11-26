package feed

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/yilong-musk/feedmesh/internal/apperr"
	"github.com/yilong-musk/feedmesh/internal/pinterest"
)

// Pinterest feed bounds and cache lifetime. One feed slice carries at most 25
// pins and has no pagination; polling more often than the reader needs only
// adds Pinterest traffic, so the list is fresh for ten minutes.
const (
	// DefaultPinterestLimit is the number of pins returned when the caller is silent.
	DefaultPinterestLimit = pinterest.DefaultLimit
	// MaxPinterestLimit is the widest answer: one feed slice.
	MaxPinterestLimit = pinterest.MaxLimit

	pinterestTTL = 10 * time.Minute
)

// PinterestTTL is the cache lifetime of one profile or board feed.
func PinterestTTL() time.Duration { return pinterestTTL }

// GetPinterest builds a profile feed (board empty) or a board feed from
// Pinterest's public RSS endpoints.
func (s *Service) GetPinterest(ctx context.Context, user, board string, limit int, refresh bool) (*Feed, error) {
	if s.clients.Pinterest == nil {
		return nil, &apperr.Error{Code: apperr.CodeInternal, Message: "pinterest provider is not configured"}
	}
	user, err := pinterest.NormalizeUser(user)
	if err != nil {
		return nil, &apperr.Error{Code: apperr.CodeBadRequest, Message: "invalid pinterest user " + strconv.Quote(strings.TrimSpace(user)), Err: err}
	}
	if board != "" {
		raw := board
		board, err = pinterest.NormalizeBoard(board)
		if err != nil {
			return nil, &apperr.Error{Code: apperr.CodeBadRequest, Message: "invalid pinterest board " + strconv.Quote(strings.TrimSpace(raw)), Err: err}
		}
	}
	if limit <= 0 {
		limit = DefaultPinterestLimit
	}
	if limit > MaxPinterestLimit {
		limit = MaxPinterestLimit
	}

	key := "pinterest:user:" + strings.ToLower(user)
	if board != "" {
		key = "pinterest:board:" + strings.ToLower(user) + "/" + strings.ToLower(board)
	}
	// The provider always reads the full 25-pin slice and the cache stores the
	// whole result, so a narrow request warms the same entry a wide one uses
	// and the channel metadata survives a cache hit.
	res, cached, degraded, err := fetchCached(ctx, s, key, pinterestTTL, refresh,
		func(ctx context.Context) (*pinterest.Result, error) {
			return s.clients.Pinterest.Fetch(ctx, user, board, pinterest.MaxLimit)
		})
	if err != nil {
		return nil, pinterestError(err, user, board)
	}
	if res == nil {
		return nil, &apperr.Error{Code: apperr.CodeInternal, Message: "pinterest provider returned no result"}
	}
	return pinterestFeed(res, limit, cached, degraded), nil
}

func pinterestFeed(res *pinterest.Result, limit int, cached, degraded bool) *Feed {
	title := strings.TrimSpace(res.Title)
	screen := title
	if screen == "" {
		screen = res.User
	}
	description := strings.TrimSpace(res.Description)
	if description == "" {
		if res.Board != "" {
			description = "Pinterest board " + res.User + "/" + res.Board
		} else {
			description = "Pinterest user @" + res.User
		}
	}

	n := len(res.Pins)
	if limit < n {
		n = limit
	}
	items := make([]Item, 0, n)
	for _, p := range res.Pins[:n] {
		link := safeURLString(p.URL)
		if link == "" {
			continue
		}
		text := strings.TrimSpace(p.Text)
		pinTitle := strings.TrimSpace(p.Title)
		if pinTitle == "" {
			pinTitle = text
		}
		if pinTitle == "" {
			pinTitle = "(no content)"
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
			id = link
		}
		items = append(items, Item{
			ID:        id,
			Title:     pinTitle,
			Text:      text,
			HTML:      pinterestHTML(link, image, text),
			URL:       link,
			Images:    images,
			CreatedAt: created,
		})
	}
	return &Feed{
		UID:         pinterestUID(res),
		ScreenName:  screen,
		Description: description,
		FetchedAt:   res.FetchedAt.UTC(),
		Cached:      cached,
		Degraded:    degraded,
		TitleText:   screen + " on Pinterest",
		LinkText:    res.Link,
		TTL:         int(pinterestTTL.Minutes()),
		Items:       items,
	}
}

func pinterestUID(res *pinterest.Result) string {
	if res.Board != "" {
		return res.User + "/" + res.Board
	}
	return res.User
}

// pinterestHTML renders one pin: the 736px thumbnail, the caption when there is
// one, and a link back to the pin page.
func pinterestHTML(pinURL, image, text string) string {
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
	b.WriteString(EscapeXML(pinURL))
	b.WriteString(`">View pin on Pinterest</a></p>`)
	return b.String()
}

// pinterestError classifies a provider failure for the HTTP layer.
func pinterestError(err error, user, board string) error {
	if err == nil {
		return nil
	}
	var we *apperr.Error
	if errors.As(err, &we) {
		return err
	}
	target := user
	if board != "" {
		target = user + "/" + board
	}
	switch {
	case errors.Is(err, pinterest.ErrInvalidUser), errors.Is(err, pinterest.ErrInvalidBoard):
		return &apperr.Error{Code: apperr.CodeBadRequest, Message: fmt.Sprintf("invalid pinterest target %q", target), Err: err}
	case errors.Is(err, pinterest.ErrUserNotFound):
		return &apperr.Error{Code: apperr.CodeUserNotFound, Message: "pinterest user " + user + " not found", Err: err}
	case errors.Is(err, pinterest.ErrBoardNotFound):
		return &apperr.Error{Code: apperr.CodeUserNotFound, Message: "pinterest board " + target + " not found", Err: err}
	case errors.Is(err, pinterest.ErrRateLimited):
		return &apperr.Error{Code: apperr.CodeRateLimited, Message: "pinterest rate limited", Err: err}
	case errors.Is(err, pinterest.ErrBlocked):
		return &apperr.Error{Code: apperr.CodeUpstreamBlocked, Message: "pinterest blocked the request for " + target, Err: err}
	case errors.Is(err, pinterest.ErrUnexpectedPage), errors.Is(err, pinterest.ErrBadFeed):
		return &apperr.Error{Code: apperr.CodeUpstreamUnavailable, Message: "pinterest feed for " + target + " could not be parsed", Err: err}
	case errors.Is(err, context.DeadlineExceeded):
		return &apperr.Error{Code: apperr.CodeTimeout, Message: "pinterest feed timed out", Err: err}
	case errors.Is(err, pinterest.ErrUpstream):
		return &apperr.Error{Code: apperr.CodeUpstreamUnavailable, Message: "pinterest feed for " + target + " is unavailable", Err: err}
	default:
		return &apperr.Error{Code: apperr.CodeNetwork, Message: err.Error(), Err: err}
	}
}
