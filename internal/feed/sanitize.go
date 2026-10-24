package feed

import (
	stdhtml "html"
	"net/url"
	"strings"
)

// The sanitizer is deliberately conservative: provider HTML is untrusted input
// and may come from third-party mirrors or user-generated posts. Unknown tags
// are dropped (their text is kept), dangerous raw-text elements are dropped
// with their content, and URLs are resolved against the provider origin and
// restricted to http/https.

var allowedTags = map[string]bool{
	"a": true, "b": true, "blockquote": true, "br": true, "code": true,
	"del": true, "div": true, "em": true, "figcaption": true, "figure": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
	"hr": true, "i": true, "img": true, "ins": true, "li": true, "ol": true,
	"p": true, "pre": true, "s": true, "source": true, "span": true,
	"strong": true, "table": true, "tbody": true, "td": true, "th": true,
	"thead": true, "tr": true, "u": true, "ul": true, "video": true,
}

var voidTags = map[string]bool{
	"br": true, "embed": true, "hr": true, "img": true, "source": true,
}

// rawTextTags are dropped together with their contents. Keeping script/style
// text as visible text is not useful, and dropping the contents also avoids
// leaving a half-parsed tag behind.
var rawTextTags = map[string]bool{
	"script": true, "style": true, "iframe": true, "object": true,
	"embed": true, "svg": true, "math": true, "template": true, "noscript": true,
}

// NormalizeHTML sanitises a fragment without a base URL. Relative links are
// dropped because they would resolve against the feed reader, not the origin.
func NormalizeHTML(s string) string { return NormalizeHTMLBase(s, "") }

// NormalizeHTMLBase normalises a provider HTML fragment. Relative links and
// images are resolved against base when it is non-empty; only http/https URLs
// survive.
func NormalizeHTMLBase(s, base string) string {
	if s == "" {
		return ""
	}
	s = emojiSpan.ReplaceAllStringFunc(s, func(block string) string {
		if m := altAttr.FindStringSubmatch(block); len(m) == 2 {
			return m[1]
		}
		return ""
	})
	s = brTag.ReplaceAllString(s, "<br />")

	var baseURL *url.URL
	if base != "" {
		if u, err := url.Parse(base); err == nil && u.Scheme != "" && u.Host != "" {
			baseURL = u
		}
	}
	return strings.TrimSpace(sanitizeHTML(s, baseURL))
}

type htmlAttr struct {
	name  string
	value string
}

func sanitizeHTML(s string, base *url.URL) string {
	var b strings.Builder
	b.Grow(len(s))

	for i := 0; i < len(s); {
		if s[i] != '<' {
			j := strings.IndexByte(s[i:], '<')
			if j < 0 {
				b.WriteString(escapeHTMLText(s[i:]))
				break
			}
			b.WriteString(escapeHTMLText(s[i : i+j]))
			i += j
			continue
		}

		// Comments, doctype and processing instructions are discarded.
		if strings.HasPrefix(s[i:], "<!--") {
			end := strings.Index(s[i+4:], "-->")
			if end < 0 {
				break
			}
			i += 4 + end + 3
			continue
		}
		if i+1 < len(s) && (s[i+1] == '!' || s[i+1] == '?') {
			end := strings.IndexByte(s[i:], '>')
			if end < 0 {
				break
			}
			i += end + 1
			continue
		}

		// Closing tag.
		if i+1 < len(s) && s[i+1] == '/' {
			name, j, ok := readTagName(s, i+2)
			if !ok {
				b.WriteString("&lt;")
				i++
				continue
			}
			end := strings.IndexByte(s[j:], '>')
			if end < 0 {
				break
			}
			i = j + end + 1
			name = strings.ToLower(name)
			if allowedTags[name] && !voidTags[name] && !rawTextTags[name] {
				b.WriteString("</")
				b.WriteString(name)
				b.WriteString(">")
			}
			continue
		}

		// Opening tag.
		name, j, ok := readTagName(s, i+1)
		if !ok {
			b.WriteString("&lt;")
			i++
			continue
		}
		name = strings.ToLower(name)
		attrs, selfClosing, next, ok := parseAttrs(s, j)
		if !ok {
			// A malformed tag is kept as escaped text; stopping here also
			// avoids rescanning the same unterminated suffix for every '<'.
			b.WriteString(escapeHTMLText(s[i:]))
			break
		}
		i = next

		if rawTextTags[name] {
			if !selfClosing && !voidTags[name] {
				i = skipRawText(s, i, name)
			}
			continue
		}
		if !allowedTags[name] {
			// Drop the tag but keep any inner text.
			continue
		}
		if name == "img" || name == "source" {
			hasSrc := false
			for _, attr := range attrs {
				if attr.name == "src" && safeURL(attr.value, base) != "" {
					hasSrc = true
					break
				}
			}
			if !hasSrc {
				continue
			}
		}

		b.WriteString("<")
		b.WriteString(name)
		href := ""
		for _, attr := range attrs {
			switch attr.name {
			case "href":
				if name == "a" {
					if v := safeURL(attr.value, base); v != "" {
						href = v
						b.WriteString(` href="`)
						b.WriteString(escapeHTMLAttr(v))
						b.WriteString(`"`)
					}
				}
			case "src":
				if name == "img" || name == "video" || name == "source" {
					if v := safeURL(attr.value, base); v != "" {
						b.WriteString(` src="`)
						b.WriteString(escapeHTMLAttr(v))
						b.WriteString(`"`)
					}
				}
			case "poster":
				if name == "video" {
					if v := safeURL(attr.value, base); v != "" {
						b.WriteString(` poster="`)
						b.WriteString(escapeHTMLAttr(v))
						b.WriteString(`"`)
					}
				}
			case "cite":
				if name == "blockquote" {
					if v := safeURL(attr.value, base); v != "" {
						b.WriteString(` cite="`)
						b.WriteString(escapeHTMLAttr(v))
						b.WriteString(`"`)
					}
				}
			case "alt", "title":
				if name == "img" || name == "a" || name == "video" {
					b.WriteString(" " + attr.name + `="`)
					b.WriteString(escapeHTMLAttr(attr.value))
					b.WriteString(`"`)
				}
			case "width", "height":
				if name == "img" || name == "video" {
					if v := sanitizeDimensionValue(attr.value); v != "" {
						b.WriteString(" " + attr.name + `="` + v + `"`)
					}
				}
			case "colspan", "rowspan":
				if name == "td" || name == "th" {
					if v := sanitizeDimensionValue(attr.value); v != "" {
						b.WriteString(" " + attr.name + `="` + v + `"`)
					}
				}
			case "loading":
				if name == "img" {
					v := strings.ToLower(strings.TrimSpace(attr.value))
					if v == "lazy" || v == "eager" {
						b.WriteString(` loading="` + v + `"`)
					}
				}
			case "preload":
				if name == "video" {
					v := strings.ToLower(strings.TrimSpace(attr.value))
					if v == "none" || v == "metadata" || v == "auto" {
						b.WriteString(` preload="` + v + `"`)
					}
				}
			case "controls":
				if name == "video" {
					b.WriteString(" controls")
				}
			case "type":
				if name == "source" {
					if v := sanitizeMIME(attr.value); v != "" {
						b.WriteString(` type="` + v + `"`)
					}
				}
			case "class":
				if name == "span" {
					if v := sanitizeClass(attr.value); v != "" {
						b.WriteString(` class="`)
						b.WriteString(escapeHTMLAttr(v))
						b.WriteString(`"`)
					}
				}
			}
		}
		if name == "a" && href != "" {
			b.WriteString(` rel="nofollow noopener noreferrer"`)
		}
		if voidTags[name] {
			b.WriteString(" />")
		} else {
			b.WriteString(">")
		}
	}
	return b.String()
}

func readTagName(s string, i int) (string, int, bool) {
	start := i
	if i >= len(s) || !isASCIILetter(s[i]) {
		return "", i, false
	}
	i++
	for i < len(s) {
		c := s[i]
		if isASCIILetter(c) || isASCIIDigit(c) || c == '-' || c == ':' {
			i++
			continue
		}
		break
	}
	return s[start:i], i, true
}

func parseAttrs(s string, i int) ([]htmlAttr, bool, int, bool) {
	var attrs []htmlAttr
	for i < len(s) {
		for i < len(s) && isHTMLSpace(s[i]) {
			i++
		}
		if i >= len(s) {
			return nil, false, i, false
		}
		if s[i] == '>' {
			return attrs, false, i + 1, true
		}
		if s[i] == '/' && i+1 < len(s) && s[i+1] == '>' {
			return attrs, true, i + 2, true
		}

		start := i
		for i < len(s) && !isHTMLSpace(s[i]) && s[i] != '=' && s[i] != '>' && s[i] != '/' {
			i++
		}
		if start == i {
			i++
			continue
		}
		name := strings.ToLower(s[start:i])
		for i < len(s) && isHTMLSpace(s[i]) {
			i++
		}
		value := ""
		if i < len(s) && s[i] == '=' {
			i++
			for i < len(s) && isHTMLSpace(s[i]) {
				i++
			}
			if i >= len(s) {
				return nil, false, i, false
			}
			if s[i] == '"' || s[i] == '\'' {
				quote := s[i]
				i++
				start = i
				for i < len(s) && s[i] != quote {
					i++
				}
				if i >= len(s) {
					return nil, false, i, false
				}
				value = s[start:i]
				i++
			} else {
				start = i
				for i < len(s) && !isHTMLSpace(s[i]) && s[i] != '>' {
					i++
				}
				value = s[start:i]
			}
		}
		attrs = append(attrs, htmlAttr{name: name, value: stdhtml.UnescapeString(value)})
	}
	return nil, false, i, false
}

func skipRawText(s string, i int, name string) int {
	needle := "</" + name
	for j := i; j+len(needle) <= len(s); j++ {
		if s[j] != '<' || !strings.EqualFold(s[j:j+len(needle)], needle) {
			continue
		}
		end := j + len(needle)
		if end < len(s) && s[end] != '>' && !isHTMLSpace(s[end]) {
			// A lookalike such as </scriptfoo>; keep scanning.
			continue
		}
		if end < len(s) {
			if gt := strings.IndexByte(s[end:], '>'); gt >= 0 {
				return end + gt + 1
			}
		}
		return len(s)
	}
	return len(s)
}

func escapeHTMLText(s string) string {
	return stdhtml.EscapeString(stripXMLControls(stdhtml.UnescapeString(s)))
}

func escapeHTMLAttr(s string) string {
	return stdhtml.EscapeString(stripXMLControls(stdhtml.UnescapeString(s)))
}

func sanitizeDimensionValue(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || len(s) > 6 {
		return ""
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return ""
		}
	}
	return s
}

func sanitizeClass(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || len(s) > 200 {
		return ""
	}
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '_' || r == '-':
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}

func sanitizeMIME(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || len(s) > 100 {
		return ""
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z') && !(r >= 'A' && r <= 'Z') &&
			!(r >= '0' && r <= '9') && r != '/' && r != '+' && r != '.' && r != '-' {
			return ""
		}
	}
	return s
}

func safeURLString(raw string) string { return safeURL(raw, nil) }

func resolveSafeURL(raw, base string) string {
	u, err := url.Parse(base)
	if err != nil {
		return safeURL(raw, nil)
	}
	return safeURL(raw, u)
}

// safeURL resolves raw against base and returns an absolute http/https URL.
// Everything else (javascript:, data:, relative when no base, userinfo tricks)
// is rejected.
func safeURL(raw string, base *url.URL) string {
	raw = strings.TrimSpace(stdhtml.UnescapeString(raw))
	if raw == "" {
		return ""
	}
	for _, r := range raw {
		if r < 0x20 || r == 0x7f {
			return ""
		}
	}
	u, err := url.Parse(raw)
	if err != nil || u.User != nil {
		return ""
	}
	if base != nil {
		u = base.ResolveReference(u)
	}
	u.Scheme = strings.ToLower(u.Scheme)
	if u.Scheme != "http" && u.Scheme != "https" {
		return ""
	}
	if u.Host == "" {
		return ""
	}
	return u.String()
}

func isASCIILetter(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isASCIIDigit(c byte) bool { return c >= '0' && c <= '9' }

func isHTMLSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f'
}

// xmlRuneOK reports whether r is allowed in XML 1.0 character data.
func xmlRuneOK(r rune) bool {
	switch {
	case r == '\t' || r == '\n' || r == '\r':
		return true
	case r >= 0x20 && r <= 0xD7FF:
		return true
	case r >= 0xE000 && r <= 0xFFFD:
		return true
	case r >= 0x10000 && r <= 0x10FFFF:
		return true
	default:
		return false
	}
}

// stripXMLControls removes characters that XML 1.0 cannot represent. It is
// applied to CDATA bodies as well as ordinary text and attributes.
func stripXMLControls(s string) string {
	clean := true
	for _, r := range s {
		if !xmlRuneOK(r) {
			clean = false
			break
		}
	}
	if clean {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if xmlRuneOK(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}
