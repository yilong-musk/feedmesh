package feed

import (
	"context"
	"errors"
	"fmt"
	"html"
	"regexp"
	"strings"
	"time"

	"github.com/yilong-musk/feedmesh/internal/apperr"
	"github.com/yilong-musk/feedmesh/internal/cachekey"
	"github.com/yilong-musk/feedmesh/internal/linuxdo"
)

// LINUX DO feed bounds. The forum publishes 25-30 topics per RSS page and
// Cloudflare is sensitive to bursts, so a fifteen-minute cache plus a global
// five-second read slot keeps a multi-feed refresh gentle. The last good copy
// is kept for a day: a Cloudflare challenge can outlive a short cache window.
const (
	// DefaultLinuxDoLimit is the number of topics returned when the caller is silent.
	DefaultLinuxDoLimit = linuxdo.DefaultLimit
	// MaxLinuxDoLimit is the widest answer, three RSS pages.
	MaxLinuxDoLimit = linuxdo.MaxLimit

	linuxdoTTL = 15 * time.Minute
)

// LinuxDoTTL is the cache lifetime of one LINUX DO feed.
func LinuxDoTTL() time.Duration { return linuxdoTTL }

// block tags are turned into line breaks before the tags are stripped; without
// it a multi-paragraph forum post becomes one line.
var (
	blockOpenRe  = regexp.MustCompile(`(?i)<(p|div|blockquote|li|h[1-6]|tr)\b[^>]*>`)
	blockCloseRe = regexp.MustCompile(`(?i)</(p|div|blockquote|li|h[1-6]|tr)>`)
)

// GetLinuxDo builds a feed from the forum's official RSS endpoints.
func (s *Service) GetLinuxDo(ctx context.Context, raw linuxdo.Ref, limit int, refresh bool) (*Feed, error) {
	if s.clients.LinuxDo == nil {
		return nil, &apperr.Error{Code: apperr.CodeInternal, Message: "linux.do provider is not configured"}
	}
	ref, err := raw.Validate()
	if err != nil {
		return nil, &apperr.Error{Code: apperr.CodeBadRequest, Message: "invalid linux.do feed", Err: err}
	}
	if limit <= 0 {
		limit = DefaultLinuxDoLimit
	}
	if limit > MaxLinuxDoLimit {
		limit = MaxLinuxDoLimit
	}
	if budget := s.clients.LinuxDo.Budget(); budget > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, budget)
		defer cancel()
	}

	key := cachekey.LinuxDo.Key("feed", ref.Key())
	entry, cached, degraded, err := fetchCachedList(ctx, s, key, linuxdoTTL, refresh, limit,
		func(ctx context.Context, n int) ([]linuxdo.Topic, error) {
			f, ferr := s.clients.LinuxDo.Fetch(ctx, ref, n)
			if ferr != nil {
				return nil, ferr
			}
			return f.Topics, nil
		})
	if err != nil {
		return nil, linuxdoError(err, ref)
	}
	return linuxdoFeed(ref, entry, limit, cached, degraded), nil
}

func linuxdoFeed(ref linuxdo.Ref, c cachedList[linuxdo.Topic], limit int, cached, degraded bool) *Feed {
	display, link := linuxdoHeader(ref, c.Items)
	n := len(c.Items)
	if limit < n {
		n = limit
	}
	items := make([]Item, 0, n)
	for _, t := range c.Items[:n] {
		it := Item{
			ID:        t.ID,
			CreatedAt: t.CreatedAt.UTC(),
			Text:      linuxdoText(t.HTML),
			HTML:      linuxdoHTML(t),
			URL:       t.URL,
			Title:     t.Title,
			Author:    t.Author,
			Category:  t.Category,
			Pinned:    t.Pinned,
			Closed:    t.Closed,
			Archived:  t.Archived,
		}
		for _, img := range t.Images {
			if u := safeURLString(img); u != "" {
				it.Images = append(it.Images, u)
			}
		}
		items = append(items, it)
	}
	if display == "" {
		display = "LINUX DO"
	}
	return &Feed{
		Language:    langZhCN,
		UID:         ref.Key(),
		ScreenName:  display,
		FetchedAt:   c.FetchedAt,
		TitleText:   display,
		LinkText:    link,
		Cached:      cached,
		Degraded:    degraded,
		Items:       items,
		Description: "LINUX DO topics via the official RSS feed.",
	}
}

// linuxdoHeader builds the feed title and the human page link. Category titles
// use the category name the feed carries (that is upstream data); the other
// kinds get a stable English label.
func linuxdoHeader(ref linuxdo.Ref, topics []linuxdo.Topic) (title, link string) {
	switch ref.Kind {
	case linuxdo.KindLatest:
		link = "https://linux.do/latest"
		if ref.Order == "activity" {
			link += "?order=activity"
			return "LINUX DO · Latest activity", link
		}
		return "LINUX DO · Latest topics", link
	case linuxdo.KindTop:
		label := strings.ToUpper(ref.Period[:1]) + ref.Period[1:]
		return "LINUX DO · Top " + label, "https://linux.do/top?period=" + ref.Period
	case linuxdo.KindHot:
		return "LINUX DO · Hot", "https://linux.do/hot"
	case linuxdo.KindCategory:
		name := ""
		for _, t := range topics {
			if t.Category != "" {
				name = t.Category
				break
			}
		}
		if name == "" {
			name = ref.Slug
		}
		if name == "" {
			name = ref.ID
		}
		slug := ref.Slug
		if slug == "" {
			slug = "-"
		}
		return "LINUX DO · " + name, "https://linux.do/c/" + slug + "/" + ref.ID
	case linuxdo.KindTag:
		return "LINUX DO · #" + ref.Tag, "https://linux.do/tag/" + ref.Tag
	}
	return "LINUX DO", "https://linux.do/"
}

// linuxdoHTML sanitises the first-post HTML and adds the metadata the RSS
// document keeps outside the body (category, author and topic state).
func linuxdoHTML(t linuxdo.Topic) string {
	var b strings.Builder
	b.WriteString(NormalizeHTMLBase(t.HTML, "https://linux.do"))
	meta := make([]string, 0, 4)
	if t.Category != "" {
		meta = append(meta, "Category "+t.Category)
	}
	if t.Author != "" {
		meta = append(meta, "by @"+t.Author)
	}
	if t.Pinned {
		meta = append(meta, "Pinned")
	}
	if t.Closed {
		meta = append(meta, "Closed")
	}
	if t.Archived {
		meta = append(meta, "Archived")
	}
	if len(meta) > 0 {
		b.WriteString("<p>")
		b.WriteString(html.EscapeString(strings.Join(meta, " · ")))
		b.WriteString("</p>")
	}
	if b.Len() == 0 && t.Title != "" {
		b.WriteString("<p>")
		b.WriteString(html.EscapeString(t.Title))
		b.WriteString("</p>")
	}
	return b.String()
}

// linuxdoText flattens the post HTML but keeps paragraph breaks. The forum's
// RSS description ends with a "read the full topic" footer and a reply count;
// both are kept because they are part of the upstream document.
func linuxdoText(raw string) string {
	if raw == "" {
		return ""
	}
	s := blockOpenRe.ReplaceAllString(raw, "")
	s = blockCloseRe.ReplaceAllString(s, "\n")
	s = brTag.ReplaceAllString(s, "\n")
	s = anyTag.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(wsRun.ReplaceAllString(line, " "))
		if line == "" && (len(out) == 0 || out[len(out)-1] == "") {
			continue
		}
		out = append(out, line)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

func linuxdoError(err error, ref linuxdo.Ref) error {
	if err == nil {
		return nil
	}
	var we *apperr.Error
	if errors.As(err, &we) {
		return err
	}
	switch {
	case errors.Is(err, linuxdo.ErrInvalidRef):
		return &apperr.Error{Code: apperr.CodeBadRequest, Message: "invalid linux.do feed " + ref.Key(), Err: err}
	case errors.Is(err, linuxdo.ErrNotFound):
		return &apperr.Error{Code: apperr.CodeUserNotFound, Message: "linux.do feed " + ref.Key() + " not found", Err: err}
	case errors.Is(err, linuxdo.ErrChallenge):
		return &apperr.Error{Code: apperr.CodeUpstreamBlocked, Message: "linux.do is challenging automated requests; try again later", Err: err}
	case errors.Is(err, linuxdo.ErrNoItems):
		return &apperr.Error{Code: apperr.CodeUpstreamUnavailable, Message: "linux.do feed " + ref.Key() + " has no topics", Err: err}
	case errors.Is(err, linuxdo.ErrUnexpectedPage):
		return &apperr.Error{Code: apperr.CodeUpstreamUnavailable, Message: "linux.do returned a non-RSS page", Err: err}
	case errors.Is(err, context.DeadlineExceeded):
		return &apperr.Error{Code: apperr.CodeTimeout, Message: "linux.do feed timed out", Err: err}
	default:
		return &apperr.Error{Code: apperr.CodeInternal, Message: fmt.Sprintf("linux.do: %v", err), Err: err}
	}
}
