package feed

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/yilong-musk/feedmesh/internal/apperr"
	"github.com/yilong-musk/feedmesh/internal/bilibili"
	"github.com/yilong-musk/feedmesh/internal/cache"
	"github.com/yilong-musk/feedmesh/internal/logx"
)

const testBilibiliMid = "946974"

type stubBilibili struct {
	tl  *bilibili.Timeline
	err error
}

func (s stubBilibili) Timeline(_ context.Context, _ string, _ int) (*bilibili.Timeline, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.tl, nil
}

func bilibiliTimeline() *bilibili.Timeline {
	return &bilibili.Timeline{
		Mid:       testBilibiliMid,
		Author:    "影视飓风",
		Fans:      19100953,
		FetchedAt: time.Unix(1800000100, 0).UTC(),
		Videos: []bilibili.Video{
			{
				BVID:      "BV1kZWX62ENY",
				Title:     "为什么是最后一期？",
				Desc:      "简介 <b>不是 html</b>",
				Author:    "影视飓风",
				AuthorID:  testBilibiliMid,
				CreatedAt: time.Unix(1800000000, 0).UTC(),
				Duration:  19 * time.Minute,
				Cover:     "https://i2.hdslb.com/bfs/archive/a.jpg",
				Play:      6608624,
				Danmaku:   1234,
				Comment:   567,
				TypeName:  "影视杂谈",
			},
			{
				BVID:      "BV1NS4t6oEXQ",
				Title:     "第二期",
				Author:    "影视飓风",
				CreatedAt: time.Unix(1700000000, 0).UTC(),
				Duration:  59 * time.Second,
				Cover:     "javascript:alert(1)",
			},
		},
	}
}

func TestGetBilibiliBuildsFeed(t *testing.T) {
	svc := NewService(Clients{Bilibili: stubBilibili{tl: bilibiliTimeline()}}, nil, logx.Discard(), DefaultTTLs())
	f, err := svc.GetBilibili(context.Background(), testBilibiliMid, 10, false)
	if err != nil {
		t.Fatal(err)
	}
	if f.Title() != "影视飓风 on Bilibili" {
		t.Fatalf("title = %q", f.Title())
	}
	if f.Link() != "https://space.bilibili.com/"+testBilibiliMid {
		t.Fatalf("link = %q", f.Link())
	}
	if len(f.Items) != 2 {
		t.Fatalf("items = %d", len(f.Items))
	}
	first := f.Items[0]
	if first.URL != "https://www.bilibili.com/video/BV1kZWX62ENY" {
		t.Fatalf("url = %q", first.URL)
	}
	if first.Title != "为什么是最后一期？" {
		t.Fatalf("title = %q", first.Title)
	}
	if !strings.Contains(first.HTML, "&lt;b&gt;") {
		t.Fatalf("html not escaped: %s", first.HTML)
	}
	if !strings.Contains(first.HTML, `<img src="https://i2.hdslb.com/bfs/archive/a.jpg"`) {
		t.Fatalf("cover missing: %s", first.HTML)
	}
	for _, want := range []string{"Duration 19:00", "Play 6.6M", "Danmaku 1.2K", "Comment 567", "影视杂谈"} {
		if !strings.Contains(first.HTML, want) {
			t.Fatalf("metadata %q missing: %s", want, first.HTML)
		}
	}
	if vx, ok := first.Extra.(*ViewsExtra); !ok || vx.Views != 6608624 {
		t.Fatalf("extra = %#v, want views=6608624", first.Extra)
	}
	if len(f.Items[1].Images) != 0 {
		t.Fatalf("unsafe cover survived: %v", f.Items[1].Images)
	}
}

func TestGetBilibiliLimit(t *testing.T) {
	svc := NewService(Clients{Bilibili: stubBilibili{tl: bilibiliTimeline()}}, nil, logx.Discard(), DefaultTTLs())
	f, err := svc.GetBilibili(context.Background(), testBilibiliMid, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Items) != 1 {
		t.Fatalf("items = %d", len(f.Items))
	}
}

type countingBilibili struct {
	tl    *bilibili.Timeline
	calls []int
}

func (c *countingBilibili) Timeline(_ context.Context, _ string, limit int) (*bilibili.Timeline, error) {
	c.calls = append(c.calls, limit)
	out := *c.tl
	if limit > 0 && limit < len(out.Videos) {
		out.Videos = out.Videos[:limit]
	}
	return &out, nil
}

func TestGetBilibiliCacheWidens(t *testing.T) {
	c, err := cache.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	stub := &countingBilibili{tl: bilibiliTimeline()}
	svc := NewService(Clients{Bilibili: stub}, c, logx.Discard(), DefaultTTLs())
	ctx := context.Background()

	narrow, err := svc.GetBilibili(ctx, testBilibiliMid, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(narrow.Items) != 1 {
		t.Fatalf("narrow items = %d", len(narrow.Items))
	}
	wide, err := svc.GetBilibili(ctx, testBilibiliMid, 2, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(wide.Items) != 2 {
		t.Fatalf("wide items = %d", len(wide.Items))
	}
	if _, err := svc.GetBilibili(ctx, testBilibiliMid, 2, false); err != nil {
		t.Fatal(err)
	}
	if len(stub.calls) != 2 || stub.calls[0] != 1 || stub.calls[1] != 2 {
		t.Fatalf("upstream widths = %v, want [1 2]", stub.calls)
	}
}

func TestGetBilibiliErrors(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"invalid", bilibili.ErrInvalidUser, apperr.CodeBadRequest},
		{"missing", bilibili.ErrNotFound, apperr.CodeUserNotFound},
		{"rate limited", bilibili.ErrRateLimited, apperr.CodeUpstreamBlocked},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := NewService(Clients{Bilibili: stubBilibili{err: tc.err}}, nil, logx.Discard(), DefaultTTLs())
			_, err := svc.GetBilibili(context.Background(), testBilibiliMid, 10, false)
			if err == nil {
				t.Fatal("expected an error")
			}
			if code := apperr.CodeOf(err); code != tc.want {
				t.Fatalf("code = %s, want %s", code, tc.want)
			}
		})
	}
}

func TestGetBilibiliMissingUser(t *testing.T) {
	svc := NewService(Clients{Bilibili: stubBilibili{tl: bilibiliTimeline()}}, nil, logx.Discard(), DefaultTTLs())
	_, err := svc.GetBilibili(context.Background(), "  ", 10, false)
	if err == nil || apperr.CodeOf(err) != apperr.CodeBadRequest {
		t.Fatalf("err = %v", err)
	}
}
