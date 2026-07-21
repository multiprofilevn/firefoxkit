package firefoxkit

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"sync"
	"testing"
	"time"
)

// gateUnsubscribe scripts session.unsubscribe (the RPC page.cleanup issues for
// tracked server events) to block until the returned release func is called.
// Simulates slow/backed-up cleanup RPCs under heavy tab churn.
func gateUnsubscribe(m *mockBiDi) (release func()) {
	gate := make(chan struct{})
	m.handle("session.unsubscribe", func(json.RawMessage) (any, string) {
		<-gate
		return nil, ""
	})
	return sync.OnceFunc(func() { close(gate) })
}

// TestContextDestroyed_Flood_NoPageLeak: under a burst of contextDestroyed
// events with slow cleanup RPCs, EVERY corresponding page must still leave
// b.pages. With detach coupled to the blocking cleanup on the single
// subscription worker, the 256-cap event queue overflows, events are dropped,
// and the dropped pages leak forever — this test proves (and then locks) the
// fix: detach must not depend on cleanup speed.
func TestContextDestroyed_Flood_NoPageLeak(t *testing.T) {
	b, p1, m := newMockBrowser(t)
	p1.trackServerEvent("log.entryAdded")

	// 300 pages > 1 (in-flight) + 256 (queue cap) → guaranteed drops when the
	// subscription worker is wedged behind a gated cleanup RPC.
	const flood = 300
	ids := []string{p1.contextID}
	for i := 2; i <= flood; i++ {
		id := fmt.Sprintf("ctx-%d", i)
		p := addMockPage(b, id)
		p.trackServerEvent("log.entryAdded")
		ids = append(ids, id)
	}

	release := gateUnsubscribe(m)
	defer release()

	if err := b.watchContextLifecycle(context.Background()); err != nil {
		t.Fatalf("watchContextLifecycle: %v", err)
	}
	for _, id := range ids {
		m.injectEvent(t, "browsingContext.contextDestroyed", map[string]any{"context": id})
	}

	// Wait until the first cleanup RPC is in flight (gated), then give the
	// client reader time to dispatch — and, with the old coupled code, drop —
	// every remaining event.
	if !pollUntilTrue(t, 2*time.Second, func() bool { return m.callCount("session.unsubscribe") >= 1 }) {
		t.Fatal("no cleanup RPC observed — harness broken")
	}
	time.Sleep(300 * time.Millisecond)
	release()

	if !pollUntilTrue(t, 5*time.Second, func() bool { return len(b.Pages()) == 0 }) {
		t.Fatalf("pages leaked under contextDestroyed flood: %d remain, want 0", len(b.Pages()))
	}
}

// TestContextDestroyed_Detach_IsSynchronousAndCheap: a destroyed page must
// leave Pages() while its cleanup RPC is still blocked — detach must never
// wait on cleanup.
func TestContextDestroyed_Detach_IsSynchronousAndCheap(t *testing.T) {
	b, p1, m := newMockBrowser(t)
	p1.trackServerEvent("log.entryAdded")

	release := gateUnsubscribe(m)
	defer release()

	if err := b.watchContextLifecycle(context.Background()); err != nil {
		t.Fatalf("watchContextLifecycle: %v", err)
	}
	m.injectEvent(t, "browsingContext.contextDestroyed", map[string]any{"context": p1.contextID})

	// Gate is still closed here, so cleanup cannot have completed — yet the
	// page must already be detached.
	if !pollUntilTrue(t, 2*time.Second, func() bool { return len(b.Pages()) == 0 }) {
		t.Fatal("page not detached before cleanup completed")
	}
	release()
	// Cleanup still runs (best-effort) after detach.
	if !pollUntilTrue(t, 2*time.Second, func() bool { return m.callCount("session.unsubscribe") >= 1 }) {
		t.Fatal("cleanup RPC never issued for detached page")
	}
}

// TestContextDestroyed_UnknownContext_NoOpUnderDrainer: an event for an
// untracked context (child frames also emit contextDestroyed) must not panic
// or disturb the page list, with the cleanup drainer active.
func TestContextDestroyed_UnknownContext_NoOpUnderDrainer(t *testing.T) {
	b, _, m := newMockBrowser(t)
	if err := b.watchContextLifecycle(context.Background()); err != nil {
		t.Fatalf("watchContextLifecycle: %v", err)
	}
	m.injectEvent(t, "browsingContext.contextDestroyed", map[string]any{"context": "ctx-unknown"})
	time.Sleep(100 * time.Millisecond)
	if got := len(b.Pages()); got != 1 {
		t.Fatalf("pages = %d, want 1", got)
	}
}

// TestCleanupDrainer_StopsOnClose: Browser.Close must join the drainer workers
// even while a cleanup RPC is gated — base-ctx cancellation unblocks the
// in-flight call — and goroutines must return to baseline afterwards.
// The gate is released only AFTER the workers are observed joined, so the join
// is provably driven by Close, not by the RPC completing. (The gate also
// wedges the mock's single serve goroutine, which stalls the WS close
// handshake inside Browser.Close — a harness artifact — hence Close's overall
// return is asserted after release, separately from the drainer join.)
func TestCleanupDrainer_StopsOnClose(t *testing.T) {
	b, p1, m := newMockBrowser(t)
	p1.trackServerEvent("log.entryAdded")

	release := gateUnsubscribe(m)
	defer release() // unblock the mock serve goroutine at test end

	g0 := runtime.NumGoroutine()
	if err := b.watchContextLifecycle(context.Background()); err != nil {
		t.Fatalf("watchContextLifecycle: %v", err)
	}
	m.injectEvent(t, "browsingContext.contextDestroyed", map[string]any{"context": p1.contextID})
	if !pollUntilTrue(t, 2*time.Second, func() bool { return m.callCount("session.unsubscribe") >= 1 }) {
		t.Fatal("no cleanup RPC in flight")
	}

	closeDone := make(chan error, 1)
	go func() { closeDone <- b.Close() }()

	// Drainer workers must join promptly while the cleanup RPC is STILL gated.
	drainerJoined := make(chan struct{})
	go func() { b.cleanupWG.Wait(); close(drainerJoined) }()
	select {
	case <-drainerJoined:
	case <-time.After(2 * time.Second):
		t.Fatal("drainer workers not joined on Close while cleanup RPC gated")
	}

	release()
	select {
	case <-closeDone:
	case <-time.After(3 * time.Second):
		t.Fatal("Close did not return after gate release")
	}
	if !pollUntilTrue(t, 2*time.Second, func() bool { return runtime.NumGoroutine() <= g0 }) {
		t.Fatalf("goroutines = %d, want <= %d after Close (drainer leaked)", runtime.NumGoroutine(), g0)
	}
}
