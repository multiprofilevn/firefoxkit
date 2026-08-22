package firefoxkit

import (
	"testing"
	"time"

	"github.com/multiprofilevn/firefoxkit/internal/bidi"
)

func TestConvertCookies(t *testing.T) {
	in := []bidi.CookieInfo{
		{
			Name:   "session",
			Value:  bidi.BiDiBytes{Type: "string", Value: "abc123"},
			Domain: "example.com",
			Path:   "/",
			Secure: true, HTTPOnly: true, SameSite: "lax",
			Expiry: time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC).Unix(),
		},
		{
			Name:  "t",
			Value: bidi.BiDiBytes{Type: "string", Value: ""},
		},
	}
	got := convertCookies(in)
	if len(got) != 2 {
		t.Fatalf("len=%d", len(got))
	}
	if got[0].Name != "session" || got[0].Value != "abc123" || !got[0].Secure || got[0].SameSite != "lax" {
		t.Errorf("cookie[0]: %+v", got[0])
	}
	if got[0].Expires.Year() != 2030 {
		t.Errorf("cookie[0] expires not decoded: %v", got[0].Expires)
	}
	if !got[1].Expires.IsZero() {
		t.Errorf("cookie[1] should have zero expires, got %v", got[1].Expires)
	}
}

func TestContextPartition(t *testing.T) {
	p := bidi.ContextPartition("ctx-abc")
	if p.Type != "context" || p.Context != "ctx-abc" {
		t.Errorf("partition: %+v", p)
	}
}

func TestStringBytes(t *testing.T) {
	b := bidi.StringBytes("hello")
	if b.Type != "string" || b.Value != "hello" {
		t.Errorf("bytes: %+v", b)
	}
}

// TestNormalizeSameSite verifies case-insensitive input is canonicalized.
func TestNormalizeSameSite(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"Lax", "lax"},
		{"lax", "lax"},
		{"LAX", "lax"},
		{"Strict", "strict"},
		{"STRICT", "strict"},
		{"strict", "strict"},
		{"None", "none"},
		{"NONE", "none"},
		{"none", "none"},
		{"", ""},
		{"unknown", "unknown"}, // passthrough
	}
	for _, tc := range cases {
		got := normalizeSameSite(tc.in)
		if got != tc.want {
			t.Errorf("normalizeSameSite(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestDomainFromURL verifies URL → hostname extraction used in Get(urls...) filter.
// H2 fix: net/url-based extraction strips port and unwraps IPv6 brackets.
func TestDomainFromURL(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"https://example.com/path", "example.com"},
		{"http://sub.example.com", "sub.example.com"},
		{"https://example.com", "example.com"},
		{"example.com", "example.com"},           // no scheme passthrough
		{"", ""},                                  // empty passthrough
		{"http://localhost:3000/api", "localhost"}, // H2: port stripped
		{"https://host:8443", "host"},              // H2: port stripped, no path
		{"http://[::1]:9000/", "::1"},              // H2: IPv6 brackets unwrapped
	}
	for _, tc := range cases {
		got := domainFromURL(tc.in)
		if got != tc.want {
			t.Errorf("domainFromURL(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestCookieURL_FieldPresent verifies Cookie struct has URL field (compile-time check).
func TestCookieURL_FieldPresent(t *testing.T) {
	c := Cookie{
		Name:  "test",
		Value: "v",
		URL:   "https://example.com",
	}
	if c.URL != "https://example.com" {
		t.Errorf("Cookie.URL not set correctly: %v", c.URL)
	}
}
