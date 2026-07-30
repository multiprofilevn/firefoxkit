//go:build windows

package firefoxkit

import (
	"errors"
	"testing"
	"time"
)

// 1. TestNavigate_NativeUsesKeyboard: native backend types into the address
// bar instead of calling browsingContext.navigate.
func TestNavigate_NativeUsesKeyboard(t *testing.T) {
	p, rec, m := newNativeMockPage(t)

	// Kept short deliberately: Keyboard.Type sleeps 50-150ms per rune (the
	// human-cadence typing chromekit's omnibox path also uses), so a longer
	// URL would need a proportionally longer timeout budget here.
	const url = "http://a.io/z"
	if err := p.Navigate(url, 5*time.Second); err != nil {
		t.Fatalf("Navigate: %v", err)
	}

	events := rec.snapshot()
	want := []string{"combo:Ctrl+L", "combo:Ctrl+A"}
	for _, ch := range url {
		want = append(want, "char:"+string(ch))
	}
	want = append(want, "combo:End", "combo:Enter")

	if len(events) != len(want) {
		t.Fatalf("event count = %d, want %d\ngot:  %v\nwant: %v", len(events), len(want), events, want)
	}
	for i := range want {
		if events[i] != want[i] {
			t.Fatalf("event[%d] = %q, want %q\ngot:  %v\nwant: %v", i, events[i], want[i], events, want)
		}
	}
	if n := m.callCount("browsingContext.navigate"); n != 0 {
		t.Fatalf("browsingContext.navigate called %d times, want 0", n)
	}
}

// 2. TestNavigate_RemoteUnchanged: non-native backend still calls
// browsingContext.navigate and emits zero keystrokes. Regression gate on the
// pre-phase-07 path.
func TestNavigate_RemoteUnchanged(t *testing.T) {
	b, p, m := newMockBrowser(t)
	if b.cfg.inputBackend == BackendNative {
		t.Fatal("test setup: default backend must not be native")
	}
	rec := &navKeyRecorder{}
	p.bkCache.bk = rec

	if err := p.Navigate("http://127.0.0.1:9/x", 2*time.Second); err != nil {
		t.Fatalf("Navigate: %v", err)
	}

	if n := m.callCount("browsingContext.navigate"); n != 1 {
		t.Fatalf("browsingContext.navigate called %d times, want 1", n)
	}
	if events := rec.snapshot(); len(events) != 0 {
		t.Fatalf("remote path emitted keystrokes: %v", events)
	}
}

// 5. TestNavigate_NativeRespectsForegroundGuard: the guard tripping mid-type
// aborts the sequence with ErrForegroundLost and never reaches Enter.
func TestNavigate_NativeRespectsForegroundGuard(t *testing.T) {
	p, rec, _ := newNativeMockPage(t)
	// Events: 1=Ctrl+L, 2=Ctrl+A, 3=first char, 4=second char -> fail here.
	rec.failOnEvent = 4

	err := p.Navigate("example.com", 2*time.Second)
	if !errors.Is(err, ErrForegroundLost) {
		t.Fatalf("Navigate error = %v, want ErrForegroundLost", err)
	}

	events := rec.snapshot()
	for _, ev := range events {
		if ev == "combo:End" || ev == "combo:Enter" {
			t.Fatalf("End/Enter sent after foreground loss: %v", events)
		}
	}
	if len(events) != 4 {
		t.Fatalf("event count = %d, want exactly 4 (stop at the failing event): %v", len(events), events)
	}
}
