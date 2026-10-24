package feed_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/yilong-musk/feedmesh/internal/cache"
	"github.com/yilong-musk/feedmesh/internal/feed"
	"github.com/yilong-musk/feedmesh/internal/logx"
	"github.com/yilong-musk/feedmesh/internal/v2ex"
)

// v2exUpstream serves 12 index.xml entries and 12 hot-tab rows, so both feeds
// can be asked for more than the smallest limit a previous request used.
func v2exUpstream(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/index.xml":
			entries := ""
			for i := 0; i < 12; i++ {
				id := 900 + i
				title := fmt.Sprintf("测试帖%d", i)
				content := "<p>正文</p>"
				pub := "2026-10-23T10:00:00Z"
				if i == 0 {
					id, title = 777, "[分享创造] 测试帖"
					content = `<p>正文</p><img src="https://img.example/x.png" />`
				}
				entries += fmt.Sprintf(`<entry>
<title>%s</title>
<link rel="alternate" href="https://www.v2ex.com/t/%d#reply2" />
<published>%s</published>
<author><name>alice</name></author>
<content type="html"><![CDATA[%s]]></content>
</entry>`, title, id, pub, content)
			}
			fmt.Fprintf(w, `<?xml version="1.0" encoding="utf-8"?><feed xmlns="http://www.w3.org/2005/Atom">%s</feed>`, entries)
		case r.URL.Path == "/" && r.URL.Query().Get("tab") == "hot":
			replies := []int{3, 40, 12, 7, 1, 22, 9, 15, 31, 4, 18, 6}
			blocks := ""
			for i, rep := range replies {
				blocks += fmt.Sprintf(`<div class="cell item"><a href="/t/%d"></a>
<span class="item_title"><a href="/t/%d">热帖%d</a></span>
<a class="node" href="/go/programmer">程序员</a>
<a class="count_livid" href="/t/%d">%d</a></div>`, 800+i, 800+i, i, 800+i, rep)
			}
			fmt.Fprint(w, "<html><body>"+blocks+"</body></html>")
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func v2exService(t *testing.T) *feed.Service {
	t.Helper()
	vx := v2ex.New(v2ex.Config{BaseURL: v2exUpstream(t).URL, Timeout: 2 * time.Second})
	c, err := cache.New(t.TempDir())
	if err != nil {
		t.Fatalf("cache: %v", err)
	}
	return feed.NewService(feed.Clients{V2ex: vx}, c, logx.Discard(), feed.DefaultTTLs())
}

func TestGetV2exLatest(t *testing.T) {
	f, err := v2exService(t).GetV2ex(context.Background(), "latest", 5, false)
	if err != nil {
		t.Fatalf("GetV2ex: %v", err)
	}
	if f.Title() != "V2EX Latest" || f.Link() != "https://www.v2ex.com" {
		t.Fatalf("feed meta = %q / %q", f.Title(), f.Link())
	}
	if len(f.Items) != 5 {
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
	if f.Title() != "V2EX Hot" {
		t.Fatalf("title = %q", f.Title())
	}
	if len(f.Items) != 5 {
		t.Fatalf("items = %d", len(f.Items))
	}
	if f.Items[0].Title != "热帖1" {
		t.Fatalf("first item = %q, want the 40-reply topic", f.Items[0].Title)
	}
	if !strings.Contains(f.Items[0].HTML, "40 replies") || !strings.Contains(f.Items[0].HTML, "#程序员") {
		t.Fatalf("hot meta missing: %s", f.Items[0].HTML)
	}
}

func TestGetV2exRejectsUnknownSection(t *testing.T) {
	_, err := v2exService(t).GetV2ex(context.Background(), "nope", 5, false)
	if err == nil {
		t.Fatal("expected an error")
	}
}

// Both sections used to cache the caller's slice, so one ?limit=3 request cut
// every later request down to three items for the whole TTL.
func TestGetV2exSmallLimitDoesNotTruncateCache(t *testing.T) {
	for _, section := range []string{"latest", "hot"} {
		t.Run(section, func(t *testing.T) {
			svc := v2exService(t)
			if _, err := svc.GetV2ex(context.Background(), section, 3, false); err != nil {
				t.Fatalf("warm limit=3: %v", err)
			}
			f, err := svc.GetV2ex(context.Background(), section, 10, false)
			if err != nil {
				t.Fatalf("limit=10: %v", err)
			}
			if len(f.Items) != 10 {
				t.Fatalf("items after warm limit=3 = %d, want 10", len(f.Items))
			}

			g, err := svc.GetV2ex(context.Background(), section, 5, false)
			if err != nil {
				t.Fatalf("limit=5: %v", err)
			}
			if !g.Cached {
				t.Fatal("limit=5 after limit=10 should be a cache hit")
			}
			if len(g.Items) != 5 {
				t.Fatalf("cached limit=5 items = %d", len(g.Items))
			}
		})
	}
}

// Asking for more than upstream carries must not turn into a refetch loop:
// the entry remembers how wide it was fetched, so 50 against 12 rows is final.
func TestGetV2exOverAskStaysCached(t *testing.T) {
	svc := v2exService(t)
	first, err := svc.GetV2ex(context.Background(), "hot", 50, false)
	if err != nil {
		t.Fatalf("limit=50: %v", err)
	}
	if len(first.Items) != 12 {
		t.Fatalf("limit=50 items = %d, want all 12 upstream rows", len(first.Items))
	}
	second, err := svc.GetV2ex(context.Background(), "hot", 50, false)
	if err != nil {
		t.Fatalf("second limit=50: %v", err)
	}
	if !second.Cached {
		t.Fatal("repeat limit=50 should reuse the cached list")
	}
	if len(second.Items) != 12 {
		t.Fatalf("cached limit=50 items = %d", len(second.Items))
	}
}
