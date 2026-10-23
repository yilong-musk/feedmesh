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
		return "微博 " + f.UID
	}
	return f.ScreenName + " 的微博"
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
