package feed

import (
	"html"
	"regexp"
	"strings"
)

var (
	// emojiSpan matches weibo's image-emoji wrapper: <span class="url-icon">…</span>.
	emojiSpan = regexp.MustCompile(`(?s)<span[^>]*class="[^"]*\burl-icon\b[^"]*"[^>]*>.*?</span>`)
	altAttr   = regexp.MustCompile(`(?i)\balt="([^"]*)"`)
	anyTag    = regexp.MustCompile(`(?s)<[^>]*>`)
	wsRun     = regexp.MustCompile(`[\s\x{00a0}]+`)
	brTag     = regexp.MustCompile(`(?i)<br\s*/?>`)
)

// HTMLToText flattens post HTML to plain text.
func HTMLToText(s string) string {
	if s == "" {
		return ""
	}
	s = NormalizeHTML(s)
	s = anyTag.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	s = wsRun.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

// Truncate shortens s to at most n runes, appending an ellipsis when cut.
func Truncate(s string, n int) string {
	if n <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "…"
}

// ItemTitle derives the RSS title: weibo's own status title when present,
// otherwise the first 25 characters of the plain text.
func ItemTitle(statusTitle, plainText string) string {
	if t := strings.TrimSpace(statusTitle); t != "" {
		return strings.TrimSpace(wsRun.ReplaceAllString(t, " "))
	}
	text := strings.TrimSpace(wsRun.ReplaceAllString(plainText, " "))
	if text == "" {
		return "(no content)"
	}
	return Truncate(text, 25)
}

// EscapeXML escapes the five XML metacharacters.
func EscapeXML(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 16)
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '"':
			b.WriteString("&quot;")
		case '\'':
			b.WriteString("&apos;")
		default:
			// Drop characters XML 1.0 cannot represent.
			if !xmlRuneOK(r) {
				continue
			}
			b.WriteRune(r)
		}
	}
	return b.String()
}
