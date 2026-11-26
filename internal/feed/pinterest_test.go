package feed

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/yilong-musk/feedmesh/internal/apperr"
	"github.com/yilong-musk/feedmesh/internal/cache"
	"github.com/yilong-musk/feedmesh/internal/logx"
	"github.com/yilong-musk/feedmesh/internal/pinterest"
)

// pinterestFeedXML is a two-pin profile feed: one titled pin, one pin with no
// title and no caption (only an image).
const pinterestFeedXML = `<?xml version="1.0" encoding="utf-8"?><rss version="2.0">
<channel><title>NASA</title><link>https://www.pinterest.com/nasa/</link>
<description>Official NASA boards.</description>
<item><title>Moon Base</title><link>https://www.pinterest.com/pin/142567144449130105/</link>
<description>&lt;a href=&quot;https://www.pinterest.com/pin/142567144449130105/&quot;&gt;&lt;img src=&quot;https://i.pinimg.com/236x/7f/b0/7c/7fb07c79a6280dd11eab9165b4120300.jpg&quot;&gt;&lt;/a&gt;Moon Base caption.</description>
<pubDate>Fri, 20 Nov 2026 16:39:34 GMT</pubDate><guid>https://www.pinterest.com/pin/142567144449130105/</guid></item>
<item><title> </title><link>https://www.pinterest.com/pin/142567144449081155/</link>
<description>&lt;a href=&quot;https://www.pinterest.com/pin/142567144449081155/&quot;&gt;&lt;img src=&quot;https://i.pinimg.com/236x/bb/a1/e7/bba1e7fb6fe37433e1314ad3cf4e3434.jpg&quot;&gt;&lt;/a&gt;</description>
<pubDate>Fri, 20 Nov 2026 15:00:00 GMT</pubDate><guid>https://www.pinterest.com/pin/142567144449081155/</guid></item>
</channel></rss>`

// pinterestUnsafeXML carries a non-http permalink next to a valid one.
const pinterestUnsafeXML = `<?xml version="1.0" encoding="utf-8"?><rss version="2.0">
<channel><title>Unsafe</title><link>https://www.pinterest.com/unsafe/</link><description></description>
<item><title>Bad</title><link>javascript:alert(1)</link>
<description>bad</description><pubDate>Fri, 20 Nov 2026 16:39:34 GMT</pubDate></item>
<item><title>Good</title><link>https://www.pinterest.com/pin/1/</link>
<description>good</description><pubDate>Fri, 20 Nov 2026 15:00:00 GMT</pubDate></item>
</channel></rss>`

func newPinterestService(t *testing.T, handler http.HandlerFunc) *Service {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c, err := cache.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return NewService(Clients{
		Pinterest: pinterest.New(pinterest.Config{BaseURL: srv.URL, Timeout: 2 * time.Second}),
	}, c, logx.Discard(), DefaultTTLs())
}

func TestGetPinterestWithoutProvider(t *testing.T) {
	s := NewService(Clients{}, nil, logx.Discard(), DefaultTTLs())
	if _, err := s.GetPinterest(context.Background(), "nasa", "", 5, false); err == nil {
		t.Fatal("GetPinterest must fail when the provider is not configured")
	}
}

func TestGetPinterestUserFeed(t *testing.T) {
	s := newPinterestService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nasa/feed.rss" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(pinterestFeedXML))
	})

	f, err := s.GetPinterest(context.Background(), "nasa", "", 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if f.Title() != "NASA on Pinterest" {
		t.Fatalf("title = %q", f.Title())
	}
	if f.Link() != "https://www.pinterest.com/nasa/" {
		t.Fatalf("link = %q", f.Link())
	}
	if f.TTL != 10 {
		t.Fatalf("ttl = %d", f.TTL)
	}
	if len(f.Items) != 2 {
		t.Fatalf("items = %d", len(f.Items))
	}
	first := f.Items[0]
	if first.ID != "142567144449130105" || first.Title != "Moon Base" {
		t.Fatalf("first item = %+v", first)
	}
	if first.Images[0] != "https://i.pinimg.com/736x/7f/b0/7c/7fb07c79a6280dd11eab9165b4120300.jpg" {
		t.Fatalf("image = %v", first.Images)
	}
	if !strings.Contains(first.HTML, "View pin on Pinterest") || !strings.Contains(first.HTML, "Moon Base caption.") {
		t.Fatalf("html = %s", first.HTML)
	}
	// A pin with no title and no caption still needs a readable title.
	if f.Items[1].Title != "(no content)" {
		t.Fatalf("untitled pin = %q", f.Items[1].Title)
	}

	// A second call is served from the cache and keeps the channel metadata.
	again, err := s.GetPinterest(context.Background(), "nasa", "", 1, false)
	if err != nil {
		t.Fatal(err)
	}
	if !again.Cached {
		t.Fatal("second call must be a cache hit")
	}
	if len(again.Items) != 1 {
		t.Fatalf("limit=1 returned %d items", len(again.Items))
	}
	if again.Title() != "NASA on Pinterest" {
		t.Fatalf("cached title = %q", again.Title())
	}
}

func TestGetPinterestBoardFeed(t *testing.T) {
	s := newPinterestService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nasa/earth.rss" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(strings.ReplaceAll(pinterestFeedXML, "<title>NASA</title>", "<title>Earth</title>")))
	})
	f, err := s.GetPinterest(context.Background(), "nasa", "earth", 5, false)
	if err != nil {
		t.Fatal(err)
	}
	if f.UID != "nasa/earth" {
		t.Fatalf("uid = %q", f.UID)
	}
	if f.Title() != "Earth on Pinterest" {
		t.Fatalf("title = %q", f.Title())
	}
	if f.Description != "Official NASA boards." {
		t.Fatalf("description = %q", f.Description)
	}
}

func TestGetPinterestNotFound(t *testing.T) {
	s := newPinterestService(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("User not found"))
	})
	_, err := s.GetPinterest(context.Background(), "ghost", "", 5, false)
	var we *apperr.Error
	if !errors.As(err, &we) || we.Code != apperr.CodeUserNotFound {
		t.Fatalf("err = %v", err)
	}
}

func TestGetPinterestRateLimited(t *testing.T) {
	s := newPinterestService(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	})
	_, err := s.GetPinterest(context.Background(), "nasa", "", 5, false)
	var we *apperr.Error
	if !errors.As(err, &we) || we.Code != apperr.CodeRateLimited {
		t.Fatalf("err = %v", err)
	}
}

func TestGetPinterestInvalidTarget(t *testing.T) {
	s := newPinterestService(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("invalid input must not reach the upstream")
	})
	_, err := s.GetPinterest(context.Background(), "nasa/earth", "", 5, false)
	var we *apperr.Error
	if !errors.As(err, &we) || we.Code != apperr.CodeBadRequest {
		t.Fatalf("err = %v", err)
	}
}

func TestPinterestSkipsUnsafeLink(t *testing.T) {
	s := newPinterestService(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(pinterestUnsafeXML))
	})
	f, err := s.GetPinterest(context.Background(), "unsafe", "", 5, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Items) != 1 || f.Items[0].Title != "Good" {
		t.Fatalf("items = %+v", f.Items)
	}
}
