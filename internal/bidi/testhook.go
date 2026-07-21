package bidi

import "time"

// WSConn re-exports the conn-level seam so the root package's tests can inject
// a fake connection (e.g. blockable writes simulating a dead peer) into a real
// Client + wsTransport stack. Lives in internal/ — never public API surface.
type WSConn = wsConn

// NewClientForTest wires a Client over conn with the given per-frame write
// timeout. Test-only constructor for the root package; production code must
// keep using Dial.
func NewClientForTest(conn WSConn, writeTimeout time.Duration, logger Logger) *Client {
	return newClient(&wsTransport{conn: conn, writeTimeout: writeTimeout}, logger)
}
