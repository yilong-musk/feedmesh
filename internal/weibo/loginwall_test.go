package weibo_test

import (
	"context"
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yilong-musk/feedmesh/internal/weibo"
)

// loginWallBody is weibo's soft login wall: HTTP 200 and ok:1, yet no posts.
// Measured 2026-11-02 for one account: the same request seconds later returns
// the real timeline, so the payload must be treated as a retryable block.
const loginWallBody = `{"ok":1,"data":{"cardlistInfo":{"containerid":"1076031734741902","total":0,"page":null},"cards":[{"card_type":7,"desc":"登录注册后查看更多微博","desc_alignment":1}]}}`

// emptyAccountBody is what a real account without posts returns.
const emptyAccountBody = `{"ok":1,"data":{"cardlistInfo":{"containerid":"1076031734741902","total":0,"page":null},"cards":[{"card_type":58,"itemid":"","name":"暂无微博"}]}}`

const timelinePageBody = `{"ok":1,"data":{"cardlistInfo":{"containerid":"1076031734741902","total":1},"cards":[{"card_type":9,"mblog":{"id":"1001","created_at":"Fri Oct 23 14:30:04 +0800 2026","text":"<p>hello timeline</p>"}}]}}`

// walledUpstream serves the visitor cookie, then body until it is swapped.
func walledUpstream(t *testing.T, hits *atomic.Int32, body func() string) string {
	t.Helper()
	srv := fakeUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/visitor" {
			http.SetCookie(w, &http.Cookie{Name: "SUB", Value: "x", Path: "/"})
			fmt.Fprint(w, `{"retcode":20000000}`)
			return
		}
		hits.Add(1)
		fmt.Fprint(w, body())
	})
	return srv.URL
}

func wallClient(t *testing.T, base string, wallRetries int) *weibo.Client {
	t.Helper()
	return weibo.New(weibo.Config{
		BaseURL:     base,
		VisitorURL:  base + "/visitor",
		Timeout:     2 * time.Second,
		Backoff:     5 * time.Millisecond,
		WallRetries: wallRetries,
	})
}

func TestTimelineRetriesPastTheLoginWall(t *testing.T) {
	var hits atomic.Int32
	// Two walls, then the real page: this is the upstream's actual behaviour.
	base := walledUpstream(t, &hits, func() string {
		if hits.Load() < 3 {
			return loginWallBody
		}
		return timelinePageBody
	})

	c := wallClient(t, base, 3)
	tl, err := c.Timeline(context.Background(), "1734741902", "", 0)
	if err != nil {
		t.Fatalf("Timeline: %v", err)
	}
	if len(tl.Items) != 1 || tl.Items[0].IDString() != "1001" {
		t.Fatalf("items = %+v, want the single post from the third attempt", tl.Items)
	}
	if got := hits.Load(); got != 3 {
		t.Fatalf("upstream hits = %d, want 3 (two walls then a page)", got)
	}
}

func TestTimelineReportsTheLoginWallAfterRetries(t *testing.T) {
	var hits atomic.Int32
	base := walledUpstream(t, &hits, func() string { return loginWallBody })

	c := wallClient(t, base, 2)
	_, err := c.Timeline(context.Background(), "1734741902", "", 0)
	if err == nil {
		t.Fatal("expected an error once every attempt hit the login wall")
	}
	if code := weibo.CodeOf(err); code != weibo.CodeLoginRequired {
		t.Fatalf("code = %q, want LOGIN_REQUIRED", code)
	}
	if got := hits.Load(); got != 3 {
		t.Fatalf("upstream hits = %d, want 3 (1 attempt + 2 wall retries)", got)
	}
}

func TestTimelineAcceptsAGenuinelyEmptyAccount(t *testing.T) {
	var hits atomic.Int32
	base := walledUpstream(t, &hits, func() string { return emptyAccountBody })

	c := wallClient(t, base, 3)
	tl, err := c.Timeline(context.Background(), "1734741902", "", 0)
	if err != nil {
		t.Fatalf("an empty account is an answer, not a block: %v", err)
	}
	if len(tl.Items) != 0 {
		t.Fatalf("items = %d, want 0", len(tl.Items))
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("upstream hits = %d, want 1 (an empty account must not be retried)", got)
	}
}
