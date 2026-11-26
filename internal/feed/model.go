// Package feed turns provider responses into a unified feed model and renders
// it as RSS 2.0. Everything in this file is pure: no network, no disk.
package feed

import (
	"time"
)

// Video describes a video attached to an item or a quoted post.
type Video struct {
	URL        string `json:"url,omitempty"`
	Thumbnail  string `json:"thumbnail,omitempty"`
	DurationMS int    `json:"duration_ms,omitempty"`
	Views      int64  `json:"views,omitempty"`
}

// DownloadMedia holds short-lived video rendition details consumed by the
// dedicated downloader endpoints (/api/v1/douyin/timeline, /api/v1/tiktok/timeline).
// PlayURL is signed and expires within hours, so it stays out of generic feed
// JSON and RSS.
type DownloadMedia struct {
	PlayURL    string
	DurationMS int
	Width      int
	Height     int
	IsTop      bool
}

// ViewsExtra carries an upstream play or view counter for providers that expose
// one (YouTube, Bilibili, Telegram).
type ViewsExtra struct {
	Views int64 `json:"views"`
}

// Item is one entry normalised for RSS and JSON consumers. Provider-specific
// fields live in Extra (a typed struct defined by the provider) so Item stays
// stable as new platforms are added.
type Item struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Author    string    `json:"author,omitempty"`
	Text      string    `json:"text"`
	HTML      string    `json:"html"`
	URL       string    `json:"url"`
	Images    []string  `json:"images"`
	Video     *Video    `json:"video,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	// Extra carries provider-specific item metadata and is omitted when nil.
	Extra any `json:"extra,omitempty"`
	// Download carries short-lived rendition details for downloader endpoints.
	Download *DownloadMedia `json:"-"`
}

// Feed is a rendered feed from one provider.
type Feed struct {
	UID         string
	ScreenName  string
	Description string
	FetchedAt   time.Time
	Cached      bool
	// Degraded marks a feed served from the last good cache entry because the
	// upstream read failed.
	Degraded bool
	// Source names where the list came from when a provider has more than one
	// upstream (YouTube: atom, browse, stale; Reddit: atom, stale).
	Source string
	Items  []Item
	// TitleText/LinkText override the account-derived channel title and link.
	TitleText string
	LinkText  string
	// TTL overrides the channel <ttl> in minutes; zero means the default 15.
	TTL int
	// Language is the BCP 47 tag of the upstream content, published as the
	// channel <language>. Providers whose content language is not known up
	// front leave it empty and the element is omitted.
	Language string
}

// langZhCN tags providers whose upstream content is Simplified Chinese. It
// describes the data, not the service's own output language.
const langZhCN = "zh-cn"

// Title returns the human-facing feed title.
func (f *Feed) Title() string {
	if f == nil {
		return ""
	}
	if f.TitleText != "" {
		return f.TitleText
	}
	if f.ScreenName == "" {
		return "Weibo " + f.UID
	}
	return f.ScreenName + " on Weibo"
}

// Link returns the canonical channel link.
func (f *Feed) Link() string {
	if f == nil {
		return ""
	}
	if f.LinkText != "" {
		return f.LinkText
	}
	return "https://m.weibo.cn/u/" + f.UID
}
