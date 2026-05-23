// Package bidi implements a minimal WebDriver BiDi client for Firefox.
//
// BiDi is a bidirectional WebSocket + JSON protocol standardized by W3C.
// Commands carry an id; responses match by id; events have method+params
// with no id. The reader goroutine dispatches by message shape.
//
// This package is intentionally internal. Public firefoxkit API in the
// repo root wraps these primitives. Implement-on-demand: only add typed
// command wrappers when a consuming phase imports them.
package bidi
