//go:build windows

package firefoxkit

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
)

// 3. TestNavigate_SchemeHandling: Firefox's address bar is a plain text
// field (see the Navigate doc comment in page_navigate.go for the empirical
// finding) — every form types VERBATIM, no stripScheme equivalent.
func TestNavigate_SchemeHandling(t *testing.T) {
	cases := []string{
		"https://x.com",
		"http://x.com",
		"x.com",
		"localhost:3000",
		"file:///C:/tmp/probe.html",
	}
	for _, url := range cases {
		t.Run(url, func(t *testing.T) {
			p, rec, _ := newNativeMockPage(t)
			if err := p.Navigate(url, 5*time.Second); err != nil {
				t.Fatalf("Navigate(%q): %v", url, err)
			}
			var typed []rune
			for _, ev := range rec.snapshot() {
				if r, ok := charEventRune(ev); ok {
					typed = append(typed, r)
				}
			}
			if string(typed) != url {
				t.Fatalf("typed %q, want verbatim %q (no scheme stripping)", string(typed), url)
			}
		})
	}
}

// 4. TestNavigate_TimeoutContractMatchesRemote: a page that never finishes
// loading times out with the SAME error type on both paths, within a
// comparable window — native's post-Enter wait (waitNavCommitted, built on
// pollUntil) and remote's server-side BiDi wait must not drift apart.
func TestNavigate_TimeoutContractMatchesRemote(t *testing.T) {
	const budget = 400 * time.Millisecond

	_, remoteP, remoteM := newMockBrowser(t)
	block := make(chan struct{})
	t.Cleanup(func() { close(block) })
	remoteM.handle("browsingContext.navigate", func(json.RawMessage) (any, string) {
		<-block // never responds within the caller's deadline
		return map[string]any{}, ""
	})
	start := time.Now()
	remoteErr := remoteP.Navigate("http://a.io/z", budget)
	remoteElapsed := time.Since(start)

	nativeP, _, nativeM := newNativeMockPage(t)
	nativeM.handle("script.evaluate", neverCompleteEvalHandler)
	start = time.Now()
	nativeErr := nativeP.Navigate("http://a.io/z", budget)
	nativeElapsed := time.Since(start)

	if !errors.Is(remoteErr, ErrTimeout) {
		t.Fatalf("remote error = %v, want ErrTimeout", remoteErr)
	}
	if !errors.Is(nativeErr, ErrTimeout) {
		t.Fatalf("native error = %v, want ErrTimeout", nativeErr)
	}
	// Same ballpark, not exact — both bounded by budget, generous slack for
	// scheduling jitter under -race.
	for _, d := range []time.Duration{remoteElapsed, nativeElapsed} {
		if d < budget/2 || d > budget*5 {
			t.Fatalf("elapsed %v outside expected window around budget %v (remote=%v native=%v)", d, budget, remoteElapsed, nativeElapsed)
		}
	}
}
