package sspai_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/yilong-musk/feedmesh/internal/sspai"
)

func fakeSSPai(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/article/index/page/get":
			offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
			items := make([]map[string]any, 0, 10)
			for i := 0; i < 10; i++ {
				id := 1000 + offset + i
				if offset >= 20 {
					break
				}
				items = append(items, map[string]any{
					"id": id, "title": fmt.Sprintf("文章 %d", id),
					"summary": "摘要", "banner": fmt.Sprintf("2026/10/23/b%d.png", id),
					"released_time": 1792750000 + id,
					"author":        map[string]any{"nickname": "作者"},
				})
			}
			fmt.Fprintf(w, `{"error":0,"data":%s}`, mustJSON(items))
		case "/api/v1/article/info/get":
			id := r.URL.Query().Get("id")
			fmt.Fprintf(w, `{"error":0,"data":{"body":"<p>正文 %s</p>","keywords":"效率,工具","is_pay":false,"is_free":true}}`, id)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func newClient(base string) *sspai.Client {
	return sspai.New(sspai.Config{BaseURL: base, Timeout: 2 * time.Second})
}

func TestListPagesThroughAPI(t *testing.T) {
	srv := fakeSSPai(t)
	items, err := newClient(srv.URL).List(context.Background(), 15)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 15 {
		t.Fatalf("items = %d, want 15", len(items))
	}
	if items[0].URL() != "https://sspai.com/post/1000" {
		t.Fatalf("url = %q", items[0].URL())
	}
	if items[0].CoverURL() != sspai.ImageBaseURL+"2026/10/23/b1000.png" {
		t.Fatalf("cover = %q", items[0].CoverURL())
	}
	if items[0].Author.Nickname != "作者" {
		t.Fatalf("author = %q", items[0].Author.Nickname)
	}
}

func TestDetailParsesBodyAndPaywall(t *testing.T) {
	srv := fakeSSPai(t)
	a, err := newClient(srv.URL).Detail(context.Background(), 1001)
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	if a.Body == "" || a.Keywords != "效率,工具" || a.IsPay {
		t.Fatalf("detail = %+v", a)
	}
}
