package bidi

import (
	"context"
	"encoding/json"
	"runtime"
	"sync/atomic"
	"testing"
	"time"
)

func pollUntil(t *testing.T, timeout time.Duration, cond func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return cond()
}

// TestDispatch_HandlerCallsSend_NoDeadlock: H5 — a handler issuing an RPC must
// complete instead of deadlocking the reader (old code ran handlers inline on
// the readLoop, so Send could never see its response).
func TestDispatch_HandlerCallsSend_NoDeadlock(t *testing.T) {
	ft := newFakeTransport()
	ft.onWrite = func(cmd command) {
		ft.injectResponse(cmd.ID, map[string]any{"ok": true})
	}
	c := newClient(ft, nil)
	defer c.Close()

	var done atomic.Bool
	sub := c.Subscribe("test.event", func(json.RawMessage) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := c.Send(ctx, "test.fromHandler", nil, nil); err != nil {
			t.Errorf("Send from handler: %v", err)
		}
		done.Store(true)
	})
	defer sub.Unsubscribe()

	ft.injectEvent("test.event", map[string]any{})
	if !pollUntil(t, 2*time.Second, done.Load) {
		t.Fatal("handler did not complete — Send from handler deadlocked")
	}
}

// TestDispatch_SlowHandler_DoesNotStallOtherSubs: one stuck subscription must
// not delay delivery to others.
func TestDispatch_SlowHandler_DoesNotStallOtherSubs(t *testing.T) {
	bus := newEventBus(nil)
	defer bus.closeAll()

	block := make(chan struct{})
	slow := bus.subscribe("ev", func(json.RawMessage) { <-block })
	defer slow.Unsubscribe()

	var fast atomic.Int32
	fastSub := bus.subscribe("ev", func(json.RawMessage) { fast.Add(1) })
	defer fastSub.Unsubscribe()

	for i := 0; i < 10; i++ {
		bus.dispatch("ev", nil)
	}
	if !pollUntil(t, 2*time.Second, func() bool { return fast.Load() == 10 }) {
		t.Fatalf("fast subscriber got %d/10 events while slow one was stuck", fast.Load())
	}
	close(block)
}

// TestDispatch_QueueFull_DropsNotBlocks: M3 — with a wedged handler, dispatch
// must stay non-blocking and count drops past the queue cap.
func TestDispatch_QueueFull_DropsNotBlocks(t *testing.T) {
	bus := newEventBus(nil)

	block := make(chan struct{})
	sub := bus.subscribe("ev", func(json.RawMessage) { <-block })

	const extra = 100
	start := time.Now()
	// worker may pull 1 event before blocking; +1 fills the queue deterministically.
	for i := 0; i < subQueueCap+1+extra; i++ {
		bus.dispatch("ev", nil)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("dispatch blocked: %v for %d events", elapsed, subQueueCap+1+extra)
	}

	var entry *subEntry
	bus.mu.RLock()
	for _, e := range bus.handlers["ev"] {
		entry = e
	}
	bus.mu.RUnlock()
	if entry == nil {
		t.Fatal("subscription entry missing")
	}
	if got := entry.dropped.Load(); got < extra {
		t.Fatalf("dropped = %d, want >= %d", got, extra)
	}
	close(block)
	sub.Unsubscribe()
	bus.closeAll()
}

// TestUnsubscribe_StopsWorker_NoLeak: 100 subscribe/unsubscribe cycles must
// not leave worker goroutines behind.
func TestUnsubscribe_StopsWorker_NoLeak(t *testing.T) {
	bus := newEventBus(nil)
	defer bus.closeAll()

	runtime.GC()
	before := runtime.NumGoroutine()
	for i := 0; i < 100; i++ {
		s := bus.subscribe("ev", func(json.RawMessage) {})
		s.Unsubscribe()
	}
	ok := pollUntil(t, 2*time.Second, func() bool {
		runtime.GC()
		return runtime.NumGoroutine() <= before+2
	})
	if !ok {
		t.Fatalf("goroutines: before=%d after=%d", before, runtime.NumGoroutine())
	}
}

// TestClose_WaitsWorkers: after Client.Close returns, no handler may still be
// running (or ever run again).
func TestClose_WaitsWorkers(t *testing.T) {
	ft := newFakeTransport()
	c := newClient(ft, nil)

	var running atomic.Int32
	var after atomic.Bool
	c.Subscribe("ev", func(json.RawMessage) {
		running.Add(1)
		time.Sleep(50 * time.Millisecond)
		running.Add(-1)
	})
	ft.injectEvent("ev", map[string]any{})

	// Let the worker pick the event up.
	pollUntil(t, time.Second, func() bool { return running.Load() > 0 })

	_ = c.Close()
	if running.Load() != 0 {
		t.Fatal("Close returned while a handler was still running")
	}
	// New events after Close must never fire a handler.
	c.Subscribe("ev2", func(json.RawMessage) { after.Store(true) })
	c.events.dispatch("ev2", nil)
	time.Sleep(50 * time.Millisecond)
	if after.Load() {
		t.Fatal("handler fired after Close")
	}
}

// TestDispatch_OrderPreservedPerSubscription: one subscription sees events in
// dispatch order.
func TestDispatch_OrderPreservedPerSubscription(t *testing.T) {
	bus := newEventBus(nil)
	defer bus.closeAll()

	const n = 50
	got := make([]int, 0, n)
	done := make(chan struct{})
	sub := bus.subscribe("ev", func(p json.RawMessage) {
		var v int
		_ = json.Unmarshal(p, &v)
		got = append(got, v) // single worker → no race
		if len(got) == n {
			close(done)
		}
	})
	defer sub.Unsubscribe()

	for i := 0; i < n; i++ {
		p, _ := json.Marshal(i)
		bus.dispatch("ev", p)
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("received %d/%d events", len(got), n)
	}
	for i, v := range got {
		if v != i {
			t.Fatalf("order broken at %d: got %d", i, v)
		}
	}
}
