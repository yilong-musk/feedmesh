package feed

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/yilong-musk/feedmesh/internal/cachekey"
)

// Cache keys are built through internal/cachekey so the cache manages (GC,
// stats, purge) every namespace a provider writes. A raw "douyin:…" literal
// would bypass that list again; this test keeps it from coming back.
func TestCacheKeysAreBuiltThroughTheRegistry(t *testing.T) {
	names := make([]string, 0, len(cachekey.All()))
	for _, n := range cachekey.All() {
		names = append(names, regexp.QuoteMeta(string(n)))
	}
	// A string literal that starts with a namespace followed by ':' or '_'
	// and then a key part or the closing quote ("x:", "hot:top", "feed_").
	raw := regexp.MustCompile(`"(` + strings.Join(names, "|") + `)[:_]([A-Za-z0-9]|")`)

	for _, dir := range []string{".", filepath.Join("..", "..", "cmd", "feedmesh")} {
		files, err := filepath.Glob(filepath.Join(dir, "*.go"))
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range files {
			if strings.HasSuffix(f, "_test.go") {
				continue
			}
			src, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			for i, line := range strings.Split(string(src), "\n") {
				if m := raw.FindString(line); m != "" {
					t.Errorf("%s:%d builds a cache key from the literal %s; use internal/cachekey", f, i+1, m)
				}
			}
		}
	}
}
