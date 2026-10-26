package siteurl

import "testing"

func TestNormalizePublicOrigin(t *testing.T) {
	for input, want := range map[string]string{
		" shop.huihuahui.xyz/ ":     "https://shop.huihuahui.xyz",
		"//shop.huihuahui.xyz":      "https://shop.huihuahui.xyz",
		"http://SHOP.HUIHUAHUI.XYZ": "https://shop.huihuahui.xyz",
		"http://localhost:5173/":    "http://localhost:5173",
		"http://127.0.0.1:8080":     "http://127.0.0.1:8080",
		"https://example.com:443/":  "https://example.com",
		"http://example.com:80":     "https://example.com",
		"http://[::1]:8080":         "http://[::1]:8080",
	} {
		got, err := Normalize(input)
		if err != nil || got != want {
			t.Errorf("Normalize(%q) = %q, %v; want %q", input, got, err, want)
		}
	}
	for _, input := range []string{"", "https://user:secret@example.com", "https://example.com/path", "https://example.com?x=1", "https://example.com/#section", "https://example.com/#", "https://example.com:99999", "javascript:alert(1)", "https://"} {
		if got, err := Normalize(input); err == nil {
			t.Errorf("accepted invalid origin %q as %q", input, got)
		}
	}
}
