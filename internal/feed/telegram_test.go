package feed

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/yilong-musk/feedmesh/internal/apperr"
	"github.com/yilong-musk/feedmesh/internal/cache"
	"github.com/yilong-musk/feedmesh/internal/logx"
	"github.com/yilong-musk/feedmesh/internal/telegram"
)

const testTelegramChannel = "sspai"

func telegramChannel() *telegram.Channel {
	created := time.Date(2026, 11, 9, 4, 20, 38, 0, time.UTC)
	return &telegram.Channel{
		Name:        testTelegramChannel,
		Title:       "少数派sspai",
		Description: "少数派 (sspai.com) 官方频道",
		Subscribers: "39.2K",
		FetchedAt:   time.Unix(1800000100, 0).UTC(),
		Posts: []telegram.Post{
			{
				Channel: testTelegramChannel, ID: 14874, CreatedAt: created.Add(4 * time.Hour),
				TextHTML:           `第二篇 <b>标题</b><br/><br/>正文段落<br/><a href="https://sspai.com/post/2">https://sspai.com/post/2</a>`,
				Author:             "少数派sspai",
				Views:              3060,
				LinkURL:            "https://sspai.com/post/2",
				LinkSite:           "少数派",
				LinkTitle:          "第二篇标题",
				LinkText:           "卡片摘要",
				LinkImage:          "https://cdn4.telesco.pe/file/card.jpg",
				ChannelTitle:       "少数派sspai",
				ChannelDescription: "少数派 (sspai.com) 官方频道",
				ChannelSubscribers: "39.2K",
			},
			{
				Channel: testTelegramChannel, ID: 14873, CreatedAt: created,
				TextHTML:           `第一篇`,
				Author:             "少数派sspai",
				Signature:          "水獭Store",
				Views:              3050,
				Images:             []string{"https://cdn4.telesco.pe/file/photo.jpg", "javascript:alert(1)"},
				PosterURL:          "https://cdn4.telesco.pe/file/poster.jpg",
				Media:              "photo",
				ForwardedFrom:      "少数派编辑部",
				ChannelTitle:       "少数派sspai",
				ChannelDescription: "少数派 (sspai.com) 官方频道",
				ChannelSubscribers: "39.2K",
			},
			{
				Channel: testTelegramChannel, ID: 14872, CreatedAt: created.Add(-time.Hour),
				Media:              "video",
				PosterURL:          "https://cdn4.telesco.pe/file/video.jpg",
				ChannelTitle:       "少数派sspai",
				ChannelDescription: "少数派 (sspai.com) 官方频道",
			},
		},
	}
}

type stubTelegram struct {
	ch   *telegram.Channel
	err  error
	call int
}

func (s *stubTelegram) Fetch(_ context.Context, _ string, limit int) (*telegram.Channel, error) {
	s.call++
	if s.err != nil {
		return nil, s.err
	}
	out := *s.ch
	if limit > 0 && limit < len(out.Posts) {
		out.Posts = out.Posts[:limit]
	}
	return &out, nil
}

func TestGetTelegramBuildsFeed(t *testing.T) {
	svc := NewService(Clients{Telegram: &stubTelegram{ch: telegramChannel()}}, nil, logx.Discard(), DefaultTTLs())
	f, err := svc.GetTelegram(context.Background(), "@sspai", 10, false)
	if err != nil {
		t.Fatal(err)
	}
	if f.Title() != "少数派sspai on Telegram" {
		t.Fatalf("title = %q", f.Title())
	}
	if f.Link() != "https://t.me/sspai" {
		t.Fatalf("link = %q", f.Link())
	}
	if f.Description != "少数派 (sspai.com) 官方频道" {
		t.Fatalf("description = %q", f.Description)
	}
	if len(f.Items) != 3 {
		t.Fatalf("items = %d", len(f.Items))
	}
	first := f.Items[0]
	if first.ID != "14874" || first.URL != "https://t.me/sspai/14874" {
		t.Fatalf("first = %+v", first)
	}
	if first.CreatedAt.Location() != time.UTC {
		t.Fatalf("created_at is not UTC: %s", first.CreatedAt)
	}
	if !strings.Contains(first.Text, "\n") {
		t.Fatalf("plain text lost paragraph breaks: %q", first.Text)
	}
	if !strings.Contains(first.HTML, "<b>标题</b>") || !strings.Contains(first.HTML, `href="https://sspai.com/post/2"`) {
		t.Fatalf("body html missing: %s", first.HTML)
	}
	if !strings.Contains(first.HTML, "第二篇标题") || !strings.Contains(first.HTML, "卡片摘要") {
		t.Fatalf("link card missing: %s", first.HTML)
	}
	if first.Title != strings.TrimSpace(first.Text) {
		t.Fatalf("title does not keep the whole caption: %q", first.Title)
	}
	second := f.Items[1]
	if second.Author != "水獭Store" {
		t.Fatalf("author = %q", second.Author)
	}
	if len(second.Images) != 1 || second.Images[0] != "https://cdn4.telesco.pe/file/photo.jpg" {
		t.Fatalf("unsafe image survived: %v", second.Images)
	}
	if !strings.Contains(second.HTML, "Forwarded from 少数派编辑部") {
		t.Fatalf("forward label missing: %s", second.HTML)
	}
	if !strings.Contains(second.HTML, "3.0K views") {
		t.Fatalf("view counter missing: %s", second.HTML)
	}
	third := f.Items[2]
	if third.Title != "Video post" {
		t.Fatalf("media-only title = %q", third.Title)
	}
	if third.Video == nil || third.Video.Thumbnail != "https://cdn4.telesco.pe/file/video.jpg" {
		t.Fatalf("video = %#v", third.Video)
	}
}

func TestGetTelegramRSS(t *testing.T) {
	svc := NewService(Clients{Telegram: &stubTelegram{ch: telegramChannel()}}, nil, logx.Discard(), DefaultTTLs())
	f, err := svc.GetTelegram(context.Background(), testTelegramChannel, 10, false)
	if err != nil {
		t.Fatal(err)
	}
	f.TTL = 10 * 60
	out := string(RenderRSS(f))
	for _, want := range []string{
		"<title>少数派sspai on Telegram</title>",
		"<media:thumbnail url=\"https://cdn4.telesco.pe/file/photo.jpg\" />",
		"<pubDate>Mon, 09 Nov 2026 08:20:38 +0000</pubDate>",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("RSS missing %q:\n%s", want, out)
		}
	}
}

func TestTelegramPlainTextKeepsParagraphs(t *testing.T) {
	got := telegramPlainText(`第一行<br/><br/>第二行 <b>加粗</b> 尾巴<br><a href="https://x.test">链接</a>`)
	want := "第一行\n\n第二行 加粗 尾巴\n链接"
	if got != want {
		t.Fatalf("plain text = %q, want %q", got, want)
	}
}

func TestGetTelegramLimitAndCacheWiden(t *testing.T) {
	c, err := cache.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	stub := &stubTelegram{ch: telegramChannel()}
	svc := NewService(Clients{Telegram: stub}, c, logx.Discard(), DefaultTTLs())
	ctx := context.Background()
	narrow, err := svc.GetTelegram(ctx, testTelegramChannel, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(narrow.Items) != 1 {
		t.Fatalf("narrow items = %d", len(narrow.Items))
	}
	wide, err := svc.GetTelegram(ctx, testTelegramChannel, 3, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(wide.Items) != 3 {
		t.Fatalf("wide items = %d", len(wide.Items))
	}
	again, err := svc.GetTelegram(ctx, testTelegramChannel, 2, false)
	if err != nil {
		t.Fatal(err)
	}
	if !again.Cached {
		t.Fatal("second read was not served from cache")
	}
	if len(again.Items) != 2 {
		t.Fatalf("cached items = %d", len(again.Items))
	}
	if stub.call != 2 {
		t.Fatalf("upstream calls = %d, want 2", stub.call)
	}
}

func TestGetTelegramErrors(t *testing.T) {
	cases := []struct {
		name string
		in   string
		err  error
		want string
	}{
		{"invalid name", "not a channel", nil, apperr.CodeBadRequest},
		{"missing", testTelegramChannel, telegram.ErrNotFound, apperr.CodeUserNotFound},
		{"no preview", testTelegramChannel, telegram.ErrNoPreview, apperr.CodeUserNotFound},
		{"rate limited", testTelegramChannel, telegram.ErrRateLimited, apperr.CodeRateLimited},
		{"no posts", testTelegramChannel, telegram.ErrNoPosts, apperr.CodeUpstreamUnavailable},
		{"garbage", testTelegramChannel, telegram.ErrUnexpectedPage, apperr.CodeUpstreamUnavailable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := NewService(Clients{Telegram: &stubTelegram{err: tc.err}}, nil, logx.Discard(), DefaultTTLs())
			_, err := svc.GetTelegram(context.Background(), tc.in, 10, false)
			if err == nil {
				t.Fatal("expected an error")
			}
			if code := apperr.CodeOf(err); code != tc.want {
				t.Fatalf("code = %s, want %s (err %v)", code, tc.want, err)
			}
		})
	}
}
