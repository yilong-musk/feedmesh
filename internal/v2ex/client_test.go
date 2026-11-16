package v2ex_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestHotRejectsShortPage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><body><div class="cell item">broken</div></body></html>`)
	}))
	t.Cleanup(srv.Close)

	c := v2ex.New(v2ex.Config{BaseURL: srv.URL, Timeout: 2 * time.Second})
	if _, err := c.Hot(context.Background(), 10); err == nil {
		t.Fatal("expected an error instead of a substituted list")
	}
}

func TestHotRejectsCloudflareChallenge(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><head><title>Just a moment...</title></head><body></body></html>`)
	}))
	t.Cleanup(srv.Close)

	c := v2ex.New(v2ex.Config{BaseURL: srv.URL, Timeout: 2 * time.Second})
	_, err := c.Hot(context.Background(), 10)
	if err == nil || !strings.Contains(err.Error(), "Cloudflare") {
		t.Fatalf("err = %v, want a Cloudflare challenge error", err)
	}
}

func TestClientSendsNoAcceptLanguage(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("Accept-Language")
		fmt.Fprint(w, hotFixture)
	}))
	t.Cleanup(srv.Close)

	c := v2ex.New(v2ex.Config{BaseURL: srv.URL, Timeout: 2 * time.Second})
	if _, err := c.Hot(context.Background(), 10); err != nil {
		t.Fatalf("Hot: %v", err)
	}
	if got != "" {
		t.Fatalf("Accept-Language = %q; V2EX answers it with a Cloudflare challenge", got)
	}
}
