package tiktok

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// challengePage builds a WAF page whose puzzle answer is known.
func challengePage(answer int) (page string, cookieName string) {
	seed := []byte("feedmesh-seed")
	digest := sha256.Sum256(append(seed, []byte(fmt.Sprint(answer))...))
	// Build the document by hand: TikTok sends "v" first and the solver must
	// preserve that order when it splices in the answer.
	blob := fmt.Sprintf(`{"v":{"a":%q,"c":%q},"t":"test"}`,
		base64.StdEncoding.EncodeToString(seed),
		base64.StdEncoding.EncodeToString(digest[:]))
	enc := base64.StdEncoding.EncodeToString([]byte(blob))
	return `<html><body>Please wait...<p id="cs" class="` + enc + `"></p><p id="wci" class="_wafchallengeid"></p></body></html>`, "_wafchallengeid"
}

func TestSolveChallengeFindsAnswer(t *testing.T) {
	page, name := challengePage(42)
	gotName, gotVal, err := solveChallenge(page)
	if err != nil {
		t.Fatalf("solve: %v", err)
	}
	if gotName != name {
		t.Fatalf("cookie name = %q, want %q", gotName, name)
	}
	raw, err := base64.StdEncoding.DecodeString(gotVal)
	if err != nil {
		t.Fatalf("cookie is not base64: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("cookie is not json: %v", err)
	}
	d, _ := decoded["d"].(string)
	plain, _ := base64.StdEncoding.DecodeString(d)
	if string(plain) != "42" {
		t.Fatalf("d = %q, want the answer 42", plain)
	}
	// Field order must survive: the v block still leads the document.
	if !strings.HasPrefix(string(raw), `{"v":`) {
		t.Fatalf("field order changed: %s", raw)
	}
}

func TestSolveChallengeRejectsGarbage(t *testing.T) {
	if _, _, err := solveChallenge("<html>no challenge here</html>"); err == nil {
		t.Fatal("expected an error for a page without a cs element")
	}
}

const testSecUID = "MS4wLjABAAAAtestsecuid0000000000"

func profilePage(handle, nickname string, status int) string {
	detail := map[string]any{
		"userInfo": map[string]any{
			"user": map[string]any{
				"id": "7664638705177150477", "uniqueId": handle, "nickname": nickname,
				"secUid": testSecUID, "signature": "Making the impossible possible",
				"avatarLarger": "https://p16-common-sign.tiktokcdn.com/avatar.jpeg",
			},
		},
		"statusCode": status,
	}
	blob, _ := json.Marshal(map[string]any{
		"__DEFAULT_SCOPE__": map[string]any{"webapp.user-detail": detail},
	})
	return `<html><script id="__UNIVERSAL_DATA_FOR_REHYDRATION__" type="application/json">` + string(blob) + `</script></html>`
}

func itemListPage() string {
	return `{"statusCode":0,"hasMorePrevious":false,"itemList":[
	  {"id":"7708408003065580831","desc":"first <b>caption</b>","createTime":1794753620,
	   "stats":{"playCount":275700,"diggCount":2773,"commentCount":950,"shareCount":292},
	   "video":{"duration":79,"width":720,"height":1280,
	     "playAddr":"https://v16-webapp-prime.tiktok.com/default.mp4",
	     "cover":"https://p16-common-sign.tiktokcdn.com/cover.jpeg",
	     "originCover":"https://p16-common-sign.tiktokcdn.com/origin.jpeg",
	     "bitrateInfo":[
	       {"Bitrate":344361,"PlayAddr":{"UrlList":["https://cdn/540p.mp4"]}},
	       {"Bitrate":1037801,"PlayAddr":{"UrlList":["https://cdn/1080p.mp4"]}}]},
	   "author":{"id":"1","uniqueId":"nasa","nickname":"NASA","secUid":"` + testSecUID + `"},
	   "music":{"title":"original sound","authorName":"NASA"}},
	  {"id":"7704774145829063967","desc":"photo post","createTime":1794600000,
	   "imagePost":{"images":[{"urlList":["https://p16-common-sign.tiktokcdn.com/photo1.jpeg"]}]},
	   "author":{"id":"1","uniqueId":"nasa","nickname":"NASA","secUid":"` + testSecUID + `"}}
	]}`
}

func TestTimelineThroughChallenge(t *testing.T) {
	var challengeServed bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/@"):
			if _, err := r.Cookie("_wafchallengeid"); err == nil {
				_, _ = w.Write([]byte(profilePage("nasa", "NASA", 0)))
				return
			}
			page, _ := challengePage(7)
			challengeServed = true
			_, _ = w.Write([]byte(page))
		case r.URL.Path == "/api/creator/item_list/":
			if r.URL.Query().Get("count") != "15" {
				t.Errorf("count = %q, want the fixed page size 15", r.URL.Query().Get("count"))
			}
			if r.URL.Query().Get("secUid") != testSecUID {
				t.Errorf("secUid = %q", r.URL.Query().Get("secUid"))
			}
			_, _ = w.Write([]byte(itemListPage()))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	c := New(Config{BaseURL: upstream.URL, Timeout: 5 * time.Second})
	tl, err := c.Timeline(context.Background(), "@nasa", 10)
	if err != nil {
		t.Fatalf("timeline: %v", err)
	}
	if !challengeServed {
		t.Fatal("the client never solved the challenge")
	}
	if tl.SecUID != testSecUID || tl.Author != "nasa" || tl.Nickname != "NASA" {
		t.Fatalf("timeline identity = %+v", tl)
	}
	if len(tl.Items) != 2 {
		t.Fatalf("items = %d", len(tl.Items))
	}
	first := tl.Items[0]
	if first.ID != "7708408003065580831" || first.Desc != "first <b>caption</b>" {
		t.Fatalf("item = %+v", first)
	}
	if first.PlayURL != "https://cdn/1080p.mp4" {
		t.Fatalf("play url = %q, want the highest-bitrate rendition", first.PlayURL)
	}
	if first.CoverURL != "https://p16-common-sign.tiktokcdn.com/origin.jpeg" {
		t.Fatalf("cover = %q, want originCover", first.CoverURL)
	}
	if first.DurationMS != 79000 || first.Width != 720 || first.Height != 1280 {
		t.Fatalf("media fields = %+v", first)
	}
	if !first.CreatedAt.Equal(time.Unix(1794753620, 0).UTC()) {
		t.Fatalf("created at = %v", first.CreatedAt)
	}
	if first.PlayCount != 275700 || first.DiggCount != 2773 {
		t.Fatalf("stats = %+v", first)
	}
	second := tl.Items[1]
	if !second.IsImage || second.CoverURL != "https://p16-common-sign.tiktokcdn.com/photo1.jpeg" {
		t.Fatalf("photo post = %+v", second)
	}
}

func TestTimelineNotFoundAndPrivate(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		want   error
	}{
		{"missing", 10221, ErrNotFound},
		{"private", 10222, ErrPrivate},
	} {
		t.Run(tc.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(profilePage("nobody", "", tc.status)))
			}))
			defer upstream.Close()
			c := New(Config{BaseURL: upstream.URL, Timeout: 5 * time.Second})
			if _, err := c.Timeline(context.Background(), "nobody", 5); err != tc.want {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestTimelineRejectsBadUser(t *testing.T) {
	c := New(Config{BaseURL: "http://127.0.0.1:1", Timeout: time.Second})
	for _, bad := range []string{"", "bad user", "a/b", strings.Repeat("x", 65)} {
		if _, err := c.Timeline(context.Background(), bad, 5); err != ErrInvalidUser {
			t.Fatalf("%q err = %v, want ErrInvalidUser", bad, err)
		}
	}
}

func TestTimelineBySecUID(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/creator/item_list/" {
			t.Errorf("secUid requests must skip the profile page, got %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(itemListPage()))
	}))
	defer upstream.Close()

	c := New(Config{BaseURL: upstream.URL, Timeout: 5 * time.Second})
	tl, err := c.Timeline(context.Background(), testSecUID, 5)
	if err != nil {
		t.Fatal(err)
	}
	if tl.Author != "nasa" {
		t.Fatalf("author = %q, want it recovered from the first item", tl.Author)
	}
}
