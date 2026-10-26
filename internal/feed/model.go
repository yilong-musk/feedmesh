// Package feed turns weibo posts into a stable feed model and renders it as
// RSS 2.0. Everything here is pure: no network, no disk.
package feed

import (
	"time"
)

// Item is one post, normalised for downstream consumers.
type Item struct {
	ID        string    `json:"id"`
	BID       string    `json:"bid"`
	CreatedAt time.Time `json:"created_at"`
	Text      string    `json:"text"`
	HTML      string    `json:"html"`
	URL       string    `json:"url"`
	Images    []string  `json:"images"`
	VideoURL  string    `json:"video_url,omitempty"`
	PageURL   string    `json:"page_url,omitempty"`
	Title     string    `json:"-"`
	// Author is the upstream poster name for providers that expose one. The
	// SMZDM JSON view publishes it; other providers leave it empty and keep
	// their existing output.
	Author string `json:"-"`
	// Rank is the upstream display rank (hot search only); it is deliberately
	// kept out of the generic item JSON.
	Rank int `json:"-"`
	// Heat is the upstream heat counter (hot search only). The hot JSON view
	// exposes it as a number so clients never have to parse display text.
	Heat int64 `json:"-"`
}

// Feed is a rendered timeline for one account.
type Feed struct {
	UID         string    `json:"uid"`
	ScreenName  string    `json:"screen_name"`
	Description string    `json:"description"`
	FetchedAt   time.Time `json:"fetched_at"`
	Cached      bool      `json:"cached"`
	Items       []Item    `json:"items"`
	// TitleText/LinkText override the account-derived channel title and link;
	// the hot search feed uses them because it has no account behind it.
	TitleText string `json:"-"`
	LinkText  string `json:"-"`
	// TTL overrides the channel <ttl> in minutes; zero means the default 15.
	TTL int `json:"-"`
}

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
