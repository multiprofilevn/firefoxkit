//go:build wsintegration

package bidi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// Real-WebSocket fidelity test for the write-teardown guard. coder/websocket
// closes the WHOLE connection when the ctx passed to Write is cancelled
// mid-frame. This test forces a mid-write cancel via TCP backpressure (server
// does not read until signalled, client writes a multi-MB frame) and asserts
// the guard keeps the connection alive for other Sends.
//
// Kept out of default `go test` via the wsintegration build tag.
func TestWSIntegration_CallerCancelMidWrite_ConnSurvives(t *testing.T) {
	startRead := make(chan struct{})
	var once sync.Once
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Errorf("accept: %v", err)
			return
		}
		conn.SetReadLimit(64 * 1024 * 1024)
		defer conn.Close(websocket.StatusNormalClosure, "server done")

		// Hold off reading so the client's large write blocks on TCP
		// backpressure while its caller ctx expires.
		<-startRead

		ctx := context.Background()
		for {
			_, data, err := conn.Read(ctx)
			if err != nil {
				return
			}
			var cmd command
			if err := json.Unmarshal(data, &cmd); err != nil {
				continue
			}
			resp, _ := json.Marshal(map[string]any{
				"id": cmd.ID, "type": "success", "result": map[string]any{},
			})
			if err := conn.Write(ctx, websocket.MessageText, resp); err != nil {
				return
			}
		}
	}))
	defer srv.Close()
	defer once.Do(func() { close(startRead) }) // unblock handler on early exit

	url := "ws://" + strings.TrimPrefix(srv.URL, "http://")
	dctx, dcancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer dcancel()
	c, err := Dial(dctx, url, Options{})
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()

	// Large payload so conn.Write blocks on backpressure (server not reading).
	big := strings.Repeat("x", 8*1024*1024)

	errCh := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		errCh <- c.Send(ctx, "test.big", map[string]any{"blob": big}, nil)
	}()

	// Let the caller ctx expire while the write is (likely) still in flight.
	time.Sleep(400 * time.Millisecond)
	once.Do(func() { close(startRead) })

	select {
	case err := <-errCh:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("first Send: expected DeadlineExceeded, got %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("first Send did not return")
	}

	select {
	case <-c.Done():
		t.Fatal("client closed — caller cancel mid-write tore down the connection")
	default:
	}

	// The connection must still round-trip a normal command.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.Send(ctx, "test.ok", nil, nil); err != nil {
		t.Fatalf("second Send after mid-write cancel: %v", err)
	}
}
