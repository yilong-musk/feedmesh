package feed_test

import (
	"strings"
	"testing"
	"time"

	"github.com/yilong-musk/weibo-rss/internal/feed"
)

func TestEscapeXMLCoversAllFiveMetacharacters(t *testing.T) {
	in := `A & B < C > D " E ' F`
	want := "A &amp; B &lt; C &gt; D &quot; E &apos; F"
	if got := feed.EscapeXML(in); got != want {
		t.Fatalf("EscapeXML(%q)\n got %q\nwant %q", in, got, want)
	}
}

func TestEscapeXMLDropsIllegalControlCharacters(t *testing.T) {
	in := "ok\x00\x01\x1fend"
	got := feed.EscapeXML(in)
	if got != "okend" {
		t.Fatalf("got %q, want %q", got, "okend")
	}
	// Tab, newline and carriage return are legal XML and must survive.
	if got := feed.EscapeXML("a\tb\nc\rd"); got != "a\tb\nc\rd" {
		t.Fatalf("whitespace must survive, got %q", got)
	}
}

func TestNormalizeHTMLTurnsEmojiSpansIntoAltText(t *testing.T) {
	in := `转发<a href="x"><span class="surl-text">#话题#</span></a>` +
		`<span class="url-icon"><img alt="[哈哈]" src="https://h5.sinaimg.cn/m/emoticon/icon/lxh/lxh_haha.png" style="width:1em;height:1em;"></span>` +
		`好文<span class="url-icon"><img alt="[赞]" src="https://example.invalid/a.png"></span>`
	got := feed.NormalizeHTML(in)
	if strings.Contains(got, "url-icon") {
		t.Fatalf("emoji span survived: %q", got)
	}
	if !strings.Contains(got, "[哈哈]") || !strings.Contains(got, "[赞]") {
		t.Fatalf("alt text missing: %q", got)
	}
	if !strings.Contains(got, `class="surl-text"`) {
		t.Fatalf("topic link must survive: %q", got)
	}
}

func TestHTMLToText(t *testing.T) {
	in := `【标题】<a href="https://m.weibo.cn/x"><span class="surl-text">#话题#</span></a>` +
		`&amp; 更多<br />内容&nbsp;结束`
	got := feed.HTMLToText(in)
	for _, bad := range []string{"<", ">", "&amp;", "&nbsp;"} {
		if strings.Contains(got, bad) {
			t.Fatalf("plain text still contains %q: %q", bad, got)
		}
	}
	if !strings.Contains(got, "【标题】") || !strings.Contains(got, "内容 结束") {
		t.Fatalf("unexpected text: %q", got)
	}
	if strings.Contains(got, "\n") {
		t.Fatalf("plain text must be a single line: %q", got)
	}
}

func TestItemTitlePrefersStatusTitle(t *testing.T) {
	if got := feed.ItemTitle("  官方标题  ", "很长的正文……"); got != "官方标题" {
		t.Fatalf("got %q", got)
	}
}

func TestItemTitleTruncatesTo25Runes(t *testing.T) {
	text := strings.Repeat("中", 40)
	got := feed.ItemTitle("", text)
	runes := []rune(got)
	if len(runes) != 26 || !strings.HasSuffix(got, "…") {
		t.Fatalf("got %d runes (%q), want 25 runes plus ellipsis", len(runes), got)
	}
	if !strings.HasPrefix(got, strings.Repeat("中", 25)) {
		t.Fatalf("unexpected prefix: %q", got)
	}
}

func TestItemTitleEmptyText(t *testing.T) {
	if got := feed.ItemTitle("", "   "); got != "(无正文)" {
		t.Fatalf("got %q", got)
	}
}

func TestParseCreatedAt(t *testing.T) {
	got, ok := feed.ParseCreatedAt("Fri Oct 23 14:30:04 +0800 2026")
	if !ok {
		t.Fatal("expected created_at to parse")
	}
	want := time.Date(2026, 10, 23, 14, 30, 4, 0, time.FixedZone("", 8*3600))
	if !got.Equal(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if _, ok := feed.ParseCreatedAt(""); ok {
		t.Fatal("empty created_at must not parse")
	}
	if _, ok := feed.ParseCreatedAt("not a date"); ok {
		t.Fatal("garbage must not parse")
	}
}
