// Command rsscheck validates a rendered feed: well-formedness and field
// sanity. It mirrors the manual check described in docs/ARCHITECTURE.md.
package main

import (
	"encoding/xml"
	"fmt"
	"os"
	"strings"

	"github.com/yilong-musk/feedmesh/tools/internal/textutil"
)

const usage = "usage: rsscheck <feed.xml>"

type guid struct {
	IsPermaLink string `xml:"isPermaLink,attr"`
	Value       string `xml:",chardata"`
}

type item struct {
	Title       *string `xml:"title"`
	Link        *string `xml:"link"`
	GUID        *guid   `xml:"guid"`
	Description *string `xml:"description"`
	PubDate     *string `xml:"pubDate"`
}

type channel struct {
	Title       *string `xml:"title"`
	Link        *string `xml:"link"`
	TTL         *string `xml:"ttl"`
	Description *string `xml:"description"`
	Items       []item  `xml:"item"`
}

// document binds the root element without constraining it, so a well-formed
// document with another root is reported as such instead of as malformed XML.
type document struct {
	XMLName xml.Name
	Version string   `xml:"version,attr"`
	Channel *channel `xml:"channel"`
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}

	raw, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "rsscheck: %v\n", err)
		os.Exit(2)
	}

	doc, problem := parseFeed(raw)
	if problem != "" {
		fmt.Fprintf(os.Stderr, "rsscheck: %s\n", problem)
		os.Exit(1)
	}
	ch := channelOf(doc)

	fmt.Printf("root tag: %s version: %s\n", doc.XMLName.Local, doc.Version)
	fmt.Printf("channel title: %s\n", text(ch.Title))
	fmt.Printf("ttl: %s\n", text(ch.TTL))
	fmt.Printf("link: %s\n", text(ch.Link))
	fmt.Printf("items: %d\n", len(ch.Items))

	problems := problemsFor(doc)

	if len(ch.Items) > 0 {
		first := ch.Items[0]
		fmt.Println("\nfirst item:")
		fmt.Printf("  title: %s\n", textutil.Head(text(first.Title), 60))
		fmt.Printf("  link : %s\n", text(first.Link))
		fmt.Printf("  date : %s\n", text(first.PubDate))
		fmt.Printf("  desc : %s\n", textutil.Head(strings.ReplaceAll(text(first.Description), "\n", " "), 120))
	}

	empties := 0
	for _, it := range ch.Items {
		if strings.TrimSpace(text(it.Description)) == "" {
			empties++
		}
	}
	fmt.Printf("\nitems with empty description: %d\n", empties)

	if len(problems) > 0 {
		fmt.Printf("problems: %s\n", strings.Join(problems, "; "))
		os.Exit(1)
	}
	fmt.Println("problems: none")
}

// parseFeed unmarshals raw and returns either the document or the reason it
// cannot be checked, separating malformed XML from a well-formed document
// whose root is not <rss>.
func parseFeed(raw []byte) (*document, string) {
	var doc document
	if err := xml.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Sprintf("not well-formed XML: %v", err)
	}
	if doc.XMLName.Local != "rss" {
		return nil, fmt.Sprintf("root element is <%s>, want <rss>", doc.XMLName.Local)
	}
	return &doc, ""
}

// channelOf returns the channel, or an empty one when the feed has none, so
// the report keeps its shape while problemsFor carries the diagnosis.
func channelOf(doc *document) *channel {
	if doc.Channel != nil {
		return doc.Channel
	}
	return &channel{}
}

func text(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// problemsFor returns every field-sanity violation in doc, in document order.
func problemsFor(doc *document) []string {
	if doc.Channel == nil {
		return []string{"missing channel"}
	}
	var problems []string
	for i, it := range doc.Channel.Items {
		required := []struct {
			name  string
			value *string
		}{
			{"title", it.Title},
			{"link", it.Link},
			{"guid", guidValue(it.GUID)},
			{"description", it.Description},
			{"pubDate", it.PubDate},
		}
		for _, field := range required {
			if field.value == nil {
				problems = append(problems, fmt.Sprintf("item %d: missing %s", i, field.name))
			}
		}
		if it.Description != nil && strings.Contains(*it.Description, "]]>") {
			problems = append(problems, fmt.Sprintf("item %d: description contains a bare ]]>", i))
		}
		if it.GUID != nil && it.GUID.IsPermaLink != "true" {
			problems = append(problems, fmt.Sprintf("item %d: guid isPermaLink missing", i))
		}
	}
	return problems
}

// guidValue treats a guid element as present even when its text is empty, so
// the required-field check matches element presence rather than content.
func guidValue(g *guid) *string {
	if g == nil {
		return nil
	}
	value := g.Value
	return &value
}
