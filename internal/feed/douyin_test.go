package feed

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"strings"
	"testing"
	"time"

	"github.com/yilong-musk/feedmesh/internal/apperr"
	"github.com/yilong-musk/feedmesh/internal/cache"
	"github.com/yilong-musk/feedmesh/internal/douyin"
	"github.com/yilong-musk/feedmesh/internal/logx"
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
	if f.Items[1].Download == nil || !f.Items[1].Download.IsTop {
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

// countingDouyin records the width upstream was asked for and answers only that
// many videos, the way the real client truncates one upstream page.
type countingDouyin struct {
	tl    *douyin.Timeline
	calls []int
}

func (c *countingDouyin) Timeline(_ context.Context, _ string, limit int) (*douyin.Timeline, error) {
	c.calls = append(c.calls, limit)
	out := *c.tl
	if limit > 0 && limit < len(out.Items) {
		out.Items = out.Items[:limit]
	}
	return &out, nil
}

// TestGetDouyinCacheWidens pins the cache contract: a narrow first read must
// not narrow later reads for the whole TTL, and a cache hit of the same width
// must not touch upstream at all.
func TestGetDouyinCacheWidens(t *testing.T) {
	c, err := cache.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	stub := &countingDouyin{tl: douyinTimeline()}
	svc := NewService(Clients{Douyin: stub}, c, logx.Discard(), DefaultTTLs())
	ctx := context.Background()
	const secUID = "MS4wLjABAAAAtestsecuid0000000000"

	narrow, err := svc.GetDouyin(ctx, secUID, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(narrow.Items) != 1 {
		t.Fatalf("narrow items = %d", len(narrow.Items))
	}

	wide, err := svc.GetDouyin(ctx, secUID, 2, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(wide.Items) != 2 {
		t.Fatalf("wide items = %d, want the widened cache to answer 2", len(wide.Items))
	}

	// Same width again: answered from the cache, no third upstream read.
	if _, err := svc.GetDouyin(ctx, secUID, 2, false); err != nil {
		t.Fatal(err)
	}
	if len(stub.calls) != 2 || stub.calls[0] != 1 || stub.calls[1] != 2 {
		t.Fatalf("upstream widths = %v, want [1 2]", stub.calls)
	}
	e, ok := readCachedList[douyin.Item](svc, "douyin:user:"+secUID, 2, douyinTTL)
	if !ok || len(e.Items) != 2 || e.Want != 2 {
		t.Fatalf("cache entry = %+v, ok = %v", e, ok)
	}
}

// TestDouyinLegacyCacheEntryDecodes keeps a cache written by the previous
// release readable: the entry had sec_uid/author at the top level and no Want,
// so the new code must still find the items and treat the entry as narrow.
func TestDouyinLegacyCacheEntryDecodes(t *testing.T) {
	const raw = `{"sec_uid":"MS4wLjABAAAAtestsecuid0000000000","author":"Test Creator",
		"fetched_at":"2026-11-16T07:09:03Z",
		"items":[{"ID":"7002","Desc":"old caption","Author":"Test Creator",
		          "SecUID":"MS4wLjABAAAAtestsecuid0000000000","CreatedAt":"2026-11-09T11:08:58Z"}]}`
	var e cachedList[douyin.Item]
	if err := json.Unmarshal([]byte(raw), &e); err != nil {
		t.Fatalf("legacy entry no longer decodes: %v", err)
	}
	if len(e.Items) != 1 || e.Items[0].ID != "7002" || e.FetchedAt.IsZero() {
		t.Fatalf("legacy entry = %+v", e)
	}
	if e.covers(2) {
		t.Fatal("a one-item legacy entry must not claim to cover a wider read")
	}
}

func TestGetDouyinErrors(t *testing.T) {
	svc := NewService(Clients{Douyin: stubDouyin{err: douyin.ErrInvalidUser}}, nil, logx.Discard(), DefaultTTLs())
	_, err := svc.GetDouyin(context.Background(), "bad user", 10, false)
	if err == nil {
		t.Fatal("expected an error")
	}
	if code := apperr.CodeOf(err); code != apperr.CodeBadRequest {
		t.Fatalf("code = %s", code)
	}

	svc = NewService(Clients{Douyin: stubDouyin{err: douyin.ErrNotReady}}, nil, logx.Discard(), DefaultTTLs())
	_, err = svc.GetDouyin(context.Background(), "MS4wLjABAAAAtestsecuid0000000000", 10, false)
	if code := apperr.CodeOf(err); code != apperr.CodeUpstreamUnavailable {
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
	const secUID = "MS4wLjABAAAAtestsecuid0000000000"
	f := douyinFeed(secUID, cachedList[douyin.Item]{
		FetchedAt: time.Unix(1800000100, 0).UTC(),
		Items: []douyin.Item{{
			ID:         "7706127120845719717",
			Desc:       caption,
			Author:     "Test Creator",
			SecUID:     secUID,
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
