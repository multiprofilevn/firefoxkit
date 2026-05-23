package bidi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"

	"github.com/coder/websocket"
)

// Logger is the diagnostic interface the bidi client expects. The root
// firefoxkit package defines a matching public type and adapts it here —
// keeping the interface inside this package avoids an import cycle.
type Logger interface {
	Debugf(format string, args ...any)
	Infof(format string, args ...any)
	Warnf(format string, args ...any)
	Errorf(format string, args ...any)
}

// noopLogger is the default when no Logger is injected.
type noopLogger struct{}

func (noopLogger) Debugf(string, ...any) {}
func (noopLogger) Infof(string, ...any)  {}
func (noopLogger) Warnf(string, ...any)  {}
func (noopLogger) Errorf(string, ...any) {}

// Client is the WebDriver BiDi protocol client. Goroutine-safe.
type Client struct {
	t      transport
	logger Logger
	events *eventBus

	nextID atomic.Int64

	mu      sync.Mutex
	pending map[int]chan *frame

	closeOnce sync.Once
	closed    chan struct{}
	closeErr  error // captured cause for diagnostics; pending get ErrClientClosed
}

// Options for Dial.
type Options struct {
	HTTPClient *http.Client
	Logger     Logger
}

// Dial connects to the BiDi endpoint at url (e.g. ws://127.0.0.1:9222/session)
// and starts the reader goroutine. Caller must Close() to release resources.
func Dial(ctx context.Context, url string, opts Options) (*Client, error) {
	t, err := dial(ctx, url, opts.HTTPClient)
	if err != nil {
		return nil, err
	}
	return newClient(t, opts.Logger), nil
}

// newClient wires a Client around any transport — used by tests with a fake.
func newClient(t transport, logger Logger) *Client {
	if logger == nil {
		logger = noopLogger{}
	}
	c := &Client{
		t:       t,
		logger:  logger,
		events:  newEventBus(logger),
		pending: make(map[int]chan *frame),
		closed:  make(chan struct{}),
	}
	go c.readLoop()
	return c
}

// Send sends method+params and decodes the result into out (may be nil to
// discard). Returns *BiDiError on protocol-level errors, ctx.Err() on
// cancellation, ErrClientClosed if the client shuts down mid-flight.
func (c *Client) Send(ctx context.Context, method string, params, out any) error {
	id := int(c.nextID.Add(1))
	ch := make(chan *frame, 1)

	c.mu.Lock()
	select {
	case <-c.closed:
		c.mu.Unlock()
		return ErrClientClosed
	default:
	}
	c.pending[id] = ch
	c.mu.Unlock()

	defer c.cancelPending(id)

	cmd := command{ID: id, Method: method, Params: params}
	if err := c.t.writeJSON(ctx, cmd); err != nil {
		return fmt.Errorf("bidi: write %s: %w", method, err)
	}

	select {
	case f := <-ch:
		if f == nil {
			// Channel closed by drainPending — client shutting down.
			return ErrClientClosed
		}
		if f.isError() {
			return &BiDiError{
				Code:       firstNonEmpty(f.Error, f.Type),
				Message:    f.Message,
				Stacktrace: f.Stacktrace,
			}
		}
		if out != nil && len(f.Result) > 0 {
			if err := json.Unmarshal(f.Result, out); err != nil {
				return fmt.Errorf("bidi: decode %s result: %w", method, err)
			}
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-c.closed:
		return ErrClientClosed
	}
}

// Subscribe registers a handler for events with the given method name.
// Caller must Unsubscribe to release. Independent of BiDi session.subscribe
// (which controls what events the server emits) — call that separately.
func (c *Client) Subscribe(method string, h Handler) *Subscription {
	return c.events.subscribe(method, h)
}

// Close cancels all in-flight commands and shuts the reader down. Safe to
// call multiple times.
func (c *Client) Close() error {
	var firstErr error
	c.closeOnce.Do(func() {
		close(c.closed)
		firstErr = c.t.close(websocket.StatusNormalClosure, "client close")
		c.drainPending()
	})
	return firstErr
}

// Done returns a channel closed when the client shuts down (Close or reader
// fatal). Callers can <-Done() to wait for lifetime end.
func (c *Client) Done() <-chan struct{} { return c.closed }

// readLoop is the single reader. Dispatches by frame shape.
func (c *Client) readLoop() {
	defer func() {
		// Reader exit always implies client is dead — make sure pending
		// callers and Done() observers see it.
		c.closeOnce.Do(func() {
			close(c.closed)
			_ = c.t.close(websocket.StatusInternalError, "reader exit")
			c.drainPending()
		})
	}()

	ctx := context.Background()
	for {
		// Bail before blocking if Close already fired.
		select {
		case <-c.closed:
			return
		default:
		}

		var f frame
		if err := c.t.readJSON(ctx, &f); err != nil {
			if !errors.Is(err, context.Canceled) {
				c.closeErr = err
				c.logger.Debugf("bidi: reader exit: %v", err)
			}
			return
		}

		switch {
		case f.isResponse():
			c.deliverResponse(&f)
		case f.isEvent():
			c.events.dispatch(f.Method, f.Params)
		default:
			c.logger.Warnf("bidi: %v: %s", ErrMalformedFrame, mustJSON(&f))
		}
	}
}

func (c *Client) deliverResponse(f *frame) {
	c.mu.Lock()
	ch, ok := c.pending[*f.ID]
	if ok {
		delete(c.pending, *f.ID)
	}
	c.mu.Unlock()
	if ok {
		ch <- f
	} else {
		// Late response after ctx cancel — drop. Caller has already moved on.
		c.logger.Debugf("bidi: orphan response id=%d", *f.ID)
	}
}

// cancelPending removes a pending entry when the caller's Send returns
// (typically via ctx cancel). Without this the map leaks one entry per
// timed-out call.
func (c *Client) cancelPending(id int) {
	c.mu.Lock()
	delete(c.pending, id)
	c.mu.Unlock()
}

func (c *Client) drainPending() {
	c.mu.Lock()
	pending := c.pending
	c.pending = make(map[int]chan *frame)
	c.mu.Unlock()
	for id, ch := range pending {
		// Deliver a synthetic error frame so blocked Sends return ErrClientClosed
		// via the same select branch as ctx cancel — no need for a separate path.
		_ = id
		close(ch)
	}
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "<unmarshalable>"
	}
	return string(b)
}
