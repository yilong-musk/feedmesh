package feed

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/yilong-musk/feedmesh/internal/cache"
	"github.com/yilong-musk/feedmesh/internal/logx"
	"github.com/yilong-musk/feedmesh/internal/tiktok"
	"github.com/yilong-musk/feedmesh/internal/weibo"
)

const testTikTokSecUID = "MS4wLjABAAAAtestsecuid0000000000"

type stubTikTok struct {
	tl  *tiktok.Timeline
	err error
}

func (s stubTikTok) Timeline(_ context.Context, _ string, _ int) (*tiktok.Timeline, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.tl, nil
}

func tiktokTimeline() *tiktok.Timeline {
	return &tiktok.Timeline{
		SecUID:    testTikTokSecUID,
		Author:    "nasa",
		Nickname:  "NASA",
		FetchedAt: time.Unix(1800000100, 0).UTC(),
		Items: []tiktok.Item{
			{
				ID:         "7708408003065580831",
				Desc:       "first <b>caption</b>",
				Author:     "nasa",
				Nickname:   "NASA",
				SecUID:     testTikTokSecUID,
				CreatedAt:  time.Unix(1800000000, 0).UTC(),
				DurationMS: 79000,
				Width:      1080,
				Height:     1920,
				PlayURL:    "https://v16-webapp-prime.tiktok.com/a.mp4",
				CoverURL:   "https://p16-common-sign.tiktokcdn.com/c.jpeg",
				PlayCount:  275700,
				DiggCount:  2773,
				CommentCnt: 950,
				ShareCount: 292,
				MusicTitle: "original sound",
				MusicBy:    "NASA",
			},
			{
				ID:         "7704774145829063967",
				Desc:       "photo post",
				Author:     "nasa",
				Nickname:   "NASA",
				SecUID:     testTikTokSecUID,
				CreatedAt:  time.Unix(1700000000, 0).UTC(),
				IsImage:    true,
				CoverURL:   "javascript:alert(1)",
				IsTop:      true,
				PlayCount:  1200,
				DiggCount:  100,
				CommentCnt: 12,
				ShareCount: 3,
			},
		},
	}
}

func TestGetTikTokBuildsFeed(t *testing.T) {
	svc := NewService(Clients{TikTok: stubTikTok{tl: tiktokTimeline()}}, nil, logx.Discard(), DefaultTTLs())
	f, err := svc.GetTikTok(context.Background(), "@nasa", 10, false)
	if err != nil {
		t.Fatal(err)
	}
	if f.Title() != "NASA on TikTok" {
		t.Fatalf("title = %q", f.Title())
	}
	if f.Link() != "https://www.tiktok.com/@nasa" {
		t.Fatalf("link = %q", f.Link())
	}
	if len(f.Items) != 2 {
		t.Fatalf("items = %d", len(f.Items))
	}
	first := f.Items[0]
	if first.URL != "https://www.tiktok.com/@nasa/video/7708408003065580831" {
		t.Fatalf("url = %q", first.URL)
	}
	if first.Title != "first <b>caption</b>" {
		t.Fatalf("title = %q", first.Title)
	}
	if !strings.Contains(first.HTML, "&lt;b&gt;") {
		t.Fatalf("html not escaped: %s", first.HTML)
	}
	if !strings.Contains(first.HTML, `<img src="https://p16-common-sign.tiktokcdn.com/c.jpeg"`) {
		t.Fatalf("cover missing: %s", first.HTML)
	}
	for _, want := range []string{"1:19", "1080x1920", "Play 275.7K", "Like 2.8K", "Comment 950", "Share 292", "Music original sound"} {
		if !strings.Contains(first.HTML, want) {
			t.Fatalf("metadata %q missing: %s", want, first.HTML)
		}
	}
	if len(first.Images) != 1 || first.Images[0] != "https://p16-common-sign.tiktokcdn.com/c.jpeg" {
		t.Fatalf("images = %v", first.Images)
	}
	if first.PlayURL == "" || first.DurationMS != 79000 || first.Width != 1080 {
		t.Fatalf("timeline fields lost: %+v", first)
	}
	if !f.Items[1].IsTop {
		t.Fatal("pinned marker lost")
	}
	// A javascript: cover must never reach the feed.
	if len(f.Items[1].Images) != 0 {
		t.Fatalf("unsafe cover survived: %v", f.Items[1].Images)
	}
}

func TestGetTikTokLimit(t *testing.T) {
	svc := NewService(Clients{TikTok: stubTikTok{tl: tiktokTimeline()}}, nil, logx.Discard(), DefaultTTLs())
	f, err := svc.GetTikTok(context.Background(), "nasa", 1, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Items) != 1 {
		t.Fatalf("items = %d", len(f.Items))
	}
}

// countingTikTok answers only the width it was asked for, the way the real
// client stops at the requested limit.
type countingTikTok struct {
	tl    *tiktok.Timeline
	calls []int
}

func (c *countingTikTok) Timeline(_ context.Context, _ string, limit int) (*tiktok.Timeline, error) {
	c.calls = append(c.calls, limit)
	out := *c.tl
	if limit > 0 && limit < len(out.Items) {
		out.Items = out.Items[:limit]
	}
	return &out, nil
}

func TestGetTikTokCacheWidens(t *testing.T) {
	c, err := cache.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	stub := &countingTikTok{tl: tiktokTimeline()}
	svc := NewService(Clients{TikTok: stub}, c, logx.Discard(), DefaultTTLs())
	ctx := context.Background()

	narrow, err := svc.GetTikTok(ctx, "nasa", 1, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(narrow.Items) != 1 {
		t.Fatalf("narrow items = %d", len(narrow.Items))
	}
	wide, err := svc.GetTikTok(ctx, "nasa", 2, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(wide.Items) != 2 {
		t.Fatalf("wide items = %d, want the widened cache to answer 2", len(wide.Items))
	}
	if _, err := svc.GetTikTok(ctx, "nasa", 2, false); err != nil {
		t.Fatal(err)
	}
	if len(stub.calls) != 2 || stub.calls[0] != 1 || stub.calls[1] != 2 {
		t.Fatalf("upstream widths = %v, want [1 2]", stub.calls)
	}
}

func TestGetTikTokErrors(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"invalid", tiktok.ErrInvalidUser, weibo.CodeBadRequest},
		{"missing", tiktok.ErrNotFound, weibo.CodeUserNotFound},
		{"private", tiktok.ErrPrivate, weibo.CodeUserNotFound},
		{"challenge", tiktok.ErrChallenge, weibo.CodeUpstreamUnavailable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := NewService(Clients{TikTok: stubTikTok{err: tc.err}}, nil, logx.Discard(), DefaultTTLs())
			_, err := svc.GetTikTok(context.Background(), "nasa", 10, false)
			if err == nil {
				t.Fatal("expected an error")
			}
			if code := weibo.CodeOf(err); code != tc.want {
				t.Fatalf("code = %s, want %s", code, tc.want)
			}
		})
	}
}

func TestGetTikTokMissingUser(t *testing.T) {
	svc := NewService(Clients{TikTok: stubTikTok{tl: tiktokTimeline()}}, nil, logx.Discard(), DefaultTTLs())
	_, err := svc.GetTikTok(context.Background(), "   ", 10, false)
	if err == nil || weibo.CodeOf(err) != weibo.CodeBadRequest {
		t.Fatalf("err = %v", err)
	}
}

func TestTikTokTitleKeepsWholeCaption(t *testing.T) {
	long := strings.Repeat("很长的文案", 40)
	if got := tiktokTitle(long); got != long {
		t.Fatalf("caption was truncated: %d -> %d runes", len([]rune(long)), len([]rune(got)))
	}
	if got := tiktokTitle("  "); got != "(no content)" {
		t.Fatalf("empty caption = %q", got)
	}
}
