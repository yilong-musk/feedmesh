package feed

import (
	"context"
	"strings"
	"testing"

	"github.com/yilong-musk/feedmesh/internal/logx"
	"github.com/yilong-musk/feedmesh/internal/smzdm"
)

func TestGetSmzdmWithoutProvider(t *testing.T) {
	s := NewService(Clients{}, nil, logx.Discard(), DefaultTTLs())
	if _, err := s.GetSmzdm(context.Background(), 5, false); err == nil {
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
	html := smzdmHTML(d)
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
	html := smzdmHTML(smzdm.Deal{ID: "1", URL: "https://www.smzdm.com/p/1/"})
	if strings.Contains(html, "Posted by") || strings.Contains(html, "<img") {
		t.Fatalf("empty deal rendered optional blocks:\n%s", html)
	}
	if !strings.Contains(html, "https://www.smzdm.com/p/1/") {
		t.Fatalf("deal link missing:\n%s", html)
	}
}
