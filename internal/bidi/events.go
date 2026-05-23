package bidi

import (
	"encoding/json"
	"sync"
	"sync/atomic"
)

// Handler is invoked for each matching event. It receives the raw params
// JSON; callers decode into a typed struct. Handler runs on a dispatcher
// goroutine — keep it fast or hand off to a worker pool.
type Handler func(params json.RawMessage)

// Subscription is the handle returned by Subscribe. Call Unsubscribe to stop
// receiving — idempotent and safe from any goroutine.
type Subscription struct {
	bus    *eventBus
	method string
	id     uint64
	done   atomic.Bool
}

// Unsubscribe removes the handler. Safe to call multiple times.
func (s *Subscription) Unsubscribe() {
	if s == nil || s.done.Swap(true) {
		return
	}
	s.bus.remove(s.method, s.id)
}

// eventBus is the internal subscription registry. Goroutine-safe.
type eventBus struct {
	mu       sync.RWMutex
	handlers map[string]map[uint64]Handler
	nextID   atomic.Uint64
	logger   Logger
}

func newEventBus(logger Logger) *eventBus {
	return &eventBus{
		handlers: make(map[string]map[uint64]Handler),
		logger:   logger,
	}
}

// subscribe registers h for method. Returns a Subscription handle.
func (b *eventBus) subscribe(method string, h Handler) *Subscription {
	id := b.nextID.Add(1)
	b.mu.Lock()
	if b.handlers[method] == nil {
		b.handlers[method] = make(map[uint64]Handler)
	}
	b.handlers[method][id] = h
	b.mu.Unlock()
	return &Subscription{bus: b, method: method, id: id}
}

func (b *eventBus) remove(method string, id uint64) {
	b.mu.Lock()
	if m, ok := b.handlers[method]; ok {
		delete(m, id)
		if len(m) == 0 {
			delete(b.handlers, method)
		}
	}
	b.mu.Unlock()
}

// dispatch invokes every handler subscribed to method. Handler panics are
// recovered so one bad listener doesn't kill the reader goroutine.
func (b *eventBus) dispatch(method string, params json.RawMessage) {
	b.mu.RLock()
	subs := b.handlers[method]
	// Snapshot to release the read lock before invoking — handlers may
	// re-enter (subscribe / unsubscribe) without deadlocking.
	handlers := make([]Handler, 0, len(subs))
	for _, h := range subs {
		handlers = append(handlers, h)
	}
	b.mu.RUnlock()

	for _, h := range handlers {
		b.safeInvoke(method, h, params)
	}
}

func (b *eventBus) safeInvoke(method string, h Handler, params json.RawMessage) {
	defer func() {
		if r := recover(); r != nil && b.logger != nil {
			b.logger.Warnf("bidi: event handler for %q panicked: %v", method, r)
		}
	}()
	h(params)
}
