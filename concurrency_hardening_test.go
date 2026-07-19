package firefoxkit

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"
)

// timeoutC returns a channel firing after a short grace period for hang checks.
func timeoutC(t *testing.T) <-chan time.Time {
	t.Helper()
	return time.After(3 * time.Second)
}

// TestPageCleanup_ReleasesInput locks in M1: cleanup must issue
// input.releaseActions so a KeyDown without its Up can't leave sticky
// modifiers in the browsing context.
func TestPageCleanup_ReleasesInput(t *testing.T) {
	_, p, m := newMockBrowser(t)

	// Force the BiDi input backend to initialize (marks bkCache.ready).
	_ = p.inputBackend()

	p.cleanup(context.Background())

	if got := m.callCount("input.releaseActions"); got == 0 {
		t.Fatal("cleanup must call input.releaseActions to clear held keys/buttons")
	}
}

// TestPageCleanup_NoBackend_NoRelease: a page that never used input must not
// force-build a backend just to release it.
func TestPageCleanup_NoBackend_NoRelease(t *testing.T) {
	_, p, m := newMockBrowser(t)
	p.cleanup(context.Background())
	if got := m.callCount("input.releaseActions"); got != 0 {
		t.Fatalf("input.releaseActions called %d times for unused input", got)
	}
}

// TestSetExtraHeaders_ConcurrentInstall_SingleIntercept locks in M5: two
// concurrent SetExtraHeaders must install exactly one intercept (no orphaned
// interceptor that keeps injecting after removal).
func TestSetExtraHeaders_ConcurrentInstall_SingleIntercept(t *testing.T) {
	_, p, m := newMockBrowser(t)
	m.handle("network.addIntercept", func(json.RawMessage) (any, string) {
		return map[string]any{"intercept": "iid-1"}, ""
	})

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = p.SetExtraHeaders(map[string]string{"X-Test": "1"})
		}()
	}
	wg.Wait()

	if got := m.callCount("network.addIntercept"); got != 1 {
		t.Fatalf("network.addIntercept calls = %d, want 1 (concurrent install race)", got)
	}
}

// TestCleanup_RacesFirstSubscribe locks in review findings #2/#3: cleanup()
// running concurrently with a first OnConsole/OnDialog/SetExtraHeaders on the
// same page must not race or orphan a subscription. Run with -race.
func TestCleanup_RacesFirstSubscribe(t *testing.T) {
	for i := 0; i < 20; i++ {
		_, p, _ := newMockBrowser(t)
		var wg sync.WaitGroup
		wg.Add(4)
		go func() { defer wg.Done(); p.OnConsole(func(ConsoleEvent) {}) }()
		go func() { defer wg.Done(); p.OnDialog(func(DialogEvent) DialogAction { return DialogAction{} }) }()
		go func() { defer wg.Done(); _ = p.SetExtraHeaders(map[string]string{"X": "1"}) }()
		go func() { defer wg.Done(); p.cleanup(context.Background()) }()
		wg.Wait()
	}
}

// TestClient_CloseJoinsReader is covered in the bidi package
// (TestClose_WaitsWorkers); this asserts the root-level Browser.Close path
// stays deadlock-free with input + listeners active.
func TestBrowserClose_WithActiveListeners_NoHang(t *testing.T) {
	b, p, _ := newMockBrowser(t)
	p.OnConsole(func(ConsoleEvent) {})
	p.OnDialog(func(DialogEvent) DialogAction { return DialogAction{} })
	_ = p.inputBackend()

	done := make(chan struct{})
	go func() {
		_ = b.Close()
		close(done)
	}()
	select {
	case <-done:
	case <-timeoutC(t):
		t.Fatal("Browser.Close hung with active listeners")
	}
}
