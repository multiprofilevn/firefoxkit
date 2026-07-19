package bidi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/coder/websocket"
)

// transport wraps a WebSocket connection. Behind an interface so tests can
// inject a fake without spinning a real ws server when convenient.
type transport interface {
	writeJSON(ctx context.Context, v any) error
	readJSON(ctx context.Context, v any) error
	close(code websocket.StatusCode, reason string) error
}

// wsTransport is the production implementation built on coder/websocket.
type wsTransport struct {
	conn *websocket.Conn
}

// dial opens a WebSocket to url. The Firefox BiDi endpoint is unauthenticated
// localhost-only; we still pass a customizable HTTPClient so callers can plug
// proxies in for testing.
func dial(ctx context.Context, url string, httpClient *http.Client) (*wsTransport, error) {
	opts := &websocket.DialOptions{HTTPClient: httpClient}
	conn, _, err := websocket.Dial(ctx, url, opts)
	if err != nil {
		return nil, fmt.Errorf("bidi: dial %s: %w", url, err)
	}
	// BiDi messages can be large (screenshots base64). Bump from default 32K.
	conn.SetReadLimit(64 * 1024 * 1024)
	return &wsTransport{conn: conn}, nil
}

// writeJSON marshals v and writes it as one text frame.
//
// Known behavior (coder/websocket): if ctx is cancelled midway through a Write,
// the library closes the whole connection — not just this write. So a single
// Send with a very short deadline carrying a large payload (long script,
// continueRequest body, base64 upload) can tear down the session on a heavily
// loaded machine, failing every other in-flight call with ErrClientClosed.
// Rare on localhost; give large-payload calls a generous timeout. Not worked
// around here on purpose (measured in the leak-test suite first).
func (t *wsTransport) writeJSON(ctx context.Context, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("bidi: marshal: %w", err)
	}
	return t.conn.Write(ctx, websocket.MessageText, data)
}

func (t *wsTransport) readJSON(ctx context.Context, v any) error {
	_, data, err := t.conn.Read(ctx)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

func (t *wsTransport) close(code websocket.StatusCode, reason string) error {
	return t.conn.Close(code, reason)
}

// defaultDialTimeout caps initial connect; sessions live longer via ctx.
const defaultDialTimeout = 10 * time.Second
