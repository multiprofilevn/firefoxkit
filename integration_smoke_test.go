//go:build integration

package firefoxkit_test

import (
	"os"
	"testing"
	"time"

	"github.com/tuwibu/firefoxkit"
)

// firefoxBinary returns the path from FIREFOXKIT_FIREFOX_PATH or skips the
// test. Mirrors chromekit's CHROMEKIT_CHROME_PATH gating pattern so CI runs
// only on hosts with Firefox installed.
func firefoxBinary(t *testing.T) string {
	p := os.Getenv("FIREFOXKIT_FIREFOX_PATH")
	if p == "" {
		t.Skip("FIREFOXKIT_FIREFOX_PATH not set; skipping integration")
	}
	if _, err := os.Stat(p); err != nil {
		t.Skipf("firefox binary missing at %s: %v", p, err)
	}
	return p
}

func TestIntegration_LaunchNavigateScreenshotClose(t *testing.T) {
	ff := firefoxBinary(t)
	b, err := firefoxkit.Launch(ff, firefoxkit.WithHeadless(true))
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	defer b.Close()

	p := b.Current()
	if p == nil {
		t.Fatal("Current returned nil")
	}

	if err := p.Navigate("about:blank", 30*time.Second); err != nil {
		t.Fatalf("Navigate about:blank: %v", err)
	}

	if err := p.WaitForReady(10 * time.Second); err != nil {
		t.Fatalf("WaitForReady: %v", err)
	}

	img, err := p.Screenshot()
	if err != nil {
		t.Fatalf("Screenshot: %v", err)
	}
	if len(img) < 100 {
		t.Errorf("screenshot suspiciously small: %d bytes", len(img))
	}
}

func TestIntegration_EvaluatePrimitives(t *testing.T) {
	ff := firefoxBinary(t)
	b, err := firefoxkit.Launch(ff, firefoxkit.WithHeadless(true))
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	defer b.Close()

	p := b.Current()

	if err := p.Navigate("about:blank", 30*time.Second); err != nil {
		t.Fatalf("Navigate: %v", err)
	}

	var sum float64
	if err := p.Evaluate("1 + 2", &sum); err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if sum != 3 {
		t.Errorf("got %v, want 3", sum)
	}

	var obj struct {
		A int    `json:"a"`
		B string `json:"b"`
	}
	if err := p.Evaluate(`({a: 42, b: "hi"})`, &obj); err != nil {
		t.Fatalf("Evaluate obj: %v", err)
	}
	if obj.A != 42 || obj.B != "hi" {
		t.Errorf("got %+v", obj)
	}
}

func TestIntegration_CookieRoundTrip(t *testing.T) {
	ff := firefoxBinary(t)
	b, err := firefoxkit.Launch(ff, firefoxkit.WithHeadless(true))
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	defer b.Close()

	p := b.Current()

	// data: URLs don't accept cookies — use a real-ish origin.
	if err := p.Navigate("https://example.com", 30*time.Second); err != nil {
		t.Skipf("Navigate example.com failed (likely offline): %v", err)
	}

	if err := p.Cookies().Set(firefoxkit.Cookie{
		Name: "ffk_test", Value: "12345", Domain: "example.com", Path: "/",
	}); err != nil {
		t.Fatalf("Set cookie: %v", err)
	}
	cookies, err := p.Cookies().Get("https://example.com")
	if err != nil {
		t.Fatalf("Get cookies: %v", err)
	}
	var found *firefoxkit.Cookie
	for i := range cookies {
		if cookies[i].Name == "ffk_test" {
			found = &cookies[i]
			break
		}
	}
	if found == nil {
		t.Fatal("cookie ffk_test not found")
	}
	if found.Value != "12345" {
		t.Errorf("cookie value = %q, want 12345", found.Value)
	}
}
