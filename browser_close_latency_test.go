package firefoxkit

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/tuwibu/firefoxkit/internal/bidi"
)

// blockedWriteConn implements bidi.WSConn with writes that park until the
// connection is closed — simulating a dead/hung Firefox peer whose TCP buffer
// never drains. Read parks the same way so the client reader just idles.
type blockedWriteConn struct {
	writeStarted chan struct{} // closed on first Write entry
	startOnce    sync.Once

	closeOnce sync.Once
	closedCh  chan struct{}
}

func newBlockedWriteConn() *blockedWriteConn {
	return &blockedWriteConn{
		writeStarted: make(chan struct{}),
		closedCh:     make(chan struct{}),
	}
}

func (c *blockedWriteConn) Write(ctx context.Context, _ websocket.MessageType, _ []byte) error {
	c.startOnce.Do(func() { close(c.writeStarted) })
	select {
	case <-c.closedCh:
		return errors.New("blockedWriteConn: conn closed")
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *blockedWriteConn) Read(ctx context.Context) (websocket.MessageType, []byte, error) {
	select {
	case <-c.closedCh:
		return 0, nil, errors.New("blockedWriteConn: conn closed")
	case <-ctx.Done():
		return 0, nil, ctx.Err()
	}
}

func (c *blockedWriteConn) Close(websocket.StatusCode, string) error {
	c.closeOnce.Do(func() { close(c.closedCh) })
	return nil
}

// TestClose_BoundedWhenCleanupWriteBlocked: a drainer worker wedged inside
// conn.Write (dead peer) must NOT stall Browser.Close for the full 10s
// writeTimeout. Close bounds the graceful phase by gracefulCloseBudget, then
// tears down the client (which errors the stuck write out) and joins the
// workers — so Close returns in ~budget and no goroutine leaks.
func TestClose_BoundedWhenCleanupWriteBlocked(t *testing.T) {
	g0 := runtime.NumGoroutine()

	conn := newBlockedWriteConn()
	// Production default write timeout: 10s. The whole point is that Close
	// must NOT wait for it.
	client := bidi.NewClientForTest(conn, 10*time.Second, nil)

	b := &Browser{client: client, cfg: defaultConfig()}
	b.initBaseCtx()
	b.startCleanupDrainer()

	// A detached page whose cleanup issues one session.unsubscribe RPC — its
	// frame write parks forever on the dead conn.
	p := &Page{browser: b, contextID: "ctx-blocked"}
	p.trackServerEvent("log.entryAdded")
	b.cleanupCh <- p

	select {
	case <-conn.writeStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("cleanup RPC write never started — harness broken")
	}

	start := time.Now()
	_ = b.Close()
	elapsed := time.Since(start)

	// Budget + generous CI margin; far below the 10s writeTimeout the old
	// ordering (drainer join BEFORE client.Close) had to sit out.
	if limit := gracefulCloseBudget + 2*time.Second; elapsed > limit {
		t.Fatalf("Close took %v with a write-blocked cleanup worker, want <= %v", elapsed, limit)
	}
	if !pollUntilTrue(t, 3*time.Second, func() bool { return runtime.NumGoroutine() <= g0 }) {
		t.Fatalf("goroutines = %d, want <= %d after Close (worker leaked)", runtime.NumGoroutine(), g0)
	}
}
