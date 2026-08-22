package firefoxkit

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/multiprofilevn/firefoxkit/internal/bidi"
)

// mockBiDi is an in-process WebDriver BiDi server for root-package tests.
// It accepts one WS connection, answers every command via a scripted handler
// (default: empty success), records calls, and can inject events.
type mockBiDi struct {
	srv *httptest.Server

	mu       sync.Mutex
	handlers map[string]func(params json.RawMessage) (result any, errCode string)
	calls    map[string]int
	params   map[string][]json.RawMessage

	connMu sync.Mutex
	conn   *websocket.Conn
	ready  chan struct{}
}

type mockCmd struct {
	ID     int             `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

func newMockBiDi(t *testing.T) *mockBiDi {
	t.Helper()
	m := &mockBiDi{
		handlers: make(map[string]func(json.RawMessage) (any, string)),
		calls:    make(map[string]int),
		params:   make(map[string][]json.RawMessage),
		ready:    make(chan struct{}),
	}
	m.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		m.connMu.Lock()
		m.conn = conn
		m.connMu.Unlock()
		close(m.ready)
		m.serve(conn)
	}))
	t.Cleanup(m.srv.Close)
	return m
}

func (m *mockBiDi) serve(conn *websocket.Conn) {
	ctx := context.Background()
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return
		}
		var cmd mockCmd
		if err := json.Unmarshal(data, &cmd); err != nil {
			continue
		}
		m.mu.Lock()
		m.calls[cmd.Method]++
		m.params[cmd.Method] = append(m.params[cmd.Method], cmd.Params)
		h := m.handlers[cmd.Method]
		m.mu.Unlock()

		var resp map[string]any
		if h != nil {
			result, errCode := h(cmd.Params)
			if errCode != "" {
				resp = map[string]any{"id": cmd.ID, "type": "error", "error": errCode, "message": errCode}
			} else {
				if result == nil {
					result = map[string]any{}
				}
				resp = map[string]any{"id": cmd.ID, "type": "success", "result": result}
			}
		} else {
			resp = map[string]any{"id": cmd.ID, "type": "success", "result": map[string]any{}}
		}
		out, _ := json.Marshal(resp)
		if err := conn.Write(ctx, websocket.MessageText, out); err != nil {
			return
		}
	}
}

// handle scripts the responder for a method. errCode != "" → BiDi error reply.
func (m *mockBiDi) handle(method string, h func(params json.RawMessage) (any, string)) {
	m.mu.Lock()
	m.handlers[method] = h
	m.mu.Unlock()
}

func (m *mockBiDi) callCount(method string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls[method]
}

func (m *mockBiDi) lastParams(method string) json.RawMessage {
	m.mu.Lock()
	defer m.mu.Unlock()
	ps := m.params[method]
	if len(ps) == 0 {
		return nil
	}
	return ps[len(ps)-1]
}

// injectEvent pushes an event frame to the connected client.
func (m *mockBiDi) injectEvent(t *testing.T, method string, params any) {
	t.Helper()
	select {
	case <-m.ready:
	case <-time.After(2 * time.Second):
		t.Fatal("mockBiDi: no client connected")
	}
	frm := map[string]any{"type": "event", "method": method, "params": params}
	data, _ := json.Marshal(frm)
	m.connMu.Lock()
	conn := m.conn
	m.connMu.Unlock()
	if err := conn.Write(context.Background(), websocket.MessageText, data); err != nil {
		t.Fatalf("mockBiDi: inject event: %v", err)
	}
}

func (m *mockBiDi) wsURL() string {
	return "ws" + strings.TrimPrefix(m.srv.URL, "http")
}

// newMockBrowser dials a real bidi.Client against the mock server and wraps it
// in a Browser + one Page (contextID "ctx-1"). Closed automatically on t.Cleanup.
func newMockBrowser(t *testing.T) (*Browser, *Page, *mockBiDi) {
	t.Helper()
	m := newMockBiDi(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client, err := bidi.Dial(ctx, m.wsURL(), bidi.Options{})
	if err != nil {
		t.Fatalf("dial mock bidi: %v", err)
	}
	b := &Browser{client: client, cfg: defaultConfig()}
	b.initBaseCtx()
	p := &Page{browser: b, contextID: "ctx-1"}
	b.pages = []*Page{p}
	b.current = p
	t.Cleanup(func() { _ = b.Close() })
	return b, p, m
}
