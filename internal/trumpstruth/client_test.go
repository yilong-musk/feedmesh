package trumpstruth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const sampleFeed = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:media="http://search.yahoo.com/mrss/" xmlns:truth="https://truthsocial.com/ns" xmlns:archive="https://www.trumpstruth.org/rss-extensions">
<channel>
<title><![CDATA[Trump's Truth - Latest Posts]]></title>
<link>https://www.trumpstruth.org</link>
<description><![CDATA[Archive of Donald Trump's TRUTH Social posts]]></description>
<item>
<title><![CDATA[Image post from November 27, 2026]]></title>
<link>https://www.trumpstruth.org/statuses/43761</link>
<description><![CDATA[<p></p><p><a href="https://cdn.example/a.jpg">View image</a></p>]]></description>
<dc:creator>Donald J. Trump</dc:creator>
<guid>https://www.trumpstruth.org/statuses/43761</guid>
<pubDate>Fri, 27 Nov 2026 21:56:39 +0000</pubDate>
<truth:originalUrl>https://truthsocial.com/@realDonaldTrump/117690636694612578</truth:originalUrl>
<archive:postType>original</archive:postType>
<archive:removed>false</archive:removed>
<media:content url="https://cdn.example/a.jpg" medium="image"/>
</item>
<item>
<title><![CDATA[A short statement]]></title>
<link>https://www.trumpstruth.org/statuses/43700</link>
<description><![CDATA[<p>Hello from the archive.</p>]]></description>
<dc:creator>Donald J. Trump</dc:creator>
<pubDate>Fri, 27 Nov 2026 19:00:00 +0000</pubDate>
<archive:postType>quote</archive:postType>
<archive:removed>true</archive:removed>
</item>
</channel>
</rss>`

func TestLatestParsesArchiveFeed(t *testing.T) {
	var gotLimit string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotLimit = r.URL.Query().Get("limit")
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(sampleFeed))
	}))
	defer srv.Close()
	c := New(Config{BaseURL: srv.URL, Timeout: 2 * time.Second})
	res, err := c.Latest(context.Background(), 20)
	if err != nil {
		t.Fatal(err)
	}
	if gotLimit != "20" {
		t.Fatalf("limit query = %q", gotLimit)
	}
	if len(res.Posts) != 2 {
		t.Fatalf("posts = %d", len(res.Posts))
	}
	img := res.Posts[0]
	if img.ID != "43761" || img.Image != "https://cdn.example/a.jpg" || img.Text != "" || img.PostType != "original" || img.Removed {
		t.Fatalf("image post = %+v", img)
	}
	if img.Author != "Donald J. Trump" || !strings.Contains(img.OriginalURL, "truthsocial.com") {
		t.Fatalf("image meta = %+v", img)
	}
	text := res.Posts[1]
	if text.Text != "Hello from the archive." || text.PostType != "quote" || !text.Removed {
		t.Fatalf("text post = %+v", text)
	}
}

func TestLatestRejectsHTML(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("<html><title>Attention Required!</title></html>"))
	}))
	defer srv.Close()
	c := New(Config{BaseURL: srv.URL, Timeout: 2 * time.Second})
	_, err := c.Latest(context.Background(), 5)
	if err == nil || !strings.Contains(err.Error(), "HTML") {
		t.Fatalf("err = %v", err)
	}
}

func TestLatestMapsStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()
	c := New(Config{BaseURL: srv.URL, Timeout: 2 * time.Second})
	_, err := c.Latest(context.Background(), 5)
	if err != ErrRateLimited {
		t.Fatalf("err = %v", err)
	}
}
