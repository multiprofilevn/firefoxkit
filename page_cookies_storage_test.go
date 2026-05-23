package firefoxkit

import (
	"testing"
	"time"

	"github.com/tuwibu/firefoxkit/internal/bidi"
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
