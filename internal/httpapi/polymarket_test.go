package httpapi_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

const pmUserFeed = `[
 {"id":"101","slug":"event-one","title":"Event One","description":"First event.",
  "image":"https://img.example/one.jpg","createdAt":"2026-11-25T09:10:00Z",
  "startDate":"2026-11-25T10:00:00Z","endDate":"2026-12-01T00:00:00Z",
  "volume":"1000000","volume24hr":"25000","liquidity":"9000",
  "markets":[
   {"id":"m1","question":"Will A win?","groupItemTitle":"A","slug":"will-a-win","outcomes":"[\"Yes\", \"No\"]","outcomePrices":"[\"0.4\", \"0.6\"]","volume":"100","active":true,"closed":false,"startDate":"2026-11-20T00:00:00Z","endDate":"2026-12-01T00:00:00Z"}
  ]}
]`

const pmEventFeed = `{"id":"201","slug":"event-one","title":"Event One","description":"First event.",
 "image":"https://img.example/one.jpg","createdAt":"2026-11-25T09:10:00Z",
 "startDate":"2026-11-25T10:00:00Z","endDate":"2026-12-01T00:00:00Z",
 "volume24hr":"25000","liquidity":"9000",
 "markets":[
  {"id":"m1","question":"Will A win?","groupItemTitle":"A","slug":"will-a-win","outcomes":"[\"Yes\", \"No\"]","outcomePrices":"[\"0.4\", \"0.6\"]","volume":"10","active":true,"closed":false,"startDate":"2026-11-20T00:00:00Z","endDate":"2026-12-01T00:00:00Z"},
  {"id":"m2","question":"Will B win?","groupItemTitle":"B","slug":"will-b-win","outcomes":"[\"Yes\", \"No\"]","outcomePrices":"[\"0.9\", \"0.1\"]","volume":"300","active":true,"closed":false,"startDate":"2026-11-20T00:00:00Z","endDate":"2026-12-01T00:00:00Z"}
 ]}`

const pmActivityFeed = `{"data":[
 {"proxy_wallet":"0x7c3db723f1d4d8cb9c550095203b686cb11e5c6b","timestamp":1795590667,
  "type":"TRADE","size":333.0,"usdc_size":186.48,"price":0.56,
  "transaction_hash":"0xdead","token_id":"123","side":"SELL","outcome_index":0,
  "title":"Will A win?","slug":"will-a-win","event_slug":"event-one","outcome":"Yes",
  "icon":"https://img.example/one.jpg"}
],"pagination":{"limit":1,"offset":0,"has_more":false,"next_cursor":null}}`

func TestPolymarketNewRoute(t *testing.T) {
	h := newTestServer(t)
	for _, path := range []string{"/rss/polymarket/new", "/rss/polymarket/new.xml"} {
		rec := get(t, h, path)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d body=%s", path, rec.Code, rec.Body.String())
		}
		if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/rss+xml") {
			t.Fatalf("%s content-type = %q", path, ct)
		}
		if got := rec.Header().Get("Cache-Control"); got != "public, max-age=600" {
			t.Fatalf("%s cache-control = %q", path, got)
		}
		body := rec.Body.String()
		for _, want := range []string{
			"<title>Polymarket New Markets</title>",
			"https://polymarket.com/event/event-one",
			"Yes 40.0%",
			"https://img.example/one.jpg",
		} {
			if !strings.Contains(body, want) {
				t.Fatalf("%s missing %q:\n%s", path, want, body)
			}
		}
	}
}

func TestPolymarketHotRoute(t *testing.T) {
	h := newTestServer(t)
	rec := get(t, h, "/rss/polymarket/hot?tag=politics&limit=1")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Polymarket Trending · politics") {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestPolymarketEventRoute(t *testing.T) {
	h := newTestServer(t)
	for _, path := range []string{"/rss/polymarket/event/event-one", "/rss/polymarket/event/event-one.xml"} {
		rec := get(t, h, path)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d body=%s", path, rec.Code, rec.Body.String())
		}
		if got := rec.Header().Get("Cache-Control"); got != "public, max-age=300" {
			t.Fatalf("%s cache-control = %q", path, got)
		}
		body := rec.Body.String()
		for _, want := range []string{
			"<title>Event One on Polymarket</title>",
			"https://polymarket.com/event/event-one/will-b-win",
			"Yes 90.0%",
		} {
			if !strings.Contains(body, want) {
				t.Fatalf("%s missing %q:\n%s", path, want, body)
			}
		}
	}
}

func TestPolymarketUserRoute(t *testing.T) {
	h := newTestServer(t)
	for _, path := range []string{
		"/rss/polymarket/user/0x7c3db723f1d4d8cb9c550095203b686cb11e5c6b",
		"/rss/polymarket/user/0x7c3db723f1d4d8cb9c550095203b686cb11e5c6b.xml",
	} {
		rec := get(t, h, path)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d body=%s", path, rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "Polymarket wallet 0x7c3d…5c6b") {
			t.Fatalf("body = %s", rec.Body.String())
		}
	}
}

func TestPolymarketAPIRoute(t *testing.T) {
	h := newTestServer(t)
	rec := get(t, h, "/api/v1/polymarket?feed=event&slug=event-one&limit=1")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var out struct {
		Provider string `json:"provider"`
		Title    string `json:"title"`
		Cached   bool   `json:"cached"`
		Meta     struct {
			Feed string `json:"feed"`
			Slug string `json:"slug"`
		} `json:"meta"`
		Items []struct {
			ID        string    `json:"id"`
			Title     string    `json:"title"`
			CreatedAt time.Time `json:"created_at"`
			Extra     struct {
				Volume24H float64   `json:"volume_24h"`
				EndsAt    time.Time `json:"ends_at"`
			} `json:"extra"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v\n%s", err, rec.Body.String())
	}
	if out.Provider != "polymarket" || out.Meta.Feed != "event" || out.Meta.Slug != "event-one" || out.Title != "Event One on Polymarket" {
		t.Fatalf("channel = %+v", out)
	}
	if len(out.Items) != 1 || out.Items[0].Title != "B" {
		t.Fatalf("items = %+v", out.Items)
	}
	if out.Items[0].Extra.EndsAt.IsZero() {
		t.Fatalf("extra.ends_at missing: %+v", out.Items[0].Extra)
	}
	if got := out.Items[0].CreatedAt.Format(time.RFC3339); got != "2026-11-20T00:00:00Z" {
		t.Fatalf("created_at = %s", got)
	}
	if out.Cached {
		t.Fatal("first call must not be a cache hit")
	}
}

func TestPolymarketAPIActivity(t *testing.T) {
	h := newTestServer(t)
	rec := get(t, h, "/api/v1/polymarket?feed=user&address=0x7c3db723f1d4d8cb9c550095203b686cb11e5c6b")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"feed":"user"`) ||
		!strings.Contains(rec.Body.String(), `"address":"0x7c3db723f1d4d8cb9c550095203b686cb11e5c6b"`) {
		t.Fatalf("body = %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "ends_at") {
		t.Fatalf("wallet activity has no end date, got: %s", rec.Body.String())
	}
}

func TestPolymarketAPIBadFeed(t *testing.T) {
	h := newTestServer(t)
	rec := get(t, h, "/api/v1/polymarket?feed=nope")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestPolymarketAPINotFound(t *testing.T) {
	h := newTestServer(t)
	rec := get(t, h, "/api/v1/polymarket?feed=event&slug=missing")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if code := rec.Header().Get("X-Feedmesh-Code"); code != "USER_NOT_FOUND" {
		t.Fatalf("code = %q", code)
	}
}
