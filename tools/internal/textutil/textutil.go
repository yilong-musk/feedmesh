// Package textutil holds the small text helpers shared by the repository
// tools.
package textutil

// Head returns at most n runes of s. Counting runes rather than bytes keeps
// the result valid UTF-8 and matches the character semantics of the check
// reports.
func Head(s string, n int) string {
	if n < 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n])
}
