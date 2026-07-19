package firefoxkit

import "testing"

// TestBidiSessionURL verifies the stderr-printed base URL ("ws://host:port",
// no path) is normalized to the actual BiDi WebSocket endpoint. Dialing the
// bare base returns HTTP 200 instead of a WS 101 upgrade — the "/session" path
// is required for the handshake. Regression guard for Firefox 129+ launch.
func TestBidiSessionURL(t *testing.T) {
	cases := map[string]string{
		"ws://127.0.0.1:9333":          "ws://127.0.0.1:9333/session",
		"ws://127.0.0.1:9333/":         "ws://127.0.0.1:9333/session",
		"ws://127.0.0.1:9333/session":  "ws://127.0.0.1:9333/session",
		"ws://127.0.0.1:9333/session/": "ws://127.0.0.1:9333/session",
	}
	for in, want := range cases {
		if got := bidiSessionURL(in); got != want {
			t.Errorf("bidiSessionURL(%q) = %q, want %q", in, got, want)
		}
	}
}
