// Command rsscheck validates a rendered feed: well-formedness and field
// sanity. It mirrors the manual check described in docs/ARCHITECTURE.md.
package main

import (
	"encoding/xml"
	"fmt"
	"os"
	"strings"
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

type document struct {
	XMLName xml.Name `xml:"rss"`
	Version string   `xml:"version,attr"`
	Channel struct {
		Title       *string `xml:"title"`
		Link        *string `xml:"link"`
		TTL         *string `xml:"ttl"`
		Description *string `xml:"description"`
		Items       []item  `xml:"item"`
	} `xml:"channel"`
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

	var doc document
	if err := xml.Unmarshal(raw, &doc); err != nil {
		fmt.Fprintf(os.Stderr, "rsscheck: not well-formed XML: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("root tag: %s version: %s\n", doc.XMLName.Local, doc.Version)
	fmt.Printf("channel title: %s\n", text(doc.Channel.Title))
	fmt.Printf("ttl: %s\n", text(doc.Channel.TTL))
	fmt.Printf("link: %s\n", text(doc.Channel.Link))
	fmt.Printf("items: %d\n", len(doc.Channel.Items))

	problems := problemsFor(&doc)

	if len(doc.Channel.Items) > 0 {
		first := doc.Channel.Items[0]
		fmt.Println("\nfirst item:")
		fmt.Printf("  title: %s\n", head(text(first.Title), 60))
		fmt.Printf("  link : %s\n", text(first.Link))
		fmt.Printf("  date : %s\n", text(first.PubDate))
		fmt.Printf("  desc : %s\n", head(strings.ReplaceAll(text(first.Description), "\n", " "), 120))
	}

	empties := 0
	for _, it := range doc.Channel.Items {
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

func text(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// problemsFor returns every field-sanity violation in doc, in document order.
func problemsFor(doc *document) []string {
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

func head(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n])
}
