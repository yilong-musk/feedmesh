package httpapi_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestSmzdmRSSRoutes(t *testing.T) {
	h := newTestServer(t)

	for _, path := range []string{"/rss/smzdm", "/rss/smzdm.xml"} {
		rec := get(t, h, path)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d body=%s", path, rec.Code, rec.Body.String())
		}
		if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/rss+xml") {
			t.Fatalf("%s content-type = %q", path, ct)
		}
		body := rec.Body.String()
		for _, want := range []string{
			"<title>SMZDM Deals</title>",
			"保暖内衣套装 99 元包邮",
			"https://www.smzdm.com/p/184730001/",
			"<ttl>5</ttl>",
		} {
			if !strings.Contains(body, want) {
				t.Fatalf("%s missing %q:\n%s", path, want, body)
			}
		}
	}
}

func TestSmzdmAPIRoute(t *testing.T) {
	h := newTestServer(t)

	rec := get(t, h, "/api/v1/smzdm?limit=1")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var out struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		Cached      bool   `json:"cached"`
		Items       []struct {
			ID        string    `json:"id"`
			Title     string    `json:"title"`
			Author    string    `json:"author"`
			Text      string    `json:"text"`
			URL       string    `json:"url"`
			HTML      string    `json:"html"`
			Images    []string  `json:"images"`
			CreatedAt time.Time `json:"created_at"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v\n%s", err, rec.Body.String())
	}
	if out.Title != "SMZDM Deals" || out.Description == "" {
		t.Fatalf("channel = %+v", out)
	}
	if len(out.Items) != 1 {
		t.Fatalf("limit=1 returned %d items", len(out.Items))
	}
	it := out.Items[0]
	if it.ID != "184730001" {
		t.Fatalf("newest item first, got id %q", it.ID)
	}
	if it.Text != "德绒自发热，券后 99 元。" {
		t.Fatalf("text = %q", it.Text)
	}
	if it.Author != "小值机器人" {
		t.Fatalf("author = %q, want the poster nickname split out of description", it.Author)
	}
	if len(it.Images) != 1 || !strings.Contains(it.Images[0], "b.png") {
		t.Fatalf("images = %v", it.Images)
	}
	if !strings.Contains(it.HTML, "Posted by 小值机器人") {
		t.Fatalf("html did not carry the nickname:\n%s", it.HTML)
	}
	if got := it.CreatedAt.Format(time.RFC3339); got != "2026-10-25T14:01:10Z" {
		t.Fatalf("created_at = %s, want the UTC instant", got)
	}
	if out.Cached {
		t.Fatal("first request must not be a cache hit")
	}

	// A second identical call is served from the cache.
	rec = get(t, h, "/api/v1/smzdm?limit=1")
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode second call: %v", err)
	}
	if !out.Cached {
		t.Fatal("second call must be a cache hit")
	}

	// An oversized limit is clamped to what upstream returned.
	rec = get(t, h, "/api/v1/smzdm?limit=500")
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode clamped call: %v", err)
	}
	if len(out.Items) != 2 {
		t.Fatalf("clamped limit returned %d items, want 2", len(out.Items))
	}
}

func TestSmzdmPostRoutes(t *testing.T) {
	h := newTestServer(t)

	for _, path := range []string{"/rss/smzdm/post", "/rss/smzdm/post.xml"} {
		rec := get(t, h, path)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d body=%s", path, rec.Code, rec.Body.String())
		}
		body := rec.Body.String()
		for _, want := range []string{
			"<title>SMZDM Community</title>",
			"https://post.smzdm.com/p/a3mxz8nr/",
			"Posted by 家电算账本",
			"<ttl>10</ttl>",
		} {
			if !strings.Contains(body, want) {
				t.Fatalf("%s missing %q:\n%s", path, want, body)
			}
		}
	}

	rec := get(t, h, "/api/v1/smzdm?channel=post&limit=1")
	if rec.Code != http.StatusOK {
		t.Fatalf("api status = %d body=%s", rec.Code, rec.Body.String())
	}
	var out struct {
		Channel string `json:"channel"`
		Title   string `json:"title"`
		Items   []struct {
			ID     string `json:"id"`
			Author string `json:"author"`
			Text   string `json:"text"`
			URL    string `json:"url"`
			HTML   string `json:"html"`
			Image  string `json:"-"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v\n%s", err, rec.Body.String())
	}
	if out.Channel != "post" || out.Title != "SMZDM Community" {
		t.Fatalf("channel view = %+v", out)
	}
	if len(out.Items) != 1 || out.Items[0].ID != "a3mxz8nr" {
		t.Fatalf("items = %+v", out.Items)
	}
	if !strings.HasPrefix(out.Items[0].Text, "先说结论") {
		t.Fatalf("text = %q, want the article blurb", out.Items[0].Text)
	}
	if out.Items[0].Author != "家电算账本" {
		t.Fatalf("author = %q, want the writer from <author>", out.Items[0].Author)
	}
	if !strings.Contains(out.Items[0].HTML, "Read the article on SMZDM") {
		t.Fatalf("html did not use the article label:\n%s", out.Items[0].HTML)
	}
}

func TestSmzdmHaitaoRoute(t *testing.T) {
	h := newTestServer(t)
	rec := get(t, h, "/rss/smzdm/haitao")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "<title>SMZDM Overseas Deals</title>") {
		t.Fatalf("wrong channel title:\n%s", rec.Body.String()[:200])
	}
}

func TestSmzdmUnknownChannel(t *testing.T) {
	h := newTestServer(t)
	rec := get(t, h, "/api/v1/smzdm?channel=faxian")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "BAD_REQUEST") {
		t.Fatalf("error envelope missing BAD_REQUEST: %s", rec.Body.String())
	}
}
