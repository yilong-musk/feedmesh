package main

import (
	"reflect"
	"testing"
)

func TestOffending(t *testing.T) {
	tests := []struct {
		name string
		text string
		want []string
	}{
		{"plain English", "The service listens on loopback.", nil},
		{"CJK", "保暖内衣", []string{"CJK"}},
		{"Cyrillic", "Привет", []string{"Cyrillic"}},
		{"Kana", "テスト", []string{"Kana"}},
		{"Hangul", "안녕", []string{"Hangul"}},
		{"mixed scripts report each once", "中文 Привет 中文", []string{"CJK", "Cyrillic"}},
		{"punctuation and symbols are fine", "-> … — ©", nil},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := offending(tc.text); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("offending(%q) = %v, want %v", tc.text, got, tc.want)
			}
		})
	}
}

func TestSkipped(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"internal/feed/model.go", false},
		{"docs/API.md", false},
		{"README.md", false},
		{"internal/feed/smzdm_test.go", true},
		{"docs/assets/diagram.png", true},
		{"docs/assets/notes.md", true},
		{"LICENSE", true},
		{"README.zh-CN.md", true},
		{"docs/zh-CN/API.md", true},
		{"internal/i18n/zh-CN.json", true},
		{"docs/assetsx/notes.md", false},
	}

	for _, tc := range tests {
		if got := skipped(tc.path); got != tc.want {
			t.Errorf("skipped(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}
