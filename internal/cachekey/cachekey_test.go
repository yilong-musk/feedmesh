package cachekey

import "testing"

func TestKeyAndStem(t *testing.T) {
	if got := Douyin.Key("user", "abc"); got != "douyin:user:abc" {
		t.Fatalf("Key = %q", got)
	}
	if !Hot.Owns("hot:top") || Hot.Owns("hotel:1") {
		t.Fatal("Owns must match the namespace segment only")
	}
	if Polymarket.StemPrefix() != "polymarket_" {
		t.Fatalf("StemPrefix = %q", Polymarket.StemPrefix())
	}
}

func TestNamespacesAreUniqueAndRegistered(t *testing.T) {
	seen := map[Namespace]bool{}
	for _, n := range All() {
		if seen[n] {
			t.Fatalf("namespace %q registered twice", n)
		}
		seen[n] = true
	}
	if !seen[Polymarket] || !seen[Feed] {
		t.Fatal("All() misses a declared namespace")
	}
}
