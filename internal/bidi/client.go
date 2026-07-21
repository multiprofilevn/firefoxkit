package bidi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

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

	// defaultCallTimeout is the backstop deadline applied in Send when the
	// caller ctx has none. 0 disables the backstop (legacy park-until-Close).
	defaultCallTimeout time.Duration

	mu      sync.Mutex
	pending map[int]chan *frame

	shutdownOnce sync.Once // closes closed + tears down transport
	finishOnce   sync.Once // drains pending + stops event workers
	closed       chan struct{}
	readerDone   chan struct{} // closed when readLoop actually returns
}

// Options for Dial.
type Options struct {
	HTTPClient *http.Client
	Logger     Logger
	// DefaultCallTimeout is the backstop deadline Send applies when the caller
	// ctx has no deadline of its own. A lost response on an alive connection
	// then fails with ErrCommandTimeout instead of parking the goroutine for
	// the browser lifetime. 0 disables the backstop.
	DefaultCallTimeout time.Duration
}

// Dial connects to the BiDi endpoint at url (e.g. ws://127.0.0.1:9222/session)
// and starts the reader goroutine. Caller must Close() to release resources.
func Dial(ctx context.Context, url string, opts Options) (*Client, error) {
	t, err := dial(ctx, url, opts.HTTPClient)
	if err != nil {
		return nil, err
	}
	c := newClient(t, opts.Logger)
	// Safe to set post-construction: no Send can run before Dial returns.
	c.defaultCallTimeout = opts.DefaultCallTimeout
	return c, nil
}

// newClient wires a Client around any transport — used by tests with a fake.
func newClient(t transport, logger Logger) *Client {
	if logger == nil {
		logger = noopLogger{}
	}
	c := &Client{
		t:          t,
		logger:     logger,
		events:     newEventBus(logger),
		pending:    make(map[int]chan *frame),
		closed:     make(chan struct{}),
		readerDone: make(chan struct{}),
	}
	go c.readLoop()
	return c
}

// Send sends method+params and decodes the result into out (may be nil to
// discard). Returns *BiDiError on protocol-level errors, ctx.Err() on
// cancellation, ErrClientClosed if the client shuts down mid-flight.
func (c *Client) Send(ctx context.Context, method string, params, out any) error {
	// Fail fast: the frame write no longer honors the caller ctx (see
	// wsTransport.writeJSON), so an already-cancelled caller must bail here
	// before emitting a wasted frame.
	if err := ctx.Err(); err != nil {
		return err
	}

	// Backstop: callers without a deadline get the configured default so a
	// lost response can't park this goroutine (and its pending entry) forever.
	// A caller-supplied deadline is never overridden.
	if d := c.defaultCallTimeout; d > 0 {
		if _, ok := ctx.Deadline(); !ok {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, d)
			defer cancel()
		}
	}

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
		// Wrap so callers can errors.Is(err, ErrCommandTimeout) while still
		// matching the underlying context cause via Unwrap.
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("%w: %s: %w", ErrCommandTimeout, method, ctx.Err())
		}
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
// call multiple times. Blocks until the reader goroutine has exited and every
// event worker has drained, so no handler runs after Close() returns.
func (c *Client) Close() error {
	err := c.shutdown(websocket.StatusNormalClosure, "client close")
	// The transport close above unblocks the reader's blocking Read; wait for
	// it to actually return before finishing teardown.
	<-c.readerDone
	c.finish()
	return err
}

// shutdown closes the done channel and tears down the transport exactly once.
// Called by both Close() and the reader's fatal-exit path.
func (c *Client) shutdown(code websocket.StatusCode, reason string) error {
	var err error
	c.shutdownOnce.Do(func() {
		close(c.closed)
		err = c.t.close(code, reason)
	})
	return err
}

// finish drains pending callers and stops event workers exactly once. Drain
// runs BEFORE stopping workers so a handler blocked in Send gets
// ErrClientClosed immediately instead of deadlocking closeAll's Wait.
func (c *Client) finish() {
	c.finishOnce.Do(func() {
		c.drainPending()
		c.events.closeAll()
	})
}

// Done returns a channel closed when the client shuts down (Close or reader
// fatal). Callers can <-Done() to wait for lifetime end.
func (c *Client) Done() <-chan struct{} { return c.closed }

// readLoop is the single reader. Dispatches by frame shape.
func (c *Client) readLoop() {
	defer func() {
		// Reader exit always implies the client is dead. Mark shutdown (no-op
		// if Close initiated it), signal our exit so a waiting Close unblocks,
		// then finish teardown (no-op if Close already ran it).
		_ = c.shutdown(websocket.StatusInternalError, "reader exit")
		close(c.readerDone)
		c.finish()
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
