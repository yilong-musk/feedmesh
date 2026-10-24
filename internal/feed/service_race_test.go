package feed_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/yilong-musk/feedmesh/internal/cache"
	"github.com/yilong-musk/feedmesh/internal/feed"
	"github.com/yilong-musk/feedmesh/internal/weibo"
)

// Two callers can share one *weibo.Timeline through single-flight. Hydration
// mutates MBlog.Text, so the service must copy the posts before filling in long
// text; without that this test trips the race detector.
func TestGetFeedConcurrentLongTextHydration(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/visitor":
			http.SetCookie(w, &http.Cookie{Name: "SUB", Value: "x", Path: "/"})
			fmt.Fprint(w, `{"retcode":20000000}`)
		case "/api/container/getIndex":
			time.Sleep(50 * time.Millisecond)
			if r.URL.Query().Get("containerid") == "" {
				fmt.Fprint(w, `{"ok":1,"data":{"userInfo":{"id":1,"screen_name":"u"}}}`)
				return
			}
			fmt.Fprint(w, `{"ok":1,"data":{"cards":[{"mblog":{
				"id":"1001","isLongText":true,
				"created_at":"Fri Oct 23 14:30:04 +0800 2026",
				"text":"<p>truncated</p>"}}]}}`)
		case "/statuses/extend":
			time.Sleep(50 * time.Millisecond)
			fmt.Fprint(w, `{"ok":1,"data":{"longTextContent":"<p>full body</p>"}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	client := weibo.New(weibo.Config{
		BaseURL:    srv.URL,
		VisitorURL: srv.URL + "/visitor",
		Timeout:    2 * time.Second,
		Backoff:    time.Millisecond,
	})
	c, err := cache.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	svc := feed.NewService(feed.Clients{Weibo: client}, c, nil, feed.DefaultTTLs())

	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if _, err := svc.GetFeed(context.Background(), "1", 10, false); err != nil {
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
