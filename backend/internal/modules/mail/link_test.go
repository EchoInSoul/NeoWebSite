package mail

import "testing"

func TestSiteURL(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"https://example.com", "https://example.com", true},
		{"https://example.com/", "https://example.com", true},
		{"https://example.com/blog/", "https://example.com/blog", true},
		{"http://localhost:3000", "http://localhost:3000", true},
		{"", "", false},
		{"example.com", "", false},
		{"ftp://example.com", "", false},
		{"javascript:alert(1)", "", false},
		{"https://", "", false},
		{"http://:8080", "", false},
		{"https://example.com/?a=1", "", false},
		{"https://example.com/#x", "", false},
	}
	for _, c := range cases {
		got, err := SiteURL(c.in)
		if (err == nil) != c.ok || got != c.want {
			t.Fatalf("SiteURL(%q) = (%q, %v), want (%q, ok=%v)", c.in, got, err, c.want, c.ok)
		}
	}
}
