package feed

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/yilong-musk/feedmesh/internal/reddit"
	"github.com/yilong-musk/feedmesh/internal/weibo"
)

// Reddit listing bounds and cache lifetimes. One Atom page carries at most 25
// entries and the document has no pagination, so the page width is the ceiling.
const (
	DefaultRedditLimit = reddit.DefaultLimit
	MaxRedditLimit     = reddit.MaxLimit
	// redditTTL is how long a stored listing counts as fresh. Freshness is
	// enforced in readReddit from FetchedAt, not by the cache expiry, so an
	// expired entry survives to serve the last-good path.
	redditTTL = 10 * time.Minute
	// redditStaleWindow is both the age limit of the last-good listing and
	// the stored TTL: the cache keeps the entry until it can no longer help.
	redditStaleWindow = 24 * time.Hour
	// redditSourceAtom marks a listing read from the site's Atom document.
	redditSourceAtom = "atom"
	// redditSourceStale marks a listing served from the cache because the
	// upstream read failed or the rate limiter refused it.
	redditSourceStale = "stale"
)

// cachedReddit is one listing as stored in the cache. Want records how many
// entries the fetch asked upstream for, so a narrow read cannot truncate a
// wider one for the whole TTL.
type cachedReddit struct {
	Sub         string        `json:"sub"`
	Title       string        `json:"title"`
	Description string        `json:"description"`
	Sort        string        `json:"sort"`
	Window      string        `json:"window"`
	Source      string        `json:"source"`
	Posts       []reddit.Post `json:"posts"`
	Want        int           `json:"want"`
	FetchedAt   time.Time     `json:"fetched_at"`
}

// RedditTTL is the cache lifetime for one listing.
func RedditTTL() time.Duration { return redditTTL }

// GetReddit builds a subreddit listing feed. sub may carry an "r/" prefix and
// any case; sort and window select the listing the site serves.
//
// The site allows one anonymous read per rate-limit window, so a listing that
// cannot be read right now — the window is open, or another read is in flight —
// is served from the last good entry (up to redditStaleWindow old) with
// Degraded set. Only a listing that has no usable entry fails.
func (s *Service) GetReddit(ctx context.Context, sub string, sort reddit.Sort, window reddit.Window, limit int, refresh bool) (*Feed, error) {
	if s.clients.Reddit == nil {
		return nil, &weibo.Error{Code: weibo.CodeInternal, Message: "reddit provider is not configured"}
	}
	name, err := reddit.NormalizeSub(sub)
	if err != nil {
		return nil, redditError(err, sub)
	}
	if sort == "" {
		sort = reddit.SortHot
	}
	if window == "" {
		window = reddit.WindowDay
	}
	if _, ok := reddit.ParseSort(string(sort)); !ok {
		return nil, redditError(fmt.Errorf("%w: unknown sort %q", reddit.ErrBadRequest, sort), sub)
	}
	if _, ok := reddit.ParseWindow(string(window)); !ok {
		return nil, redditError(fmt.Errorf("%w: unknown window %q", reddit.ErrBadRequest, window), sub)
	}
	if limit <= 0 {
		limit = DefaultRedditLimit
	}
	if limit > MaxRedditLimit {
		limit = MaxRedditLimit
	}

	key := "reddit:list:" + name + ":" + string(sort) + ":" + string(window)
	unlock := s.lockListKey(key)
	defer unlock()

	// A refresh bypasses the stored values but keeps the cache at least as
	// wide as the entry it replaces, so a small-limit refresh cannot narrow a
	// listing another reader just warmed.
	want := limit
	if refresh {
		if prev, ok := s.readRedditEntry(key); ok {
			if width := prev.width(); width > want {
				want = width
				if want > MaxRedditLimit {
					want = MaxRedditLimit
				}
			}
		}
	}
	if !refresh {
		if e, ok := s.readReddit(key, false, want); ok {
			return s.redditFeed(e, limit, true, false), nil
		}
	}

	listing, err := s.clients.Reddit.List(ctx, name, sort, window, want)
	if err == nil {
		if len(listing.Posts) == 0 {
			// A successful read with no entries is either an empty subreddit or
			// a partial answer. Keeping the listing the cache already has is the
			// safer reading; a genuinely emptied subreddit ages out within the
			// stale window.
			if e, ok := s.readReddit(key, true, 0); ok {
				s.log.Warn("reddit listing came back empty, keeping the last good one",
					"sub", name, "sort", string(sort))
				return s.redditFeed(e, limit, true, true), nil
			}
		}
		e := cachedReddit{
			Sub:         name,
			Title:       listing.Title,
			Description: listing.Description,
			Sort:        string(sort),
			Window:      string(window),
			Source:      redditSourceAtom,
			Posts:       listing.Posts,
			Want:        want,
			FetchedAt:   s.now().UTC(),
		}
		if s.cache != nil {
			// The entry is stored for the whole stale window; freshness is
			// decided by readReddit from FetchedAt.
			_ = s.cache.Set(key, e, redditStaleWindow)
		}
		return s.redditFeed(e, limit, false, false), nil
	}

	// The read was refused by the limiter or the site failed. Serving the last
	// good listing beats failing the reader's poll; the flag keeps the
	// degradation visible in the JSON view and the response headers.
	if e, ok := s.readReddit(key, true, 0); ok {
		s.log.Warn("reddit listing degraded to the last good one",
			"sub", name, "sort", string(sort), "error", err.Error())
		return s.redditFeed(e, limit, true, true), nil
	}
	return nil, redditError(err, sub)
}

// readRedditEntry reads the stored listing regardless of its age.
func (s *Service) readRedditEntry(key string) (cachedReddit, bool) {
	if s.cache == nil {
		return cachedReddit{}, false
	}
	var e cachedReddit
	// PeekStored, not Get: Get deletes an expired entry, and the last-good
	// path needs that entry to outlive the freshness window.
	hit, err := s.cache.PeekStored(key, &e)
	if err != nil || !hit {
		return cachedReddit{}, false
	}
	return e, true
}

// readReddit reads the cached listing. includeExpired selects the last-good
// path, which ignores the TTL but honours redditStaleWindow and the width the
// caller asked for.
func (s *Service) readReddit(key string, includeExpired bool, want int) (cachedReddit, bool) {
	e, ok := s.readRedditEntry(key)
	if !ok {
		return cachedReddit{}, false
	}
	age := s.now().Sub(e.FetchedAt)
	if includeExpired {
		// The last-good path only helps when there is something to serve.
		if len(e.Posts) == 0 || age > redditStaleWindow {
			return cachedReddit{}, false
		}
		return e, true
	}
	if age > redditTTL || !e.covers(want) {
		return cachedReddit{}, false
	}
	return e, true
}

// width reports how many entries the entry was fetched at.
func (e cachedReddit) width() int {
	if e.Want > len(e.Posts) {
		return e.Want
	}
	return len(e.Posts)
}

// covers reports whether the entry can answer a request for n entries.
func (e cachedReddit) covers(n int) bool {
	return e.Want >= n || len(e.Posts) >= n
}

// redditFeed maps a cached listing onto the feed model.
func (s *Service) redditFeed(e cachedReddit, limit int, cached, degraded bool) *Feed {
	posts := e.Posts
	if len(posts) > limit {
		posts = posts[:limit]
	}
	sort := e.Sort
	if sort == "" {
		sort = string(reddit.SortHot)
	}
	title := "r/" + e.Sub + " " + sort
	if sort == string(reddit.SortTop) && e.Window != "" {
		// Top needs its period: r/x top day and r/x top all are different
		// listings, and a reader subscribed to both must be able to tell them
		// apart.
		title += " (" + e.Window + ")"
	}
	source := e.Source
	if degraded || source == "" {
		source = redditSourceStale
	}
	description := strings.TrimSpace(e.Description)
	if description == "" {
		description = "Reddit listing " + title
	}
	f := &Feed{
		UID:         e.Sub,
		ScreenName:  title,
		Description: description,
		FetchedAt:   e.FetchedAt,
		Cached:      cached,
		Degraded:    degraded,
		Source:      source,
		TitleText:   title,
		LinkText:    "https://www.reddit.com/r/" + e.Sub + "/" + sort + "/",
		TTL:         int(redditTTL.Minutes()),
		Items:       make([]Item, 0, len(posts)),
	}
	for _, p := range posts {
		link := safeURLString(p.Permalink)
		if link == "" {
			// Without a permalink the item has no guid and no comment page;
			// dropping it beats emitting a broken <guid>.
			continue
		}
		external := safeURLString(p.ExternalURL)
		f.Items = append(f.Items, Item{
			ID:        p.ID,
			Title:     p.Title,
			Author:    p.Author,
			Text:      external,
			HTML:      redditHTML(external, link, p.Author),
			URL:       link,
			CreatedAt: p.Published.UTC(),
			// The Atom document carries no thumbnail; the field stays an empty
			// array so the JSON view keeps its shape.
			Images: []string{},
		})
	}
	return f
}

// redditHTML renders one entry: the outbound link, the submitter and a link to
// the comment page. The document carries no post body, so this is all there is.
func redditHTML(external, permalink, author string) string {
	var b strings.Builder
	if external != "" {
		b.WriteString(`<p><a href="`)
		b.WriteString(EscapeXML(external))
		b.WriteString(`">`)
		b.WriteString(EscapeXML(external))
		b.WriteString("</a></p>")
	}
	if author != "" {
		b.WriteString("<p>submitted by u/")
		b.WriteString(EscapeXML(author))
		b.WriteString("</p>")
	}
	b.WriteString(`<p><a href="`)
	b.WriteString(EscapeXML(permalink))
	b.WriteString(`">Comments on Reddit</a></p>`)
	return b.String()
}

// redditError maps client errors onto the API error envelope.
func redditError(err error, sub string) error {
	if err == nil {
		return nil
	}
	var we *weibo.Error
	if errors.As(err, &we) {
		return err
	}
	switch {
	case errors.Is(err, reddit.ErrBadRequest):
		return &weibo.Error{Code: weibo.CodeBadRequest, Message: err.Error(), Err: err}
	case errors.Is(err, reddit.ErrSubredditNotFound):
		return &weibo.Error{
			Code:    weibo.CodeUserNotFound,
			Message: fmt.Sprintf("reddit subreddit %q not found", sub),
			Err:     err,
		}
	}
	// Everything else is "not now": the site failed, the limiter refused the
	// read, or the site's quota window is open. The reader retries later.
	return &weibo.Error{Code: weibo.CodeUpstreamUnavailable, Message: err.Error(), Err: err}
}
