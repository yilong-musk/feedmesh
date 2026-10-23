package weibo_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yilong-musk/weibo-rss-pro/internal/guard"
	"github.com/yilong-musk/weibo-rss-pro/internal/weibo"
)

func TestValidateUID(t *testing.T) {
	valid := []string{"1", "2803301701", "3937348351"}
	for _, uid := range valid {
		if err := weibo.ValidateUID(uid); err != nil {
			t.Errorf("ValidateUID(%q) = %v, want nil", uid, err)
		}
	}
	invalid := []string{"", "  ", "abc", "-1", "12.5", "2803301701x", "../1", "1 2", strings.Repeat("9", 21)}
	for _, uid := range invalid {
		err := weibo.ValidateUID(uid)
		if err == nil {
			t.Errorf("ValidateUID(%q) = nil, want an error", uid)
			continue
		}
		if code := weibo.CodeOf(err); code != weibo.CodeBadRequest {
			t.Errorf("ValidateUID(%q) code = %q, want BAD_REQUEST", uid, code)
		}
	}
}

func TestValidateDomain(t *testing.T) {
	for _, d := range []string{"rmrb", "people", "a-b_c", "abc123"} {
		if err := weibo.ValidateDomain(d); err != nil {
			t.Errorf("ValidateDomain(%q) = %v, want nil", d, err)
		}
	}
	for _, d := range []string{"", "a/b", "../etc", "a b", "a?b", strings.Repeat("a", 65)} {
		if err := weibo.ValidateDomain(d); err == nil {
			t.Errorf("ValidateDomain(%q) = nil, want an error", d)
		}
	}
}

// fakeUpstream builds a minimal weibo stand-in.
func fakeUpstream(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv
}

func newClient(t *testing.T, base string) *weibo.Client {
	t.Helper()
	return weibo.New(weibo.Config{
		BaseURL:    base,
		VisitorURL: base + "/visitor",
		Timeout:    2 * time.Second,
		Backoff:    5 * time.Millisecond,
	})
}

func TestProfileSendsVisitorCookie(t *testing.T) {
	var sawCookie atomic.Value
	srv := fakeUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/visitor":
			http.SetCookie(w, &http.Cookie{Name: "SUB", Value: "_2Aktestcookie", Path: "/"})
			fmt.Fprint(w, `window.visitor_gray_callback && visitor_gray_callback({"retcode":20000000})`)
		case "/api/container/getIndex":
			sawCookie.Store(r.Header.Get("Cookie"))
			if r.Header.Get("Mweibo-Pwa") != "1" || r.Header.Get("X-Requested-With") != "XMLHttpRequest" {
				t.Errorf("missing weibo ajax headers: %v", r.Header)
			}
			fmt.Fprint(w, `{"ok":1,"data":{"userInfo":{"id":1,"screen_name":"人民日报","description":"法人微博"},
				"tabsInfo":{"tabs":[{"tab_type":"profile","containerid":"2302831"},
				                    {"tab_type":"weibo","containerid":"1076031"}]}}}`)
		default:
			http.NotFound(w, r)
		}
	})

	c := newClient(t, srv.URL)
	p, err := c.Profile(context.Background(), "1")
	if err != nil {
		t.Fatalf("Profile: %v", err)
	}
	if p.ScreenName != "人民日报" || p.ContainerID != "1076031" {
		t.Fatalf("profile = %+v", p)
	}
	cookie, _ := sawCookie.Load().(string)
	if !strings.Contains(cookie, "SUB=_2Aktestcookie") {
		t.Fatalf("visitor cookie not forwarded, got %q", cookie)
	}
}

func TestProfileNotFound(t *testing.T) {
	srv := fakeUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/visitor" {
			http.SetCookie(w, &http.Cookie{Name: "SUB", Value: "x", Path: "/"})
			fmt.Fprint(w, `{"retcode":20000000}`)
			return
		}
		fmt.Fprint(w, `{"ok":0,"msg":"这里还没有内容","data":{"cards":[]}}`)
	})

	c := newClient(t, srv.URL)
	_, err := c.Profile(context.Background(), "9999999999999999")
	if err == nil {
		t.Fatal("expected an error")
	}
	if code := weibo.CodeOf(err); code != weibo.CodeUserNotFound {
		t.Fatalf("code = %q, want USER_NOT_FOUND", code)
	}
}

func TestBlockTripsBreakerAndShortCircuits(t *testing.T) {
	var hits atomic.Int32
	srv := fakeUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/visitor" {
			http.SetCookie(w, &http.Cookie{Name: "SUB", Value: "x", Path: "/"})
			fmt.Fprint(w, `{"retcode":20000000}`)
			return
		}
		hits.Add(1)
		w.WriteHeader(http.StatusTeapot)
		fmt.Fprint(w, "blocked")
	})

	breaker := guard.NewBreaker(10 * time.Minute)
	c := weibo.New(weibo.Config{
		BaseURL:    srv.URL,
		VisitorURL: srv.URL + "/visitor",
		Timeout:    2 * time.Second,
		Backoff:    5 * time.Millisecond,
		Breaker:    breaker,
	})

	_, err := c.Profile(context.Background(), "1")
	if err == nil {
		t.Fatal("expected a block error")
	}
	if code := weibo.CodeOf(err); code != weibo.CodeUpstreamBlocked {
		t.Fatalf("code = %q, want UPSTREAM_BLOCKED", code)
	}
	if !breaker.State().Open {
		t.Fatal("breaker must be open after a 418")
	}
	firstHits := hits.Load()
	// One attempt plus one cookie-refresh retry.
	if firstHits != 2 {
		t.Fatalf("upstream hits = %d, want 2 (initial + cookie refresh)", firstHits)
	}

	_, err = c.Profile(context.Background(), "1")
	if code := weibo.CodeOf(err); code != weibo.CodeBreakerOpen {
		t.Fatalf("second call code = %q, want BREAKER_OPEN", code)
	}
	if hits.Load() != firstHits {
		t.Fatalf("breaker did not short-circuit: hits went %d -> %d", firstHits, hits.Load())
	}
}

func TestLongTextAndFallback(t *testing.T) {
	srv := fakeUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/visitor":
			http.SetCookie(w, &http.Cookie{Name: "SUB", Value: "x", Path: "/"})
			fmt.Fprint(w, `{"retcode":20000000}`)
		case "/statuses/extend":
			fmt.Fprint(w, `{"ok":1,"data":{"longTextContent":"<p>完整长文</p>"}}`)
		case "/statuses/show":
			fmt.Fprint(w, `{"ok":1,"data":{"id":1,"text":"<p>回退正文</p>"}}`)
		default:
			http.NotFound(w, r)
		}
	})

	c := newClient(t, srv.URL)
	text, err := c.LongText(context.Background(), "1")
	if err != nil || text != "<p>完整长文</p>" {
		t.Fatalf("LongText = %q, %v", text, err)
	}
	text, err = c.StatusText(context.Background(), "1")
	if err != nil || text != "<p>回退正文</p>" {
		t.Fatalf("StatusText = %q, %v", text, err)
	}
}

func TestTimeoutIsClassified(t *testing.T) {
	srv := fakeUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/visitor" {
			http.SetCookie(w, &http.Cookie{Name: "SUB", Value: "x", Path: "/"})
			fmt.Fprint(w, `{"retcode":20000000}`)
			return
		}
		time.Sleep(300 * time.Millisecond)
		fmt.Fprint(w, `{"ok":1,"data":{}}`)
	})

	c := weibo.New(weibo.Config{
		BaseURL:    srv.URL,
		VisitorURL: srv.URL + "/visitor",
		Timeout:    30 * time.Millisecond,
		Backoff:    time.Millisecond,
	})

	_, err := c.Profile(context.Background(), "1")
	if err == nil {
		t.Fatal("expected a timeout")
	}
	if code := weibo.CodeOf(err); code != weibo.CodeTimeout {
		t.Fatalf("code = %q, want TIMEOUT", code)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		// Client.Timeout surfaces as a deadline, which is what we classify on.
		t.Logf("wrapped cause = %v", err)
	}
}

func TestResolveDomain(t *testing.T) {
	srv := fakeUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/visitor":
			http.SetCookie(w, &http.Cookie{Name: "SUB", Value: "x", Path: "/"})
			fmt.Fprint(w, `{"retcode":20000000}`)
		case "/rmrb":
			http.Redirect(w, r, "/u/2803301701", http.StatusFound)
		case "/u/2803301701":
			fmt.Fprint(w, "<html>profile</html>")
		default:
			http.NotFound(w, r)
		}
	})

	c := newClient(t, srv.URL)
	uid, err := c.ResolveDomain(context.Background(), "rmrb")
	if err != nil {
		t.Fatalf("ResolveDomain: %v", err)
	}
	if uid != "2803301701" {
		t.Fatalf("uid = %q, want 2803301701", uid)
	}

	if _, err := c.ResolveDomain(context.Background(), "no-such-domain"); err == nil {
		t.Fatal("expected an error for an unknown domain")
	}
}
