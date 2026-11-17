package telegram

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func fixture(t *testing.T, name string) string {
	t.Helper()
	switch name {
	case "sspai.html":
		return sspaiFixture
	case "durov.html":
		return durovFixture
	case "newlearner.html":
		return newlearnerFixture
	default:
		t.Fatalf("unknown fixture %s", name)
		return ""
	}
}

func TestNormalizeChannel(t *testing.T) {
	good := map[string]string{
		"sspai":                         "sspai",
		"@sspai":                        "sspai",
		"@SSPai":                        "sspai",
		"t.me/sspai":                    "sspai",
		"t.me/s/sspai":                  "sspai",
		"telegram.me/sspai":             "sspai",
		"https://t.me/sspai":            "sspai",
		"https://t.me/s/sspai":          "sspai",
		"http://t.me/s/sspai/123?foo=1": "sspai",
		"  @NewlearnerChannel  ":        "newlearnerchannel",
	}
	for in, want := range good {
		got, err := NormalizeChannel(in)
		if err != nil || got != want {
			t.Errorf("NormalizeChannel(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"", "@", "a", "not a channel", "-bad", "https://example.com/sspai"} {
		if _, err := NormalizeChannel(in); !errors.Is(err, ErrInvalidChannel) {
			t.Errorf("NormalizeChannel(%q) error = %v, want ErrInvalidChannel", in, err)
		}
	}
}

func TestParseChannelPreview(t *testing.T) {
	ch, err := ParsePage("sspai", fixture(t, "sspai.html"))
	if err != nil {
		t.Fatal(err)
	}
	if ch.Title != "少数派sspai" {
		t.Errorf("title = %q", ch.Title)
	}
	if !strings.Contains(ch.Description, "sspai.com") {
		t.Errorf("description = %q, want the channel blurb", ch.Description)
	}
	if ch.Subscribers != "39.2K" {
		t.Errorf("subscribers = %q", ch.Subscribers)
	}
	if ch.Avatar == "" {
		t.Error("avatar is empty")
	}
	if len(ch.Posts) != 2 {
		t.Fatalf("posts = %d, want 2", len(ch.Posts))
	}
	p := ch.Posts[0]
	if p.ID != 14873 {
		t.Errorf("id = %d", p.ID)
	}
	if p.CreatedAt.UTC() != time.Date(2026, 11, 9, 4, 20, 38, 0, time.UTC) {
		t.Errorf("created_at = %s", p.CreatedAt)
	}
	if p.Views != 3050 {
		t.Errorf("views = %d, want 3050", p.Views)
	}
	if !strings.Contains(p.TextHTML, "双十一好物精选") {
		t.Errorf("text does not carry the post body: %q", p.TextHTML[:80])
	}
	if p.LinkURL != "https://sspai.com/post/118612" {
		t.Errorf("link = %q", p.LinkURL)
	}
	if p.LinkTitle == "" || p.LinkImage == "" || p.LinkSite == "" {
		t.Errorf("link preview incomplete: %+v", p)
	}
	if p.Author != "少数派sspai" {
		t.Errorf("author = %q", p.Author)
	}
}

func TestParseVideoAndPhotoPosts(t *testing.T) {
	ch, err := ParsePage("durov", fixture(t, "durov.html"))
	if err != nil {
		t.Fatal(err)
	}
	byID := map[int64]Post{}
	for _, p := range ch.Posts {
		byID[p.ID] = p
	}
	video := byID[546]
	if video.Media != "video" {
		t.Errorf("546 media = %q, want video", video.Media)
	}
	if video.PosterURL == "" {
		t.Error("546 has no video poster")
	}
	if len(video.Images) != 0 {
		t.Errorf("546 images = %d, want 0", len(video.Images))
	}
	photo := byID[553]
	if photo.Media != "photo" {
		t.Errorf("553 media = %q, want photo", photo.Media)
	}
	if len(photo.Images) != 1 {
		t.Fatalf("553 images = %d, want 1", len(photo.Images))
	}
	if !strings.HasPrefix(photo.Images[0], "https://cdn") {
		t.Errorf("553 image = %q", photo.Images[0])
	}
}

func TestParseAlbumWithSignature(t *testing.T) {
	ch, err := ParsePage("newlearnerchannel", fixture(t, "newlearner.html"))
	if err != nil {
		t.Fatal(err)
	}
	if ch.Title == "" || ch.Description == "" {
		t.Fatalf("channel metadata missing: %+v", ch)
	}
	if len(ch.Posts) != 1 {
		t.Fatalf("posts = %d, want 1", len(ch.Posts))
	}
	p := ch.Posts[0]
	if p.ID != 16121 {
		t.Fatalf("first id = %d", p.ID)
	}
	if len(p.Images) != 5 {
		t.Errorf("album images = %d, want 5", len(p.Images))
	}
	if !strings.Contains(p.Signature, "初学者") {
		t.Errorf("signature = %q", p.Signature)
	}
	if p.Views != 2800 {
		t.Errorf("views = %d, want 2800", p.Views)
	}
	if p.Media != "photo" {
		t.Errorf("media = %q, want photo", p.Media)
	}
}

func previewServer(t *testing.T, status int, body, location string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/s/sspai" {
			t.Errorf("request path = %q", r.URL.Path)
		}
		if r.Header.Get("User-Agent") == "" {
			t.Error("missing User-Agent")
		}
		if location != "" {
			w.Header().Set("Location", location)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestFetchSortsNewestFirstAndClamps(t *testing.T) {
	srv := previewServer(t, http.StatusOK, fixture(t, "sspai.html"), "")
	c := New(Config{BaseURL: srv.URL, Timeout: 5 * time.Second})
	ch, err := c.Fetch(context.Background(), "@sspai", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(ch.Posts) != 1 {
		t.Fatalf("posts = %d, want 1", len(ch.Posts))
	}
	if ch.Posts[0].ID != 14874 {
		t.Errorf("newest id = %d, want 14874", ch.Posts[0].ID)
	}
	if ch.Posts[0].ChannelTitle != "少数派sspai" {
		t.Errorf("cached post lost channel title: %q", ch.Posts[0].ChannelTitle)
	}
	if ch.FetchedAt.IsZero() {
		t.Error("FetchedAt is zero")
	}
}

func TestFetchMapsRedirectToNoPreview(t *testing.T) {
	srv := previewServer(t, http.StatusFound, "", "https://t.me/sspai")
	c := New(Config{BaseURL: srv.URL, Timeout: 5 * time.Second})
	if _, err := c.Fetch(context.Background(), "sspai", 10); !errors.Is(err, ErrNoPreview) {
		t.Fatalf("error = %v, want ErrNoPreview", err)
	}
}

func TestFetchMapsNotFoundAndRateLimit(t *testing.T) {
	for _, tc := range []struct {
		status int
		want   error
	}{
		{http.StatusNotFound, ErrNotFound},
		{http.StatusTooManyRequests, ErrRateLimited},
	} {
		srv := previewServer(t, tc.status, "", "")
		c := New(Config{BaseURL: srv.URL, Timeout: 5 * time.Second})
		if _, err := c.Fetch(context.Background(), "sspai", 10); !errors.Is(err, tc.want) {
			t.Errorf("status %d error = %v, want %v", tc.status, err, tc.want)
		}
	}
}

func TestFetchRejectsPagesWithoutPosts(t *testing.T) {
	page := `<html><body><div class="tgme_channel_info"><div class="tgme_channel_info_header_title"><span>Empty</span></div></div></body></html>`
	srv := previewServer(t, http.StatusOK, page, "")
	c := New(Config{BaseURL: srv.URL, Timeout: 5 * time.Second})
	if _, err := c.Fetch(context.Background(), "sspai", 10); !errors.Is(err, ErrNoPosts) {
		t.Fatalf("error = %v, want ErrNoPosts", err)
	}
}

func TestFetchRejectsUnexpectedPage(t *testing.T) {
	srv := previewServer(t, http.StatusOK, "<html><body>not a preview</body></html>", "")
	c := New(Config{BaseURL: srv.URL, Timeout: 5 * time.Second})
	if _, err := c.Fetch(context.Background(), "sspai", 10); !errors.Is(err, ErrUnexpectedPage) {
		t.Fatalf("error = %v, want ErrUnexpectedPage", err)
	}
}

func TestParseCompactCount(t *testing.T) {
	cases := map[string]int64{
		"2.8K": 2800, "1.2M": 1200000, "9.96M": 9960000, "17,300": 17300,
		"850": 850, "1.5B": 1500000000, "": 0, "n/a": 0,
	}
	for in, want := range cases {
		if got := parseCompactCount(in); got != want {
			t.Errorf("parseCompactCount(%q) = %d, want %d", in, got, want)
		}
	}
}
