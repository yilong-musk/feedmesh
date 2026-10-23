package feed

import (
	"fmt"
	"strings"
	"time"

	"github.com/yilong-musk/weibo-rss/internal/weibo"
)

// cdata wraps s in a CDATA section, splitting any embedded "]]>" so the
// section can never terminate early.
func cdata(s string) string {
	if s == "" {
		return "<![CDATA[]]>"
	}
	return "<![CDATA[" + strings.ReplaceAll(s, "]]>", "]]]]><![CDATA[>") + "]]>"
}

func rfc822(t time.Time) string {
	if t.IsZero() {
		t = time.Now()
	}
	return t.UTC().Format(time.RFC1123Z)
}

// BuildDescription renders the item body: post HTML, attached images, then the
// quoted retweet when the post is a repost.
func BuildDescription(m *weibo.MBlog) string {
	if m == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString(NormalizeHTML(m.Text))

	for _, p := range m.Pics {
		if u := p.BestURL(); u != "" {
			b.WriteString(`<p><img src="`)
			b.WriteString(EscapeXML(u))
			b.WriteString(`" alt="" /></p>`)
		}
	}

	if r := m.Retweeted; r != nil {
		b.WriteString("<blockquote>")
		author := ""
		if r.User != nil {
			author = r.User.ScreenName
		}
		b.WriteString("<p>@")
		b.WriteString(EscapeXML(author))
		b.WriteString("：</p>")
		b.WriteString(NormalizeHTML(r.Text))
		for _, p := range r.Pics {
			if u := p.BestURL(); u != "" {
				b.WriteString(`<p><img src="`)
				b.WriteString(EscapeXML(u))
				b.WriteString(`" alt="" /></p>`)
			}
		}
		b.WriteString("</blockquote>")
	}
	return b.String()
}

// RenderRSS renders the feed as RSS 2.0.
func RenderRSS(f *Feed) []byte {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<rss version="2.0">` + "\n")
	b.WriteString("  <channel>\n")
	writeElem(&b, "    ", "title", f.Title())
	writeElem(&b, "    ", "link", f.Link())
	// RSS 2.0 requires channel title/link/description, so description is always
	// emitted — an empty element beats a missing one for strict validators.
	b.WriteString("    <description>")
	b.WriteString(EscapeXML(f.Description))
	b.WriteString("</description>\n")
	b.WriteString("    <language>zh-cn</language>\n")
	b.WriteString("    <ttl>15</ttl>\n")
	b.WriteString("    <generator>weibo-rss</generator>\n")
	last := f.FetchedAt
	if last.IsZero() {
		last = time.Now()
	}
	writeElem(&b, "    ", "lastBuildDate", last.UTC().Format(time.RFC1123Z))

	for i := range f.Items {
		it := &f.Items[i]
		b.WriteString("    <item>\n")
		writeElem(&b, "      ", "title", it.Title)
		writeElem(&b, "      ", "link", it.URL)
		b.WriteString(`      <guid isPermaLink="true">`)
		b.WriteString(EscapeXML(it.URL))
		b.WriteString("</guid>\n")
		b.WriteString("      <description>")
		b.WriteString(cdata(it.HTML))
		b.WriteString("</description>\n")
		writeElem(&b, "      ", "pubDate", rfc822(it.CreatedAt))
		b.WriteString("    </item>\n")
	}

	b.WriteString("  </channel>\n")
	b.WriteString("</rss>\n")
	return []byte(b.String())
}

func writeElem(b *strings.Builder, indent, name, value string) {
	if value == "" {
		return
	}
	fmt.Fprintf(b, "%s<%s>%s</%s>\n", indent, name, EscapeXML(value), name)
}
