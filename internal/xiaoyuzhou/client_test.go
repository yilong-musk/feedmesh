package xiaoyuzhou

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func page(episodes string) string {
	return `<!DOCTYPE html><html><head></head><body><script id="__NEXT_DATA__" type="application/json">{"props":{"pageProps":{"podcast":{"title":"Sample Show","author":"Host","description":"About the show","image":{"picUrl":"https://image.example/show.jpg","largePicUrl":"https://image.example/show-large.jpg"},"episodes":[` + episodes + `]}}}}</script></body></html>`
}

func TestEpisodesParsesShowPage(t *testing.T) {
	const eps = `{"eid":"aaaaaaaaaaaaaaaaaaaaaaaa","title":"Older","description":"Old body","pubDate":"2026-11-01T00:00:00.000Z","duration":60,"playCount":10,"enclosure":{"url":"https://media.example/old.mp3"}},{"eid":"bbbbbbbbbbbbbbbbbbbbbbbb","title":"Newer","description":"New body","pubDate":"2026-11-20T00:00:00.000Z","duration":125,"playCount":40,"enclosure":{"url":"https://media.example/new.m4a"},"image":{"largePicUrl":"https://image.example/ep.jpg"}}`
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if r.URL.Path != "/podcast/6021f949a789fca4eff4492c" {
			t.Errorf("path = %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(page(eps)))
	}))
	defer srv.Close()
	c := New(Config{BaseURL: srv.URL, Timeout: 5 * time.Second})
	show, err := c.Episodes(context.Background(), "https://www.xiaoyuzhoufm.com/podcast/6021f949a789fca4eff4492c", 10)
	if err != nil {
		t.Fatal(err)
	}
	if hits != 1 || show.Title != "Sample Show" || show.Author != "Host" || show.Image != "https://image.example/show-large.jpg" {
		t.Fatalf("show = %+v hits=%d", show, hits)
	}
	if len(show.Episodes) != 2 || show.Episodes[0].Title != "Newer" {
		t.Fatalf("episodes = %+v", show.Episodes)
	}
	ep := show.Episodes[0]
	if ep.AudioURL != "https://media.example/new.m4a" || ep.Duration != 125*time.Second || ep.Plays != 40 {
		t.Fatalf("episode = %+v", ep)
	}
	if ep.ShowTitle != "Sample Show" || !strings.HasSuffix(ep.URL, "/episode/bbbbbbbbbbbbbbbbbbbbbbbb") {
		t.Fatalf("header = %+v", ep)
	}
}

func TestParseID(t *testing.T) {
	id, err := ParseID("6021F949A789FCA4EFF4492C")
	if err != nil || id != "6021f949a789fca4eff4492c" {
		t.Fatalf("ParseID = %s %v", id, err)
	}
	if _, err := ParseID("nope"); err != ErrInvalidID {
		t.Fatalf("err = %v", err)
	}
	if _, err := ParseID("https://www.xiaoyuzhoufm.com/episode/aaaaaaaaaaaaaaaaaaaaaaaa"); err != ErrInvalidID {
		t.Fatalf("episode url err = %v", err)
	}
}

func TestMissingShow(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("missing"))
	}))
	defer srv.Close()
	c := New(Config{BaseURL: srv.URL, Timeout: 5 * time.Second})
	_, err := c.Episodes(context.Background(), "6021f949a789fca4eff4492c", 5)
	if err != ErrNotFound {
		t.Fatalf("err = %v", err)
	}
}

func TestUnexpectedPage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<html>no data</html>"))
	}))
	defer srv.Close()
	c := New(Config{BaseURL: srv.URL, Timeout: 5 * time.Second})
	_, err := c.Episodes(context.Background(), "6021f949a789fca4eff4492c", 5)
	if err != ErrUnexpected {
		t.Fatalf("err = %v", err)
	}
}
