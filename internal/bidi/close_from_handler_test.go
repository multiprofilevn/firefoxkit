package bidi

import (
	"encoding/json"
	"testing"
	"time"
)

// TestClose_FromInsideHandler_NoDeadlock locks in review finding #1: calling
// Close() from within an event handler must not hang forever (the worker
// running the handler can't return until Close returns, so wg.Wait would
// otherwise deadlock). closeAll's bounded wait lets Close return.
func TestClose_FromInsideHandler_NoDeadlock(t *testing.T) {
	ft := newFakeTransport()
	c := newClient(ft, nil)

	closed := make(chan struct{})
	sub := c.Subscribe("ev", func(json.RawMessage) {
		_ = c.Close() // reentrant Close from the worker goroutine
		close(closed)
	})
	defer sub.Unsubscribe()

	ft.injectEvent("ev", map[string]any{})

	// The overall drain timeout is 5s; give the test generous headroom.
	select {
	case <-closed:
	case <-time.After(workerDrainTimeout + 3*time.Second):
		t.Fatal("Close() from inside a handler deadlocked")
	}

	select {
	case <-c.Done():
	case <-time.After(time.Second):
		t.Fatal("client Done not closed after reentrant Close")
	}
}
