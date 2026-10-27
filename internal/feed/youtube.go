package feed

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/yilong-musk/feedmesh/internal/weibo"
	"github.com/yilong-musk/feedmesh/internal/youtube"
)

// YouTube feed bounds and cache lifetimes. The upload feed carries 15 entries
// with no pagination, so the ceiling is the feed width. Ten minutes keeps a
// active channel fresh while staying far away from the endpoint's flaky
// windows; the entry itself is stored for the whole stale window so a bad
// window degrades to the last good list instead of an error.
const (
	DefaultYouTubeLimit = youtube.DefaultLimit
	MaxYouTubeLimit     = youtube.MaxLimit
	// youtubeTTL is how long a stored list counts as fresh. It is enforced in
	// readYouTube, not by the cache expiry, so an expired entry survives to
	// serve the last-good path.
	youtubeTTL = 10 * time.Minute
	// youtubeStaleWindow is both the age limit of the last-good list and the
	// stored TTL: the cache keeps the entry until it can no longer help.
	youtubeStaleWindow = 24 * time.Hour
	youtubeChannelTTL  = 24 * time.Hour
	// youtubeSourceStale marks a list served from the cache because the
	// upstream read failed; the other values come from the client.
	youtubeSourceStale = "stale"
)

// cachedYouTube is one channel's upload list as stored in the cache. The
// channel title travels with the videos because the feed carries it, and the
// source travels along so a cached read still says where the list came from.
type cachedYouTube struct {
	ChannelID string          `json:"channel_id"`
	Title     string          `json:"title"`
	Source    string          `json:"source"`
	Videos    []youtube.Video `json:"videos"`
	FetchedAt time.Time       `json:"fetched_at"`
}

// YouTubeTTL is the cache lifetime for one channel feed.
func YouTubeTTL() time.Duration { return youtubeTTL }

// GetYouTube builds a channel feed from the official upload feed. ref may be a
// channel id, an @handle or a channel URL.
//
// A failed upstream read never replaces a good list: when the cache still has
// one (up to youtubeStaleWindow old) it is served with Degraded set, because
// YouTube's feed answers 404/5xx for short windows on channels that exist.
func (s *Service) GetYouTube(ctx context.Context, ref string, limit int, refresh bool) (*Feed, error) {
	if s.clients.Youtube == nil {
		return nil, &weibo.Error{Code: weibo.CodeInternal, Message: "youtube provider is not configured"}
	}
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, &weibo.Error{Code: weibo.CodeBadRequest, Message: "missing channel"}
	}
	if limit <= 0 {
		limit = DefaultYouTubeLimit
	}
	if limit > MaxYouTubeLimit {
		limit = MaxYouTubeLimit
	}

	// One deadline for the whole lookup: the channel page and the feed each
	// have their own budget, and without this the two would add up.
	ctx, cancel := context.WithTimeout(ctx, s.clients.Youtube.Budget())
	defer cancel()

	channelID, err := s.youtubeChannelID(ctx, ref)
	if err != nil {
		return nil, youtubeError(err, ref)
	}

	key := "youtube:feed:" + channelID
	unlock := s.lockListKey(key)
	defer unlock()

	if !refresh {
		if e, ok := s.readYouTube(key, false); ok {
			return s.youtubeFeed(e, limit, true, false), nil
		}
	}

	upstream, err := s.clients.Youtube.Videos(ctx, channelID)
	if err == nil {
		if len(upstream.Videos) == 0 {
			// A successful read with no entries is either a channel without
			// uploads or a partial answer from the endpoint. Keeping the list
			// the cache already has is the safer reading; a genuinely emptied
			// channel ages out within the stale window.
			if e, ok := s.readYouTube(key, true); ok {
				s.log.Warn("youtube feed came back empty, keeping the last good list", "channel", channelID)
				return s.youtubeFeed(e, limit, true, true), nil
			}
		}
		e := cachedYouTube{
			ChannelID: upstream.ChannelID,
			Title:     upstream.Title,
			Source:    upstream.Source,
			Videos:    upstream.Videos,
			FetchedAt: s.now().UTC(),
		}
		if s.cache != nil {
			// The entry is stored for the whole stale window; freshness is
			// enforced by readYouTube from FetchedAt. Storing it with the
			// ten-minute TTL would let the cache delete the copy the
			// last-good path needs during an outage.
			_ = s.cache.Set(key, e, youtubeStaleWindow)
		}
		if upstream.Source == youtube.SourceBrowse {
			s.log.Info("youtube feed built from the web client fallback", "channel", channelID)
		}
		return s.youtubeFeed(e, limit, false, false), nil
	}

	// The endpoint flaked or the network failed. Serving the last good list is
	// better than failing the reader's poll; the flag keeps the degradation
	// visible in the JSON view and the response header.
	var fb *youtube.FallbackError
	if errors.As(err, &fb) {
		// Both sources failed. The Atom error is what the reader is told; the
		// fallback's own error only reaches the log, so a rotated web-client
		// key is visible instead of surfacing as a feed 404 forever.
		s.log.Warn("youtube web client fallback failed", "channel", channelID, "error", fb.Browse.Error())
	}
	if e, ok := s.readYouTube(key, true); ok {
		s.log.Warn("youtube feed degraded to the last good list", "channel", channelID, "error", err.Error())
		return s.youtubeFeed(e, limit, true, true), nil
	}
	return nil, youtubeError(err, ref)
}

// youtubeChannelID resolves a channel reference once per day. The public
// channel page is about 1.4 MB, so resolving it on every poll would be both
// slow and needlessly rude; the feed itself only needs the id. The key uses
// the normalised reference, so "@name", "name" and a pasted /@name URL share
// one mapping instead of triggering a page read each; the legacy /c/ and
// /user/ forms keep a key of their own, because that name can point at a
// different channel.
//
// When the page cannot be read — an outage window hits it just as the 24-hour
// mapping expires, or it answers a consent page — a stale mapping is used
// instead: it still names the right channel, and the list it points at is
// what the caller came for.
func (s *Service) youtubeChannelID(ctx context.Context, ref string) (string, error) {
	key := "youtube:channel:" + youtube.MappingKey(ref)
	if s.cache != nil {
		var id string
		// Peek, not Get: Get deletes an expired entry, and an expired mapping
		// is exactly the one the failure path below needs.
		if hit, err := s.cache.Peek(key, &id); err == nil && hit && id != "" {
			return id, nil
		}
	}
	id, err := s.clients.Youtube.ChannelID(ctx, ref)
	if err != nil {
		if errors.Is(err, youtube.ErrInvalidRef) {
			return "", err
		}
		if stale, ok := s.staleChannelID(key); ok {
			s.log.Warn("youtube channel page unavailable, using the stored mapping", "ref", ref, "error", err.Error())
			return stale, nil
		}
		return "", err
	}
	if s.cache != nil {
		_ = s.cache.Set(key, id, youtubeChannelTTL)
	}
	return id, nil
}

// staleChannelID reads a mapping regardless of its expiry. A channel id is
// stable across renames, so an expired mapping is still better than blanking a
// feed whose list is cached; the entry is only consulted when the live page
// read failed.
func (s *Service) staleChannelID(key string) (string, bool) {
	if s.cache == nil {
		return "", false
	}
	var id string
	hit, err := s.cache.PeekStored(key, &id)
	if err != nil || !hit || id == "" {
		return "", false
	}
	return id, true
}

// readYouTube reads the cached list. includeExpired selects the last-good
// path, which ignores the TTL but honours youtubeStaleWindow.
func (s *Service) readYouTube(key string, includeExpired bool) (cachedYouTube, bool) {
	if s.cache == nil {
		return cachedYouTube{}, false
	}
	var e cachedYouTube
	// PeekStored, not Get: Get deletes an expired entry, and the last-good
	// path below needs that entry to outlive the freshness window. The stored
	// TTL is the stale window, so freshness is decided here from FetchedAt.
	hit, err := s.cache.PeekStored(key, &e)
	if err != nil || !hit {
		return cachedYouTube{}, false
	}
	age := s.now().Sub(e.FetchedAt)
	if includeExpired {
		// The last-good path only helps when there is something to serve.
		if len(e.Videos) == 0 || age > youtubeStaleWindow {
			return cachedYouTube{}, false
		}
		return e, true
	}
	if age > youtubeTTL {
		return cachedYouTube{}, false
	}
	return e, true
}

// youtubeFeed maps a cached list onto the feed model.
func (s *Service) youtubeFeed(e cachedYouTube, limit int, cached, degraded bool) *Feed {
	videos := e.Videos
	if len(videos) > limit {
		videos = videos[:limit]
	}
	title := e.Title
	if title == "" {
		title = "YouTube " + e.ChannelID
	}
	link := "https://www.youtube.com/channel/" + e.ChannelID
	source := e.Source
	if degraded || source == "" {
		// A degraded read is served from the cache because the upstream
		// failed, which is a state of its own; an entry written before Source
		// existed reports nothing better than "stale".
		source = youtubeSourceStale
	}
	f := &Feed{
		UID:         e.ChannelID,
		ScreenName:  title,
		Description: "Latest videos from " + title,
		FetchedAt:   e.FetchedAt,
		Cached:      cached,
		Degraded:    degraded,
		Source:      source,
		TitleText:   title,
		LinkText:    link,
		TTL:         int(youtubeTTL.Minutes()),
		Items:       make([]Item, 0, len(videos)),
	}
	for _, v := range videos {
		link := "https://www.youtube.com/watch?v=" + v.ID
		images := []string{}
		if img := safeURLString(v.Thumbnail); img != "" {
			images = append(images, img)
		}
		f.Items = append(f.Items, Item{
			ID:        v.ID,
			Title:     v.Title,
			Text:      v.Description,
			HTML:      youtubeHTML(v, images),
			URL:       link,
			CreatedAt: v.Published,
			Images:    images,
			Views:     v.Views,
		})
	}
	return f
}

// youtubeHTML renders one entry: cover, description, view counter, and a link
// back to YouTube.
func youtubeHTML(v youtube.Video, images []string) string {
	var b strings.Builder
	if len(images) > 0 {
		writeImage(&b, images[0], "", "")
	}
	if desc := strings.TrimSpace(v.Description); desc != "" {
		b.WriteString("<p>")
		b.WriteString(EscapeXML(desc))
		b.WriteString("</p>")
	}
	if v.Views > 0 {
		b.WriteString("<p><em>")
		b.WriteString(formatViews(v.Views))
		b.WriteString(" views</em></p>")
	}
	b.WriteString(`<p><a href="`)
	b.WriteString(EscapeXML("https://www.youtube.com/watch?v=" + v.ID))
	b.WriteString(`">Watch on YouTube</a></p>`)
	return b.String()
}

// formatViews renders a play counter in compact units, dropping a trailing
// ".0" so 32,000 reads as 32K rather than 32.0K.
func formatViews(n int64) string {
	switch {
	case n >= 1_000_000_000:
		return trimZero(n, 1e9, "B")
	case n >= 1_000_000:
		return trimZero(n, 1e6, "M")
	case n >= 1_000:
		return trimZero(n, 1e3, "K")
	default:
		return strconv.FormatInt(n, 10)
	}
}

func trimZero(n int64, unit float64, suffix string) string {
	s := strconv.FormatFloat(float64(n)/unit, 'f', 1, 64)
	return strings.TrimSuffix(s, ".0") + suffix
}

// youtubeError maps client errors onto the API error envelope.
func youtubeError(err error, ref string) error {
	if err == nil {
		return nil
	}
	var we *weibo.Error
	if errors.As(err, &we) {
		return err
	}
	if errors.Is(err, youtube.ErrInvalidRef) {
		return &weibo.Error{
			Code:    weibo.CodeBadRequest,
			Message: err.Error(),
			Err:     err,
		}
	}
	if errors.Is(err, youtube.ErrChannelNotFound) {
		return &weibo.Error{
			Code:    weibo.CodeUserNotFound,
			Message: fmt.Sprintf("youtube channel %q not found", ref),
			Err:     err,
		}
	}
	return &weibo.Error{Code: weibo.CodeUpstreamUnavailable, Message: err.Error(), Err: err}
}
