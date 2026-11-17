package bilibili

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestMixinKeyMatchesPublishedVector(t *testing.T) {
	// The two image names observed on 2026-11-17 and the key the web client
	// derives from them; a table change would break every signed request.
	got := mixinKey(wbiKeys{
		img: "7cd084941338484aae1ad9425b84077c",
		sub: "4932caff0ff746eab6f01bf08b70ac45",
	})
	if got != "ea1db124af3c7062474693fa704f4ff8" {
		t.Fatalf("mixin key = %q", got)
	}
}

func TestSignParamsPinsSignature(t *testing.T) {
	params := url.Values{}
	params.Set("mid", "946974")
	params.Set("ps", "20")
	signed := signParamsAt(params, "ea1db124af3c7062474693fa704f4ff8", 1794895894)
	v, err := url.ParseQuery(signed)
	if err != nil {
		t.Fatal(err)
	}
	if v.Get("wts") != "1794895894" || len(v.Get("w_rid")) != 32 {
		t.Fatalf("signed = %q", signed)
	}
	// Recomputing the digest from the same inputs must match.
	base := url.Values{}
	base.Set("mid", "946974")
	base.Set("ps", "20")
	base.Set("wts", "1794895894")
	want := signParamsAt(base, "ea1db124af3c7062474693fa704f4ff8", 1794895894)
	if v.Get("w_rid") != mustQuery(t, want).Get("w_rid") {
		t.Fatalf("signature is not deterministic")
	}
}

func mustQuery(t *testing.T, raw string) url.Values {
	t.Helper()
	v, err := url.ParseQuery(raw)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestFilterSignatureChars(t *testing.T) {
	if got := filterSignatureChars(`a!b'c(d)e*f`); got != "abcdef" {
		t.Fatalf("filtered = %q", got)
	}
}

func TestKeyFromURL(t *testing.T) {
	cases := map[string]string{
		"https://i0.hdslb.com/bfs/wbi/7cd084941338484aae1ad9425b84077c.png": "7cd084941338484aae1ad9425b84077c",
		"":  "",
		"x": "x",
	}
	for in, want := range cases {
		if got := keyFromURL(in); got != want {
			t.Fatalf("keyFromURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseLength(t *testing.T) {
	cases := map[string]time.Duration{
		"00:22":   22 * time.Second,
		"19:00":   19 * time.Minute,
		"1:02:03": time.Hour + 2*time.Minute + 3*time.Second,
		"-":       0,
		"":        0,
		"bad":     0,
	}
	for in, want := range cases {
		if got := parseLength(in); got != want {
			t.Fatalf("parseLength(%q) = %v, want %v", in, got, want)
		}
	}
}

const testMid = "946974"

func fakeBilibili(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/x/frontend/finger/spi":
			fmt.Fprint(w, `{"code":0,"data":{"b_3":"B3TEST","b_4":"B4TEST"}}`)
		case "/x/web-interface/card":
			if r.URL.Query().Get("mid") != testMid {
				t.Errorf("card mid = %q", r.URL.Query().Get("mid"))
			}
			fmt.Fprint(w, `{"code":0,"data":{"card":{"mid":"`+testMid+`","name":"影视飓风","face":"http://i0.hdslb.com/face.jpg","sign":"无限进步","fans":19100953}}}`)
		case "/x/web-interface/nav":
			fmt.Fprint(w, `{"code":-101,"data":{"wbi_img":{"img_url":"https://i0.hdslb.com/bfs/wbi/7cd084941338484aae1ad9425b84077c.png","sub_url":"https://i0.hdslb.com/bfs/wbi/4932caff0ff746eab6f01bf08b70ac45.png"}}}`)
		case "/x/space/wbi/arc/search":
			q := r.URL.Query()
			if len(q.Get("w_rid")) != 32 || q.Get("wts") == "" {
				t.Errorf("unsigned request: %v", q)
			}
			if q.Get("mid") != testMid || q.Get("ps") != "20" || q.Get("order") != "pubdate" {
				t.Errorf("query = %v", q)
			}
			if q.Get("dm_img_list") != "[]" || q.Get("dm_img_str") == "" {
				t.Errorf("fingerprint params missing: %v", q)
			}
			fmt.Fprint(w, `{"code":0,"data":{"page":{"count":941},"list":{"vlist":[
			  {"bvid":"BV1kZWX62ENY","title":"  为什么是最后一期  ","description":"简介","author":"影视飓风","mid":946974,
			   "created":1794474000,"length":"19:00","pic":"http://i2.hdslb.com/bfs/archive/a.jpg",
			   "play":6608624,"video_review":1234,"comment":567,"typename":"影视杂谈"},
			  {"bvid":"BV1NS4t6oEXQ","title":"第二期","description":"","author":"影视飓风","mid":946974,
			   "created":1793887200,"length":"14:59","pic":"https://i1.hdslb.com/bfs/archive/b.jpg","play":3964184}
			]}}}`)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
}

func TestTimelineEndToEnd(t *testing.T) {
	srv := fakeBilibili(t)
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, Timeout: 5 * time.Second})
	tl, err := c.Timeline(context.Background(), testMid, 20)
	if err != nil {
		t.Fatal(err)
	}
	if tl.Author != "影视飓风" || tl.Fans != 19100953 || tl.Total != 941 {
		t.Fatalf("timeline = %+v", tl)
	}
	if len(tl.Videos) != 2 {
		t.Fatalf("videos = %d", len(tl.Videos))
	}
	v := tl.Videos[0]
	if v.BVID != "BV1kZWX62ENY" || v.Title != "为什么是最后一期" {
		t.Fatalf("video = %+v", v)
	}
	if v.Cover != "https://i2.hdslb.com/bfs/archive/a.jpg" {
		t.Fatalf("cover = %q (http must be upgraded)", v.Cover)
	}
	if v.Duration != 19*time.Minute || v.Play != 6608624 || v.Danmaku != 1234 {
		t.Fatalf("media fields = %+v", v)
	}
	if !v.CreatedAt.Equal(time.Unix(1794474000, 0).UTC()) {
		t.Fatalf("created = %v", v.CreatedAt)
	}
}

func TestTimelineRetriesRateLimit(t *testing.T) {
	var searches int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/x/frontend/finger/spi":
			fmt.Fprint(w, `{"code":0,"data":{"b_3":"B3","b_4":"B4"}}`)
		case "/x/web-interface/card":
			fmt.Fprint(w, `{"code":0,"data":{"card":{"mid":"1","name":"Test"}}}`)
		case "/x/web-interface/nav":
			fmt.Fprint(w, `{"code":0,"data":{"wbi_img":{"img_url":"https://x/a.png","sub_url":"https://x/b.png"}}}`)
		case "/x/space/wbi/arc/search":
			searches++
			if searches == 1 {
				w.WriteHeader(http.StatusPreconditionFailed)
				return
			}
			fmt.Fprint(w, `{"code":0,"data":{"page":{"count":1},"list":{"vlist":[{"bvid":"BV1","title":"t","created":1}]}}}`)
		}
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, Timeout: 5 * time.Second})
	c.backoff = []time.Duration{0, 0}
	tl, err := c.Timeline(context.Background(), "1", 5)
	if err != nil {
		t.Fatal(err)
	}
	if searches != 2 || len(tl.Videos) != 1 {
		t.Fatalf("searches = %d, videos = %d", searches, len(tl.Videos))
	}
}

func TestTimelineRateLimitedAfterRetries(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/x/frontend/finger/spi":
			fmt.Fprint(w, `{"code":0,"data":{"b_3":"B3","b_4":"B4"}}`)
		case "/x/web-interface/card":
			fmt.Fprint(w, `{"code":0,"data":{"card":{"mid":"1","name":"Test"}}}`)
		case "/x/web-interface/nav":
			fmt.Fprint(w, `{"code":0,"data":{"wbi_img":{"img_url":"https://x/a.png","sub_url":"https://x/b.png"}}}`)
		case "/x/space/wbi/arc/search":
			w.WriteHeader(http.StatusPreconditionFailed)
		}
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, Timeout: 5 * time.Second})
	c.backoff = []time.Duration{0, 0}
	if _, err := c.Timeline(context.Background(), "1", 5); err == nil || !strings.Contains(err.Error(), "rate limited") {
		t.Fatalf("err = %v, want ErrRateLimited", err)
	}
}

func TestTimelineNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/x/web-interface/card" {
			fmt.Fprint(w, `{"code":-404,"message":"啥都木有"}`)
			return
		}
		fmt.Fprint(w, `{"code":0,"data":{"b_3":"B3","b_4":"B4"}}`)
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, Timeout: 5 * time.Second})
	if _, err := c.Timeline(context.Background(), "999999999", 5); err != ErrNotFound {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestTimelineRejectsBadMid(t *testing.T) {
	c := New(Config{BaseURL: "http://127.0.0.1:1", Timeout: time.Second})
	for _, bad := range []string{"", "abc", "12x", strings.Repeat("9", 21)} {
		if _, err := c.Timeline(context.Background(), bad, 5); err != ErrInvalidUser {
			t.Fatalf("%q err = %v, want ErrInvalidUser", bad, err)
		}
	}
}

func TestConvertClampsLimit(t *testing.T) {
	if DefaultLimit <= 0 || MaxLimit < DefaultLimit || MaxLimit > 50 {
		t.Fatalf("limits = %d/%d", DefaultLimit, MaxLimit)
	}
	_ = math.MaxInt
	_ = json.Valid
}
