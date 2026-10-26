package main

import (
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
	doc, problem := parseFeed([]byte(raw))
	if problem != "" {
		t.Fatalf("parseFeed: %s", problem)
	}
	return doc
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

func TestProblemsForNoItems(t *testing.T) {
	// A channel with no items is a valid (just empty) feed; the report must
	// stay clean instead of tripping over the missing first item.
	raw := `<rss version="2.0"><channel><title>T</title><link>l</link><ttl>5</ttl></channel></rss>`
	if got := problemsFor(parse(t, raw)); len(got) != 0 {
		t.Fatalf("problems = %v, want none", got)
	}
}

func TestProblemsForMissingChannel(t *testing.T) {
	raw := `<rss version="2.0"></rss>`
	got := problemsFor(parse(t, raw))
	if len(got) != 1 || got[0] != "missing channel" {
		t.Fatalf("problems = %v, want [missing channel]", got)
	}
}

func TestParseFeedRejectsNonRSSRoot(t *testing.T) {
	raw := `<feed xmlns="http://www.w3.org/2005/Atom"><title>A</title></feed>`
	doc, problem := parseFeed([]byte(raw))
	if doc != nil {
		t.Fatalf("doc = %+v, want nil", doc)
	}
	if !strings.Contains(problem, "root element is <feed>, want <rss>") {
		t.Fatalf("problem = %q, want the root-element message", problem)
	}
	if strings.Contains(problem, "not well-formed") {
		t.Fatalf("problem = %q, must not call well-formed XML malformed", problem)
	}
}

func TestParseFeedRejectsMalformedXML(t *testing.T) {
	raw := `<rss version="2.0"><channel></rss>`
	if _, problem := parseFeed([]byte(raw)); !strings.Contains(problem, "not well-formed XML") {
		t.Fatalf("problem = %q, want the malformed-XML message", problem)
	}
}
