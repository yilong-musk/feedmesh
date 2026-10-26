package main

import (
	"encoding/xml"
	"strings"
	"testing"
)

const validFeed = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel><title>T</title><link>https://example.test/</link><ttl>5</ttl>
<item><title>One</title><link>https://example.test/1</link>
<guid isPermaLink="true">https://example.test/1</guid>
<description>body</description><pubDate>Sun, 25 Oct 2026 21:39:37 +0800</pubDate></item>
</channel></rss>`

func parse(t *testing.T, raw string) *document {
	t.Helper()
	var doc document
	if err := xml.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return &doc
}

func TestProblemsForValidFeed(t *testing.T) {
	if got := problemsFor(parse(t, validFeed)); len(got) != 0 {
		t.Fatalf("problems = %v, want none", got)
	}
}

func TestProblemsForMissingFields(t *testing.T) {
	raw := `<rss version="2.0"><channel><title>T</title>
<item><title>One</title><guid>x</guid></item></channel></rss>`
	got := problemsFor(parse(t, raw))
	want := []string{
		"item 0: missing link",
		"item 0: missing description",
		"item 0: missing pubDate",
		"item 0: guid isPermaLink missing",
	}
	if strings.Join(got, " | ") != strings.Join(want, " | ") {
		t.Fatalf("problems = %v, want %v", got, want)
	}
}

func TestProblemsForBareCDATATerminator(t *testing.T) {
	// The XML parser accepts the escaped terminator; the sanity check flags
	// the resulting text because a raw ]]> cannot be wrapped in CDATA again.
	raw := `<rss version="2.0"><channel><title>T</title>
<item><title>One</title><link>l</link><guid isPermaLink="true">g</guid>
<description>a ]]&gt; b</description><pubDate>d</pubDate></item></channel></rss>`
	got := problemsFor(parse(t, raw))
	want := "item 0: description contains a bare ]]>"
	if len(got) != 1 || got[0] != want {
		t.Fatalf("problems = %v, want [%s]", got, want)
	}
}
