package feed

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/yilong-musk/feedmesh/internal/cache"
	"github.com/yilong-musk/feedmesh/internal/logx"
	"github.com/yilong-musk/feedmesh/internal/polymarket"
	"github.com/yilong-musk/feedmesh/internal/weibo"
)

const pmEventsJSON = `[
 {"id":"101","slug":"event-one","title":"Event One","description":"First event.",
  "image":"https://img.example/one.jpg","createdAt":"2026-11-25T09:10:00Z",
  "startDate":"2026-11-25T10:00:00Z","endDate":"2026-12-01T00:00:00Z",
  "volume":"1000000","volume24hr":"25000","liquidity":"9000",
  "markets":[
   {"id":"m1","question":"Will A win?","groupItemTitle":"A","slug":"will-a-win","outcomes":"[\"Yes\", \"No\"]","outcomePrices":"[\"0.4\", \"0.6\"]","volume":"100","volume24hr":"10","liquidityNum":"5","startDate":"2026-11-20T00:00:00Z","endDate":"2026-12-01T00:00:00Z","active":true,"closed":false},
   {"id":"m2","question":"Will B win?","groupItemTitle":"B","slug":"will-b-win","outcomes":"[\"Yes\", \"No\"]","outcomePrices":"[\"0.9\", \"0.1\"]","volume":"900","volume24hr":"90","liquidityNum":"50","startDate":"2026-11-20T00:00:00Z","endDate":"2026-12-01T00:00:00Z","active":true,"closed":false}
  ]},
 {"id":"102","slug":"event-two","title":"","description":"",
  "createdAt":"2026-11-24T09:10:00Z","volume24hr":"120","liquidity":"30","markets":[]}
]`

const pmEventJSON = `{"id":"201","slug":"event-one","title":"Event One","description":"First event.",
 "image":"https://img.example/one.jpg","createdAt":"2026-11-25T09:10:00Z",
 "startDate":"2026-11-25T10:00:00Z","endDate":"2026-12-01T00:00:00Z",
 "volume24hr":"25000","liquidity":"9000",
 "markets":[
  {"id":"m1","question":"Will A win?","groupItemTitle":"A","slug":"will-a-win","outcomes":"[\"Yes\", \"No\"]","outcomePrices":"[\"0.4\", \"0.6\"]","volume":"10","active":true,"closed":false,"startDate":"2026-11-20T00:00:00Z","endDate":"2026-12-01T00:00:00Z"},
  {"id":"m2","question":"Will B win?","groupItemTitle":"B","slug":"will-b-win","outcomes":"[\"Yes\", \"No\"]","outcomePrices":"[\"0.9\", \"0.1\"]","volume":"300","active":true,"closed":false,"startDate":"2026-11-20T00:00:00Z","endDate":"2026-12-01T00:00:00Z"}
 ]}`

const pmActivityJSON = `{"data":[
 {"proxy_wallet":"0x7c3db723f1d4d8cb9c550095203b686cb11e5c6b","timestamp":1795590667,
  "type":"TRADE","size":333.0,"usdc_size":186.48,"price":0.56,
  "transaction_hash":"0xdead","token_id":"123","side":"SELL","outcome_index":0,
  "title":"Will A win?","slug":"will-a-win","event_slug":"event-one","outcome":"Yes",
  "icon":"https://img.example/one.jpg"},
 {"proxy_wallet":"0x7c3db723f1d4d8cb9c550095203b686cb11e5c6b","timestamp":1795588163,
  "type":"REDEEM","size":12.5,"usdc_size":12.5,"price":1.0,
  "transaction_hash":"0xbeef","token_id":"456","outcome_index":1,
  "title":"Will B win?","slug":"will-b-win","event_slug":"event-one","outcome":"No",
  "icon":"https://img.example/two.jpg"}
],"pagination":{"limit":2,"offset":0,"has_more":false,"next_cursor":null}}`

func newPolymarketService(t *testing.T) *Service {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(pmEventsJSON)) })
	mux.HandleFunc("/events/slug/event-one", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(pmEventJSON)) })
	mux.HandleFunc("/events/slug/missing", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"slug not found"}`))
	})
	mux.HandleFunc("/v2/activity", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(pmActivityJSON)) })
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c, err := cache.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return NewService(Clients{
		Polymarket: polymarket.New(polymarket.Config{BaseURL: srv.URL, DataURL: srv.URL + "/v2", Timeout: 2 * time.Second}),
	}, c, logx.Discard(), DefaultTTLs())
}

func TestGetPolymarketWithoutProvider(t *testing.T) {
	s := NewService(Clients{}, nil, logx.Discard(), DefaultTTLs())
	if _, err := s.GetPolymarketEvents(context.Background(), polymarket.OrderNew, "", 5, false); err == nil {
		t.Fatal("must fail without a provider")
	}
}

func TestGetPolymarketNew(t *testing.T) {
	s := newPolymarketService(t)
	f, err := s.GetPolymarketEvents(context.Background(), polymarket.OrderNew, "", 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if f.Title() != "Polymarket New Markets" {
		t.Fatalf("title = %q", f.Title())
	}
	if len(f.Items) != 2 {
		t.Fatalf("items = %d", len(f.Items))
	}
	it := f.Items[0]
	if it.Title != "Event One" || it.URL != "https://polymarket.com/event/event-one" {
		t.Fatalf("item = %+v", it)
	}
	if !strings.Contains(it.HTML, "<strong>A</strong>: Yes 40.0% · No 60.0%") {
		t.Fatalf("html missing odds:\n%s", it.HTML)
	}
	if !strings.Contains(it.HTML, "<strong>B</strong>: Yes 90.0% · No 10.0%") {
		t.Fatalf("html missing second market:\n%s", it.HTML)
	}
	if it.Volume24H != 25000 || it.Liquidity != 9000 {
		t.Fatalf("numbers = %v/%v", it.Volume24H, it.Liquidity)
	}
	// A blank event title still needs a readable item title.
	if f.Items[1].Title != "(no content)" {
		t.Fatalf("blank event title = %q", f.Items[1].Title)
	}
}

func TestGetPolymarketHotTag(t *testing.T) {
	s := newPolymarketService(t)
	f, err := s.GetPolymarketEvents(context.Background(), polymarket.OrderHot, "politics", 1, false)
	if err != nil {
		t.Fatal(err)
	}
	if f.Title() != "Polymarket Trending · politics" {
		t.Fatalf("title = %q", f.Title())
	}
	if len(f.Items) != 1 {
		t.Fatalf("limit=1 returned %d", len(f.Items))
	}
}

func TestGetPolymarketEvent(t *testing.T) {
	s := newPolymarketService(t)
	f, err := s.GetPolymarketEvent(context.Background(), "event-one", 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if f.Title() != "Event One on Polymarket" {
		t.Fatalf("title = %q", f.Title())
	}
	if len(f.Items) != 2 {
		t.Fatalf("items = %d", len(f.Items))
	}
	if f.Items[0].Title != "B" || !strings.Contains(f.Items[0].Text, "Yes 90.0%") {
		t.Fatalf("first market = %+v", f.Items[0])
	}
	if f.Items[0].URL != "https://polymarket.com/event/event-one/will-b-win" {
		t.Fatalf("url = %q", f.Items[0].URL)
	}
}

func TestGetPolymarketEventCacheWidens(t *testing.T) {
	s := newPolymarketService(t)
	first, err := s.GetPolymarketEvent(context.Background(), "event-one", 1, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Items) != 1 {
		t.Fatalf("limit=1 returned %d", len(first.Items))
	}
	second, err := s.GetPolymarketEvent(context.Background(), "event-one", 2, false)
	if err != nil {
		t.Fatal(err)
	}
	if !second.Cached {
		t.Fatal("second call must hit the cache")
	}
	if len(second.Items) != 2 {
		t.Fatalf("a narrow first request truncated the cached event: %d items", len(second.Items))
	}
}

func TestGetPolymarketNotFound(t *testing.T) {
	s := newPolymarketService(t)
	_, err := s.GetPolymarketEvent(context.Background(), "missing", 0, false)
	var we *weibo.Error
	if !errors.As(err, &we) || we.Code != weibo.CodeUserNotFound {
		t.Fatalf("err = %v", err)
	}
}

func TestGetPolymarketBadTag(t *testing.T) {
	s := newPolymarketService(t)
	_, err := s.GetPolymarketEvents(context.Background(), polymarket.OrderNew, "bad tag", 0, false)
	var we *weibo.Error
	if !errors.As(err, &we) || we.Code != weibo.CodeBadRequest {
		t.Fatalf("err = %v", err)
	}
}

func TestGetPolymarketActivity(t *testing.T) {
	s := newPolymarketService(t)
	f, err := s.GetPolymarketActivity(context.Background(), "0x7c3db723f1d4d8cb9c550095203b686cb11e5c6b", 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Items) != 2 {
		t.Fatalf("items = %d", len(f.Items))
	}
	it := f.Items[0]
	if it.Title != "SELL Yes · Will A win?" {
		t.Fatalf("title = %q", it.Title)
	}
	if !strings.Contains(it.Text, "$186") || !strings.Contains(it.Text, "at 0.560") {
		t.Fatalf("text = %q", it.Text)
	}
	if it.URL != "https://polymarket.com/event/event-one" {
		t.Fatalf("url = %q", it.URL)
	}
}

func TestGetPolymarketActivityCached(t *testing.T) {
	s := newPolymarketService(t)
	addr := "0x7c3db723f1d4d8cb9c550095203b686cb11e5c6b"
	if _, err := s.GetPolymarketActivity(context.Background(), addr, 1, false); err != nil {
		t.Fatal(err)
	}
	f, err := s.GetPolymarketActivity(context.Background(), addr, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	if !f.Cached {
		t.Fatal("second call must be a cache hit")
	}
}
