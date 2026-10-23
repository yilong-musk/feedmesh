package feed_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/yilong-musk/feedmesh/internal/feed"
	"github.com/yilong-musk/feedmesh/internal/logx"
	"github.com/yilong-musk/feedmesh/internal/v2ex"
)

func v2exService(t *testing.T) *feed.Service {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/index.xml":
			fmt.Fprint(w, `<?xml version="1.0" encoding="utf-8"?>
<feed xmlns="http://www.w3.org/2005/Atom"><entry>
<title>[分享创造] 测试帖</title>
<link rel="alternate" href="https://www.v2ex.com/t/777#reply2" />
<published>2026-10-23T10:00:00Z</published>
<author><name>alice</name></author>
<content type="html"><![CDATA[<p>正文</p><img src="https://img.example/x.png" />]]></content>
</entry></feed>`)
		case r.URL.Path == "/" && r.URL.Query().Get("tab") == "hot":
			blocks := ""
			for i, replies := range []int{3, 40, 12, 7, 1} {
				blocks += fmt.Sprintf(`<div class="cell item"><a href="/t/%d"></a>
<span class="item_title"><a href="/t/%d">热帖%d</a></span>
<a class="node" href="/go/programmer">程序员</a>
<a class="count_livid" href="/t/%d">%d</a></div>`, 800+i, 800+i, i, 800+i, replies)
			}
			fmt.Fprint(w, "<html><body>"+blocks+"</body></html>")
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	vx := v2ex.New(v2ex.Config{BaseURL: srv.URL, Timeout: 2 * time.Second})
	return feed.NewService(feed.Clients{V2ex: vx}, nil, logx.Discard(), feed.DefaultTTLs())
}

func TestGetV2exLatest(t *testing.T) {
	f, err := v2exService(t).GetV2ex(context.Background(), "latest", 5, false)
	if err != nil {
		t.Fatalf("GetV2ex: %v", err)
	}
	if f.Title() != "V2EX 最新" || f.Link() != "https://www.v2ex.com" {
		t.Fatalf("feed meta = %q / %q", f.Title(), f.Link())
	}
	if len(f.Items) != 1 {
		t.Fatalf("items = %d", len(f.Items))
	}
	it := f.Items[0]
	if it.URL != "https://www.v2ex.com/t/777" || !strings.Contains(it.HTML, "正文") {
		t.Fatalf("item = %+v", it)
	}
	if !strings.Contains(it.HTML, "@alice") {
		t.Fatalf("author missing: %s", it.HTML)
	}
	if len(it.Images) != 1 || it.Images[0] != "https://img.example/x.png" {
		t.Fatalf("images = %v", it.Images)
	}
}

func TestGetV2exHotSortsByReplies(t *testing.T) {
	f, err := v2exService(t).GetV2ex(context.Background(), "hot", 5, false)
	if err != nil {
		t.Fatalf("GetV2ex hot: %v", err)
	}
	if f.Title() != "V2EX 最热" {
		t.Fatalf("title = %q", f.Title())
	}
	if len(f.Items) != 5 {
		t.Fatalf("items = %d", len(f.Items))
	}
	if f.Items[0].Title != "热帖1" {
		t.Fatalf("first item = %q, want the 40-reply topic", f.Items[0].Title)
	}
	if !strings.Contains(f.Items[0].HTML, "40 条回复") || !strings.Contains(f.Items[0].HTML, "#程序员") {
		t.Fatalf("hot meta missing: %s", f.Items[0].HTML)
	}
}

func TestGetV2exRejectsUnknownSection(t *testing.T) {
	_, err := v2exService(t).GetV2ex(context.Background(), "nope", 5, false)
	if err == nil {
		t.Fatal("expected an error")
	}
}
