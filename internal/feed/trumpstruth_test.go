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
	"github.com/yilong-musk/feedmesh/internal/trumpstruth"
)

type fakeTrumpTruth struct {
	res   *trumpstruth.Result
	err   error
	calls int
	limit int
}

func (f *fakeTrumpTruth) Latest(_ context.Context, limit int) (*trumpstruth.Result, error) {
	f.calls++
	f.limit = limit
	if f.err != nil {
		return nil, f.err
	}
	return f.res, nil
}

func testTrumpTruthService(t *testing.T, client *fakeTrumpTruth) *Service {
	t.Helper()
	c, err := cache.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return NewService(Clients{TrumpTruth: client}, c, logx.Discard(), DefaultTTLs())
}

func TestGetTrumpTruthBuildsFeed(t *testing.T) {
	when := time.Date(2026, 11, 27, 21, 56, 39, 0, time.UTC)
	client := &fakeTrumpTruth{res: &trumpstruth.Result{
		Title: "Trump's Truth - Latest Posts", Link: "https://www.trumpstruth.org/",
		Description: "Archive of Donald Trump's TRUTH Social posts", FetchedAt: when,
		Posts: []trumpstruth.Post{{
			ID: "43761", Title: "Image post", Text: "",
			URL:         "https://www.trumpstruth.org/statuses/43761",
			OriginalURL: "https://truthsocial.com/@realDonaldTrump/1",
			Image:       "https://cdn.example/a.jpg", Published: when,
			PostType: "original", Author: "Donald J. Trump",
		}, {
			ID: "1", Title: "skip", URL: "javascript:alert(1)",
		}},
	}}
	svc := testTrumpTruthService(t, client)
	f, err := svc.GetTrumpTruth(context.Background(), 10, false)
	if err != nil {
		t.Fatal(err)
	}
	if client.limit != trumpstruth.MaxLimit {
		t.Fatalf("upstream limit = %d", client.limit)
	}
	if f.Title() != "Trump's Truth - Latest Posts" || len(f.Items) != 1 {
		t.Fatalf("feed = %q items=%d", f.Title(), len(f.Items))
	}
	it := f.Items[0]
	if it.URL != "https://www.trumpstruth.org/statuses/43761" || it.Images[0] != "https://cdn.example/a.jpg" {
		t.Fatalf("item = %+v", it)
	}
	extra, ok := it.Extra.(*TrumpTruthExtra)
	if !ok || extra.PostType != "original" || extra.OriginalURL == "" {
		t.Fatalf("extra = %#v", it.Extra)
	}
	if !strings.Contains(it.HTML, "cdn.example/a.jpg") || !strings.Contains(it.HTML, "View on Trump's Truth") {
		t.Fatalf("html = %s", it.HTML)
	}
	again, err := svc.GetTrumpTruth(context.Background(), 1, false)
	if err != nil {
		t.Fatal(err)
	}
	if !again.Cached || client.calls != 1 || len(again.Items) != 1 {
		t.Fatalf("cached=%v calls=%d items=%d", again.Cached, client.calls, len(again.Items))
	}
}

func TestGetTrumpTruthErrors(t *testing.T) {
	svc := testTrumpTruthService(t, &fakeTrumpTruth{err: trumpstruth.ErrBlocked})
	_, err := svc.GetTrumpTruth(context.Background(), 10, false)
	var ae *apperr.Error
	if !errors.As(err, &ae) || ae.Code != apperr.CodeUpstreamBlocked {
		t.Fatalf("err = %v", err)
	}
}
