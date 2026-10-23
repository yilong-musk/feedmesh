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
	"github.com/yilong-musk/feedmesh/internal/weibo"
)

func hotService(t *testing.T) *feed.Service {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/visitor":
			http.SetCookie(w, &http.Cookie{Name: "SUB", Value: "x", Path: "/"})
			fmt.Fprint(w, `{"retcode":20000000}`)
		case "/hot":
			fmt.Fprint(w, `{"ok":1,"data":{"realtime":[
				{"word":"小城良方 剧情硬伤","word_scheme":"小城良方 剧情硬伤","num":1488601,"label_name":"新","flag_desc":"剧集","topic_flag":0,"realpos":1},
				{"word":"晋江式演技正在毁掉长剧演员","word_scheme":"#晋江式演技正在毁掉长剧演员#","num":116836,"label_name":"热","flag_desc":"综艺","topic_flag":1,"realpos":2}
			]}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	client := weibo.New(weibo.Config{
		BaseURL: srv.URL, VisitorURL: srv.URL + "/visitor", HotURL: srv.URL + "/hot",
		Timeout: 2 * time.Second, Backoff: 5 * time.Millisecond,
	})
	return feed.NewService(feed.Clients{Weibo: client}, nil, logx.Discard(), feed.DefaultTTLs())
}

func TestGetHotBuildsFeed(t *testing.T) {
	svc := hotService(t)
	f, err := svc.GetHot(context.Background(), 2, false)
	if err != nil {
		t.Fatalf("GetHot: %v", err)
	}
	if f.Title() != "微博热搜" {
		t.Fatalf("title = %q", f.Title())
	}
	if f.Link() != "https://s.weibo.com/top/summary" {
		t.Fatalf("link = %q", f.Link())
	}
	if len(f.Items) != 2 {
		t.Fatalf("items = %d", len(f.Items))
	}
	if f.Items[0].Title != "1. 小城良方 剧情硬伤" {
		t.Fatalf("first title = %q", f.Items[0].Title)
	}
	if !strings.Contains(f.Items[0].HTML, "热度 148.9 万") || !strings.Contains(f.Items[0].HTML, "剧集") {
		t.Fatalf("description missing heat/category: %s", f.Items[0].HTML)
	}
	if !strings.Contains(f.Items[1].URL, "%23") {
		t.Fatalf("topic link should wrap the word in hashes: %s", f.Items[1].URL)
	}

	raw := string(feed.RenderRSS(f))
	if !strings.Contains(raw, "<title>微博热搜</title>") || !strings.Contains(raw, "在微博搜索") {
		t.Fatalf("rendered hot feed is missing expected content:\n%s", raw)
	}
	if !strings.Contains(raw, "<ttl>5</ttl>") {
		t.Fatalf("hot feed should advertise ttl 5:\n%s", raw)
	}
}

func TestGetHotRespectsLimit(t *testing.T) {
	svc := hotService(t)
	f, err := svc.GetHot(context.Background(), 1, false)
	if err != nil {
		t.Fatalf("GetHot: %v", err)
	}
	if len(f.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(f.Items))
	}
}

func TestFormatHeat(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{1, "1"},
		{9999, "9999"},
		{10000, "1.0 万"},
		{1488601, "148.9 万"},
		{123456789, "1.2 亿"},
	}
	for _, c := range cases {
		if got := feed.FormatHeat(c.in); got != c.want {
			t.Errorf("FormatHeat(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestHotSearchLink(t *testing.T) {
	topic := weibo.HotItem{Word: "某话题", TopicFlag: 1}
	if got := feed.HotSearchLink(topic); !strings.Contains(got, "%23%E6%9F%90%E8%AF%9D%E9%A2%98%23") {
		t.Fatalf("topic link = %q", got)
	}
	plain := weibo.HotItem{Word: "某关键词"}
	if got := feed.HotSearchLink(plain); strings.Contains(got, "%23") {
		t.Fatalf("plain link should not add hashes: %q", got)
	}
}
