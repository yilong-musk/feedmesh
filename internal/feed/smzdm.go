package feed

import (
	"context"
	"strings"
	"time"

	"github.com/yilong-musk/feedmesh/internal/smzdm"
	"github.com/yilong-musk/feedmesh/internal/weibo"
)

// SMZDM feed bounds and cache lifetimes. The deal slices rotate every few
// minutes, so a short TTL keeps the feed moving without hammering the CDN;
// community articles change much more slowly.
const (
	DefaultSmzdmLimit = 20
	MaxSmzdmLimit     = smzdm.MaxLimit
	smzdmDealsTTL     = 5 * time.Minute
	smzdmPostTTL      = 10 * time.Minute
)

type smzdmChannelInfo struct {
	title       string
	description string
	link        string
}

// smzdmChannels carries the per-channel presentation metadata.
var smzdmChannels = map[smzdm.Channel]smzdmChannelInfo{
	smzdm.ChannelDeals:  {"SMZDM Deals", "Selected deals from SMZDM", "https://www.smzdm.com"},
	smzdm.ChannelPost:   {"SMZDM Community", "Community articles from SMZDM", "https://post.smzdm.com"},
	smzdm.ChannelHaitao: {"SMZDM Overseas Deals", "Overseas deals from SMZDM", "https://haitao.smzdm.com"},
}

// SmzdmTTL is the cache lifetime for one channel.
func SmzdmTTL(ch smzdm.Channel) time.Duration {
	if ch == smzdm.ChannelPost {
		return smzdmPostTTL
	}
	return smzdmDealsTTL
}

// GetSmzdm builds one SMZDM channel from the official RSS feed.
func (s *Service) GetSmzdm(ctx context.Context, ch smzdm.Channel, limit int, refresh bool) (*Feed, error) {
	info, ok := smzdmChannels[ch]
	if !ok {
		return nil, &weibo.Error{Code: weibo.CodeBadRequest, Message: "unknown smzdm channel " + string(ch)}
	}
	if s.clients.Smzdm == nil {
		return nil, &weibo.Error{Code: weibo.CodeInternal, Message: "smzdm provider is not configured"}
	}
	if limit <= 0 {
		limit = DefaultSmzdmLimit
	}
	if limit > MaxSmzdmLimit {
		limit = MaxSmzdmLimit
	}

	ttl := SmzdmTTL(ch)
	deals, hit, err := fetchCachedList(ctx, s, "smzdm:"+string(ch), ttl, refresh, limit,
		func(n int) ([]smzdm.Deal, error) { return s.clients.Smzdm.List(ctx, ch, n) })
	if err != nil {
		return nil, sspaiError(err)
	}
	if len(deals) > limit {
		deals = deals[:limit]
	}

	now := s.now().UTC()
	f := &Feed{
		TitleText:   info.title,
		LinkText:    info.link,
		Description: info.description,
		FetchedAt:   now,
		Cached:      hit,
		TTL:         int(ttl.Minutes()),
		Items:       make([]Item, 0, len(deals)),
	}
	for _, d := range deals {
		created := d.Published
		if created.IsZero() {
			created = now
		} else {
			// API timestamps are UTC; the feed stamps Beijing time.
			created = created.UTC()
		}
		images := []string{}
		if img := safeURLString(d.Image); img != "" {
			images = append(images, img)
		}
		f.Items = append(f.Items, Item{
			ID:        d.ID,
			Title:     d.Title,
			Text:      d.Summary,
			HTML:      smzdmHTML(ch, d),
			URL:       d.URL,
			CreatedAt: created,
			Images:    images,
		})
	}
	return f, nil
}

// smzdmHTML renders one entry: cover, blurb (when the feed carries one), the
// author, and a link back to SMZDM.
func smzdmHTML(ch smzdm.Channel, d smzdm.Deal) string {
	var b strings.Builder
	if img := safeURLString(d.Image); img != "" {
		writeImage(&b, img, "", "")
	}
	if sum := strings.TrimSpace(d.Summary); sum != "" {
		b.WriteString("<p>")
		b.WriteString(EscapeXML(sum))
		b.WriteString("</p>")
	}
	if who := strings.TrimSpace(d.Author); who != "" {
		b.WriteString("<p><em>Posted by ")
		b.WriteString(EscapeXML(who))
		b.WriteString("</em></p>")
	}
	label := "View deal on SMZDM"
	if ch == smzdm.ChannelPost {
		label = "Read the article on SMZDM"
	}
	b.WriteString(`<p><a href="`)
	b.WriteString(EscapeXML(d.URL))
	b.WriteString(`">`)
	b.WriteString(label)
	b.WriteString(`</a></p>`)
	return b.String()
}
