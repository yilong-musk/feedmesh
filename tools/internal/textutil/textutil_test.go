package textutil

import (
	"strings"
	"testing"
)

func TestHead(t *testing.T) {
	tests := []struct {
		name string
		in   string
		n    int
		want string
	}{
		{"shorter than n", "abc", 5, "abc"},
		{"exact length", "abc", 3, "abc"},
		{"cuts ASCII", "abcdef", 3, "abc"},
		{"cuts on rune boundaries", strings.Repeat("\u4e2d", 5), 2, strings.Repeat("\u4e2d", 2)},
		{"empty input", "", 10, ""},
		{"zero keeps nothing", "abc", 0, ""},
		{"negative keeps nothing", "abc", -1, ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Head(tc.in, tc.n); got != tc.want {
				t.Fatalf("Head(%q, %d) = %q, want %q", tc.in, tc.n, got, tc.want)
			}
		})
	}
}
