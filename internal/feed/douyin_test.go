package feed

import (
	"context"
	"encoding/xml"
	"strings"
	"testing"
	"time"

	"github.com/yilong-musk/feedmesh/internal/douyin"
	"github.com/yilong-musk/feedmesh/internal/logx"
	"github.com/yilong-musk/feedmesh/internal/weibo"
)

type stubDouyin struct {
	tl  *douyin.Timeline
	err error
}

func (s stubDouyin) Timeline(_ context.Context, secUID string, _ int) (*douyin.Timeline, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.tl, nil
}

func douyinTimeline() *douyin.Timeline {
	return &douyin.Timeline{
		SecUID:    "MS4wLjABAAAAtestsecuid0000000000",
		Author:    "Test Creator",
		FetchedAt: time.Unix(1800000100, 0).UTC(),
		Items: []douyin.Item{
			{
				ID:         "7002",
				Desc:       "newest video <b>not html</b>",
				Author:     "Test Creator",
				SecUID:     "MS4wLjABAAAAtestsecuid0000000000",
				CreatedAt:  time.Unix(1800000000, 0).UTC(),
				DurationMS: 93000,
				Width:      1080,
				Height:     1920,
				PlayURL:    "https://v3-dy-o.zjcdn.com/a.mp4",
				CoverURL:   "https://p3.douyinpic.com/c.jpeg",
			},
			{
				ID:         "7001",
				Desc:       "pinned older",
				Author:     "Test Creator",
				SecUID:     "MS4wLjABAAAAtestsecuid0000000000",
				CreatedAt:  time.Unix(1700000000, 0).UTC(),
				DurationMS: 15000,
				Width:      1080,
				Height:     1920,
				IsTop:      true,
			},
		},
	}
}

func TestGetDouyinBuildsFeed(t *testing.T) {
	svc := NewService(Clients{Douyin: stubDouyin{tl: douyinTimeline()}}, nil, logx.Discard(), DefaultTTLs())
	f, err := svc.GetDouyin(context.Background(), "MS4wLjABAAAAtestsecuid0000000000", 10, false)
	if err != nil {
		t.Fatal(err)
	}
	if f.Title() != "Test Creator on Douyin" {
		t.Fatalf("title = %q", f.Title())
	}
	if f.Link() != "https://www.douyin.com/user/MS4wLjABAAAAtestsecuid0000000000" {
		t.Fatalf("link = %q", f.Link())
	}
	if len(f.Items) != 2 {
		t.Fatalf("items = %d", len(f.Items))
	}
	first := f.Items[0]
	if first.URL != "https://www.douyin.com/video/7002" {
		t.Fatalf("url = %q", first.URL)
	}
	if first.Title != "newest video <b>not html</b>" {
		t.Fatalf("title = %q", first.Title)
	}
	if !strings.Contains(first.HTML, "&lt;b&gt;") {
		t.Fatalf("html not escaped: %s", first.HTML)
	}
	if !strings.Contains(first.HTML, "<img src=\"https://p3.douyinpic.com/c.jpeg\"") {
		t.Fatalf("cover missing: %s", first.HTML)
	}
	if !strings.Contains(first.HTML, "1:33") || !strings.Contains(first.HTML, "1080x1920") {
		t.Fatalf("metadata missing: %s", first.HTML)
	}
	if len(first.Images) != 1 || first.Images[0] != "https://p3.douyinpic.com/c.jpeg" {
		t.Fatalf("images = %v", first.Images)
	}
	if f.Items[1].IsTop != true {
		t.Fatal("pinned marker lost")
	}
}

func TestGetDouyinLimit(t *testing.T) {
	svc := NewService(Clients{Douyin: stubDouyin{tl: douyinTimeline()}}, nil, logx.Discard(), DefaultTTLs())
	f, err := svc.GetDouyin(context.Background(), "MS4wLjABAAAAtestsecuid0000000000", 1, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Items) != 1 {
		t.Fatalf("items = %d", len(f.Items))
	}
}

func TestGetDouyinErrors(t *testing.T) {
	svc := NewService(Clients{Douyin: stubDouyin{err: douyin.ErrInvalidUser}}, nil, logx.Discard(), DefaultTTLs())
	_, err := svc.GetDouyin(context.Background(), "bad user", 10, false)
	if err == nil {
		t.Fatal("expected an error")
	}
	if code := weibo.CodeOf(err); code != weibo.CodeBadRequest {
		t.Fatalf("code = %s", code)
	}

	svc = NewService(Clients{Douyin: stubDouyin{err: douyin.ErrNotReady}}, nil, logx.Discard(), DefaultTTLs())
	_, err = svc.GetDouyin(context.Background(), "MS4wLjABAAAAtestsecuid0000000000", 10, false)
	if code := weibo.CodeOf(err); code != weibo.CodeUpstreamUnavailable {
		t.Fatalf("code = %s", code)
	}
}

func TestDouyinTitle(t *testing.T) {
	if got := douyinTitle(""); got != "(no content)" {
		t.Fatalf("empty title = %q", got)
	}
	// The caption is the only text field Douyin offers, so nothing may be
	// dropped from it: no first-line rule and no length cap.
	const multiline = "first line\nsecond line"
	if got := douyinTitle(multiline); got != multiline {
		t.Fatalf("multiline title = %q", got)
	}
	long := strings.Repeat("x", 300)
	if got := douyinTitle(long); got != long {
		t.Fatalf("long title = %d runes, want 300", len([]rune(got)))
	}
}

// TestDouyinRSSKeepsWholeCaption pins the data contract end to end: a
// multi-line caption must survive into the rendered <title> and leave the
// document well-formed, since a reader may see only the title.
func TestDouyinRSSKeepsWholeCaption(t *testing.T) {
	const caption = "第一行标题\n第二行补充 #话题 #另一个话题"
	f := douyinFeed(cachedDouyin{
		SecUID:    "MS4wLjABAAAAtestsecuid0000000000",
		Author:    "Test Creator",
		FetchedAt: time.Unix(1800000100, 0).UTC(),
		Items: []douyin.Item{{
			ID:         "7706127120845719717",
			Desc:       caption,
			Author:     "Test Creator",
			CreatedAt:  time.Unix(1800000000, 0).UTC(),
			DurationMS: 93204,
			Width:      1080,
			Height:     1920,
		}},
	}, 10, false, false)

	if got := f.Items[0].Title; got != caption {
		t.Fatalf("feed title = %q, want the whole caption", got)
	}

	var doc struct {
		Channel struct {
			Items []struct {
				Title string `xml:"title"`
			} `xml:"item"`
		} `xml:"channel"`
	}
	raw := RenderRSS(f)
	if err := xml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("rendered RSS is not well-formed: %v\n%s", err, raw)
	}
	if len(doc.Channel.Items) != 1 {
		t.Fatalf("items = %d", len(doc.Channel.Items))
	}
	if got := doc.Channel.Items[0].Title; got != caption {
		t.Fatalf("RSS title = %q, want %q", got, caption)
	}
}
