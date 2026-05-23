package bidi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// fakeTransport implements transport for in-process tests. Round-trip uses
// in-memory channels; no real WebSocket.
type fakeTransport struct {
	mu      sync.Mutex
	out     chan []byte // frames the client writes
	in      chan []byte // frames the test injects to be read
	closed  atomic.Bool
	onWrite func(cmd command) // optional auto-responder
}

func newFakeTransport() *fakeTransport {
	return &fakeTransport{
		out: make(chan []byte, 64),
		in:  make(chan []byte, 64),
	}
}

func (f *fakeTransport) writeJSON(_ context.Context, v any) error {
	if f.closed.Load() {
		return errors.New("transport closed")
	}
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	// Non-blocking send to out; drop if no consumer to keep test fakes simple.
	select {
	case f.out <- data:
	default:
	}
	if f.onWrite != nil {
		var cmd command
		if jerr := json.Unmarshal(data, &cmd); jerr == nil {
			go f.onWrite(cmd)
		}
	}
	return nil
}

func (f *fakeTransport) readJSON(ctx context.Context, v any) error {
	select {
	case data, ok := <-f.in:
		if !ok {
			return errors.New("transport closed")
		}
		return json.Unmarshal(data, v)
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (f *fakeTransport) close(_ websocket.StatusCode, _ string) error {
	if f.closed.Swap(true) {
		return nil
	}
	close(f.in)
	return nil
}

func (f *fakeTransport) injectResponse(id int, result any) {
	frm := map[string]any{"id": id, "type": "success", "result": result}
	data, _ := json.Marshal(frm)
	f.in <- data
}

func (f *fakeTransport) injectError(id int, code, msg string) {
	frm := map[string]any{"id": id, "type": "error", "error": code, "message": msg}
	data, _ := json.Marshal(frm)
	f.in <- data
}

func (f *fakeTransport) injectEvent(method string, params any) {
	frm := map[string]any{"type": "event", "method": method, "params": params}
	data, _ := json.Marshal(frm)
	f.in <- data
}

// Adapter so wsTransport.close signature matches; tests bypass production
// close path. The real transport.close uses websocket.StatusCode int alias.
//
// We satisfy transport interface by having close accept any int-shaped code.

func TestClient_SendRoundTrip(t *testing.T) {
	ft := newFakeTransport()
	// Hook in an auto-responder so the test doesn't have to read out manually.
	ft.onWrite = func(cmd command) {
		ft.injectResponse(cmd.ID, map[string]any{"ready": true, "message": "ok"})
	}
	c := newClient(ft, nil)
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	var out StatusResult
	if err := c.Send(ctx, "session.status", struct{}{}, &out); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if !out.Ready || out.Message != "ok" {
		t.Fatalf("unexpected result: %+v", out)
	}
	// Drain out queue
	<-ft.out
}

func TestClient_ConcurrentSends(t *testing.T) {
	ft := newFakeTransport()
	ft.onWrite = func(cmd command) {
		ft.injectResponse(cmd.ID, map[string]any{"id": cmd.ID})
	}
	c := newClient(ft, nil)
	defer c.Close()

	const N = 100
	var wg sync.WaitGroup
	var failed atomic.Int32
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			var out struct {
				ID int `json:"id"`
			}
			if err := c.Send(ctx, "test.echo", nil, &out); err != nil {
				failed.Add(1)
				t.Errorf("Send: %v", err)
				return
			}
			// out.ID echoes back the request id — verifies correlation.
			if out.ID == 0 {
				failed.Add(1)
				t.Errorf("zero id in response")
			}
		}()
	}
	wg.Wait()
	if failed.Load() != 0 {
		t.Fatalf("%d of %d sends failed", failed.Load(), N)
	}
}

func TestClient_BiDiErrorResponse(t *testing.T) {
	ft := newFakeTransport()
	ft.onWrite = func(cmd command) {
		ft.injectError(cmd.ID, "unknown command", "no such method")
	}
	c := newClient(ft, nil)
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	err := c.Send(ctx, "bogus.method", nil, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var be *BiDiError
	if !errors.As(err, &be) {
		t.Fatalf("expected *BiDiError, got %T: %v", err, err)
	}
	if be.Code != "unknown command" {
		t.Errorf("Code = %q", be.Code)
	}
	if be.Message != "no such method" {
		t.Errorf("Message = %q", be.Message)
	}
}

func TestClient_ContextCancel(t *testing.T) {
	ft := newFakeTransport()
	// No onWrite — response never arrives. ctx must cancel.
	c := newClient(ft, nil)
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err := c.Send(ctx, "test.never", nil, nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected DeadlineExceeded, got %v", err)
	}
	// Drain so test doesn't leak
	<-ft.out
}

func TestClient_CloseDrainsPending(t *testing.T) {
	ft := newFakeTransport()
	c := newClient(ft, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- c.Send(ctx, "test.never", nil, nil)
	}()

	// Let Send register before closing.
	time.Sleep(20 * time.Millisecond)
	if err := c.Close(); err != nil {
		t.Logf("close err (ignored): %v", err)
	}

	select {
	case err := <-done:
		if !errors.Is(err, ErrClientClosed) {
			t.Fatalf("expected ErrClientClosed, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Send did not return after Close")
	}
}

func TestClient_SendAfterClose(t *testing.T) {
	ft := newFakeTransport()
	c := newClient(ft, nil)
	_ = c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err := c.Send(ctx, "test.x", nil, nil)
	if !errors.Is(err, ErrClientClosed) {
		t.Fatalf("expected ErrClientClosed, got %v", err)
	}
}

func TestClient_DoneCloses(t *testing.T) {
	ft := newFakeTransport()
	c := newClient(ft, nil)

	select {
	case <-c.Done():
		t.Fatal("Done closed prematurely")
	default:
	}

	_ = c.Close()

	select {
	case <-c.Done():
	case <-time.After(time.Second):
		t.Fatal("Done did not close after Close()")
	}
}

func TestClient_MalformedFrameSurvives(t *testing.T) {
	ft := newFakeTransport()
	c := newClient(ft, nil)
	defer c.Close()

	// Inject a frame that's neither response nor event (no id, no method).
	bad := map[string]any{"type": "garbage"}
	data, _ := json.Marshal(bad)
	ft.in <- data

	// Then send a valid command — reader must still be alive.
	ft.onWrite = func(cmd command) {
		ft.injectResponse(cmd.ID, map[string]any{"ok": true})
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := c.Send(ctx, "test.ok", nil, nil); err != nil {
		t.Fatalf("Send after malformed frame: %v", err)
	}
	<-ft.out
}

// Ensure send doesn't block forever if pending map is leaked (regression).
func TestClient_NoMapLeakOnTimeout(t *testing.T) {
	ft := newFakeTransport()
	c := newClient(ft, nil)
	defer c.Close()

	for i := 0; i < 10; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		_ = c.Send(ctx, "test.never", nil, nil)
		cancel()
		<-ft.out
	}
	c.mu.Lock()
	got := len(c.pending)
	c.mu.Unlock()
	if got != 0 {
		t.Fatalf("pending map leaked %d entries", got)
	}
}

// Helper to format errors uniformly across tests.
func errStr(e error) string {
	if e == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%T: %s", e, e.Error())
}

var _ = errStr // referenced from events_test.go
