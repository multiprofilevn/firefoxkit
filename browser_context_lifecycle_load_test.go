package firefoxkit

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
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

// TestContextDestroyed_DrainerSaturation_DropsCleanup: once the drainer is
// saturated — every worker wedged on a slow cleanup RPC and the queue full —
// the subscription worker must keep detaching pages at event speed and DROP
// the cleanup. Running it inline instead stalls event dispatch for a full
// cleanup timeout per event, which overflows the 256-slot event queue: the
// contextDestroyed events lost there are what leaks b.pages for good.
func TestContextDestroyed_DrainerSaturation_DropsCleanup(t *testing.T) {
	b, p1, m := newMockBrowser(t)
	p1.trackServerEvent("log.entryAdded")

	// More destroyed contexts than the queue plus the in-flight ones the
	// workers hold, so the tail is guaranteed to hit the drop path.
	const flood = cleanupChCap + cleanupWorkers + 128
	ids := []string{p1.contextID}
	for i := 2; i <= flood; i++ {
		id := fmt.Sprintf("ctx-%d", i)
		addMockPage(b, id).trackServerEvent("log.entryAdded")
		ids = append(ids, id)
	}
	// Destroyed last, so its cleanup is guaranteed to take the drop path. Its
	// local handler is the one the drop must still release: the RPCs are
	// expendable, a handler left in the dispatcher pins the page forever.
	probe := addMockPage(b, "ctx-probe")
	probe.trackServerEvent("log.entryAdded")
	var probeHits atomic.Int64
	probe.trackSub(b.client.Subscribe("log.entryAdded", func(json.RawMessage) { probeHits.Add(1) }))

	release := gateUnsubscribe(m)
	defer release()

	if err := b.watchContextLifecycle(context.Background()); err != nil {
		t.Fatalf("watchContextLifecycle: %v", err)
	}
	for i, id := range ids {
		m.injectEvent(t, "browsingContext.contextDestroyed", map[string]any{"context": id})
		// Paced so this test exercises drainer saturation only — the event
		// queue's own drop-on-full path is a separate concern and would make
		// the leak assertion below ambiguous.
		if i%32 == 31 {
			time.Sleep(2 * time.Millisecond)
		}
	}

	if !pollUntilTrue(t, 3*time.Second, func() bool { return len(b.Pages()) == 1 }) {
		t.Fatalf("pages = %d, want 1 (only the probe) — detach stalled behind saturated cleanup", len(b.Pages()))
	}
	if got := b.cleanupDropped.Load(); got == 0 {
		t.Fatal("cleanupDropped = 0, want > 0 — saturation path never taken, test is not proving anything")
	}

	// Control: the probe's local handler is live while it is still tracked.
	m.injectEvent(t, "log.entryAdded", map[string]any{})
	if !pollUntilTrue(t, 2*time.Second, func() bool { return probeHits.Load() > 0 }) {
		t.Fatal("probe handler never fired — harness broken, the drop assertion below would pass for free")
	}

	// The worker must still be responsive right after the flood.
	dropsBefore := b.cleanupDropped.Load()
	m.injectEvent(t, "browsingContext.contextDestroyed", map[string]any{"context": probe.contextID})
	if !pollUntilTrue(t, 500*time.Millisecond, func() bool { return len(b.Pages()) == 0 }) {
		t.Fatal("subscription worker unresponsive after saturation flood")
	}
	if b.cleanupDropped.Load() == dropsBefore {
		t.Fatal("probe cleanup was queued, not dropped — assertions below no longer cover the drop path")
	}

	// Dropping the RPCs must not drop the local teardown: the page is claimed
	// and its handler is out of the dispatcher.
	if !probe.cleanedUp.Load() {
		t.Fatal("dropped page not marked cleaned up — a later cleanup would double-run")
	}
	probe.subsMu.Lock()
	nsubs := len(probe.subs)
	probe.subsMu.Unlock()
	if nsubs != 0 {
		t.Fatalf("dropped page still tracks %d local subscription(s)", nsubs)
	}
	hitsAtDrop := probeHits.Load()
	m.injectEvent(t, "log.entryAdded", map[string]any{})
	time.Sleep(200 * time.Millisecond)
	if got := probeHits.Load(); got != hitsAtDrop {
		t.Fatalf("handler of dropped page still registered: fired %d more time(s)", got-hitsAtDrop)
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
// within a bounded window even while a cleanup RPC is gated, and goroutines
// must return to baseline afterwards. The gate is released only AFTER the
// workers are observed joined, so the join is provably driven by Close, not by
// the RPC completing. Worker cleanups run under Background+cleanupTimeout (a
// page picked up during Close must still reach a healthy peer), so the join is
// bounded by the graceful budget plus the client teardown / cleanup timeout
// that error the gated call out — not by base-ctx cancellation. (The gate also
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

	// Drainer workers must join within the bounded window while the cleanup
	// RPC is STILL gated.
	drainerJoined := make(chan struct{})
	go func() { b.cleanupWG.Wait(); close(drainerJoined) }()
	joinLimit := gracefulCloseBudget + cleanupTimeout + time.Second
	select {
	case <-drainerJoined:
	case <-time.After(joinLimit):
		t.Fatalf("drainer workers not joined within %v on Close while cleanup RPC gated", joinLimit)
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
