package feed

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/yilong-musk/feedmesh/internal/apperr"
	"github.com/yilong-musk/feedmesh/internal/apple"
	"github.com/yilong-musk/feedmesh/internal/cache"
	"github.com/yilong-musk/feedmesh/internal/logx"
)

type fakeApple struct {
	episodes []apple.Episode
	chart    []apple.ChartEntry
	err      error
	calls    int
}

func (f *fakeApple) Episodes(context.Context, string, int) (*apple.Show, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return &apple.Show{ID: "1200361736", Title: "The Daily", Episodes: f.episodes}, nil
}

func (f *fakeApple) Chart(context.Context, string, int) (*apple.Chart, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return &apple.Chart{Country: "us", Title: "Top Shows", Entries: f.chart}, nil
}

func testAppleService(t *testing.T, client *fakeApple) *Service {
	t.Helper()
	c, err := cache.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return NewService(Clients{Apple: client}, c, logx.Discard(), DefaultTTLs())
}

func TestGetAppleBuildsFeed(t *testing.T) {
	when := time.Date(2026, 11, 27, 11, 0, 0, 0, time.UTC)
	client := &fakeApple{episodes: []apple.Episode{{
		ID: "new", Title: "Newer", Summary: "<p>Hello</p>", URL: "https://www.nytimes.com/newer",
		EnclosureURL: "https://cdn.example/new.mp3", EnclosureType: "audio/mpeg", EnclosureLength: 2048,
		Image: "https://img.example/ep.jpg", Published: when, Duration: 62 * time.Minute,
		Author: "The New York Times", ShowTitle: "The Daily", ShowAuthor: "The New York Times",
		ShowURL: "https://podcasts.apple.com/us/podcast/the-daily/id1200361736", Language: "en-us",
	}}}
	svc := testAppleService(t, client)
	f, err := svc.GetApple(context.Background(), "https://podcasts.apple.com/us/podcast/the-daily/id1200361736", 10, false)
	if err != nil {
		t.Fatal(err)
	}
	if f.Title() != "The Daily on Apple Podcasts" || f.Language != "en-us" || len(f.Items) != 1 {
		t.Fatalf("feed = %+v", f)
	}
	it := f.Items[0]
	if it.Video == nil || it.Video.MIME != "audio/mpeg" || it.Video.Bytes != 2048 || it.Video.DurationMS != 62*60*1000 {
		t.Fatalf("video = %+v", it.Video)
	}
	extra, ok := it.Extra.(*AppleExtra)
	if !ok || extra.Duration != 62*60 {
		t.Fatalf("extra = %#v", it.Extra)
	}
	body := string(RenderRSS(f))
	for _, want := range []string{
		`<enclosure url="https://cdn.example/new.mp3" type="audio/mpeg" length="2048" />`,
		`medium="audio"`,
		"<title>Newer</title>",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("rss missing %s\n%s", want, body)
		}
	}
	if _, err := svc.GetApple(context.Background(), "1200361736", 10, false); err != nil {
		t.Fatal(err)
	}
	if client.calls != 1 {
		t.Fatalf("calls = %d, want a cache hit", client.calls)
	}
}

func TestGetAppleChart(t *testing.T) {
	client := &fakeApple{chart: []apple.ChartEntry{{
		ID: "1", Name: "One", Artist: "A", URL: "https://podcasts.apple.com/us/podcast/one/id1", Genre: "News",
	}}}
	svc := testAppleService(t, client)
	f, err := svc.GetAppleChart(context.Background(), "US", 10, false)
	if err != nil {
		t.Fatal(err)
	}
	if f.Title() != "Top podcasts (US)" || f.Items[0].Title != "One" || f.Items[0].URL == "" {
		t.Fatalf("feed = %+v items=%+v", f, f.Items)
	}
}

func TestGetAppleErrors(t *testing.T) {
	svc := testAppleService(t, &fakeApple{err: apple.ErrNotFound})
	_, err := svc.GetApple(context.Background(), "nope", 10, false)
	var we *apperr.Error
	if !errors.As(err, &we) || we.Code != apperr.CodeBadRequest {
		t.Fatalf("bad id: %v", err)
	}
	_, err = svc.GetApple(context.Background(), "1200361736", 10, false)
	if !errors.As(err, &we) || we.Code != apperr.CodeUserNotFound {
		t.Fatalf("missing: %v", err)
	}
}
