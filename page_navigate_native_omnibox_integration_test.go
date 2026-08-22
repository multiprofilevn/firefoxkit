//go:build integration

package firefoxkit_test

// TestNavigateReal_FirefoxNativeOmnibox is manual-only, gated by the same
// FIREFOXKIT_FIREFOX_PATH convention as integration_smoke_test.go. It is not
// run by the default `go test ./...` or by CI — build/vet coverage for this
// file comes from `go vet -tags=integration ./...`; running it for real
// requires a visible (non-headless) Firefox window, since native input needs
// one, and drives the real OS keyboard.
//
//	FIREFOXKIT_FIREFOX_PATH=<path-to-firefox.exe> go test -tags=integration \
//	  -run TestNavigateReal_FirefoxNativeOmnibox -v .

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/multiprofilevn/firefoxkit"
)

func TestNavigateReal_FirefoxNativeOmnibox(t *testing.T) {
	ff := firefoxBinary(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html><body>probe</body></html>"))
	}))
	defer srv.Close()
	target := srv.URL + "/"

	b, err := firefoxkit.Launch(ff,
		firefoxkit.WithHeadless(false),
		firefoxkit.WithInputBackend(firefoxkit.BackendNative),
		firefoxkit.WithNativeWindow(100, 100, 1.0),
	)
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	defer b.Close()

	if err := b.Focus(t.Context()); err != nil {
		t.Logf("Focus: %v (input may miss on a headless CI box)", err)
	}

	p := b.Current()
	if err := p.Navigate(target, 30*time.Second); err != nil {
		t.Fatalf("Navigate: %v", err)
	}
	if err := p.WaitForReady(10 * time.Second); err != nil {
		t.Fatalf("WaitForReady: %v", err)
	}

	got, err := p.GetURL()
	if err != nil {
		t.Fatalf("GetURL: %v", err)
	}
	if got != target {
		t.Fatalf("landed on %q, want %q — address bar typing did not resolve correctly", got, target)
	}

	// document.referrer empty confirms this was a fresh address-bar
	// navigation, not a link click or script-driven redirect carrying a
	// referrer.
	var referrer string
	if err := p.Evaluate(`document.referrer`, &referrer); err != nil {
		t.Fatalf("Evaluate document.referrer: %v", err)
	}
	if referrer != "" {
		t.Fatalf("document.referrer = %q, want empty (address-bar navigation)", referrer)
	}
}
