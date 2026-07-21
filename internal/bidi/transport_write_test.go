package bidi

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// fakeWSConn fakes *websocket.Conn at the conn level so the REAL
// wsTransport.writeJSON logic is exercised. It reproduces the coder/websocket
// footgun: a ctx cancelled midway through Write tears down the whole
// connection, not just that write.
type fakeWSConn struct {
	gate    chan struct{} // when non-nil, Write blocks until this closes
	in      chan []byte   // frames served to Read
	onFrame func(data []byte)

	closeOnce sync.Once
	closedCh  chan struct{}
}

func newFakeWSConn() *fakeWSConn {
	return &fakeWSConn{
		in:       make(chan []byte, 64),
		closedCh: make(chan struct{}),
	}
}

func (f *fakeWSConn) isClosed() bool {
	select {
	case <-f.closedCh:
		return true
	default:
		return false
	}
}

func (f *fakeWSConn) markClosed() {
	f.closeOnce.Do(func() { close(f.closedCh) })
}

func (f *fakeWSConn) Write(ctx context.Context, _ websocket.MessageType, p []byte) error {
	if f.isClosed() {
		return errors.New("fakeWSConn: conn closed")
	}
	if f.gate != nil {
		select {
		case <-f.gate:
		case <-ctx.Done():
			// coder/websocket behavior: cancel mid-write closes the WHOLE conn.
			f.markClosed()
			return ctx.Err()
		case <-f.closedCh:
			return errors.New("fakeWSConn: conn closed")
		}
	}
	if f.onFrame != nil {
		f.onFrame(append([]byte(nil), p...))
	}
	return nil
}

func (f *fakeWSConn) Read(ctx context.Context) (websocket.MessageType, []byte, error) {
	select {
	case data := <-f.in:
		return websocket.MessageText, data, nil
	case <-f.closedCh:
		return 0, nil, errors.New("fakeWSConn: conn closed")
	case <-ctx.Done():
		return 0, nil, ctx.Err()
	}
}

func (f *fakeWSConn) Close(_ websocket.StatusCode, _ string) error {
	f.markClosed()
	return nil
}

// respondSuccess parses a written command frame and queues a success response.
func (f *fakeWSConn) respondSuccess(data []byte) {
	var cmd command
	if err := json.Unmarshal(data, &cmd); err != nil {
		return
	}
	frm := map[string]any{"id": cmd.ID, "type": "success", "result": map[string]any{}}
	out, _ := json.Marshal(frm)
	select {
	case f.in <- out:
	default:
	}
}

// TestWrite_CallerCancelMidWrite_ConnSurvives: a Send whose caller ctx expires
// while the frame write is blocked must NOT tear down the connection — a
// second Send with a wide ctx still gets its response.
func TestWrite_CallerCancelMidWrite_ConnSurvives(t *testing.T) {
	fc := newFakeWSConn()
	gate := make(chan struct{})
	fc.gate = gate
	fc.onFrame = fc.respondSuccess

	tr := &wsTransport{conn: fc, writeTimeout: 5 * time.Second}
	c := newClient(tr, nil)
	defer c.Close()

	errCh := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		errCh <- c.Send(ctx, "test.slow", nil, nil)
	}()

	// Let the caller ctx expire while the write is still blocked on the gate.
	time.Sleep(150 * time.Millisecond)
	close(gate)

	select {
	case err := <-errCh:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("first Send: expected DeadlineExceeded, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("first Send did not return")
	}

	if fc.isClosed() {
		t.Fatal("conn was torn down by caller cancel mid-write")
	}
	select {
	case <-c.Done():
		t.Fatal("client closed after caller cancel mid-write")
	default:
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := c.Send(ctx, "test.ok", nil, nil); err != nil {
		t.Fatalf("second Send after mid-write cancel: %v", err)
	}
}

// TestWrite_StuckWrite_BoundedByWriteTimeout: a write that hangs must be
// bounded by the transport's own writeTimeout — never park forever, even with
// a caller ctx that has no deadline.
func TestWrite_StuckWrite_BoundedByWriteTimeout(t *testing.T) {
	fc := newFakeWSConn()
	fc.gate = make(chan struct{}) // never released — write is stuck

	tr := &wsTransport{conn: fc, writeTimeout: 100 * time.Millisecond}

	done := make(chan error, 1)
	start := time.Now()
	go func() {
		done <- tr.writeJSON(context.Background(), map[string]any{"id": 1, "method": "test.stuck"})
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected error from stuck write, got nil")
		}
		if elapsed := time.Since(start); elapsed > time.Second {
			t.Fatalf("writeJSON took %v, expected ~100ms", elapsed)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("writeJSON hung — stuck write not bounded by writeTimeout")
	}
}

// countingTransport wraps fakeTransport to count writeJSON invocations.
type countingTransport struct {
	*fakeTransport
	writes atomic.Int32
}

func (c *countingTransport) writeJSON(ctx context.Context, v any) error {
	c.writes.Add(1)
	return c.fakeTransport.writeJSON(ctx, v)
}

// TestSend_CallerCtxCanceledBeforeWrite_FailsFast: a ctx already cancelled
// before Send must return immediately WITHOUT emitting a frame.
func TestSend_CallerCtxCanceledBeforeWrite_FailsFast(t *testing.T) {
	ct := &countingTransport{fakeTransport: newFakeTransport()}
	c := newClient(ct, nil)
	defer c.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancelled

	start := time.Now()
	err := c.Send(ctx, "test.never", nil, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Fatalf("Send took %v, expected immediate return", elapsed)
	}
	if n := ct.writes.Load(); n != 0 {
		t.Fatalf("writeJSON called %d times for pre-cancelled ctx, want 0", n)
	}
}
