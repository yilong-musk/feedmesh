package feed

import (
	"fmt"
	"strings"
	"time"

	"github.com/yilong-musk/feedmesh/internal/weibo"
)

// cdata wraps s in a CDATA section, splitting any embedded "]]>" so the
// section can never terminate early.
func cdata(s string) string {
	s = stripXMLControls(s)
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

// cardNoise lists page_info types that add nothing to a feed: a topic card just
// repeats a hashtag that is already a link in the post body.
func cardNoise(kind string) bool {
	switch kind {
	case "", "search_topic":
		return true
	}
	return false
}

func writeImage(b *strings.Builder, url, width, height string) {
	url = safeURLString(url)
	if url == "" {
		return
	}
	b.WriteString(`<p><img src="`)
	b.WriteString(EscapeXML(url))
	b.WriteString(`" alt=""`)
	if width != "" && height != "" {
		b.WriteString(` width="`)
		b.WriteString(width)
		b.WriteString(`" height="`)
		b.WriteString(height)
		b.WriteString(`"`)
	}
	b.WriteString(` style="max-width:100%;height:auto" loading="lazy" /></p>`)
}

func formatDuration(seconds float64) string {
	if seconds <= 0 {
		return ""
	}
	if seconds < 60 {
		return fmt.Sprintf("%.1f 秒", seconds)
	}
	total := int(seconds + 0.5)
	return fmt.Sprintf("%d:%02d", total/60, total%60)
}

// cardText picks the card headline and the secondary line. Weibo often puts the
// publisher name in page_title ("蓝鲸新闻") and the real headline in content1,
// so a very short heading swaps with a longer summary.
func cardText(p *weibo.PageInfo) (heading, summary string) {
	heading = p.Heading()
	summary = strings.TrimSpace(p.Content1)
	if summary == heading {
		return heading, ""
	}
	if len([]rune(heading)) <= 8 && len([]rune(summary)) > len([]rune(heading)) {
		return summary, heading
	}
	return heading, summary
}

// writeMediaCard renders a video or link card as a clickable cover plus a
// title/summary block. The cover keeps the dimensions weibo reports so readers
// that honour width/height don't blow the image up to full width.
func writeMediaCard(b *strings.Builder, p *weibo.PageInfo) {
	if p == nil || cardNoise(p.Type) {
		return
	}
	link := safeURLString(p.Link())
	heading, summary := cardText(p)
	cover := ""
	width, height := "", ""
	if p.PagePic != nil {
		cover = safeURLString(p.PagePic.URL)
		// PagePic dimensions come from upstream JSON and must never be trusted
		// as raw attribute text.
		width = sanitizeDimensionValue(string(p.PagePic.Width))
		height = sanitizeDimensionValue(string(p.PagePic.Height))
	}
	video := safeURLString(p.BestVideoURL())

	if p.IsVideo() && video != "" {
		duration := 0.0
		if p.MediaInfo != nil {
			duration = p.MediaInfo.Duration
		}
		dur := formatDuration(duration)
		openLink := func() {
			if link != "" {
				b.WriteString(`<p><a href="`)
				b.WriteString(EscapeXML(link))
				b.WriteString(`">`)
			} else {
				b.WriteString("<p>")
			}
		}
		closeLink := func() {
			if link != "" {
				b.WriteString("</a>")
			}
			b.WriteString("</p>")
		}
		openLink()
		if cover != "" {
			writeCardImage(b, cover, width, height)
		} else {
			b.WriteString("▶ 视频")
		}
		closeLink()
		b.WriteString(`<p>▶ 视频`)
		if dur != "" {
			b.WriteString(" · " + dur)
		}
		if link != "" {
			b.WriteString(` · <a href="`)
			b.WriteString(EscapeXML(link))
			b.WriteString(`">直接播放 MP4</a>`)
		}
		b.WriteString("</p>")
		b.WriteString(`<video controls preload="none" src="`)
		b.WriteString(EscapeXML(video))
		if cover != "" {
			b.WriteString(`" poster="` + EscapeXML(cover))
		}
		b.WriteString(`" style="max-width:100%"></video>`)
		return
	}
	if link == "" && heading == "" && cover == "" {
		return
	}
	switch {
	case link != "":
		b.WriteString(`<p><a href="`)
		b.WriteString(EscapeXML(link))
		b.WriteString(`">`)
		if cover != "" {
			writeCardImage(b, cover, width, height)
		} else if heading != "" {
			b.WriteString(EscapeXML(heading))
		}
		b.WriteString(`</a></p>`)
	case cover != "":
		b.WriteString("<p>")
		writeCardImage(b, cover, width, height)
		b.WriteString("</p>")
	case heading != "":
		b.WriteString("<p>")
		b.WriteString(EscapeXML(heading))
		b.WriteString("</p>")
	}
	if heading != "" {
		b.WriteString(`<p><strong>`)
		b.WriteString(EscapeXML(heading))
		b.WriteString(`</strong>`)
		if summary != "" && summary != heading {
			b.WriteString(`<br />`)
			b.WriteString(EscapeXML(summary))
		}
		b.WriteString(`</p>`)
	}
}

func writeCardImage(b *strings.Builder, url, width, height string) {
	b.WriteString(`<img src="`)
	b.WriteString(EscapeXML(url))
	b.WriteString(`" alt=""`)
	if width != "" && height != "" {
		b.WriteString(` width="` + width + `" height="` + height + `"`)
	}
	b.WriteString(` style="max-width:100%;height:auto" loading="lazy" />`)
}

// renderPost renders one post (and, one level deep, its quoted retweet).
func renderPost(m *weibo.MBlog, depth int) string {
	if m == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString(NormalizeHTMLBase(m.Text, "https://weibo.com"))

	for _, p := range m.Pics {
		if u := p.BestURL(); u != "" {
			w, h := p.Dimensions()
			writeImage(&b, u, w, h)
		}
	}

	writeMediaCard(&b, m.PageInfo)

	if r := m.Retweeted; r != nil && depth < 1 {
		b.WriteString("<blockquote>")
		author := ""
		if r.User != nil {
			author = r.User.ScreenName
		}
		b.WriteString("<p>@")
		b.WriteString(EscapeXML(author))
		b.WriteString("：</p>")
		b.WriteString(renderPost(r, depth+1))
		b.WriteString("</blockquote>")
	}
	return b.String()
}

// BuildDescription renders the item body: post HTML, attached images, media
// card (video or link preview) and then the quoted retweet when the post is a
// repost.
func BuildDescription(m *weibo.MBlog) string {
	if m == nil {
		return ""
	}
	return renderPost(m, 0)
}

// RenderRSS renders the feed as RSS 2.0 with Media RSS and content:encoded so
// readers can show images, covers and playable video instead of bare text.
func RenderRSS(f *Feed) []byte {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<rss version="2.0" xmlns:content="http://purl.org/rss/1.0/modules/content/" xmlns:media="http://search.yahoo.com/mrss/">` + "\n")
	b.WriteString("  <channel>\n")
	writeElem(&b, "    ", "title", f.Title())
	writeElem(&b, "    ", "link", f.Link())
	// RSS 2.0 requires channel title/link/description, so description is always
	// emitted — an empty element beats a missing one for strict validators.
	b.WriteString("    <description>")
	b.WriteString(EscapeXML(f.Description))
	b.WriteString("</description>\n")
	b.WriteString("    <language>zh-cn</language>\n")
	ttl := f.TTL
	if ttl <= 0 {
		ttl = 15
	}
	fmt.Fprintf(&b, "    <ttl>%d</ttl>\n", ttl)
	b.WriteString("    <generator>feedmesh</generator>\n")
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
		b.WriteString("      <content:encoded>")
		b.WriteString(cdata(it.HTML))
		b.WriteString("</content:encoded>\n")
		if len(it.Images) > 0 {
			b.WriteString(`      <media:thumbnail url="`)
			b.WriteString(EscapeXML(it.Images[0]))
			b.WriteString("\" />\n")
			for _, img := range it.Images {
				b.WriteString(`      <media:content url="`)
				b.WriteString(EscapeXML(img))
				b.WriteString(`" medium="image" type="image/jpeg" />` + "\n")
			}
		}
		if it.VideoURL != "" {
			b.WriteString(`      <enclosure url="`)
			b.WriteString(EscapeXML(it.VideoURL))
			b.WriteString(`" type="video/mp4" length="0" />` + "\n")
			b.WriteString(`      <media:content url="`)
			b.WriteString(EscapeXML(it.VideoURL))
			b.WriteString(`" medium="video" type="video/mp4" />` + "\n")
		}
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
