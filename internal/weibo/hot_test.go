package weibo_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/yilong-musk/feedmesh/internal/weibo"
)

func newHotClient(t *testing.T, base string) *weibo.Client {
	t.Helper()
	return weibo.New(weibo.Config{
		BaseURL:    base,
		VisitorURL: base + "/visitor",
		HotURL:     base + "/hot",
		Timeout:    2 * time.Second,
		Backoff:    5 * time.Millisecond,
	})
}

func TestHotSearchParsesAndRanks(t *testing.T) {
	srv := fakeUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/visitor":
			http.SetCookie(w, &http.Cookie{Name: "SUB", Value: "x", Path: "/"})
			fmt.Fprint(w, `{"retcode":20000000}`)
		case "/hot":
			if !strings.HasPrefix(r.Header.Get("Referer"), "https://weibo.com") {
				t.Errorf("hot request referer = %q, want weibo.com", r.Header.Get("Referer"))
			}
			fmt.Fprint(w, `{"ok":1,"data":{"realtime":[
				{"word":"小城良方 剧情硬伤","word_scheme":"小城良方 剧情硬伤","num":1488601,"label_name":"新","flag_desc":"剧集","topic_flag":0,"realpos":1},
				{"word":"晋江式演技正在毁掉长剧演员","word_scheme":"#晋江式演技正在毁掉长剧演员#","num":116836,"label_name":"热","flag_desc":"综艺","topic_flag":1,"realpos":2},
				{"word":"霜降是秋天最后的告别","num":1055554,"icon_desc":"荐"},
				{"word":"","word_scheme":"","num":1}
			]}}`)
		default:
			http.NotFound(w, r)
		}
	})

	items, err := newHotClient(t, srv.URL).HotSearch(context.Background())
	if err != nil {
		t.Fatalf("HotSearch: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("items = %d, want 2 (empty word must be skipped)", len(items))
	}
	if items[0].Rank != 1 || items[1].Rank != 2 {
		t.Fatalf("ranks = %d,%d, want 1,2", items[0].Rank, items[1].Rank)
	}
	if items[1].WordScheme != "晋江式演技正在毁掉长剧演员" {
		t.Fatalf("word_scheme should lose its hashes, got %q", items[1].WordScheme)
	}
	if items[0].Heat != 1488601 {
		t.Fatalf("heat = %d", items[0].Heat)
	}
}

func TestHotSearchRejectsBadPayload(t *testing.T) {
	srv := fakeUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/visitor" {
			http.SetCookie(w, &http.Cookie{Name: "SUB", Value: "x", Path: "/"})
			fmt.Fprint(w, `{"retcode":20000000}`)
			return
		}
		fmt.Fprint(w, `{"ok":0,"data":{"realtime":[]}}`)
	})

	_, err := newHotClient(t, srv.URL).HotSearch(context.Background())
	if err == nil {
		t.Fatal("expected an error for ok=0")
	}
	if code := weibo.CodeOf(err); code != weibo.CodeUpstreamUnavailable {
		t.Fatalf("code = %q, want UPSTREAM_UNAVAILABLE", code)
	}
}
