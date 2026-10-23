package feed_test

import (
	"encoding/json"
	"encoding/xml"
	"strings"
	"testing"
	"time"

	"github.com/yilong-musk/weibo-rss/internal/feed"
	"github.com/yilong-musk/weibo-rss/internal/weibo"
)

func mblog(t *testing.T, raw string) *weibo.MBlog {
	t.Helper()
	var m weibo.MBlog
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("decode mblog fixture: %v", err)
	}
	return &m
}

type rssDoc struct {
	XMLName xml.Name `xml:"rss"`
	Version string   `xml:"version,attr"`
	Channel struct {
		Title       string `xml:"title"`
		Link        string `xml:"link"`
		Description string `xml:"description"`
		TTL         int    `xml:"ttl"`
		Items       []struct {
			Title       string `xml:"title"`
			Link        string `xml:"link"`
			GUID        string `xml:"guid"`
			Description string `xml:"description"`
			PubDate     string `xml:"pubDate"`
		} `xml:"item"`
	} `xml:"channel"`
}

func parseRSS(t *testing.T, raw []byte) rssDoc {
	t.Helper()
	var doc rssDoc
	if err := xml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("rendered RSS is not well-formed XML: %v\n%s", err, raw)
	}
	return doc
}

func TestRenderRSSWellFormed(t *testing.T) {
	f := &feed.Feed{
		UID:         "2803301701",
		ScreenName:  `人民日报 & <friends>`,
		Description: `法人微博 "quote" 'apostrophe'`,
		FetchedAt:   time.Date(2026, 10, 23, 6, 30, 0, 0, time.UTC),
		Items: []feed.Item{{
			ID:        "5357189637868854",
			BID:       "RnZLlx130",
			Title:     `标题 & <tag> "x" 'y'`,
			CreatedAt: time.Date(2026, 10, 23, 6, 30, 4, 0, time.UTC),
			Text:      "正文",
			HTML:      `<p>正文</p><p><img src="https://wx1.sinaimg.cn/a.jpg" alt="" /></p>`,
			URL:       "https://weibo.com/2803301701/RnZLlx130",
			Images:    []string{"https://wx1.sinaimg.cn/a.jpg"},
		}},
	}

	doc := parseRSS(t, feed.RenderRSS(f))

	if doc.Version != "2.0" {
		t.Fatalf("rss version = %q, want 2.0", doc.Version)
	}
	if doc.Channel.TTL != 15 {
		t.Fatalf("ttl = %d, want 15", doc.Channel.TTL)
	}
	if doc.Channel.Title != f.ScreenName+` 的微博` {
		t.Fatalf("title round-trip failed: %q", doc.Channel.Title)
	}
	if doc.Channel.Description != f.Description {
		t.Fatalf("description round-trip failed: %q", doc.Channel.Description)
	}
	if len(doc.Channel.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(doc.Channel.Items))
	}
	it := doc.Channel.Items[0]
	if it.Title != `标题 & <tag> "x" 'y'` {
		t.Fatalf("item title round-trip failed: %q", it.Title)
	}
	if it.Link != f.Items[0].URL || it.GUID != f.Items[0].URL {
		t.Fatalf("link/guid = %q/%q", it.Link, it.GUID)
	}
	if it.Description != f.Items[0].HTML {
		t.Fatalf("description round-trip failed: %q", it.Description)
	}
	wantPub := "Fri, 23 Oct 2026 06:30:04 +0000"
	if it.PubDate != wantPub {
		t.Fatalf("pubDate = %q, want %q", it.PubDate, wantPub)
	}
}

func TestRenderRSSCDATACannotBeTerminatedEarly(t *testing.T) {
	nasty := `before]]>after]]]]>more`
	f := &feed.Feed{
		UID:        "1",
		ScreenName: "x",
		FetchedAt:  time.Now(),
		Items: []feed.Item{{
			Title:     "t",
			HTML:      nasty,
			URL:       "https://weibo.com/1/a",
			CreatedAt: time.Now(),
		}},
	}
	raw := feed.RenderRSS(f)
	if strings.Count(string(raw), "<![CDATA[") != strings.Count(string(raw), "]]>") {
		t.Fatalf("unbalanced CDATA markers:\n%s", raw)
	}
	doc := parseRSS(t, raw)
	if got := doc.Channel.Items[0].Description; got != nasty {
		t.Fatalf("CDATA round-trip failed:\n got %q\nwant %q", got, nasty)
	}
}

func TestBuildDescriptionKeepsRetweetAndImages(t *testing.T) {
	m := mblog(t, `{
		"id":"5357189637868854",
		"bid":"RnZLlx130",
		"text":"<p>转发理由</p>",
		"pics":[{"pid":"p1","url":"https://wx1.sinaimg.cn/orj360/p1.jpg",
		         "large":{"size":"large","url":"https://wx1.sinaimg.cn/mw2000/p1.jpg"}}],
		"retweeted_status":{
			"id":"5357180000000000",
			"text":"<p>原文 &amp; 引用</p>",
			"user":{"screen_name":"被转发的人"},
			"pics":[{"pid":"p2","url":"https://wx2.sinaimg.cn/orj360/p2.jpg"}]
		}
	}`)

	html := feed.BuildDescription(m)

	if !strings.Contains(html, "转发理由") {
		t.Fatalf("own text missing: %s", html)
	}
	if !strings.Contains(html, `<img src="https://wx1.sinaimg.cn/mw2000/p1.jpg"`) {
		t.Fatalf("own image must use the large rendition: %s", html)
	}
	if !strings.Contains(html, "<blockquote>") || !strings.Contains(html, "@被转发的人") {
		t.Fatalf("retweet quote block missing: %s", html)
	}
	if !strings.Contains(html, "原文 &amp; 引用") {
		t.Fatalf("retweet body must be preserved: %s", html)
	}
	if !strings.Contains(html, "https://wx2.sinaimg.cn/orj360/p2.jpg") {
		t.Fatalf("retweet image missing: %s", html)
	}
}

func TestBuildDescriptionNilIsEmpty(t *testing.T) {
	if got := feed.BuildDescription(nil); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestRenderRSSEmptyFeedStillValid(t *testing.T) {
	f := &feed.Feed{UID: "42", ScreenName: "空号", FetchedAt: time.Now()}
	doc := parseRSS(t, feed.RenderRSS(f))
	if len(doc.Channel.Items) != 0 {
		t.Fatalf("items = %d, want 0", len(doc.Channel.Items))
	}
	if doc.Channel.Link != "https://m.weibo.cn/u/42" {
		t.Fatalf("link = %q", doc.Channel.Link)
	}
}

func TestRenderRSSAlwaysEmitsChannelDescription(t *testing.T) {
	// RSS 2.0 requires title/link/description on the channel; an account with no
	// bio must still produce a (empty) description element.
	f := &feed.Feed{UID: "42", ScreenName: "无简介", FetchedAt: time.Now()}
	raw := string(feed.RenderRSS(f))
	if !strings.Contains(raw, "<description></description>") {
		t.Fatalf("empty description was omitted:\n%s", raw)
	}
	doc := parseRSS(t, []byte(raw))
	if doc.Channel.Description != "" {
		t.Fatalf("description = %q, want empty", doc.Channel.Description)
	}
}

func TestItemURLUsesBIDPermalink(t *testing.T) {
	m := mblog(t, `{"id":"5357189637868854","bid":"RnZLlx130"}`)
	if got := feed.ItemURL("2803301701", m); got != "https://weibo.com/2803301701/RnZLlx130" {
		t.Fatalf("got %q", got)
	}
	noBID := mblog(t, `{"id":"5357189637868854"}`)
	if got := feed.ItemURL("2803301701", noBID); got != "https://m.weibo.cn/detail/5357189637868854" {
		t.Fatalf("got %q", got)
	}
}
