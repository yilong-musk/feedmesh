package httpapi_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/yilong-musk/feedmesh/internal/cache"
	"github.com/yilong-musk/feedmesh/internal/feed"
	"github.com/yilong-musk/feedmesh/internal/guard"
	"github.com/yilong-musk/feedmesh/internal/httpapi"
	"github.com/yilong-musk/feedmesh/internal/logx"
	"github.com/yilong-musk/feedmesh/internal/smzdm"
	"github.com/yilong-musk/feedmesh/internal/sspai"
	"github.com/yilong-musk/feedmesh/internal/v2ex"
	"github.com/yilong-musk/feedmesh/internal/weibo"
	"github.com/yilong-musk/feedmesh/internal/xnitter"
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

// xRSS is a two-item Nitter timeline: one plain post with an image, one
// retweet, both carrying instance-local links that must be rewritten.
const xRSS = `<?xml version="1.0" encoding="UTF-8"?>
<rss xmlns:atom="http://www.w3.org/2005/Atom" xmlns:dc="http://purl.org/dc/elements/1.1/" version="2.0">
<channel><title>Elon Musk / @elonmusk</title><link>https://nitter.test/elonmusk</link>
<description>feed</description><ttl>40</ttl>
<item><title>Hello from X</title><dc:creator>@elonmusk</dc:creator>
<description><![CDATA[<p>Hello <a href="https://nitter.test/bot">@Bot</a></p><img src="https://pbs.twimg.com/media/a.jpg"/>]]></description>
<pubDate>Fri, 23 Oct 2026 05:50:22 GMT</pubDate><guid>2113508310267674684</guid>
<link>https://nitter.test/elonmusk/status/2113508310267674684#m</link></item>
<item><title>RT by @elonmusk: breaking news</title><dc:creator>@cb_doge</dc:creator>
<description><![CDATA[<p>breaking news</p>]]></description>
<pubDate>Thu, 22 Oct 2026 17:15:55 GMT</pubDate><guid>2113318444934562215</guid>
<link>https://nitter.test/cb_doge/status/2113318444934562215#m</link></item>
</channel></rss>`

// smzdmRSS is a two-item SMZDM deal feed: one nickname-only entry and one with
// a blurb in content:encoded, out of order on purpose.
const smzdmRSS = `<?xml version="1.0" encoding="UTF-8"?>
<rss xmlns:content="http://purl.org/rss/1.0/modules/content/" version="2.0">
<channel><title>什么值得买</title><link>http://www.smzdm.com</link><description>deals</description>
<item><title><![CDATA[富光 保温杯 620ml 28.84元]]></title>
<link><![CDATA[https://www.smzdm.com/p/184734707/]]></link>
<pubDate><![CDATA[Sun, 25 Oct 2026 21:39:37]]></pubDate>
<description><![CDATA[chromes]]></description>
<focus_pic><![CDATA[https://y.zdmimg.com/a.png_a200.jpg]]></focus_pic>
<content:encoded><![CDATA[<p>本文来自什么值得买网站（www.smzdm.com）。</p>]]></content:encoded></item>
<item><title><![CDATA[保暖内衣套装 99 元包邮]]></title>
<link><![CDATA[https://www.smzdm.com/p/184730001/]]></link>
<pubDate><![CDATA[Sun, 25 Oct 2026 22:01:10]]></pubDate>
<description><![CDATA[小值机器人德绒自发热，券后 99 元。]]></description>
<focus_pic><![CDATA[https://y.zdmimg.com/b.png_a200.jpg]]></focus_pic>
<content:encoded><![CDATA[<p itemprop="description">德绒自发热，券后 99 元。</p><p>本文来自什么值得买网站（www.smzdm.com）。</p>]]></content:encoded></item>
</channel></rss>`

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
		case r.URL.Path == "/index.xml":
			entries := ""
			for i := 0; i < 12; i++ {
				title := fmt.Sprintf("V2EX 帖子%d", i+1)
				if i == 0 {
					title = "[分享] V2EX 测试"
				}
				entries += fmt.Sprintf(`<entry>
<title>%s</title><link rel="alternate" href="https://www.v2ex.com/t/%d" />
<published>2026-10-23T10:00:00Z</published><author><name>bob</name></author>
<content type="html"><![CDATA[<p>V2EX 正文</p>]]></content></entry>`, title, 9001+i)
			}
			fmt.Fprintf(w, `<?xml version="1.0" encoding="utf-8"?><feed xmlns="http://www.w3.org/2005/Atom">%s</feed>`, entries)
		case r.URL.Path == "/" && r.URL.Query().Get("tab") == "hot":
			replies := []int{1, 30, 25, 28, 2, 21, 8, 14, 11, 3, 17, 6}
			blocks := ""
			for i, rep := range replies {
				blocks += fmt.Sprintf(`<div class="cell item"><a href="/t/%d"></a><span class="item_title"><a href="/t/%d">热帖%d</a></span><a class="node" href="/go/qna">问与答</a><a class="count_livid" href="/t/%d">%d</a></div>`, 9100+i, 9100+i, i, 9100+i, rep)
			}
			fmt.Fprint(w, "<html><body>"+blocks+"</body></html>")
		case r.URL.Path == "/elonmusk/rss":
			w.Header().Set("Content-Type", "application/rss+xml")
			// Nitter links point at whichever mirror answered, so the fixture
			// must use this server's own origin for the rewrite to apply.
			fmt.Fprint(w, strings.ReplaceAll(xRSS, "https://nitter.test", "http://"+r.Host))
		case r.URL.Path == "/api/v1/article/index/page/get":
			all := make([]map[string]any, 0, 12)
			for i := 0; i < 12; i++ {
				all = append(all, map[string]any{
					"id": 3001 + i, "title": fmt.Sprintf("少数派文章%d", i+1), "summary": "摘要文本",
					"banner": "2026/10/23/a.png", "released_time": 1792750000 - i*1000,
					"author": map[string]any{"nickname": "作者甲"},
				})
			}
			all[0]["title"] = "少数派文章"
			offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
			limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
			if offset > len(all) {
				offset = len(all)
			}
			end := len(all)
			if limit > 0 && offset+limit < end {
				end = offset + limit
			}
			b, _ := json.Marshal(map[string]any{"error": 0, "data": all[offset:end]})
			w.Write(b)
		case r.URL.Path == "/api/v1/article/info/get":
			fmt.Fprint(w, `{"error":0,"data":{"body":"<p>少数派正文</p>","keywords":"效率,工具","is_pay":false,"is_free":true}}`)
		case r.URL.Path == "/" && r.URL.Query().Get("tab") == "":
			w.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")
			fmt.Fprint(w, smzdmRSS)
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
	sp := sspai.New(sspai.Config{BaseURL: upstream.URL, Timeout: 2 * time.Second})
	vx := v2ex.New(v2ex.Config{BaseURL: upstream.URL, Timeout: 2 * time.Second})
	xn := xnitter.New(xnitter.Config{
		Instances: []string{upstream.URL},
		// The status directory is optional; a 404 makes the client use the
		// static list above.
		StatusURL: upstream.URL + "/status",
		Timeout:   2 * time.Second,
	})
	svc := feed.NewService(feed.Clients{Weibo: client, Sspai: sp, V2ex: vx, X: xn, Smzdm: smzdm.New(smzdm.Config{BaseURL: upstream.URL, Timeout: 2 * time.Second})}, c, logx.Discard(), feed.DefaultTTLs())
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
	if !strings.Contains(body, "<title>Weibo Hot Search</title>") {
		t.Fatalf("/rss/hot missing channel title:\n%s", body)
	}
	if !strings.Contains(body, "Heat 1.5M") {
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
			Heat  int64  `json:"heat"`
			Title string `json:"title"`
			URL   string `json:"url"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode /api/v1/hot: %v", err)
	}
	if out.Title != "Weibo Hot Search" || len(out.Items) != 2 {
		t.Fatalf("/api/v1/hot = %+v", out)
	}
	if out.Items[0].Rank != 1 || out.Items[0].Heat != 1488601 || out.Items[0].Title != "1. 小城良方 剧情硬伤" {
		t.Fatalf("/api/v1/hot first item = %+v", out.Items[0])
	}

	rec = get(t, h, "/api/v1/hot?limit=2&refresh=1")
	var refreshed struct {
		Cached bool `json:"cached"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &refreshed); err != nil {
		t.Fatalf("decode refresh response: %v", err)
	}
	if refreshed.Cached {
		t.Fatal("/api/v1/hot?refresh=1 must bypass the cache")
	}
}

func TestSspaiRoutes(t *testing.T) {
	h := newTestServer(t)

	rec := get(t, h, "/rss/sspai?limit=1&full=1")
	if rec.Code != http.StatusOK {
		t.Fatalf("/rss/sspai status = %d body=%s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/rss+xml") {
		t.Fatalf("/rss/sspai content-type = %q", ct)
	}
	body := rec.Body.String()
	for _, want := range []string{"<title>sspai</title>", "少数派正文", "https://sspai.com/post/3001"} {
		if !strings.Contains(body, want) {
			t.Fatalf("/rss/sspai missing %q:\n%s", want, body)
		}
	}

	rec = get(t, h, "/api/v1/sspai?limit=1&full=1")
	if rec.Code != http.StatusOK {
		t.Fatalf("/api/v1/sspai status = %d body=%s", rec.Code, rec.Body.String())
	}
	var out struct {
		Title string `json:"title"`
		Items []struct {
			Title  string   `json:"title"`
			URL    string   `json:"url"`
			HTML   string   `json:"html"`
			Images []string `json:"images"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Title != "sspai" || len(out.Items) != 1 || out.Items[0].Title != "少数派文章" {
		t.Fatalf("api = %+v", out)
	}
	if len(out.Items[0].Images) != 1 {
		t.Fatalf("images = %v", out.Items[0].Images)
	}
}

func TestV2exRoutes(t *testing.T) {
	h := newTestServer(t)

	rec := get(t, h, "/rss/v2ex?limit=5")
	if rec.Code != http.StatusOK {
		t.Fatalf("/rss/v2ex status = %d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "<title>V2EX Latest</title>") || !strings.Contains(body, "V2EX 正文") {
		t.Fatalf("/rss/v2ex body unexpected:\n%s", body)
	}

	rec = get(t, h, "/rss/v2ex/hot?limit=5")
	if rec.Code != http.StatusOK {
		t.Fatalf("/rss/v2ex/hot status = %d body=%s", rec.Code, rec.Body.String())
	}
	body = rec.Body.String()
	if !strings.Contains(body, "<title>V2EX Hot</title>") || !strings.Contains(body, "热帖1") {
		t.Fatalf("/rss/v2ex/hot body unexpected:\n%s", body)
	}
	if strings.Index(body, "热帖1") > strings.Index(body, "热帖3") {
		t.Fatalf("hot feed is not sorted by replies")
	}

	rec = get(t, h, "/api/v1/v2ex?section=hot&limit=2")
	if rec.Code != http.StatusOK {
		t.Fatalf("/api/v1/v2ex status = %d body=%s", rec.Code, rec.Body.String())
	}
	var out struct {
		Title string `json:"title"`
		Items []struct {
			Title string `json:"title"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Title != "V2EX Hot" || len(out.Items) != 2 {
		t.Fatalf("api = %+v", out)
	}
}

// Regression: the feeds used to cache the caller's already-truncated slice, so
// one small ?limit request capped every later one for the whole TTL.
func TestSmallLimitDoesNotTruncateCachedFeeds(t *testing.T) {
	h := newTestServer(t)

	cases := []struct {
		name string
		warm string
		path string
	}{
		{"sspai", "/rss/sspai?limit=3", "/rss/sspai?limit=10"},
		{"v2ex-latest", "/rss/v2ex?limit=3", "/rss/v2ex?limit=10"},
		{"v2ex-hot", "/rss/v2ex/hot?limit=3", "/rss/v2ex/hot?limit=10"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if rec := get(t, h, tc.warm); rec.Code != http.StatusOK {
				t.Fatalf("warm %s status = %d body=%s", tc.warm, rec.Code, rec.Body.String())
			}
			rec := get(t, h, tc.path)
			if rec.Code != http.StatusOK {
				t.Fatalf("%s status = %d body=%s", tc.path, rec.Code, rec.Body.String())
			}
			if n := strings.Count(rec.Body.String(), "<item>"); n != 10 {
				t.Fatalf("%s after warm %s returned %d items, want 10", tc.path, tc.warm, n)
			}
		})
	}

	// The JSON view shares the list a wide RSS fetch cached (different limit).
	if rec := get(t, h, "/rss/sspai?limit=30"); rec.Code != http.StatusOK {
		t.Fatalf("/rss/sspai?limit=30 status = %d body=%s", rec.Code, rec.Body.String())
	}
	rec := get(t, h, "/api/v1/sspai?limit=10")
	if rec.Code != http.StatusOK {
		t.Fatalf("/api/v1/sspai status = %d body=%s", rec.Code, rec.Body.String())
	}
	var out struct {
		Cached bool `json:"cached"`
		Items  []struct {
			Title string `json:"title"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.Items) != 10 || !out.Cached {
		t.Fatalf("/api/v1/sspai after limit=30 = %d items, cached=%v; want 10 from cache", len(out.Items), out.Cached)
	}
}
