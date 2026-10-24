package feed_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
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
	if f.Title() != "Weibo Hot Search" {
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
	if f.Items[0].Rank != 1 || f.Items[1].Rank != 2 {
		t.Fatalf("ranks = %d,%d", f.Items[0].Rank, f.Items[1].Rank)
	}
	if !strings.Contains(f.Items[0].HTML, "Heat 1.5M") || !strings.Contains(f.Items[0].HTML, "剧集") {
		t.Fatalf("description missing heat/category: %s", f.Items[0].HTML)
	}
	if !strings.Contains(f.Items[1].URL, "%23") {
		t.Fatalf("topic link should wrap the word in hashes: %s", f.Items[1].URL)
	}

	raw := string(feed.RenderRSS(f))
	if !strings.Contains(raw, "<title>Weibo Hot Search</title>") || !strings.Contains(raw, "Search on Weibo") {
		t.Fatalf("rendered hot feed is missing expected content:\n%s", raw)
	}
	if !strings.Contains(raw, "<ttl>5</ttl>") {
		t.Fatalf("hot feed should advertise ttl 5:\n%s", raw)
	}
}

func TestGetHotUsesRealposOrderAndRank(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/visitor":
			http.SetCookie(w, &http.Cookie{Name: "SUB", Value: "x", Path: "/"})
			fmt.Fprint(w, `{"retcode":20000000}`)
		case "/hot":
			fmt.Fprint(w, `{"ok":1,"data":{"realtime":[
				{"word":"third","word_scheme":"third","num":100,"realpos":3},
				{"word":"first","word_scheme":"first","num":300,"realpos":1}
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
	svc := feed.NewService(feed.Clients{Weibo: client}, nil, logx.Discard(), feed.DefaultTTLs())

	f, err := svc.GetHot(context.Background(), 2, false)
	if err != nil {
		t.Fatalf("GetHot: %v", err)
	}
	if len(f.Items) != 2 {
		t.Fatalf("items = %d", len(f.Items))
	}
	if f.Items[0].Title != "1. first" || f.Items[0].Rank != 1 {
		t.Fatalf("first item = %+v", f.Items[0])
	}
	if f.Items[1].Title != "3. third" || f.Items[1].Rank != 3 {
		t.Fatalf("second item = %+v", f.Items[1])
	}
}

func TestGetHotConcurrentSharedValue(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/visitor":
			http.SetCookie(w, &http.Cookie{Name: "SUB", Value: "x", Path: "/"})
			fmt.Fprint(w, `{"retcode":20000000}`)
		case "/hot":
			time.Sleep(50 * time.Millisecond)
			fmt.Fprint(w, `{"ok":1,"data":{"realtime":[
				{"word":"second","word_scheme":"second","num":100,"realpos":2},
				{"word":"first","word_scheme":"first","num":300,"realpos":1}
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
	svc := feed.NewService(feed.Clients{Weibo: client}, nil, logx.Discard(), feed.DefaultTTLs())

	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if _, err := svc.GetHot(context.Background(), 2, false); err != nil {
				errs <- err
			}
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
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
		{999, "999"},
		{1000, "1.0K"},
		{9999, "10.0K"},
		{10000, "10.0K"},
		{1488601, "1.5M"},
		{123456789, "123.5M"},
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
