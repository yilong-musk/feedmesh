package smzdm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// feedXML mirrors the live feed: CDATA payloads, a zone-less pubDate, the
// footer-only content:encoded most entries carry, and one entry with a real
// blurb. The third item is deliberately out of order.
const feedXML = `<?xml version="1.0" encoding="UTF-8"?>
<rss xmlns:content="http://purl.org/rss/1.0/modules/content/" version="2.0">
<channel>
<title>什么值得买</title><link>http://www.smzdm.com</link><description>deals</description>
<item>
  <title><![CDATA[富光 FGZ2417-620 保温杯 620ml 冬日白 28.84元（需买2件）]]></title>
  <link><![CDATA[https://www.smzdm.com/p/184734707/]]></link>
  <pubDate><![CDATA[Sun, 25 Oct 2026 21:39:37]]></pubDate>
  <description><![CDATA[chromes]]></description>
  <focus_pic><![CDATA[https://y.zdmimg.com/202511/08/690e3be56fd2b3775.png_a200.jpg]]></focus_pic>
  <content:encoded><![CDATA[<p>本文来自<a href="https://www.smzdm.com/p/184734707/">什么值得买网站（www.smzdm.com）</a>。</p>]]></content:encoded>
</item>
<item>
  <title><![CDATA[丝麦耘 新疆冬麦挂面800g 3.91元（需用券）]]></title>
  <link><![CDATA[https://www.smzdm.com/p/184734541/]]></link>
  <pubDate><![CDATA[Sun, 25 Oct 2026 21:35:02]]></pubDate>
  <description><![CDATA[试水将军兼翻船都督]]></description>
  <focus_pic><![CDATA[https://y.zdmimg.com/202509/11/68c2978bf65378997.jpg_a200.jpg]]></focus_pic>
  <content:encoded><![CDATA[<p>本文来自什么值得买网站（www.smzdm.com）。</p>]]></content:encoded>
</item>
<item>
  <title><![CDATA[保暖内衣套装 男女士加绒 99 元包邮]]></title>
  <link><![CDATA[https://www.smzdm.com/p/184730001/]]></link>
  <pubDate><![CDATA[Sun, 25 Oct 2026 22:01:10]]></pubDate>
  <description><![CDATA[小值机器人德绒自发热，三层加厚，券后 99 元。]]></description>
  <focus_pic><![CDATA[https://y.zdmimg.com/202510/01/abc.jpg_a200.jpg]]></focus_pic>
  <content:encoded><![CDATA[<p itemprop="description">德绒自发热，三层加厚，券后 99 元。</p><p>本文来自<a href="https://www.smzdm.com/p/184730001/">什么值得买网站（www.smzdm.com）</a>。</p>]]></content:encoded>
</item>
<item>
  <title><![CDATA[no permalink, must be skipped]]></title>
  <link><![CDATA[https://www.smzdm.com/youhui/]]></link>
  <pubDate><![CDATA[Sun, 25 Oct 2026 22:02:00]]></pubDate>
  <description><![CDATA[nick]]></description>
</item>
</channel></rss>`

func feedServer(t *testing.T, body string, status int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestListParsesLiveShape(t *testing.T) {
	srv := feedServer(t, feedXML, http.StatusOK)
	c := New(Config{BaseURL: srv.URL, Timeout: 2 * time.Second})

	deals, err := c.List(context.Background(), 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(deals) != 3 {
		t.Fatalf("got %d deals, want 3 (the permalink-less item must be skipped)", len(deals))
	}
	// Out-of-order entries are sorted newest first before truncation.
	if deals[0].ID != "184730001" || deals[1].ID != "184734707" || deals[2].ID != "184734541" {
		t.Fatalf("order = %s, %s, %s", deals[0].ID, deals[1].ID, deals[2].ID)
	}

	first := deals[0]
	if first.Author != "小值机器人" {
		t.Fatalf("author = %q, want the nickname prefix", first.Author)
	}
	if first.Summary != "德绒自发热，三层加厚，券后 99 元。" {
		t.Fatalf("summary = %q", first.Summary)
	}
	if first.Title != "保暖内衣套装 男女士加绒 99 元包邮" {
		t.Fatalf("title = %q", first.Title)
	}
	if first.URL != "https://www.smzdm.com/p/184730001/" {
		t.Fatalf("url = %q", first.URL)
	}
	if first.Image != "https://y.zdmimg.com/202510/01/abc.jpg_a200.jpg" {
		t.Fatalf("image = %q", first.Image)
	}
	if first.Published.IsZero() || first.Published.Format(time.RFC3339) != "2026-10-25T22:01:10+08:00" {
		t.Fatalf("published = %v, want a +08:00 stamp", first.Published)
	}

	// Nickname-only entry: the nickname is the author and there is no blurb.
	nick := deals[1]
	if nick.Author != "chromes" || nick.Summary != "" {
		t.Fatalf("nickname-only item = author %q summary %q", nick.Author, nick.Summary)
	}
	if deals[2].Author != "试水将军兼翻船都督" || deals[2].Summary != "" {
		t.Fatalf("second nickname-only item = author %q summary %q", deals[2].Author, deals[2].Summary)
	}
}

func TestListHonoursLimitAndClamp(t *testing.T) {
	srv := feedServer(t, feedXML, http.StatusOK)
	c := New(Config{BaseURL: srv.URL, Timeout: 2 * time.Second})

	deals, err := c.List(context.Background(), 1)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(deals) != 1 || deals[0].ID != "184730001" {
		t.Fatalf("limit 1 = %+v", deals)
	}

	deals, err = c.List(context.Background(), 0)
	if err != nil {
		t.Fatalf("List(0): %v", err)
	}
	if len(deals) != 3 {
		t.Fatalf("default limit returned %d, want all 3", len(deals))
	}

	deals, err = c.List(context.Background(), 500)
	if err != nil {
		t.Fatalf("List(500): %v", err)
	}
	if len(deals) != 3 {
		t.Fatalf("clamped limit returned %d, want 3", len(deals))
	}
}

func TestListErrors(t *testing.T) {
	t.Run("http error", func(t *testing.T) {
		srv := feedServer(t, "blocked", http.StatusForbidden)
		c := New(Config{BaseURL: srv.URL, Timeout: 2 * time.Second})
		if _, err := c.List(context.Background(), 5); err == nil || !strings.Contains(err.Error(), "HTTP 403") {
			t.Fatalf("err = %v, want an HTTP 403 error", err)
		}
	})

	t.Run("empty channel", func(t *testing.T) {
		srv := feedServer(t, `<rss version="2.0"><channel><title>x</title></channel></rss>`, http.StatusOK)
		c := New(Config{BaseURL: srv.URL, Timeout: 2 * time.Second})
		if _, err := c.List(context.Background(), 5); err == nil || !strings.Contains(err.Error(), "no entries") {
			t.Fatalf("err = %v, want an empty-feed error", err)
		}
	})

	t.Run("malformed xml", func(t *testing.T) {
		srv := feedServer(t, `<rss><channel>`, http.StatusOK)
		c := New(Config{BaseURL: srv.URL, Timeout: 2 * time.Second})
		if _, err := c.List(context.Background(), 5); err == nil || !strings.Contains(err.Error(), "parse") {
			t.Fatalf("err = %v, want a parse error", err)
		}
	})
}

func TestSplitDescription(t *testing.T) {
	cases := []struct {
		name, desc, summary, wantAuthor, wantText string
	}{
		{"nickname only", "chromes", "", "chromes", ""},
		{"long text without summary", "这是一段比较长的爆料正文，没有昵称前缀", "", "", "这是一段比较长的爆料正文，没有昵称前缀"},
		{"nickname plus summary", "小值机器人德绒自发热，券后 99 元。", "德绒自发热，券后 99 元。", "小值机器人", "德绒自发热，券后 99 元。"},
		{"summary not embedded", "nick", "另一个摘要", "", "另一个摘要"},
		{"empty", "", "", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			author, text := splitDescription(tc.desc, tc.summary)
			if author != tc.wantAuthor || text != tc.wantText {
				t.Fatalf("splitDescription(%q, %q) = (%q, %q), want (%q, %q)",
					tc.desc, tc.summary, author, text, tc.wantAuthor, tc.wantText)
			}
		})
	}
}
