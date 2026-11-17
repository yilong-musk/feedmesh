package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/yilong-musk/feedmesh/internal/telegram"
)

type fakeTelegram struct{}

func (fakeTelegram) Fetch(_ context.Context, channel string, limit int) (*telegram.Channel, error) {
	created := time.Date(2026, 11, 9, 4, 20, 38, 0, time.UTC)
	ch := &telegram.Channel{
		Name:        channel,
		Title:       "sspai Channel",
		Description: "sspai official channel",
		Subscribers: "39.2K",
		FetchedAt:   time.Unix(1800000100, 0).UTC(),
		Posts: []telegram.Post{
			{
				Channel: channel, ID: 14874, CreatedAt: created.Add(4 * time.Hour),
				TextHTML:           `<b>Second post</b><br/><br/>Body line<br/><a href="https://sspai.com/post/2">https://sspai.com/post/2</a>`,
				Author:             "sspai Channel",
				Views:              3060,
				LinkURL:            "https://sspai.com/post/2",
				LinkSite:           "sspai",
				LinkTitle:          "Second headline",
				ChannelTitle:       "sspai Channel",
				ChannelDescription: "sspai official channel",
			},
			{
				Channel: channel, ID: 14873, CreatedAt: created,
				TextHTML:           `<b>First post</b>`,
				Author:             "sspai Channel",
				Signature:          "Author Sig",
				Views:              3050,
				Images:             []string{"https://cdn4.telesco.pe/file/photo.jpg"},
				Media:              "photo",
				ChannelTitle:       "sspai Channel",
				ChannelDescription: "sspai official channel",
			},
			{
				Channel: channel, ID: 14872, CreatedAt: created.Add(-time.Hour),
				Media:              "video",
				PosterURL:          "https://cdn4.telesco.pe/file/video.jpg",
				ChannelTitle:       "sspai Channel",
				ChannelDescription: "sspai official channel",
			},
		},
	}
	if limit > 0 && limit < len(ch.Posts) {
		ch.Posts = ch.Posts[:limit]
	}
	return ch, nil
}

func TestTelegramRSSRoute(t *testing.T) {
	h := newTestServer(t)
	for _, path := range []string{"/rss/telegram/sspai", "/rss/telegram/sspai.xml"} {
		rec := get(t, h, path)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d body=%s", path, rec.Code, rec.Body.String())
		}
		if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/rss+xml") {
			t.Fatalf("%s content-type = %q", path, ct)
		}
		if got := rec.Header().Get("Cache-Control"); got != "public, max-age=600" {
			t.Fatalf("%s cache-control = %q", path, got)
		}
		body := rec.Body.String()
		for _, want := range []string{
			"<title>sspai Channel on Telegram</title>",
			"https://t.me/sspai/14874",
			"https://cdn4.telesco.pe/file/photo.jpg",
			"<title>First post</title>",
		} {
			if !strings.Contains(body, want) {
				t.Fatalf("%s body missing %q", path, want)
			}
		}
	}
}

func TestTelegramAPIRoute(t *testing.T) {
	h := newTestServer(t)
	rec := get(t, h, "/api/v1/telegram?channel=@sspai&limit=3")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Channel     string `json:"channel"`
		Title       string `json:"title"`
		Description string `json:"description"`
		Cached      bool   `json:"cached"`
		Items       []struct {
			ID        string    `json:"id"`
			Title     string    `json:"title"`
			Text      string    `json:"text"`
			URL       string    `json:"url"`
			HTML      string    `json:"html"`
			Images    []string  `json:"images"`
			Author    string    `json:"author"`
			Views     int64     `json:"views"`
			CreatedAt time.Time `json:"created_at"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Channel != "sspai" || resp.Title != "sspai Channel on Telegram" {
		t.Fatalf("envelope = %+v", resp)
	}
	if resp.Description != "sspai official channel" {
		t.Fatalf("description = %q", resp.Description)
	}
	if len(resp.Items) != 3 {
		t.Fatalf("items = %d", len(resp.Items))
	}
	item := resp.Items[0]
	if item.ID != "14874" || item.URL != "https://t.me/sspai/14874" {
		t.Fatalf("item = %+v", item)
	}
	if item.Views != 3060 {
		t.Fatalf("views = %d", item.Views)
	}
	if !item.CreatedAt.Equal(time.Date(2026, 11, 9, 8, 20, 38, 0, time.UTC)) {
		t.Fatalf("created_at = %v", item.CreatedAt)
	}
	if !strings.Contains(item.HTML, "<b>Second post</b>") {
		t.Fatalf("html = %s", item.HTML)
	}
	if resp.Items[1].Author != "Author Sig" || len(resp.Items[1].Images) != 1 {
		t.Fatalf("second item = %+v", resp.Items[1])
	}
	if resp.Items[1].Title != "First post" {
		t.Fatalf("photo caption title = %q", resp.Items[1].Title)
	}
	if resp.Items[2].Title != "Video post" {
		t.Fatalf("media-only title = %q", resp.Items[2].Title)
	}
}

func TestTelegramCachedSecondRead(t *testing.T) {
	h := newTestServer(t)
	first := get(t, h, "/api/v1/telegram?channel=sspai&limit=1")
	if first.Code != http.StatusOK {
		t.Fatalf("first status = %d", first.Code)
	}
	second := get(t, h, "/api/v1/telegram?channel=sspai&limit=1")
	if second.Code != http.StatusOK {
		t.Fatalf("second status = %d", second.Code)
	}
	var resp struct {
		Cached bool `json:"cached"`
	}
	if err := json.Unmarshal(second.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.Cached {
		t.Fatal("second read was not served from cache")
	}
}

func TestTelegramMissingChannel(t *testing.T) {
	h := newTestServer(t)
	for _, path := range []string{"/rss/telegram/not%20a%20channel", "/api/v1/telegram?channel="} {
		rec := get(t, h, path)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s status = %d body=%s", path, rec.Code, rec.Body.String())
		}
		if got := rec.Header().Get("X-Feedmesh-Code"); got != "BAD_REQUEST" {
			t.Fatalf("%s error code = %q", path, got)
		}
	}
}
