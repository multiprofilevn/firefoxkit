package firefoxkit

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/multiprofilevn/firefoxkit/internal/bidi"
)

// newMockBrowserWithBackstop mirrors newMockBrowser but dials with a
// production-style Send backstop (DefaultCallTimeout), so tests can prove a
// caller-supplied ctx deadline takes precedence over it.
func newMockBrowserWithBackstop(t *testing.T, backstop time.Duration) (*Browser, *Page, *mockBiDi) {
	t.Helper()
	m := newMockBiDi(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client, err := bidi.Dial(ctx, m.wsURL(), bidi.Options{DefaultCallTimeout: backstop})
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

// TestEvaluateCtx_CallerDeadlineOverridesBackstop: with the 60s default
// backstop armed, an EvaluateCtx caller passing its own (tiny) deadline must
// get THAT deadline's error promptly — proving the caller ctx, not the
// backstop, governs the script.evaluate await.
func TestEvaluateCtx_CallerDeadlineOverridesBackstop(t *testing.T) {
	_, p, m := newMockBrowserWithBackstop(t, 60*time.Second)

	gate := make(chan struct{})
	release := sync.OnceFunc(func() { close(gate) })
	defer release() // unwedge the mock serve goroutine before Close
	m.handle("script.evaluate", func(json.RawMessage) (any, string) {
		<-gate // response never arrives within the caller's deadline
		return map[string]any{"type": "success", "realm": "r"}, ""
	})

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	start := time.Now()
	err := p.EvaluateCtx(ctx, `new Promise(() => {})`, nil)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("EvaluateCtx: expected deadline error, got nil")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("EvaluateCtx: expected DeadlineExceeded in chain, got %v", err)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("EvaluateCtx returned after %v — caller deadline did not govern", elapsed)
	}
}

// TestEvaluate_NoCtx_StillWorks: the ctx-less Evaluate keeps working through
// the mock (delegates to EvaluateCtx with the browser base ctx).
func TestEvaluate_NoCtx_StillWorks(t *testing.T) {
	_, p, m := newMockBrowser(t)
	m.handle("script.evaluate", func(json.RawMessage) (any, string) {
		return map[string]any{
			"type":   "success",
			"realm":  "r",
			"result": map[string]any{"type": "number", "value": 42},
		}, ""
	})

	var out float64
	if err := p.Evaluate(`42`, &out); err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if out != 42 {
		t.Fatalf("Evaluate result = %v, want 42", out)
	}
}
