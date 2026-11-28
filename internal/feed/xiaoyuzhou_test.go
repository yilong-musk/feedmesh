package feed

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/yilong-musk/feedmesh/internal/apperr"
	"github.com/yilong-musk/feedmesh/internal/cache"
	"github.com/yilong-musk/feedmesh/internal/logx"
	"github.com/yilong-musk/feedmesh/internal/xiaoyuzhou"
)

type fakeXiaoyuzhou struct {
	episodes []xiaoyuzhou.Episode
	err      error
	calls    int
}

func (f *fakeXiaoyuzhou) Episodes(context.Context, string, int) (*xiaoyuzhou.Show, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return &xiaoyuzhou.Show{ID: "6021f949a789fca4eff4492c", Title: "Sample Show", Episodes: f.episodes}, nil
}

func testXiaoyuzhouService(t *testing.T, client *fakeXiaoyuzhou) *Service {
	t.Helper()
	c, err := cache.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return NewService(Clients{Xiaoyuzhou: client}, c, logx.Discard(), DefaultTTLs())
}

func TestGetXiaoyuzhouBuildsFeed(t *testing.T) {
	when := time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC)
	client := &fakeXiaoyuzhou{episodes: []xiaoyuzhou.Episode{{
		ID: "bbbbbbbbbbbbbbbbbbbbbbbb", Title: "Newer", Summary: "<p>Hello</p>",
		URL:      "https://www.xiaoyuzhoufm.com/episode/bbbbbbbbbbbbbbbbbbbbbbbb",
		AudioURL: "https://media.example/new.m4a", Image: "https://image.example/ep.jpg",
		Published: when, Duration: 125 * time.Second, Plays: 40,
		ShowTitle: "Sample Show", ShowAuthor: "Host",
		ShowURL: "https://www.xiaoyuzhoufm.com/podcast/6021f949a789fca4eff4492c",
	}}}
	svc := testXiaoyuzhouService(t, client)
	f, err := svc.GetXiaoyuzhou(context.Background(), "https://www.xiaoyuzhoufm.com/podcast/6021F949A789FCA4EFF4492C", 10, false)
	if err != nil {
		t.Fatal(err)
	}
	if f.Title() != "Sample Show on Xiaoyuzhou" || f.Language != "zh-cn" || len(f.Items) != 1 {
		t.Fatalf("feed = %+v", f)
	}
	it := f.Items[0]
	if it.Video == nil || it.Video.MIME != "audio/mp4" || it.Video.DurationMS != 125000 {
		t.Fatalf("video = %+v", it.Video)
	}
	extra, ok := it.Extra.(*XiaoyuzhouExtra)
	if !ok || extra.Duration != 125 || extra.Views != 40 {
		t.Fatalf("extra = %#v", it.Extra)
	}
	body := string(RenderRSS(f))
	for _, want := range []string{
		`<enclosure url="https://media.example/new.m4a" type="audio/mp4" length="0" />`,
		`medium="audio"`,
		"<title>Newer</title>",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("rss missing %s\n%s", want, body)
		}
	}
	if _, err := svc.GetXiaoyuzhou(context.Background(), "6021f949a789fca4eff4492c", 10, false); err != nil {
		t.Fatal(err)
	}
	if client.calls != 1 {
		t.Fatalf("calls = %d, want a cache hit", client.calls)
	}
}

func TestGetXiaoyuzhouErrors(t *testing.T) {
	svc := testXiaoyuzhouService(t, &fakeXiaoyuzhou{err: xiaoyuzhou.ErrNotFound})
	_, err := svc.GetXiaoyuzhou(context.Background(), "nope", 10, false)
	var we *apperr.Error
	if !errors.As(err, &we) || we.Code != apperr.CodeBadRequest {
		t.Fatalf("bad id: %v", err)
	}
	_, err = svc.GetXiaoyuzhou(context.Background(), "6021f949a789fca4eff4492c", 10, false)
	if !errors.As(err, &we) || we.Code != apperr.CodeUserNotFound {
		t.Fatalf("missing: %v", err)
	}
}
