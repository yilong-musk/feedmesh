package polymarket

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// eventsJSON is a trimmed copy of a live /events answer: two events, the first
// with five markets (one closed) so the summary cap and the closed filter have
// something to act on.
const eventsJSON = `[
 {"id":"101","slug":"event-one","title":"Event One","description":"First event.",
  "image":"https://img.example/one.jpg","createdAt":"2026-11-25T09:10:00.123456Z",
  "startDate":"2026-11-25T10:00:00Z","endDate":"2026-12-01T00:00:00Z",
  "volume":"1000000","volume24hr":"25000","liquidity":"9000",
  "tags":[{"slug":"politics","label":"Politics"}],
  "markets":[
   {"id":"m1","question":"Will A win?","groupItemTitle":"A","slug":"will-a-win","outcomes":"[\"Yes\", \"No\"]","outcomePrices":"[\"0.4\", \"0.6\"]","volume":"100","volume24hr":"10","liquidityNum":"5","oneDayPriceChange":"0.05","startDate":"2026-11-20T00:00:00Z","endDate":"2026-12-01T00:00:00Z","active":true,"closed":false},
   {"id":"m2","question":"Will B win?","groupItemTitle":"B","slug":"will-b-win","outcomes":"[\"Yes\", \"No\"]","outcomePrices":"[\"0.9\", \"0.1\"]","volume":"900","volume24hr":"90","liquidityNum":"50","startDate":"2026-11-20T00:00:00Z","endDate":"2026-12-01T00:00:00Z","active":true,"closed":false},
   {"id":"m3","question":"Will C win?","groupItemTitle":"C","slug":"will-c-win","outcomes":"[\"Yes\", \"No\"]","outcomePrices":"[\"0.2\", \"0.8\"]","volume":"400","volume24hr":"40","liquidityNum":"20","startDate":"2026-11-20T00:00:00Z","endDate":"2026-12-01T00:00:00Z","active":true,"closed":false},
   {"id":"m4","question":"Will D win?","groupItemTitle":"D","slug":"will-d-win","outcomes":"[\"Yes\", \"No\"]","outcomePrices":"[\"0.5\", \"0.5\"]","volume":"200","volume24hr":"20","liquidityNum":"10","startDate":"2026-11-20T00:00:00Z","endDate":"2026-12-01T00:00:00Z","active":true,"closed":false},
   {"id":"m5","question":"Closed market","groupItemTitle":"E","slug":"closed","outcomes":"[\"Yes\", \"No\"]","outcomePrices":"[\"1\", \"0\"]","volume":"9999","active":true,"closed":true}
  ]},
 {"id":"102","slug":"event-two","title":"Event Two","description":"Second event.",
  "image":"https://img.example/two.jpg","createdAt":"2026-11-24T09:10:00Z",
  "startDate":"2026-11-24T10:00:00Z","endDate":"2026-12-02T00:00:00Z",
  "volume":"500","volume24hr":"120","liquidity":"30",
  "markets":[]}
]`

// eventJSON is a trimmed copy of /events/slug/<slug>: three active markets
// with volumes 10, 300 and 100.
const eventJSON = `{"id":"201","slug":"event-one","title":"Event One","description":"First event.",
 "image":"https://img.example/one.jpg","createdAt":"2026-11-25T09:10:00Z",
 "startDate":"2026-11-25T10:00:00Z","endDate":"2026-12-01T00:00:00Z",
 "volume":"1000000","volume24hr":"25000","liquidity":"9000",
 "markets":[
  {"id":"m1","question":"Will A win?","groupItemTitle":"A","slug":"will-a-win","outcomes":"[\"Yes\", \"No\"]","outcomePrices":"[\"0.4\", \"0.6\"]","volume":"10","active":true,"closed":false,"startDate":"2026-11-20T00:00:00Z","endDate":"2026-12-01T00:00:00Z"},
  {"id":"m2","question":"Will B win?","groupItemTitle":"B","slug":"will-b-win","outcomes":"[\"Yes\", \"No\"]","outcomePrices":"[\"0.9\", \"0.1\"]","volume":"300","active":true,"closed":false,"startDate":"2026-11-20T00:00:00Z","endDate":"2026-12-01T00:00:00Z"},
  {"id":"m3","question":"Will C win?","groupItemTitle":"C","slug":"will-c-win","outcomes":"[\"Yes\", \"No\"]","outcomePrices":"[\"0.2\", \"0.8\"]","volume":"100","active":true,"closed":false,"startDate":"2026-11-20T00:00:00Z","endDate":"2026-12-01T00:00:00Z"}
 ]}`

// activityJSON is a trimmed copy of a Data API v2 answer.
const activityJSON = `{"data":[
 {"proxy_wallet":"0x7C3DB723F1D4D8CB9C550095203B686CB11E5C6B","timestamp":1795590667,
  "type":"TRADE","size":333.0,"usdc_size":186.48,"price":0.56,
  "transaction_hash":"0xdead","token_id":"123","side":"SELL","outcome_index":0,
  "title":"Will A win?","slug":"will-a-win","event_slug":"event-one","outcome":"Yes",
  "icon":"https://img.example/one.jpg"},
 {"proxy_wallet":"0x7c3db723f1d4d8cb9c550095203b686cb11e5c6b","timestamp":1795588163,
  "type":"REDEEM","size":12.5,"usdc_size":12.5,"price":1.0,
  "transaction_hash":"0xbeef","token_id":"456","outcome_index":1,
  "title":"Will B win?","slug":"will-b-win","event_slug":"event-one","outcome":"No",
  "icon":"https://img.example/two.jpg"}
],"pagination":{"limit":2,"offset":0,"has_more":true,"next_cursor":"abc"}}`

func newTestClient(t *testing.T) *Client {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/events/slug/event-one", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(eventJSON))
	})
	mux.HandleFunc("/events/slug/missing", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"type":"not found error","error":"slug not found"}`))
	})
	mux.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(eventsJSON))
	})
	mux.HandleFunc("/v2/activity", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("user") == "0x00000000000000000000000000000000000000ff" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"invalid","code":"invalid_request"}`))
			return
		}
		_, _ = w.Write([]byte(activityJSON))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(Config{BaseURL: srv.URL, DataURL: srv.URL + "/v2", Timeout: 2 * time.Second})
	c.now = func() time.Time { return time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC) }
	return c
}

func TestListEventsNew(t *testing.T) {
	c := newTestClient(t)
	events, err := c.ListEvents(context.Background(), OrderNew, "politics", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("events = %d", len(events))
	}
	if events[0].Title != "Event One" || events[0].Slug != "event-one" {
		t.Fatalf("first = %+v", events[0])
	}
	if len(events[0].Markets) != 3 {
		t.Fatalf("summary markets = %d, want 3 (closed filtered, capped)", len(events[0].Markets))
	}
	// Summary markets are ranked by volume before the cap.
	if events[0].Markets[0].ID != "m2" || events[0].Markets[1].ID != "m3" || events[0].Markets[2].ID != "m4" {
		t.Fatalf("summary markets = %v", []string{events[0].Markets[0].ID, events[0].Markets[1].ID, events[0].Markets[2].ID})
	}
	if got := events[0].CreatedAt.Format(time.RFC3339Nano); got != "2026-11-25T09:10:00.123456Z" {
		t.Fatalf("created = %s", got)
	}
	if events[0].Volume24h != 25000 || events[0].Liquidity != 9000 {
		t.Fatalf("numbers = %v/%v", events[0].Volume24h, events[0].Liquidity)
	}
	if events[0].FetchedAt.IsZero() {
		t.Fatal("FetchedAt must be stamped from the client clock")
	}
}

func TestListEventsRejectsBadTag(t *testing.T) {
	c := newTestClient(t)
	if _, err := c.ListEvents(context.Background(), OrderNew, "Not A Tag", 5); !errors.Is(err, ErrInvalidTag) {
		t.Fatalf("err = %v", err)
	}
}

func TestEventSortsAndLimits(t *testing.T) {
	c := newTestClient(t)
	ev, err := c.Event(context.Background(), "event-one", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(ev.Markets) != 2 {
		t.Fatalf("markets = %d", len(ev.Markets))
	}
	if ev.Markets[0].ID != "m2" || ev.Markets[1].ID != "m3" {
		t.Fatalf("order = %s,%s, want m2,m3", ev.Markets[0].ID, ev.Markets[1].ID)
	}
	if len(ev.Markets[0].Prices) != 2 || ev.Markets[0].Prices[0] != 0.9 {
		t.Fatalf("prices = %v", ev.Markets[0].Prices)
	}
}

func TestEventNotFound(t *testing.T) {
	c := newTestClient(t)
	if _, err := c.Event(context.Background(), "missing", 5); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestActivity(t *testing.T) {
	c := newTestClient(t)
	acts, err := c.Activity(context.Background(), "0x7C3DB723F1D4D8CB9C550095203B686CB11E5C6B", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(acts) != 2 {
		t.Fatalf("activities = %d", len(acts))
	}
	a := acts[0]
	if a.Wallet != "0x7c3db723f1d4d8cb9c550095203b686cb11e5c6b" {
		t.Fatalf("wallet = %q", a.Wallet)
	}
	if a.Type != "TRADE" || a.Side != "SELL" || a.Outcome != "Yes" || a.Price != 0.56 {
		t.Fatalf("activity = %+v", a)
	}
	if got := a.Timestamp.UTC().Format(time.RFC3339); got != "2026-11-25T07:11:07Z" {
		t.Fatalf("timestamp = %s", got)
	}
}

func TestActivityBadAddress(t *testing.T) {
	c := newTestClient(t)
	if _, err := c.Activity(context.Background(), "0x123", 2); !errors.Is(err, ErrInvalidAddress) {
		t.Fatalf("err = %v", err)
	}
}

func TestActivityBadRequest(t *testing.T) {
	c := newTestClient(t)
	if _, err := c.Activity(context.Background(), "0x00000000000000000000000000000000000000FF", 2); !errors.Is(err, ErrBadRequest) {
		t.Fatalf("err = %v", err)
	}
}

func TestNormalize(t *testing.T) {
	if got, err := NormalizeTag(" Politics "); err != nil || got != "politics" {
		t.Fatalf("tag = %q, %v", got, err)
	}
	if got, err := NormalizeSlug("event-one"); err != nil || got != "event-one" {
		t.Fatalf("slug = %q, %v", got, err)
	}
	if _, err := NormalizeSlug("bad slug"); err == nil {
		t.Fatal("slug with a space must fail")
	}
	if _, err := NormalizeAddress("0xabc"); err == nil {
		t.Fatal("short address must fail")
	}
}

func TestFlexFloatAndStringList(t *testing.T) {
	c := newTestClient(t)
	ev, err := c.Event(context.Background(), "event-one", 1)
	if err != nil {
		t.Fatal(err)
	}
	if ev.Volume != 1000000 {
		t.Fatalf("string volume = %v", ev.Volume)
	}
	if len(ev.Markets[0].Outcomes) != 2 || ev.Markets[0].Outcomes[0] != "Yes" {
		t.Fatalf("outcomes = %v", ev.Markets[0].Outcomes)
	}
}
