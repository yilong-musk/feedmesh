package feed_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/yilong-musk/feedmesh/internal/cache"
	"github.com/yilong-musk/feedmesh/internal/feed"
	"github.com/yilong-musk/feedmesh/internal/logx"
	"github.com/yilong-musk/feedmesh/internal/sspai"
)

// sspaiUpstream is the fake article API. It serves 12 articles so paging and
// limit handling can be exercised (the real endpoint caps a page at ten).
func sspaiUpstream(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/article/index/page/get":
			all := make([]map[string]any, 0, 12)
			for i := 0; i < 12; i++ {
				all = append(all, map[string]any{
					"id":            2001 + i,
					"title":         fmt.Sprintf("文章 %d", 2001+i),
					"summary":       "摘要",
					"banner":        "2026/10/23/a.png",
					"released_time": 1792750000 - i*1000,
					"author":        map[string]any{"nickname": "作者乙"},
				})
			}
			all[0]["title"] = "效率工具指南"
			all[0]["summary"] = "这是一段摘要"
			all[0]["author"] = map[string]any{"nickname": "小明"}
			all[1]["title"] = "会员文章"
			all[1]["summary"] = "会员摘要"
			all[1]["author"] = map[string]any{"nickname": "小红"}

			offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
			limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
			if offset > len(all) {
				offset = len(all)
			}
			end := len(all)
			if limit > 0 && offset+limit < end {
				end = offset + limit
			}
			b, _ := json.Marshal(map[string]any{"error": 0, "data": all[offset:end]})
			w.Write(b)
		case "/api/v1/article/info/get":
			id := r.URL.Query().Get("id")
			body := fmt.Sprintf("<p>正文 %s</p>", id)
			pay := id == "2002"
			b, _ := json.Marshal(map[string]any{
				"error": 0,
				"data": map[string]any{
					"body": body, "keywords": "效率,工具",
					"is_pay": pay, "is_free": !pay,
				},
			})
			w.Write(b)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func sspaiService(t *testing.T) *feed.Service {
	t.Helper()
	sp := sspai.New(sspai.Config{BaseURL: sspaiUpstream(t).URL, Timeout: 2 * time.Second})
	c, err := cache.New(t.TempDir())
	if err != nil {
		t.Fatalf("cache: %v", err)
	}
	return feed.NewService(feed.Clients{Sspai: sp}, c, logx.Discard(), feed.DefaultTTLs())
}

func TestGetSspaiFullText(t *testing.T) {
	f, err := sspaiService(t).GetSspai(context.Background(), 2, true, false)
	if err != nil {
		t.Fatalf("GetSspai: %v", err)
	}
	if f.Title() != "少数派" || f.Link() != "https://sspai.com" {
		t.Fatalf("feed meta = %q / %q", f.Title(), f.Link())
	}
	if len(f.Items) != 2 {
		t.Fatalf("items = %d", len(f.Items))
	}
	it := f.Items[0]
	if it.Title != "效率工具指南" || it.URL != "https://sspai.com/post/2001" {
		t.Fatalf("item = %+v", it)
	}
	if !strings.Contains(it.HTML, "正文 2001") {
		t.Fatalf("full body missing: %s", it.HTML)
	}
	if !strings.Contains(it.HTML, "小明") || !strings.Contains(it.HTML, "效率,工具") {
		t.Fatalf("author/keywords missing: %s", it.HTML)
	}
	if !strings.Contains(it.HTML, `src="https://rssfile.sspai.com/2026/10/23/a.png"`) {
		t.Fatalf("cover missing: %s", it.HTML)
	}
	if len(it.Images) != 1 || it.Images[0] != "https://rssfile.sspai.com/2026/10/23/a.png" {
		t.Fatalf("images = %v", it.Images)
	}
	if !it.CreatedAt.Equal(time.Unix(1792750000, 0).UTC()) {
		t.Fatalf("created = %v", it.CreatedAt)
	}
	if !strings.Contains(f.Items[1].HTML, "会员文章，仅显示摘要") {
		t.Fatalf("paywall note missing: %s", f.Items[1].HTML)
	}
}

func TestGetSspaiSummaryOnly(t *testing.T) {
	f, err := sspaiService(t).GetSspai(context.Background(), 1, false, false)
	if err != nil {
		t.Fatalf("GetSspai: %v", err)
	}
	if len(f.Items) != 1 {
		t.Fatalf("items = %d", len(f.Items))
	}
	html := f.Items[0].HTML
	if strings.Contains(html, "正文 2001") {
		t.Fatalf("summary mode must not fetch the body: %s", html)
	}
	if !strings.Contains(html, "这是一段摘要") || !strings.Contains(html, "阅读全文") {
		t.Fatalf("summary/read-more missing: %s", html)
	}
}

// A ?limit=3 request used to store a three-item list that every later request
// for the whole TTL was truncated to.
func TestGetSspaiSmallLimitDoesNotTruncateCache(t *testing.T) {
	svc := sspaiService(t)
	if _, err := svc.GetSspai(context.Background(), 3, false, false); err != nil {
		t.Fatalf("warm limit=3: %v", err)
	}
	f, err := svc.GetSspai(context.Background(), 10, false, false)
	if err != nil {
		t.Fatalf("limit=10: %v", err)
	}
	if len(f.Items) != 10 {
		t.Fatalf("items after warm limit=3 = %d, want 10", len(f.Items))
	}

	// The widened entry then serves smaller limits straight from cache.
	g, err := svc.GetSspai(context.Background(), 5, false, false)
	if err != nil {
		t.Fatalf("limit=5: %v", err)
	}
	if !g.Cached {
		t.Fatal("limit=5 after limit=10 should be a cache hit")
	}
	if len(g.Items) != 5 {
		t.Fatalf("cached limit=5 items = %d", len(g.Items))
	}
}

// The RSS route and the JSON view share one entry: a wide fetch warms it for
// smaller readers instead of re-hitting sspai.
func TestGetSspaiWideFetchServesSmallerLimits(t *testing.T) {
	svc := sspaiService(t)
	wide, err := svc.GetSspai(context.Background(), 30, false, false)
	if err != nil {
		t.Fatalf("limit=30: %v", err)
	}
	if len(wide.Items) != 12 {
		t.Fatalf("limit=30 items = %d, want all 12 upstream articles", len(wide.Items))
	}
	narrow, err := svc.GetSspai(context.Background(), 10, false, false)
	if err != nil {
		t.Fatalf("limit=10: %v", err)
	}
	if !narrow.Cached {
		t.Fatal("limit=10 after limit=30 should reuse the cached list")
	}
	if len(narrow.Items) != 10 {
		t.Fatalf("cached limit=10 items = %d", len(narrow.Items))
	}
}
