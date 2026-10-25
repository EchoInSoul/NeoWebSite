package userinput

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
		if got := ValidEmail(tc.in); got != tc.valid {
			t.Fatalf("ValidEmail(%q) = %v, want %v", tc.in, got, tc.valid)
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
		if got := ValidNickname(tc.in); got != tc.valid {
			t.Fatalf("ValidNickname(%q) = %v, want %v", tc.in, got, tc.valid)
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
		if got := ValidPassword(tc.in, PasswordBaselineMinLength); got != tc.valid {
			t.Fatalf("ValidPassword(%q) = %v, want %v", tc.in, got, tc.valid)
		}
	}
}

func TestNormalizeEmailAndNickname(t *testing.T) {
	if got := NormalizeEmail("  A@B.COM "); got != "a@b.com" {
		t.Fatalf("NormalizeEmail = %q, want a@b.com", got)
	}
	if got := NormalizeNickname("  ab  "); got != "ab" {
		t.Fatalf("NormalizeNickname = %q, want ab", got)
	}
}

func TestValidPasswordMinLength(t *testing.T) {
	cases := []struct {
		password  string
		minLength int
		want      bool
	}{
		{"pass1234", 8, true},
		{"pass1234", 9, false},
		{"pass12345678", 11, true},
		{"pass12345678", 12, true},
		{"pass12345678", 13, false},
	}
	for _, tc := range cases {
		if got := ValidPassword(tc.password, tc.minLength); got != tc.want {
			t.Fatalf("ValidPassword(%q, %d) = %v, want %v", tc.password, tc.minLength, got, tc.want)
		}
	}
}

func TestEmailDomainAllowed(t *testing.T) {
	allowed := []string{"example.com", "sub.example.org"}
	cases := []struct {
		email string
		want  bool
	}{
		{"a@example.com", true},
		{"a@EXAMPLE.COM", true},
		{"a@sub.example.com", false},
		{"a@example.org", false},
		{"a@sub.example.org", true},
		{"no-at", false},
	}
	for _, tc := range cases {
		if got := EmailDomainAllowed(tc.email, allowed); got != tc.want {
			t.Fatalf("EmailDomainAllowed(%q) = %v, want %v", tc.email, got, tc.want)
		}
	}
}

func TestEffectivePasswordMinLength(t *testing.T) {
	cases := []struct {
		enabled    bool
		configured int64
		want       int
	}{
		{false, 0, 8},
		{false, 16, 8},
		{true, 3, 8},
		{true, 7, 8},
		{true, 8, 8},
		{true, 16, 16},
		{true, 64, 64},
		{true, 100, 64},
	}
	for _, tc := range cases {
		if got := EffectivePasswordMinLength(tc.enabled, tc.configured); got != tc.want {
			t.Fatalf("EffectivePasswordMinLength(%v, %d) = %d, want %d", tc.enabled, tc.configured, got, tc.want)
		}
	}
}
