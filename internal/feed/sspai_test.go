package feed_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/yilong-musk/feedmesh/internal/feed"
	"github.com/yilong-musk/feedmesh/internal/logx"
	"github.com/yilong-musk/feedmesh/internal/sspai"
)

func sspaiService(t *testing.T) *feed.Service {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/article/index/page/get":
			items := []map[string]any{
				{"id": 2001, "title": "效率工具指南", "summary": "这是一段摘要",
					"banner": "2026/10/23/a.png", "released_time": 1792750000,
					"author": map[string]any{"nickname": "小明"}},
				{"id": 2002, "title": "会员文章", "summary": "会员摘要",
					"banner": "2026/10/23/b.png", "released_time": 1792749000,
					"author": map[string]any{"nickname": "小红"}},
			}
			b, _ := json.Marshal(map[string]any{"error": 0, "data": items})
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
	sp := sspai.New(sspai.Config{BaseURL: srv.URL, Timeout: 2 * time.Second})
	return feed.NewService(nil, sp, nil, logx.Discard(), feed.DefaultTTLs())
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
