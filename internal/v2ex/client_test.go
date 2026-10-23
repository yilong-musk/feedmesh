package v2ex_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/yilong-musk/feedmesh/internal/v2ex"
)

const atomFixture = `<?xml version="1.0" encoding="utf-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
<title>V2EX</title>
<entry>
  <title>[分享创造] 一个测试主题</title>
  <link rel="alternate" type="text/html" href="https://www.v2ex.com/t/1234#reply3" />
  <published>2026-10-23T10:51:27Z</published>
  <author><name>alice</name></author>
  <content type="html"><![CDATA[<p>正文内容</p><img src="https://img.example/a.png" />]]></content>
</entry>
</feed>`

const hotFixture = `<html><body>
<div class="cell item"><a href="/t/100" class="topic-link"></a>
  <span class="item_title"><a href="/t/100#reply12">主题甲</a></span>
  <a class="node" href="/go/programmer">程序员</a>
  <a class="count_livid" href="/t/100#reply12">12</a>
</div>
<div class="cell item"><a href="/t/101"></a>
  <span class="item_title"><a href="/t/101">主题乙</a></span>
  <a class="node" href="/go/qna">问与答</a>
  <a class="count_livid" href="/t/101">99</a>
</div>
<div class="cell item"><a href="/t/103"></a>
  <span class="item_title"><a href="/t/103">主题103</a></span>
  <a class="node" href="/go/test">测试</a>
  <a class="count_livid" href="/t/103">5</a>
</div>
<div class="cell item"><a href="/t/104"></a>
  <span class="item_title"><a href="/t/104">主题104</a></span>
  <a class="node" href="/go/test">测试</a>
  <a class="count_livid" href="/t/104">4</a>
</div>
<div class="cell item"><a href="/t/105"></a>
  <span class="item_title"><a href="/t/105">主题105</a></span>
  <a class="node" href="/go/test">测试</a>
  <a class="count_livid" href="/t/105">3</a>
</div>
<div class="cell item"><a href="/t/102"></a>
  <span class="item_title"><a href="/t/102">推广主题</a></span>
  <a class="node" href="/go/promotion">推广</a>
  <a class="count_livid" href="/t/102">500</a>
</div>
</body></html>`

func TestLatestParsesAtom(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/index.xml" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, atomFixture)
	}))
	t.Cleanup(srv.Close)

	c := v2ex.New(v2ex.Config{BaseURL: srv.URL, Timeout: 2 * time.Second})
	items, err := c.Latest(context.Background(), 10)
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("items = %d", len(items))
	}
	it := items[0]
	if it.Title != "[分享创造] 一个测试主题" || it.URL != "https://www.v2ex.com/t/1234" {
		t.Fatalf("item = %+v", it)
	}
	if it.Author != "alice" || it.ID != 1234 {
		t.Fatalf("author/id = %q/%d", it.Author, it.ID)
	}
	if it.Published.IsZero() || it.Content == "" {
		t.Fatalf("published/content missing: %+v", it)
	}
}

func TestHotSortsAndFiltersPromoted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" && r.URL.Query().Get("tab") == "hot" {
			fmt.Fprint(w, hotFixture)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	c := v2ex.New(v2ex.Config{BaseURL: srv.URL, Timeout: 2 * time.Second})
	items, err := c.Hot(context.Background(), 10)
	if err != nil {
		t.Fatalf("Hot: %v", err)
	}
	if len(items) != 5 {
		t.Fatalf("items = %d, want 5 (promoted node must be dropped)", len(items))
	}
	if items[0].Title != "主题乙" || items[0].Replies != 99 {
		t.Fatalf("sort order wrong: %+v", items)
	}
	if items[0].Node != "问与答" || items[0].URL != "https://www.v2ex.com/t/101" {
		t.Fatalf("item = %+v", items[0])
	}
}

func TestHotFallsBackToAPI(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/" && r.URL.Query().Get("tab") == "hot":
			fmt.Fprint(w, `<html><body><div class="cell item">broken</div></body></html>`)
		case r.URL.Path == "/api/topics/hot.json":
			fmt.Fprint(w, `[{"id":200,"title":"API 主题","url":"https://www.v2ex.com/t/200","replies":7,"created":1792718561,"node":{"title":"分享"},"content_rendered":"<p>API 正文</p>"}]`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	c := v2ex.New(v2ex.Config{BaseURL: srv.URL, Timeout: 2 * time.Second})
	items, err := c.Hot(context.Background(), 10)
	if err != nil {
		t.Fatalf("Hot fallback: %v", err)
	}
	if len(items) != 1 || items[0].Title != "API 主题" || items[0].Node != "分享" {
		t.Fatalf("items = %+v", items)
	}
}
