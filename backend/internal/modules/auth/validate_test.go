package auth

import (
	"strings"
	"testing"
)

func TestValidEmail(t *testing.T) {
	cases := []struct {
		in    string
		valid bool
	}{
		{"a@b.com", true},
		{"a@b", true},
		{"", false},
		{"no-at", false},
		{"a@@b", false},
		{"Bob <bob@example.com>", false},
		{"bob@example.com (comment)", false},
		{strings.Repeat("a", 250) + "@b.com", false},
	}
	for _, tc := range cases {
		if got := validEmail(tc.in); got != tc.valid {
			t.Fatalf("validEmail(%q) = %v, want %v", tc.in, got, tc.valid)
		}
	}
}

func TestValidNickname(t *testing.T) {
	cases := []struct {
		in    string
		valid bool
	}{
		{"ab", true},
		{"中文名", true},
		{"a", false},
		{strings.Repeat("字", 33), false},
		{"a b", false},
		{"a\u3000b", false},
		{"a@b", false},
	}
	for _, tc := range cases {
		if got := validNickname(tc.in); got != tc.valid {
			t.Fatalf("validNickname(%q) = %v, want %v", tc.in, got, tc.valid)
		}
	}
}

func TestValidPassword(t *testing.T) {
	cases := []struct {
		in    string
		valid bool
	}{
		{"pass1234", true},
		{strings.Repeat("a", 71) + "1", true},
		{"short1", false},
		{"abcdefgh", false},
		{"12345678", false},
		{strings.Repeat("a", 72) + "1", false},
		{"密码密码密码密码1", false},
	}
	for _, tc := range cases {
		if got := validPassword(tc.in); got != tc.valid {
			t.Fatalf("validPassword(%q) = %v, want %v", tc.in, got, tc.valid)
		}
	}
}

func TestNormalizeEmailAndNickname(t *testing.T) {
	if got := normalizeEmail("  A@B.COM "); got != "a@b.com" {
		t.Fatalf("normalizeEmail = %q, want a@b.com", got)
	}
	if got := normalizeNickname("  ab  "); got != "ab" {
		t.Fatalf("normalizeNickname = %q, want ab", got)
	}
}
