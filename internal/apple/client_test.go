package apple

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const lookupJSON = `{
  "resultCount": 1,
  "results": [{
    "collectionName": "The Daily",
    "artistName": "The New York Times",
    "feedUrl": "FEED",
    "trackViewUrl": "https://podcasts.apple.com/us/podcast/the-daily/id1200361736",
    "artworkUrl600": "https://img.example/art.jpg"
  }]
}`

const feedXML = `<?xml version="1.0" encoding="UTF-8"?>
<rss xmlns:itunes="http://www.itunes.com/dtds/podcast-1.0.dtd" xmlns:content="http://purl.org/rss/1.0/modules/content/">
  <channel>
    <title>The Daily</title>
    <language>en-us</language>
    <itunes:author>The New York Times</itunes:author>
    <description>News</description>
    <itunes:image href="https://img.example/show.jpg"/>
    <item>
      <title>Older</title>
      <guid>old</guid>
      <pubDate>Mon, 23 Nov 2026 11:00:00 +0000</pubDate>
      <itunes:duration>20:15</itunes:duration>
      <enclosure url="https://cdn.example/old.mp3" type="audio/mpeg" length="1000"/>
      <content:encoded><![CDATA[<p>Old body</p>]]></content:encoded>
    </item>
    <item>
      <title>Newer</title>
      <link>https://www.nytimes.com/newer</link>
      <guid>new</guid>
      <pubDate>Fri, 27 Nov 2026 11:00:00 +0000</pubDate>
      <itunes:duration>1:02:03</itunes:duration>
      <enclosure url="https://cdn.example/new.mp3" type="audio/mpeg" length="2048"/>
      <itunes:image href="https://img.example/ep.jpg"/>
      <description><![CDATA[<p>New &amp; body</p>]]></description>
    </item>
  </channel>
</rss>`

func TestEpisodesParsesPublisherFeed(t *testing.T) {
	var feeds int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "lookup"):
			if r.URL.Query().Get("id") != "1200361736" {
				t.Errorf("id = %s", r.URL.Query().Get("id"))
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(strings.ReplaceAll(lookupJSON, "FEED", "http://"+r.Host+"/feed.xml")))
		default:
			feeds++
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(feedXML))
		}
	}))
	defer srv.Close()

	c := New(Config{LookupURL: srv.URL, ChartURL: srv.URL, Timeout: 5 * time.Second, AllowPrivate: true})
	show, err := c.Episodes(context.Background(), "https://podcasts.apple.com/us/podcast/the-daily/id1200361736", 10)
	if err != nil {
		t.Fatal(err)
	}
	if feeds != 1 {
		t.Fatalf("feed fetches = %d", feeds)
	}
	if show.Title != "The Daily" || show.Author != "The New York Times" || show.Language != "en-us" {
		t.Fatalf("show = %+v", show)
	}
	if show.Image != "https://img.example/show.jpg" {
		t.Fatalf("image = %s", show.Image)
	}
	if len(show.Episodes) != 2 || show.Episodes[0].Title != "Newer" {
		t.Fatalf("episodes = %+v", show.Episodes)
	}
	ep := show.Episodes[0]
	if ep.ID != "new" || ep.EnclosureURL != "https://cdn.example/new.mp3" || ep.EnclosureLength != 2048 {
		t.Fatalf("episode = %+v", ep)
	}
	if ep.Duration != 1*time.Hour+2*time.Minute+3*time.Second {
		t.Fatalf("duration = %s", ep.Duration)
	}
	if ep.Published.Day() != 27 || ep.ShowURL == "" || ep.ShowTitle != "The Daily" {
		t.Fatalf("header = %+v", ep)
	}
	if !strings.Contains(ep.Summary, "New") {
		t.Fatalf("summary = %s", ep.Summary)
	}
}

func TestEpisodesRejectsPrivateFeed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "lookup") {
			_, _ = w.Write([]byte(strings.ReplaceAll(lookupJSON, "FEED", "http://127.0.0.1/secret")))
			return
		}
		t.Errorf("publisher feed was fetched: %s", r.URL)
	}))
	defer srv.Close()
	c := New(Config{LookupURL: srv.URL, Timeout: 5 * time.Second})
	_, err := c.Episodes(context.Background(), "1200361736", 5)
	if err == nil {
		t.Fatal("expected private feed to be rejected")
	}
}

func TestParseIDAndCountry(t *testing.T) {
	id, err := ParseID("id1200361736")
	if err != nil || id != "1200361736" {
		t.Fatalf("ParseID = %s %v", id, err)
	}
	if _, err := ParseID("nope"); err != ErrInvalidID {
		t.Fatalf("err = %v", err)
	}
	country, err := ParseCountry("US")
	if err != nil || country != "us" {
		t.Fatalf("country = %s %v", country, err)
	}
	if _, err := ParseCountry("usa"); err != ErrInvalidCountry {
		t.Fatalf("err = %v", err)
	}
}

func TestChart(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/api/v2/us/podcasts/top/50/") {
			t.Errorf("path = %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"feed":{"title":"Top Shows","results":[
			{"id":"1","name":"One","artistName":"A","url":"https://podcasts.apple.com/us/podcast/one/id1","artworkUrl100":"https://img.example/1.jpg","genres":[{"name":"News"}]},
			{"id":"2","name":"Two","artistName":"B","url":"https://podcasts.apple.com/us/podcast/two/id2","artworkUrl100":"https://img.example/2.jpg","genres":[{"name":"Society"}]}
		]}}`))
	}))
	defer srv.Close()
	c := New(Config{ChartURL: srv.URL, Timeout: 5 * time.Second, AllowPrivate: true})
	chart, err := c.Chart(context.Background(), "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if chart.Country != "us" || len(chart.Entries) != 1 || chart.Entries[0].Name != "One" || chart.Entries[0].Genre != "News" {
		t.Fatalf("chart = %+v", chart)
	}
}

func TestMissingShow(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"resultCount":0,"results":[]}`))
	}))
	defer srv.Close()
	c := New(Config{LookupURL: srv.URL, Timeout: 5 * time.Second, AllowPrivate: true})
	_, err := c.Episodes(context.Background(), "1200361736", 5)
	if err != ErrNotFound {
		t.Fatalf("err = %v", err)
	}
}
