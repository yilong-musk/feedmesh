package feed_test

import (
	"encoding/xml"
	"strings"
	"testing"
	"time"

	"github.com/yilong-musk/feedmesh/internal/feed"
	"github.com/yilong-musk/feedmesh/internal/weibo"
)

func TestNormalizeHTMLRemovesDangerousMarkup(t *testing.T) {
	in := `<p onclick="alert(1)">ok</p>` +
		`<script>alert(2)</script>` +
		`<iframe src="https://evil.example/x">fallback</iframe>` +
		`<a href="javascript:alert(3)">bad</a>` +
		`<a href="/relative">relative</a>` +
		`<img src="javascript:alert(4)" onerror="alert(5)">` +
		`<img src="/pic.jpg" onload="alert(6)">` +
		`<scr<script>x</script>ipt>alert(7)</script>`
	got := feed.NormalizeHTMLBase(in, "https://example.com/base/")

	for _, bad := range []string{"onclick", "onerror", "onload", "<script", "</script", "<iframe", "javascript:"} {
		if strings.Contains(strings.ToLower(got), bad) {
			t.Fatalf("dangerous markup %q survived: %s", bad, got)
		}
	}
	if !strings.Contains(got, `href="https://example.com/relative"`) {
		t.Fatalf("relative link was not made absolute: %s", got)
	}
	if !strings.Contains(got, `src="https://example.com/pic.jpg"`) {
		t.Fatalf("relative image was not made absolute: %s", got)
	}
}

func TestBuildDescriptionRejectsPagePicAttributeInjection(t *testing.T) {
	m := &weibo.MBlog{
		ID: "1", Text: "hello",
		PageInfo: &weibo.PageInfo{
			Type: "webpage", PageTitle: "title", PageURL: "https://example.com",
			PagePic: &weibo.PagePic{URL: "https://example.com/x.jpg", Width: `100" onerror="alert(1)`, Height: "200"},
		},
	}
	html := feed.BuildDescription(m)
	if strings.Contains(html, "onerror") || strings.Contains(html, `width="100`) {
		t.Fatalf("page_pic dimensions injected an attribute: %s", html)
	}
	if !strings.Contains(html, "<img") {
		t.Fatalf("card image disappeared: %s", html)
	}
}

func TestRenderRSSStripsIllegalXMLControlsFromCDATA(t *testing.T) {
	f := &feed.Feed{
		UID: "1", ScreenName: "x", FetchedAt: time.Now(),
		Items: []feed.Item{{
			Title: "t", URL: "https://example.com/1", HTML: "bad\x01char", CreatedAt: time.Now(),
		}},
	}
	raw := feed.RenderRSS(f)
	var doc struct {
		Channel struct {
			Items []struct {
				Description string `xml:"description"`
			} `xml:"item"`
		} `xml:"channel"`
	}
	if err := xml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("rendered RSS is not well-formed: %v\n%s", err, raw)
	}
	if len(doc.Channel.Items) != 1 || doc.Channel.Items[0].Description != "badchar" {
		t.Fatalf("description = %q", doc.Channel.Items[0].Description)
	}
}
