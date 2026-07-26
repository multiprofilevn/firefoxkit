package bidi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// TestWrite_CustomShortTimeout_CutsAtDeadline: a configured writeTimeout
// shorter than the default must bound the frame write at that value.
func TestWrite_CustomShortTimeout_CutsAtDeadline(t *testing.T) {
	fc := newFakeWSConn()
	fc.gate = make(chan struct{}) // never released — write stays blocked

	tr := &wsTransport{conn: fc, writeTimeout: 50 * time.Millisecond}

	done := make(chan error, 1)
	start := time.Now()
	go func() {
		done <- tr.writeJSON(context.Background(), map[string]any{"id": 1, "method": "test.slow"})
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected error at custom write deadline, got nil")
		}
		if elapsed := time.Since(start); elapsed > 2*time.Second {
			t.Fatalf("writeJSON took %v, expected ~50ms", elapsed)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("writeJSON hung — custom writeTimeout not applied")
	}
}

// TestWrite_CustomLongTimeout_NoPrematureCut: a write slower than the default
// window must still succeed when a longer writeTimeout is configured.
func TestWrite_CustomLongTimeout_NoPrematureCut(t *testing.T) {
	fc := newFakeWSConn()
	gate := make(chan struct{})
	fc.gate = gate

	tr := &wsTransport{conn: fc, writeTimeout: 3 * time.Second}

	done := make(chan error, 1)
	go func() {
		done <- tr.writeJSON(context.Background(), map[string]any{"id": 1, "method": "test.big"})
	}()

	// Hold the write well past a short deadline, then let it through.
	time.Sleep(200 * time.Millisecond)
	select {
	case err := <-done:
		t.Fatalf("write returned before the gate opened: %v", err)
	default:
	}
	close(gate)

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("write with long timeout: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("writeJSON hung after gate opened")
	}
	if fc.isClosed() {
		t.Fatal("conn torn down despite the write completing inside its deadline")
	}
}

// TestWrite_ZeroTimeout_UsesDefault: unset (0) keeps the 10s default.
func TestWrite_ZeroTimeout_UsesDefault(t *testing.T) {
	if got := (&wsTransport{}).effectiveWriteTimeout(); got != defaultWriteTimeout {
		t.Errorf("zero writeTimeout: got %v, want %v", got, defaultWriteTimeout)
	}
	if got := (&wsTransport{writeTimeout: -1}).effectiveWriteTimeout(); got != defaultWriteTimeout {
		t.Errorf("negative writeTimeout: got %v, want %v", got, defaultWriteTimeout)
	}
	if got := (&wsTransport{writeTimeout: 30 * time.Second}).effectiveWriteTimeout(); got != 30*time.Second {
		t.Errorf("custom writeTimeout: got %v, want 30s", got)
	}
}

// TestDial_WriteTimeoutReachesTransport: Options.WriteTimeout must land on the
// transport the client writes through (both Launch and Connect go via Dial).
func TestDial_WriteTimeoutReachesTransport(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close(websocket.StatusNormalClosure, "done")
		<-r.Context().Done()
	}))
	defer srv.Close()

	url := "ws://" + strings.TrimPrefix(srv.URL, "http://")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	c, err := Dial(ctx, url, Options{WriteTimeout: 42 * time.Second})
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()

	tr, ok := c.t.(*wsTransport)
	if !ok {
		t.Fatalf("transport type %T, want *wsTransport", c.t)
	}
	if tr.writeTimeout != 42*time.Second {
		t.Errorf("transport writeTimeout: got %v, want 42s", tr.writeTimeout)
	}

	// Unset must fall back to the package default.
	c2, err := Dial(ctx, url, Options{})
	if err != nil {
		t.Fatalf("dial default: %v", err)
	}
	defer c2.Close()
	if got := c2.t.(*wsTransport).effectiveWriteTimeout(); got != defaultWriteTimeout {
		t.Errorf("default writeTimeout: got %v, want %v", got, defaultWriteTimeout)
	}
}
