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

// wsConn abstracts the subset of *websocket.Conn the transport uses, so tests
// can fake conn-level behavior (blocking writes, cancel-mid-write teardown)
// while exercising the real wsTransport logic.
type wsConn interface {
	Write(ctx context.Context, typ websocket.MessageType, p []byte) error
	Read(ctx context.Context) (websocket.MessageType, []byte, error)
	Close(code websocket.StatusCode, reason string) error
}

// wsTransport is the production implementation built on coder/websocket.
type wsTransport struct {
	conn wsConn
	// writeTimeout bounds a single frame write. 0 → defaultWriteTimeout.
	writeTimeout time.Duration
}

// dial opens a WebSocket to url. The Firefox BiDi endpoint is unauthenticated
// localhost-only; we still pass a customizable HTTPClient so callers can plug
// proxies in for testing.
func dial(ctx context.Context, url string, httpClient *http.Client, writeTimeout time.Duration) (*wsTransport, error) {
	opts := &websocket.DialOptions{HTTPClient: httpClient}
	conn, _, err := websocket.Dial(ctx, url, opts)
	if err != nil {
		return nil, fmt.Errorf("bidi: dial %s: %w", url, err)
	}
	// BiDi messages can be large (screenshots base64). Bump from default 32K.
	conn.SetReadLimit(64 * 1024 * 1024)
	return &wsTransport{conn: conn, writeTimeout: writeTimeout}, nil
}

// writeJSON marshals v and writes it as one text frame.
//
// The write runs under an INTERNAL ctx (background + writeTimeout), never the
// caller's. coder/websocket closes the whole connection — not just the write —
// when the ctx passed to Write is cancelled mid-frame, so a single Send with a
// short deadline carrying a large payload would otherwise tear down the
// session and fail every other in-flight call with ErrClientClosed.
// Trade-off (accepted): the caller cannot cancel an in-flight frame write;
// writes are localhost-fast and bounded by writeTimeout. The caller ctx still
// governs the response wait in Send.
func (t *wsTransport) writeJSON(_ context.Context, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("bidi: marshal: %w", err)
	}
	wctx, cancel := context.WithTimeout(context.Background(), t.effectiveWriteTimeout())
	defer cancel()
	return t.conn.Write(wctx, websocket.MessageText, data)
}

// effectiveWriteTimeout resolves the configured write budget, falling back to
// defaultWriteTimeout when unset (0) or invalid.
func (t *wsTransport) effectiveWriteTimeout() time.Duration {
	if t.writeTimeout <= 0 {
		return defaultWriteTimeout
	}
	return t.writeTimeout
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

// defaultWriteTimeout bounds a single frame write so a genuinely stuck write
// (dead peer, full buffers) fails instead of parking forever. Generous because
// large payloads (base64 screenshots/uploads) may take a while under load.
const defaultWriteTimeout = 10 * time.Second
