package feed_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yilong-musk/feedmesh/internal/cache"
	"github.com/yilong-musk/feedmesh/internal/feed"
	"github.com/yilong-musk/feedmesh/internal/weibo"
)

// wallUpstream is the weibo stand-in for the login-wall case: it serves a real
// timeline until walled is set, then answers with the soft login wall.
// Fixtures reproduce upstream payloads byte for byte.
type wallUpstream struct {
	walled atomic.Bool
}

const wallTimelineJSON = `{"ok":1,"data":{"cardlistInfo":{"containerid":"1076031","total":1},"cards":[{"card_type":9,"mblog":{"id":"1001","created_at":"Fri Oct 23 14:30:04 +0800 2026","text":"<p>hello timeline</p>"}}]}}`

const wallPayloadJSON = `{"ok":1,"data":{"cardlistInfo":{"containerid":"1076031","total":0,"page":null},"cards":[{"card_type":7,"desc":"登录注册后查看更多微博","desc_alignment":1}]}}`

func (u *wallUpstream) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/visitor":
			http.SetCookie(w, &http.Cookie{Name: "SUB", Value: "x", Path: "/"})
			fmt.Fprint(w, `{"retcode":20000000}`)
		case "/api/container/getIndex":
			if r.URL.Query().Get("containerid") == "" {
				fmt.Fprint(w, `{"ok":1,"data":{"userInfo":{"id":1,"screen_name":"tester"}}}`)
				return
			}
			if u.walled.Load() {
				fmt.Fprint(w, wallPayloadJSON)
				return
			}
			fmt.Fprint(w, wallTimelineJSON)
		default:
			http.NotFound(w, r)
		}
	}
}

// TestGetFeedKeepsLastGoodWhenTheTimelineIsLoginWalled pins the bug that made
// one account report zero posts: a 200 login wall must not be stored as an
// empty timeline, and the previous posts must keep being served as degraded.
func TestGetFeedKeepsLastGoodWhenTheTimelineIsLoginWalled(t *testing.T) {
	up := &wallUpstream{}
	srv := httptest.NewServer(up.handler())
	defer srv.Close()

	dir := t.TempDir()
	c, err := cache.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	client := weibo.New(weibo.Config{
		BaseURL: srv.URL, VisitorURL: srv.URL + "/visitor",
		Timeout: 2 * time.Second, Backoff: time.Millisecond, WallRetries: 1,
	})
	svc := feed.NewService(feed.Clients{Weibo: client}, c, nil, feed.DefaultTTLs())
	ctx := context.Background()

	warm, err := svc.GetFeed(ctx, "1", 10, false)
	if err != nil {
		t.Fatalf("warm read: %v", err)
	}
	if len(warm.Items) != 1 || warm.Degraded {
		t.Fatalf("warm feed = %d items, degraded=%v; want 1 healthy item", len(warm.Items), warm.Degraded)
	}

	// Past the 5-minute freshness window, still inside the stale window.
	ageCacheEntry(t, dir, "timeline:1", 6*time.Minute)
	up.walled.Store(true)

	f, err := svc.GetFeed(ctx, "1", 10, false)
	if err != nil {
		t.Fatalf("a login wall must degrade to the last good timeline, got: %v", err)
	}
	if !f.Degraded {
		t.Fatal("degraded = false, want true when the timeline is login-walled")
	}
	if len(f.Items) != 1 {
		t.Fatalf("items = %d, want the cached post instead of an empty feed", len(f.Items))
	}

	// The cache itself must still hold the post, not an overwritten empty list.
	raw, err := os.ReadFile(filepath.Join(dir, cache.SafeKey("timeline:1")+".json"))
	if err != nil {
		t.Fatalf("read cached timeline: %v", err)
	}
	var entry struct {
		Value struct {
			Items []json.RawMessage `json:"items"`
		} `json:"value"`
	}
	if err := json.Unmarshal(raw, &entry); err != nil {
		t.Fatalf("decode cached timeline: %v", err)
	}
	if len(entry.Value.Items) != 1 {
		t.Fatalf("cached items = %d, want the original post (the login wall must not be cached)", len(entry.Value.Items))
	}
}
