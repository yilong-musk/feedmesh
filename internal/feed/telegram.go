package feed

import (
	"context"
	"errors"
	"fmt"
	"html"
	"strings"
	"time"

	"github.com/yilong-musk/feedmesh/internal/apperr"
	"github.com/yilong-musk/feedmesh/internal/telegram"
)

// Telegram feed bounds and cache lifetimes. One preview page carries the
// newest ~20 posts; polling more often than the reader needs only adds t.me
// traffic, so the list is fresh for ten minutes and kept for the whole
// last-good window.
const (
	// DefaultTelegramLimit is the number of posts returned when the caller is silent.
	DefaultTelegramLimit = telegram.DefaultLimit
	// MaxTelegramLimit is the widest answer: one preview page.
	MaxTelegramLimit = telegram.MaxLimit

	telegramTTL = 10 * time.Minute
)

// TelegramTTL is the cache lifetime of one channel preview.
func TelegramTTL() time.Duration { return telegramTTL }

// GetTelegram builds a channel feed from the public t.me web preview.
func (s *Service) GetTelegram(ctx context.Context, channel string, limit int, refresh bool) (*Feed, error) {
	if s.clients.Telegram == nil {
		return nil, &apperr.Error{Code: apperr.CodeInternal, Message: "telegram provider is not configured"}
	}
	name, err := telegram.NormalizeChannel(channel)
	if err != nil {
		return nil, &apperr.Error{Code: apperr.CodeBadRequest, Message: "invalid telegram channel " + channel, Err: err}
	}
	if limit <= 0 {
		limit = DefaultTelegramLimit
	}
	if limit > MaxTelegramLimit {
		limit = MaxTelegramLimit
	}

	key := "telegram:channel:" + name
	entry, cached, degraded, err := fetchCachedList(ctx, s, key, telegramTTL, refresh, limit,
		func(ctx context.Context, n int) ([]telegram.Post, error) {
			ch, cerr := s.clients.Telegram.Fetch(ctx, name, n)
			if cerr != nil {
				return nil, cerr
			}
			return ch.Posts, nil
		})
	if err != nil {
		return nil, telegramError(err, name)
	}
	return telegramFeed(name, entry, limit, cached, degraded), nil
}

func telegramFeed(name string, c cachedList[telegram.Post], limit int, cached, degraded bool) *Feed {
	display, description, subscribers := name, "", ""
	for _, p := range c.Items {
		if display == name && p.ChannelTitle != "" {
			display = p.ChannelTitle
		}
		if description == "" {
			description = p.ChannelDescription
		}
		if subscribers == "" {
			subscribers = p.ChannelSubscribers
		}
	}
	if description == "" {
		description = "Telegram channel @" + name
	}
	n := len(c.Items)
	if limit < n {
		n = limit
	}
	items := make([]Item, 0, n)
	for _, p := range c.Items[:n] {
		plain := telegramPlainText(p.TextHTML)
		item := Item{
			ID:        fmt.Sprintf("%d", p.ID),
			CreatedAt: p.CreatedAt.UTC(),
			Text:      plain,
			HTML:      telegramHTML(p, name, plain),
			URL:       "https://t.me/" + name + "/" + fmt.Sprintf("%d", p.ID),
			Title:     telegramTitle(plain, p.Media),
			Author:    strings.TrimSpace(firstNonEmpty(p.Signature, p.Author)),
			Views:     p.Views,
		}
		for _, img := range p.Images {
			if u := safeURLString(img); u != "" {
				item.Images = append(item.Images, u)
			}
		}
		if u := safeURLString(p.PosterURL); u != "" {
			item.PosterURL = u
		}
		items = append(items, item)
	}
	return &Feed{
		UID:         name,
		ScreenName:  display,
		FetchedAt:   c.FetchedAt,
		TitleText:   display + " on Telegram",
		LinkText:    "https://t.me/" + name,
		Cached:      cached,
		Degraded:    degraded,
		Items:       items,
		Description: description,
	}
}

// telegramBase is the document base for relative links in preview HTML
// (hashtags are rendered as "?q=%23tag"). Resolving them against the channel
// preview keeps a click inside the same channel instead of the t.me home page.
func telegramBase(channel string) string {
	return "https://t.me/s/" + channel
}

// telegramHTML renders one post: the original body (sanitised), photos, a
// video still, the link card and a meta line. Telegram video URLs are signed
// and short-lived, so the body never embeds a stream URL.
func telegramHTML(p telegram.Post, channel, plain string) string {
	var b strings.Builder
	if h := NormalizeHTMLBase(p.TextHTML, telegramBase(channel)); h != "" {
		b.WriteString(h)
	}
	for _, img := range p.Images {
		writeImage(&b, img, "", "")
	}
	if p.PosterURL != "" {
		writeImage(&b, p.PosterURL, "", "")
	}
	switch p.Media {
	case "video":
		b.WriteString("<p>▶ Video</p>")
	case "document":
		b.WriteString("<p>Document</p>")
	case "audio":
		b.WriteString("<p>Audio</p>")
	case "sticker":
		b.WriteString("<p>Sticker</p>")
	case "poll":
		b.WriteString("<p>Poll</p>")
	case "location":
		b.WriteString("<p>Location</p>")
	case "contact":
		b.WriteString("<p>Contact</p>")
	}
	if link := safeURLString(p.LinkURL); link != "" {
		if img := safeURLString(p.LinkImage); img != "" {
			b.WriteString(`<p><a href="`)
			b.WriteString(EscapeXML(link))
			b.WriteString(`"><img src="`)
			b.WriteString(EscapeXML(img))
			b.WriteString(`" alt="" style="max-width:100%;height:auto" loading="lazy" /></a></p>`)
		}
		b.WriteString(`<p><a href="`)
		b.WriteString(EscapeXML(link))
		b.WriteString(`"><b>`)
		b.WriteString(EscapeXML(firstNonEmpty(p.LinkTitle, link)))
		b.WriteString(`</b></a>`)
		if p.LinkSite != "" {
			b.WriteString("<br>")
			b.WriteString(EscapeXML(p.LinkSite))
		}
		if p.LinkText != "" {
			b.WriteString("<br>")
			b.WriteString(EscapeXML(p.LinkText))
		}
		b.WriteString("</p>")
	}

	meta := make([]string, 0, 4)
	if p.ForwardedFrom != "" {
		meta = append(meta, "Forwarded from "+p.ForwardedFrom)
	}
	if p.Signature != "" && p.Signature != p.Author {
		meta = append(meta, "By "+p.Signature)
	}
	if p.Views > 0 {
		meta = append(meta, compactCount(p.Views)+" views")
	}
	if len(meta) > 0 {
		b.WriteString("<p>")
		b.WriteString(html.EscapeString(strings.Join(meta, " · ")))
		b.WriteString("</p>")
	}
	if b.Len() == 0 && plain != "" {
		b.WriteString("<p>")
		b.WriteString(html.EscapeString(plain))
		b.WriteString("</p>")
	}
	return b.String()
}

// telegramPlainText flattens post HTML while keeping paragraph breaks: the
// body is authored text, and collapsing every <br> to a space would turn a
// structured announcement into one line.
func telegramPlainText(raw string) string {
	if raw == "" {
		return ""
	}
	s := brTag.ReplaceAllString(raw, "\n")
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

// telegramTitle uses the whole caption. Telegram has no separate title field;
// cutting the first line here would be the only place where data gets shorter.
func telegramTitle(plain, media string) string {
	if text := strings.TrimSpace(plain); text != "" {
		return text
	}
	switch media {
	case "photo":
		return "Photo post"
	case "video":
		return "Video post"
	case "document":
		return "Document post"
	case "audio":
		return "Audio post"
	case "sticker":
		return "Sticker"
	case "poll":
		return "Poll"
	case "location":
		return "Location"
	case "contact":
		return "Contact"
	}
	return "(no content)"
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func telegramError(err error, channel string) error {
	if err == nil {
		return nil
	}
	var we *apperr.Error
	if errors.As(err, &we) {
		return err
	}
	switch {
	case errors.Is(err, telegram.ErrInvalidChannel):
		return &apperr.Error{Code: apperr.CodeBadRequest, Message: "invalid telegram channel " + channel, Err: err}
	case errors.Is(err, telegram.ErrNotFound):
		return &apperr.Error{Code: apperr.CodeUserNotFound, Message: "telegram channel " + channel + " not found", Err: err}
	case errors.Is(err, telegram.ErrNoPreview):
		return &apperr.Error{Code: apperr.CodeUserNotFound, Message: "telegram channel " + channel + " is not available (not found or web preview disabled)", Err: err}
	case errors.Is(err, telegram.ErrRateLimited):
		return &apperr.Error{Code: apperr.CodeRateLimited, Message: "telegram rate limited", Err: err}
	case errors.Is(err, telegram.ErrNoPosts):
		return &apperr.Error{Code: apperr.CodeUpstreamUnavailable, Message: "telegram channel " + channel + " has no visible posts", Err: err}
	case errors.Is(err, context.DeadlineExceeded):
		return &apperr.Error{Code: apperr.CodeTimeout, Message: "telegram preview timed out", Err: err}
	case errors.Is(err, telegram.ErrUnexpectedPage):
		return &apperr.Error{Code: apperr.CodeUpstreamUnavailable, Message: "telegram preview page could not be parsed", Err: err}
	default:
		return &apperr.Error{Code: apperr.CodeInternal, Message: err.Error(), Err: err}
	}
}
