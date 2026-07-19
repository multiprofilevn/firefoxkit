package bidi

import (
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"
)

// workerDrainTimeout bounds how long closeAll waits for workers to finish.
// Normal workers drain near-instantly; the timeout only trips when Close() is
// (mis)used from inside a handler — the worker running it can't return until
// Close returns, so waiting on it would deadlock. We proceed instead.
const workerDrainTimeout = 5 * time.Second

// subQueueCap bounds each subscription's event queue. When a handler falls
// behind and its queue fills, newer events for THAT subscription are dropped
// (counted + warn-logged) — the reader goroutine never blocks and other
// subscriptions are unaffected.
const subQueueCap = 256

// Handler is invoked for each matching event. It receives the raw params
// JSON; callers decode into a typed struct. Each subscription owns a worker
// goroutine with a bounded queue, so handlers MAY call Send/RPCs directly; a
// slow handler only delays (or drops) events of its own subscription.
type Handler func(params json.RawMessage)

// Subscription is the handle returned by Subscribe. Call Unsubscribe to stop
// receiving — idempotent and safe from any goroutine.
type Subscription struct {
	bus    *eventBus
	method string
	id     uint64
	done   atomic.Bool
}

// Unsubscribe removes the handler and stops its worker. Safe to call multiple
// times. Events already queued are still delivered before the worker exits.
func (s *Subscription) Unsubscribe() {
	if s == nil || s.done.Swap(true) {
		return
	}
	s.bus.remove(s.method, s.id)
}

// subEntry is one registered handler + its bounded delivery queue.
type subEntry struct {
	ch      chan json.RawMessage
	dropped atomic.Uint64
}

// eventBus is the internal subscription registry. Goroutine-safe.
// Locking contract: dispatch sends non-blockingly while holding mu.RLock;
// remove/closeAll delete entries under mu.Lock BEFORE closing their channels,
// so a channel is never closed while a send can still reach it.
type eventBus struct {
	mu       sync.RWMutex
	handlers map[string]map[uint64]*subEntry
	closed   bool
	nextID   atomic.Uint64
	wg       sync.WaitGroup
	logger   Logger
}

func newEventBus(logger Logger) *eventBus {
	return &eventBus{
		handlers: make(map[string]map[uint64]*subEntry),
		logger:   logger,
	}
}

// subscribe registers h for method and starts its worker. Returns a
// Subscription handle. After the bus is closed, returns an inert no-op
// subscription whose handler will never fire.
func (b *eventBus) subscribe(method string, h Handler) *Subscription {
	id := b.nextID.Add(1)
	entry := &subEntry{ch: make(chan json.RawMessage, subQueueCap)}

	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		s := &Subscription{bus: b, method: method, id: id}
		s.done.Store(true)
		return s
	}
	if b.handlers[method] == nil {
		b.handlers[method] = make(map[uint64]*subEntry)
	}
	b.handlers[method][id] = entry
	b.wg.Add(1)
	b.mu.Unlock()

	go func() {
		defer b.wg.Done()
		for params := range entry.ch {
			b.safeInvoke(method, h, params)
		}
	}()
	return &Subscription{bus: b, method: method, id: id}
}

func (b *eventBus) remove(method string, id uint64) {
	b.mu.Lock()
	var entry *subEntry
	if m, ok := b.handlers[method]; ok {
		entry = m[id]
		delete(m, id)
		if len(m) == 0 {
			delete(b.handlers, method)
		}
	}
	b.mu.Unlock()
	if entry != nil {
		close(entry.ch) // safe: no dispatch can hold this entry past the Lock above
	}
}

// closeAll tears down every subscription and waits for all workers to finish
// draining. Called from Client.Close — after it returns, no handler runs.
func (b *eventBus) closeAll() {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		b.wg.Wait()
		return
	}
	b.closed = true
	entries := make([]*subEntry, 0)
	for _, m := range b.handlers {
		for _, e := range m {
			entries = append(entries, e)
		}
	}
	b.handlers = make(map[string]map[uint64]*subEntry)
	b.mu.Unlock()

	for _, e := range entries {
		close(e.ch)
	}
	// Bounded wait: if Close() was called from inside a handler, the worker
	// running it is blocked until Close returns, so wg.Wait would deadlock.
	// Proceed after the timeout instead of hanging the whole client.
	done := make(chan struct{})
	go func() { b.wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(workerDrainTimeout):
		if b.logger != nil {
			b.logger.Warnf("bidi: event workers did not drain within %s — Close() likely called from inside a handler", workerDrainTimeout)
		}
	}
}

// dispatch queues params to every handler subscribed to method. Non-blocking:
// a full queue drops the event for that subscription only (counted, warned
// once per 100 drops) — the caller (reader goroutine) is never stalled.
func (b *eventBus) dispatch(method string, params json.RawMessage) {
	b.mu.RLock()
	for _, e := range b.handlers[method] {
		select {
		case e.ch <- params:
		default:
			n := e.dropped.Add(1)
			if b.logger != nil && n%100 == 1 {
				b.logger.Warnf("bidi: %q subscriber queue full — dropped %d events so far", method, n)
			}
		}
	}
	b.mu.RUnlock()
}

func (b *eventBus) safeInvoke(method string, h Handler, params json.RawMessage) {
	defer func() {
		if r := recover(); r != nil && b.logger != nil {
			b.logger.Warnf("bidi: event handler for %q panicked: %v", method, r)
		}
	}()
	h(params)
}
