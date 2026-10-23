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
}

// Title returns the human-facing feed title.
func (f *Feed) Title() string {
	if f == nil {
		return ""
	}
	if f.ScreenName == "" {
		return "微博 " + f.UID
	}
	return f.ScreenName + " 的微博"
}

// Link returns the canonical mobile profile URL.
func (f *Feed) Link() string {
	if f == nil {
		return ""
	}
	return "https://m.weibo.cn/u/" + f.UID
}
