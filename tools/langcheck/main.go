// Command langcheck fails when non-English scripts show up in authored text.
//
// AGENTS.md §1 requires every authored string in this repository to be
// English. This check scans tracked files for characters from the writing
// systems listed in ranges (Cyrillic, Hebrew, Arabic, Devanagari, Thai, Kana,
// CJK, Hangul) and reports them, apart from:
//
//   - test files (*_test.go), which reproduce upstream payloads byte for byte
//   - lines carrying an explicit i18n-ignore marker
//   - localized artifacts allowed by AGENTS.md §1.3
//   - binary assets and the license text
//
// Run it through `make lang`, which CI also executes.
package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/yilong-musk/feedmesh/tools/internal/textutil"
)

// scriptRange names a writing system that must not appear in authored text.
type scriptRange struct {
	lo, hi rune
	name   string
}

var ranges = []scriptRange{
	{0x0400, 0x04FF, "Cyrillic"},
	{0x0590, 0x05FF, "Hebrew"},
	{0x0600, 0x06FF, "Arabic"},
	{0x0900, 0x097F, "Devanagari"},
	{0x0E00, 0x0E7F, "Thai"},
	{0x3040, 0x30FF, "Kana"},
	{0x3400, 0x4DBF, "CJK"},
	{0x4E00, 0x9FFF, "CJK"},
	{0xAC00, 0xD7AF, "Hangul"},
	{0xF900, 0xFAFF, "CJK"},
}

const (
	marker    = "i18n-ignore"
	maxColumn = 120
)

var (
	skipSuffixes = []string{".png", ".jpg", ".jpeg", ".gif", ".webp", ".ico", ".pdf"}
	skipPaths    = []string{"docs/assets/"}
	skipFiles    = map[string]bool{"LICENSE": true}

	// Localized artifacts are allowed by AGENTS.md §1.3; they are separate
	// files, never edits of the English originals.
	localized = []*regexp.Regexp{
		regexp.MustCompile(`^docs/[a-z]{2,3}(?:-[A-Za-z0-9]{2,8})*/`),
		regexp.MustCompile(`^README\.[a-z]{2,3}(?:-[A-Za-z0-9]{2,8})*\.md$`),
		regexp.MustCompile(`^internal/i18n/[a-z]{2,3}(?:-[A-Za-z0-9]{2,8})*\.json$`),
	}
)

func main() {
	paths, err := trackedFiles()
	if err != nil {
		fmt.Fprintf(os.Stderr, "language check: %v\n", err)
		os.Exit(2)
	}

	hits := 0
	for _, path := range paths {
		if skipped(path) {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil || !utf8.Valid(data) {
			// Binary or unreadable files are not authored text.
			continue
		}
		for number, line := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") {
			line = strings.TrimSuffix(line, "\r")
			if strings.Contains(line, marker) {
				continue
			}
			names := offending(line)
			if len(names) == 0 {
				continue
			}
			hits++
			fmt.Printf("%s:%d: %s text: %s\n", path, number+1, strings.Join(names, "/"), textutil.Head(strings.TrimSpace(line), maxColumn))
		}
	}

	if hits > 0 {
		fmt.Fprintf(os.Stderr, "\n%d line(s) contain non-English scripts. See AGENTS.md §1: authored text stays English, translations live in separate localized files, and upstream data uses an i18n-ignore marker.\n", hits)
		os.Exit(1)
	}
	fmt.Println("language check: no non-English scripts in authored text")
}

// trackedFiles lists every file git knows about, NUL-separated so paths with
// spaces survive.
func trackedFiles() ([]string, error) {
	var out bytes.Buffer
	cmd := exec.Command("git", "ls-files", "-z")
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git ls-files: %w", err)
	}
	var paths []string
	for _, p := range strings.Split(out.String(), "\x00") {
		if p != "" {
			paths = append(paths, p)
		}
	}
	return paths, nil
}

// skipped reports whether a tracked path is exempt from the language gate.
func skipped(path string) bool {
	if skipFiles[path] {
		return true
	}
	for _, prefix := range skipPaths {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	for _, suffix := range skipSuffixes {
		if strings.HasSuffix(path, suffix) {
			return true
		}
	}
	for _, re := range localized {
		if re.MatchString(path) {
			return true
		}
	}
	// Upstream fixtures live inline in tests; they are data, not prose.
	return strings.HasSuffix(path, "_test.go")
}

// offending returns the sorted names of the writing systems found in text.
func offending(text string) []string {
	seen := map[string]bool{}
	for _, ch := range text {
		for _, r := range ranges {
			if ch >= r.lo && ch <= r.hi {
				seen[r.name] = true
				break
			}
		}
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	if len(names) == 0 {
		return nil
	}
	sort.Strings(names)
	return names
}
