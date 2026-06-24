package firefoxkit

import (
	"context"
	"testing"
	"time"
)

// newTestBrowser builds a Browser with only the base context wired — no BiDi
// client. Enough to exercise the ctx/opCtx plumbing in isolation.
func newTestBrowser() *Browser {
	b := &Browser{}
	b.initBaseCtx()
	return b
}

func TestPage_OpCtx_TimeoutDerives(t *testing.T) {
	b := newTestBrowser()
	p := &Page{browser: b}

	ctx, cancel := p.opCtx(50 * time.Millisecond)
	defer cancel()
	dl, ok := ctx.Deadline()
	if !ok {
		t.Fatal("opCtx(50ms): expected a deadline, got none")
	}
	if d := time.Until(dl); d <= 0 || d > 60*time.Millisecond {
		t.Fatalf("opCtx(50ms): deadline out of range: %v", d)
	}

	noDL, cancel2 := p.opCtx(0)
	defer cancel2()
	if _, ok := noDL.Deadline(); ok {
		t.Fatal("opCtx(0): expected no deadline")
	}
}

func TestBrowser_Close_CancelsBaseCtx(t *testing.T) {
	b := newTestBrowser()
	if err := b.baseCtx.Err(); err != nil {
		t.Fatalf("fresh base ctx should not be cancelled: %v", err)
	}
	// Close on a client-less browser exercises only the cancel path.
	_ = b.Close()
	if err := b.baseCtx.Err(); err != context.Canceled {
		t.Fatalf("after Close: want context.Canceled, got %v", err)
	}
}

func TestPage_Ctx_SharesBrowserBase(t *testing.T) {
	b := newTestBrowser()
	p := &Page{browser: b}
	if p.ctx() != b.ctx() {
		t.Fatal("page ctx should be the browser base ctx")
	}
}

func TestPage_Ctx_NilSafe(t *testing.T) {
	var p *Page
	if p.ctx() != context.Background() {
		t.Fatal("nil page ctx should fall back to context.Background()")
	}
}
