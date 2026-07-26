package firefoxkit

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
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

// mockInterceptIDs scripts network.addIntercept with a unique id per call so
// install/remove bookkeeping can be told apart across cycles.
func mockInterceptIDs(m *mockBiDi) {
	var n atomic.Int64
	m.handle("network.addIntercept", func(json.RawMessage) (any, string) {
		return map[string]any{"intercept": fmt.Sprintf("iid-%d", n.Add(1))}, ""
	})
}

// headerState reports the page's committed header count and whether an
// intercept is currently installed.
func headerState(p *Page) (installed bool, committed int) {
	hi := p.hdr
	if hi == nil {
		return false, 0
	}
	hi.mu.RLock()
	defer hi.mu.RUnlock()
	return hi.interceptID != "", len(hi.headers)
}

// TestSetExtraHeaders_SetRacesRemove locks the convergence invariant: once a
// set/remove pair quiesces, an intercept is installed if and only if headers
// are committed. Deciding the branch from the argument instead of the committed
// map lets a remove that loses the race leave an interceptor pinned with an
// empty header set — every request then pays intercept + continueRequest for
// the page's life. Run with -race.
func TestSetExtraHeaders_SetRacesRemove(t *testing.T) {
	_, p, m := newMockBrowser(t)
	mockInterceptIDs(m)

	// Enough concurrent callers that commit order and installMu order
	// decorrelate — a caller can commit early and still run its install last.
	for round := 0; round < 50; round++ {
		var wg sync.WaitGroup
		for i := 0; i < 8; i++ {
			wg.Add(2)
			go func() { defer wg.Done(); _ = p.SetExtraHeaders(map[string]string{"X-Test": "1"}) }()
			go func() { defer wg.Done(); _ = p.SetExtraHeaders(nil) }()
		}
		wg.Wait()

		installed, committed := headerState(p)
		if installed != (committed > 0) {
			t.Fatalf("round %d: intercept installed=%v with %d committed headers", round, installed, committed)
		}
	}

	if err := p.SetExtraHeaders(nil); err != nil {
		t.Fatalf("final SetExtraHeaders(nil): %v", err)
	}
	if add, rm := m.callCount("network.addIntercept"), m.callCount("network.removeIntercept"); add != rm {
		t.Fatalf("network.addIntercept=%d removeIntercept=%d — intercept left installed", add, rm)
	}
	if installed, committed := headerState(p); installed || committed != 0 {
		t.Fatalf("after remove: installed=%v committed=%d, want false/0", installed, committed)
	}
}

// TestSetExtraHeaders_InstallRemoveCycles_NoSubscriptionLeak locks teardown
// symmetry: each cycle pairs session.subscribe with session.unsubscribe and
// leaves the tracked server-event list empty, so toggling headers can't
// accumulate consumer-less event streams or duplicate tracked names.
func TestSetExtraHeaders_InstallRemoveCycles_NoSubscriptionLeak(t *testing.T) {
	_, p, m := newMockBrowser(t)
	mockInterceptIDs(m)

	trackedEvents := func() int {
		p.subsMu.Lock()
		defer p.subsMu.Unlock()
		return len(p.serverEvents)
	}

	const cycles = 5
	for i := 0; i < cycles; i++ {
		if err := p.SetExtraHeaders(map[string]string{"X-Test": "1"}); err != nil {
			t.Fatalf("cycle %d install: %v", i, err)
		}
		if got := trackedEvents(); got != 1 {
			t.Fatalf("cycle %d after install: tracked server events = %d, want 1", i, got)
		}
		if err := p.SetExtraHeaders(nil); err != nil {
			t.Fatalf("cycle %d remove: %v", i, err)
		}
		if got := trackedEvents(); got != 0 {
			t.Fatalf("cycle %d after remove: tracked server events = %d, want 0", i, got)
		}
	}

	if got := m.callCount("session.subscribe"); got != cycles {
		t.Fatalf("session.subscribe = %d, want %d", got, cycles)
	}
	if got := m.callCount("session.unsubscribe"); got != cycles {
		t.Fatalf("session.unsubscribe = %d, want %d", got, cycles)
	}
	if got := m.callCount("network.addIntercept"); got != cycles {
		t.Fatalf("network.addIntercept = %d, want %d", got, cycles)
	}
	if got := m.callCount("network.removeIntercept"); got != cycles {
		t.Fatalf("network.removeIntercept = %d, want %d", got, cycles)
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
