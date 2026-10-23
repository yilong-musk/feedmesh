package httpapi_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/yilong-musk/weibo-rss-pro/internal/cache"
	"github.com/yilong-musk/weibo-rss-pro/internal/feed"
	"github.com/yilong-musk/weibo-rss-pro/internal/guard"
	"github.com/yilong-musk/weibo-rss-pro/internal/httpapi"
	"github.com/yilong-musk/weibo-rss-pro/internal/logx"
	"github.com/yilong-musk/weibo-rss-pro/internal/weibo"
)

const profileJSON = `{"ok":1,"data":{"userInfo":{"id":2803301701,"screen_name":"人民日报",
	"description":"人民日报法人微博。"},"tabsInfo":{"tabs":[{"tab_type":"profile","containerid":"2302832803301701"},
	{"tab_type":"weibo","containerid":"1076032803301701"}]}}}`

const timelineJSON = `{"ok":1,"data":{"cards":[
 {"mblog":{"id":"1001","bid":"Abc1","created_at":"Fri Oct 23 14:30:04 +0800 2026","text":"<p>第一条</p>"}},
 {"mblog":{"id":"1002","bid":"Abc2","isLongText":true,"created_at":"Fri Oct 23 14:20:00 +0800 2026","text":"<p>第二条 截断…</p>"}},
 {"mblog":{"id":"1003","bid":"Abc3","created_at":"Fri Oct 23 14:10:00 +0800 2026","text":"<p>第三条</p>",
   "pics":[{"pid":"p1","url":"https://wx1.sinaimg.cn/orj360/p1.jpg","large":{"size":"large","url":"https://wx1.sinaimg.cn/mw2000/p1.jpg"}}]}}
]}}`

// fakeWeibo serves just enough of weibo for the whole pipeline to run.
func fakeWeibo(t *testing.T) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/visitor":
			http.SetCookie(w, &http.Cookie{Name: "SUB", Value: "_2Aktest", Path: "/"})
			fmt.Fprint(w, `window.visitor_gray_callback && visitor_gray_callback({"retcode":20000000})`)
		case r.URL.Path == "/api/container/getIndex":
			if r.URL.Query().Get("containerid") == "" {
				if r.URL.Query().Get("value") == "999" {
					fmt.Fprint(w, `{"ok":0,"msg":"这里还没有内容","data":{"cards":[]}}`)
					return
				}
				fmt.Fprint(w, profileJSON)
				return
			}
			fmt.Fprint(w, timelineJSON)
		case r.URL.Path == "/statuses/extend":
			fmt.Fprint(w, `{"ok":1,"data":{"longTextContent":"<p>第二条 完整正文</p>"}}`)
		case r.URL.Path == "/statuses/show":
			fmt.Fprint(w, `{"ok":1,"data":{"id":"1002","text":"<p>第二条 回退正文</p>"}}`)
		case r.URL.Path == "/hot":
			fmt.Fprint(w, `{"ok":1,"data":{"realtime":[
				{"word":"小城良方 剧情硬伤","word_scheme":"小城良方 剧情硬伤","num":1488601,"label_name":"新","flag_desc":"剧集","topic_flag":0,"realpos":1},
				{"word":"晋江式演技正在毁掉长剧演员","word_scheme":"#晋江式演技正在毁掉长剧演员#","num":116836,"label_name":"热","flag_desc":"综艺","topic_flag":1,"realpos":2}
			]}}`)
		case r.URL.Path == "/rmrb":
			http.Redirect(w, r, "/u/2803301701", http.StatusFound)
		case r.URL.Path == "/u/2803301701":
			fmt.Fprint(w, "<html>profile</html>")
		default:
			http.NotFound(w, r)
		}
	}
}

func newTestServer(t *testing.T) http.Handler {
	t.Helper()
	upstream := httptest.NewServer(fakeWeibo(t))
	t.Cleanup(upstream.Close)

	c, err := cache.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	breaker := guard.NewBreaker(10 * time.Minute)
	client := weibo.New(weibo.Config{
		BaseURL:    upstream.URL,
		VisitorURL: upstream.URL + "/visitor",
		HotURL:     upstream.URL + "/hot",
		Timeout:    2 * time.Second,
		Backoff:    time.Millisecond,
		Breaker:    breaker,
		Log:        logx.Discard(),
	})
	svc := feed.NewService(client, c, logx.Discard(), feed.DefaultTTLs())
	api := httpapi.New(httpapi.Config{
		Version: "test",
		Service: svc,
		Cache:   c,
		Client:  client,
		Breaker: breaker,
		Log:     logx.Discard(),
	})
	return api.Handler()
}

func get(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestHealthz(t *testing.T) {
	h := newTestServer(t)
	rec := get(t, h, "/healthz")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var body struct {
		Status  string `json:"status"`
		Version string `json:"version"`
		Uptime  int64  `json:"uptime_s"`
		Cache   struct {
			Entries int   `json:"entries"`
			Bytes   int64 `json:"bytes"`
		} `json:"cache"`
		Breaker struct {
			Open  bool `json:"open"`
			Until any  `json:"until"`
		} `json:"breaker"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, rec.Body.String())
	}
	if body.Status != "ok" || body.Version != "test" {
		t.Fatalf("body = %+v", body)
	}
	if body.Breaker.Open {
		t.Fatal("breaker must start closed")
	}
	if body.Breaker.Until != nil {
		t.Fatalf("until = %v, want null", body.Breaker.Until)
	}
}

func TestHealthzProbePopulatesSteps(t *testing.T) {
	h := newTestServer(t)
	rec := get(t, h, "/healthz?probe=1")
	var body struct {
		Probe []struct {
			Step       string `json:"step"`
			OK         bool   `json:"ok"`
			MS         int64  `json:"ms"`
			HTTPStatus int    `json:"http_status"`
		} `json:"probe"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(body.Probe) != 3 {
		t.Fatalf("probe steps = %d, want 3", len(body.Probe))
	}
	for _, st := range body.Probe {
		if !st.OK {
			t.Fatalf("step %s failed", st.Step)
		}
	}
}

func TestRSSUserRoute(t *testing.T) {
	h := newTestServer(t)

	for _, path := range []string{"/rss/user/2803301701", "/rss/user/2803301701.xml"} {
		rec := get(t, h, path)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d body=%s", path, rec.Code, rec.Body.String())
		}
		if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/rss+xml") {
			t.Fatalf("%s content-type = %q", path, ct)
		}
		body := rec.Body.String()
		if !strings.Contains(body, `<rss version="2.0"`) {
			t.Fatalf("%s is not RSS: %s", path, body[:120])
		}
		if !strings.Contains(body, "<ttl>15</ttl>") {
			t.Fatalf("%s missing ttl 15", path)
		}
		if !strings.Contains(body, "第二条 完整正文") {
			t.Fatalf("%s did not expand the long post", path)
		}
		if !strings.Contains(body, "https://weibo.com/2803301701/Abc1") {
			t.Fatalf("%s missing permalink", path)
		}
	}
}

func TestRSSDomainRoute(t *testing.T) {
	h := newTestServer(t)
	rec := get(t, h, "/rss/domain/rmrb")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "人民日报") {
		t.Fatalf("domain route did not resolve: %s", rec.Body.String()[:200])
	}
}

func TestAPIFeedShape(t *testing.T) {
	h := newTestServer(t)
	rec := get(t, h, "/api/v1/feed?uid=2803301701&limit=2")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		UID         string    `json:"uid"`
		ScreenName  string    `json:"screen_name"`
		Description string    `json:"description"`
		FetchedAt   time.Time `json:"fetched_at"`
		Cached      bool      `json:"cached"`
		Items       []struct {
			ID        string    `json:"id"`
			BID       string    `json:"bid"`
			CreatedAt time.Time `json:"created_at"`
			Text      string    `json:"text"`
			HTML      string    `json:"html"`
			URL       string    `json:"url"`
			Images    []string  `json:"images"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, rec.Body.String())
	}
	if body.UID != "2803301701" || body.ScreenName != "人民日报" {
		t.Fatalf("body = %+v", body)
	}
	if body.FetchedAt.IsZero() {
		t.Fatal("fetched_at must be set")
	}
	if len(body.Items) != 2 {
		t.Fatalf("items = %d, want 2 (limit honoured)", len(body.Items))
	}
	first := body.Items[0]
	if first.ID != "1001" || first.BID != "Abc1" || first.URL != "https://weibo.com/2803301701/Abc1" {
		t.Fatalf("first item = %+v", first)
	}
	if !strings.HasPrefix(first.CreatedAt.Format(time.RFC3339), "2026-10-23T06:30:04") {
		t.Fatalf("created_at = %v", first.CreatedAt)
	}
	if first.Images == nil {
		t.Fatal("images must be an empty array, not null")
	}

	// A second call must be served from cache.
	rec2 := get(t, h, "/api/v1/feed?uid=2803301701&limit=2")
	var body2 struct {
		Cached bool `json:"cached"`
	}
	if err := json.Unmarshal(rec2.Body.Bytes(), &body2); err != nil {
		t.Fatal(err)
	}
	if !body2.Cached {
		t.Fatal("second request should report cached=true")
	}
}

func TestErrorsCarryCodeAndStatus(t *testing.T) {
	h := newTestServer(t)

	cases := []struct {
		path   string
		status int
		code   string
	}{
		{"/rss/user/999", http.StatusNotFound, weibo.CodeUserNotFound},
		{"/rss/user/not-a-uid", http.StatusBadRequest, weibo.CodeBadRequest},
		{"/api/v1/feed", http.StatusBadRequest, weibo.CodeBadRequest},
		{"/api/v1/feed?uid=abc", http.StatusBadRequest, weibo.CodeBadRequest},
		{"/nope", http.StatusBadRequest, weibo.CodeBadRequest},
	}
	for _, tc := range cases {
		rec := get(t, h, tc.path)
		if rec.Code != tc.status {
			t.Errorf("%s status = %d, want %d", tc.path, rec.Code, tc.status)
		}
		if code := rec.Header().Get(httpapi.ErrorCodeHeader); code != tc.code {
			t.Errorf("%s header %s = %q, want %q", tc.path, httpapi.ErrorCodeHeader, code, tc.code)
		}
		var body struct {
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Errorf("%s body is not JSON: %v", tc.path, err)
			continue
		}
		if body.Error.Code != tc.code || body.Error.Message == "" {
			t.Errorf("%s body = %+v", tc.path, body.Error)
		}
	}
}

func TestWrongMethodRejected(t *testing.T) {
	h := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/healthz", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
}

func TestHotRoutes(t *testing.T) {
	h := newTestServer(t)

	rec := get(t, h, "/rss/hot?limit=2")
	if rec.Code != http.StatusOK {
		t.Fatalf("/rss/hot status = %d body=%s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/rss+xml") {
		t.Fatalf("/rss/hot content-type = %q", ct)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "<title>微博热搜</title>") {
		t.Fatalf("/rss/hot missing channel title:\n%s", body)
	}
	if !strings.Contains(body, "热度 148.9 万") {
		t.Fatalf("/rss/hot missing heat line:\n%s", body)
	}

	rec = get(t, h, "/api/v1/hot?limit=2")
	if rec.Code != http.StatusOK {
		t.Fatalf("/api/v1/hot status = %d body=%s", rec.Code, rec.Body.String())
	}
	var out struct {
		Title string `json:"title"`
		Items []struct {
			Rank  int    `json:"rank"`
			Title string `json:"title"`
			URL   string `json:"url"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode /api/v1/hot: %v", err)
	}
	if out.Title != "微博热搜" || len(out.Items) != 2 {
		t.Fatalf("/api/v1/hot = %+v", out)
	}
	if out.Items[0].Rank != 1 || out.Items[0].Title != "1. 小城良方 剧情硬伤" {
		t.Fatalf("/api/v1/hot first item = %+v", out.Items[0])
	}
}
