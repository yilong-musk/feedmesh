package httpapi_test

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestXRSSFeed(t *testing.T) {
	h := newTestServer(t)
	rec := get(t, h, "/rss/x/elonmusk")
	if rec.Code != 200 {
		t.Fatalf("/rss/x/elonmusk status = %d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		"https://x.com/elonmusk/status/2113508310267674684",
		"https://x.com/elonmusk/status/2113318444934562215",
		"https://pbs.twimg.com/media/a.jpg",
		"<ttl>40</ttl>",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("RSS body missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "127.0.0.1") {
		t.Errorf("RSS body leaked the mirror host:\n%s", body)
	}
	if got := rec.Header().Get("Cache-Control"); !strings.Contains(got, "2400") {
		t.Errorf("Cache-Control = %q, want max-age=2400", got)
	}
}

func TestXJSONView(t *testing.T) {
	h := newTestServer(t)
	rec := get(t, h, "/api/v1/x?handle=elonmusk&limit=1")
	if rec.Code != 200 {
		t.Fatalf("/api/v1/x status = %d body=%s", rec.Code, rec.Body.String())
	}
	var doc struct {
		Handle string `json:"handle"`
		Items  []struct {
			ID      string   `json:"id"`
			Title   string   `json:"title"`
			Text    string   `json:"text"`
			URL     string   `json:"url"`
			HTML    string   `json:"html"`
			Images  []string `json:"images"`
			Created string   `json:"created_at"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("decode: %v body=%s", err, rec.Body.String())
	}
	if doc.Handle != "elonmusk" {
		t.Errorf("handle = %q", doc.Handle)
	}
	if len(doc.Items) != 1 {
		t.Fatalf("limit=1 returned %d items", len(doc.Items))
	}
	it := doc.Items[0]
	if it.ID != "2113508310267674684" || it.URL != "https://x.com/elonmusk/status/2113508310267674684" {
		t.Errorf("item = %+v", it)
	}
	if len(it.Images) != 1 || !strings.HasPrefix(it.Images[0], "https://pbs.twimg.com/") {
		t.Errorf("images = %v", it.Images)
	}
	if strings.Contains(it.HTML, "127.0.0.1") {
		t.Errorf("item HTML leaked the mirror: %s", it.HTML)
	}
	if it.Created == "" {
		t.Error("created_at is empty")
	}
}

func TestXRejectsBadHandle(t *testing.T) {
	h := newTestServer(t)
	rec := get(t, h, "/rss/x/bad-handle")
	if rec.Code != 400 {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "invalid X handle") {
		t.Errorf("body = %s", rec.Body.String())
	}
}
