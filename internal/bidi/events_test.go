package bidi

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestEventBus_SubscribeDispatch(t *testing.T) {
	bus := newEventBus(nil)
	var got atomic.Int32
	sub := bus.subscribe("foo", func(json.RawMessage) { got.Add(1) })
	defer sub.Unsubscribe()

	bus.dispatch("foo", nil)
	bus.dispatch("foo", nil)
	bus.dispatch("bar", nil) // unrelated, ignored

	// Delivery is async (per-subscription worker) — poll.
	if !pollUntil(t, time.Second, func() bool { return got.Load() == 2 }) {
		t.Fatalf("handler invoked %d times, want 2", got.Load())
	}
}

func TestEventBus_Unsubscribe(t *testing.T) {
	bus := newEventBus(nil)
	var got atomic.Int32
	sub := bus.subscribe("foo", func(json.RawMessage) { got.Add(1) })

	bus.dispatch("foo", nil)
	// Queued events are still delivered on unsubscribe; wait for the first.
	if !pollUntil(t, time.Second, func() bool { return got.Load() == 1 }) {
		t.Fatalf("first event not delivered, got %d", got.Load())
	}
	sub.Unsubscribe()
	bus.dispatch("foo", nil)
	sub.Unsubscribe() // idempotent

	time.Sleep(50 * time.Millisecond) // grace: a post-unsubscribe delivery would land here
	if got.Load() != 1 {
		t.Fatalf("handler invoked %d times after unsubscribe, want 1", got.Load())
	}
}

func TestEventBus_MultipleHandlers(t *testing.T) {
	bus := newEventBus(nil)
	var a, b atomic.Int32
	subA := bus.subscribe("foo", func(json.RawMessage) { a.Add(1) })
	subB := bus.subscribe("foo", func(json.RawMessage) { b.Add(1) })
	defer subA.Unsubscribe()
	defer subB.Unsubscribe()

	bus.dispatch("foo", nil)
	if !pollUntil(t, time.Second, func() bool { return a.Load() == 1 && b.Load() == 1 }) {
		t.Fatalf("a=%d b=%d, want both 1", a.Load(), b.Load())
	}
}

func TestEventBus_HandlerPanicRecovered(t *testing.T) {
	logger := &captureLogger{}
	bus := newEventBus(logger)
	sub := bus.subscribe("foo", func(json.RawMessage) { panic("boom") })
	defer sub.Unsubscribe()

	// Must not propagate.
	bus.dispatch("foo", nil)

	if !pollUntil(t, time.Second, func() bool { return logger.warnCount() > 0 }) {
		t.Fatal("expected warn log for handler panic")
	}
}

func TestClient_EventDispatch(t *testing.T) {
	ft := newFakeTransport()
	c := newClient(ft, nil)
	defer c.Close()

	var got atomic.Int32
	var payload atomic.Value
	sub := c.Subscribe("log.entryAdded", func(p json.RawMessage) {
		got.Add(1)
		payload.Store(string(p))
	})
	defer sub.Unsubscribe()

	ft.injectEvent("log.entryAdded", map[string]any{"text": "hello"})

	// Reader is async — poll briefly.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) && got.Load() == 0 {
		time.Sleep(5 * time.Millisecond)
	}
	if got.Load() != 1 {
		t.Fatalf("handler not invoked")
	}
	if p, _ := payload.Load().(string); p == "" {
		t.Fatalf("empty payload")
	}
}

func TestEventBus_ConcurrentSubscribe(t *testing.T) {
	bus := newEventBus(nil)
	var wg sync.WaitGroup
	subs := make([]*Subscription, 50)
	for i := range subs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			subs[i] = bus.subscribe("foo", func(json.RawMessage) {})
		}(i)
	}
	wg.Wait()
	// Dispatch shouldn't deadlock with concurrent unsubscribe.
	go bus.dispatch("foo", nil)
	for _, s := range subs {
		s.Unsubscribe()
	}
}

// captureLogger collects warn-level messages for assertions.
type captureLogger struct {
	mu    sync.Mutex
	warns []string
}

func (l *captureLogger) Debugf(string, ...any) {}
func (l *captureLogger) Infof(string, ...any)  {}
func (l *captureLogger) Warnf(format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.warns = append(l.warns, format)
}
func (l *captureLogger) Errorf(string, ...any) {}

func (l *captureLogger) warnCount() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.warns)
}

// Keep context import used here for parity with usage patterns in other tests.
var _ = context.Background
