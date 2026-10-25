package feed

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/yilong-musk/feedmesh/internal/cache"
	"github.com/yilong-musk/feedmesh/internal/logx"
	"github.com/yilong-musk/feedmesh/internal/smzdm"
)

func TestGetSmzdmWithoutProvider(t *testing.T) {
	s := NewService(Clients{}, nil, logx.Discard(), DefaultTTLs())
	if _, err := s.GetSmzdm(context.Background(), smzdm.ChannelDeals, 5, false); err == nil {
		t.Fatal("GetSmzdm must fail when the provider is not configured")
	}
}

func TestSmzdmHTML(t *testing.T) {
	d := smzdm.Deal{
		ID:      "184734707",
		Title:   "富光 保温杯 620ml 28.84元",
		URL:     "https://www.smzdm.com/p/184734707/",
		Author:  "chromes",
		Summary: "券后 <99> 元",
		Image:   "https://y.zdmimg.com/a.jpg_a200.jpg",
	}
	html := smzdmHTML(smzdm.ChannelDeals, d)
	for _, want := range []string{
		"https://y.zdmimg.com/a.jpg_a200.jpg",
		"券后 &lt;99&gt; 元",
		"Posted by chromes",
		`href="https://www.smzdm.com/p/184734707/"`,
		"View deal on SMZDM",
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("HTML missing %q:\n%s", want, html)
		}
	}
	if strings.Contains(html, "<99>") {
		t.Fatalf("summary must be escaped, got:\n%s", html)
	}
}

func TestSmzdmHTMLWithoutOptionalFields(t *testing.T) {
	html := smzdmHTML(smzdm.ChannelDeals, smzdm.Deal{ID: "1", URL: "https://www.smzdm.com/p/1/"})
	if strings.Contains(html, "Posted by") || strings.Contains(html, "<img") {
		t.Fatalf("empty deal rendered optional blocks:\n%s", html)
	}
	if !strings.Contains(html, "https://www.smzdm.com/p/1/") {
		t.Fatalf("deal link missing:\n%s", html)
	}
}

// unsafeLinkRSS puts a non-http permalink next to a valid one. The unsafe entry
// still matches the /p/ pattern, so only the scheme allowlist can reject it.
const unsafeLinkRSS = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
<channel><title>deals</title>
<item><title><![CDATA[unsafe]]></title>
<link><![CDATA[javascript:alert('/p/1')]]></link>
<pubDate><![CDATA[Sun, 25 Oct 2026 22:00:00]]></pubDate>
</item>
<item><title><![CDATA[safe]]></title>
<link><![CDATA[https://www.smzdm.com/p/2/]]></link>
<pubDate><![CDATA[Sun, 25 Oct 2026 21:00:00]]></pubDate>
</item>
</channel></rss>`

func TestGetSmzdmDropsUnsafeLinks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(unsafeLinkRSS))
	}))
	t.Cleanup(srv.Close)

	c, err := cache.New(t.TempDir())
	if err != nil {
		t.Fatalf("cache: %v", err)
	}
	svc := NewService(Clients{Smzdm: smzdm.New(smzdm.Config{BaseURL: srv.URL, Timeout: 2 * time.Second})},
		c, logx.Discard(), DefaultTTLs())

	f, err := svc.GetSmzdm(context.Background(), smzdm.ChannelDeals, 10, false)
	if err != nil {
		t.Fatalf("GetSmzdm: %v", err)
	}
	if len(f.Items) != 1 || f.Items[0].URL != "https://www.smzdm.com/p/2/" {
		t.Fatalf("items = %+v, want only the http(s) entry", f.Items)
	}
	if strings.Contains(f.Items[0].HTML, "javascript") {
		t.Fatalf("unsafe link survived into the body:\n%s", f.Items[0].HTML)
	}
}
