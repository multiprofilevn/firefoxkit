//go:build windows

package firefoxkit

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"

	"github.com/multiprofilevn/firefoxkit/input"
)

// navKeyRecorder is a fake input.Backend that records every dispatched key
// event as a short readable string, so a test can assert the exact
// Ctrl+L -> Ctrl+A -> chars -> End -> Enter sequence navigateNative sends.
// SetWindow/mouse methods are no-ops — Navigate never touches them.
type navKeyRecorder struct {
	mu     sync.Mutex
	events []string
	n      int
	// failOnEvent, if > 0, makes the N-th recorded event (1-indexed) return
	// input.ErrForegroundLost instead of nil — simulates the foreground
	// guard tripping mid-sequence (test 5).
	failOnEvent int
}

func (r *navKeyRecorder) record(ev string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.n++
	r.events = append(r.events, ev)
	if r.failOnEvent > 0 && r.n == r.failOnEvent {
		return input.ErrForegroundLost
	}
	return nil
}

func (r *navKeyRecorder) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.events))
	copy(out, r.events)
	return out
}

func comboName(vk uint16, mods input.Modifier) string {
	name := map[uint16]string{
		input.VKL: "L", input.VKA: "A", input.VKEnd: "End", input.VKReturn: "Enter",
	}[vk]
	if name == "" {
		name = fmt.Sprintf("vk0x%02X", vk)
	}
	if mods&input.ModCtrl != 0 {
		return "Ctrl+" + name
	}
	return name
}

func (r *navKeyRecorder) MouseMove(context.Context, float64, float64) error { return nil }
func (r *navKeyRecorder) MouseDown(context.Context, float64, float64, input.MouseButton) error {
	return nil
}
func (r *navKeyRecorder) MouseUp(context.Context, float64, float64, input.MouseButton) error {
	return nil
}
func (r *navKeyRecorder) MouseScroll(context.Context, float64, float64, float64, float64) error {
	return nil
}
func (r *navKeyRecorder) KeyChar(_ context.Context, ch rune) error {
	return r.record("char:" + string(ch))
}
func (r *navKeyRecorder) InsertText(_ context.Context, s string) error {
	return r.record("insert:" + s)
}
func (r *navKeyRecorder) KeyCombo(_ context.Context, vk uint16, mods input.Modifier) error {
	return r.record("combo:" + comboName(vk, mods))
}
func (r *navKeyRecorder) SetWindow(input.Window) {}

// alwaysCompleteEvalHandler scripts script.evaluate to always report the
// post-Enter document as loaded, so waitNavCommitted resolves on its first
// poll instead of waiting out a full backoff cycle.
func alwaysCompleteEvalHandler(json.RawMessage) (any, string) {
	return map[string]any{
		"type": "success", "realm": "r",
		"result": map[string]any{"type": "boolean", "value": true},
	}, ""
}

// neverCompleteEvalHandler scripts script.evaluate to report the document as
// never finishing — used to drive waitNavCommitted into its own ErrTimeout.
func neverCompleteEvalHandler(json.RawMessage) (any, string) {
	return map[string]any{
		"type": "success", "realm": "r",
		"result": map[string]any{"type": "boolean", "value": false},
	}, ""
}

// newNativeMockPage wires a Page with BackendNative selected and a fake
// keyboard recorder pre-set (bypassing the real winapi backend), backed by a
// real mock BiDi server for the evaluateCtx calls navigateNative makes
// (markNavToken, waitNavCommitted).
func newNativeMockPage(t *testing.T) (*Page, *navKeyRecorder, *mockBiDi) {
	t.Helper()
	b, p, m := newMockBrowser(t)
	b.cfg.inputBackend = BackendNative
	rec := &navKeyRecorder{}
	p.bkCache.bk = rec
	m.handle("script.evaluate", alwaysCompleteEvalHandler)
	return p, rec, m
}

// charEventRune extracts the rune from a "char:x" recorder event.
func charEventRune(ev string) (rune, bool) {
	const prefix = "char:"
	if len(ev) <= len(prefix) || ev[:len(prefix)] != prefix {
		return 0, false
	}
	r := []rune(ev[len(prefix):])
	if len(r) != 1 {
		return 0, false
	}
	return r[0], true
}
