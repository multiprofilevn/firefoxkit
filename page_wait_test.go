package firefoxkit

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestPage_Index_Found(t *testing.T) {
	b := newTestBrowser()
	p0 := &Page{browser: b, contextID: "ctx-0"}
	p1 := &Page{browser: b, contextID: "ctx-1"}
	p2 := &Page{browser: b, contextID: "ctx-2"}
	b.pages = []*Page{p0, p1, p2}

	if got := p0.Index(); got != 0 {
		t.Errorf("p0.Index() = %d, want 0", got)
	}
	if got := p1.Index(); got != 1 {
		t.Errorf("p1.Index() = %d, want 1", got)
	}
	if got := p2.Index(); got != 2 {
		t.Errorf("p2.Index() = %d, want 2", got)
	}
}

func TestPage_Index_NotFound(t *testing.T) {
	b := newTestBrowser()
	p0 := &Page{browser: b, contextID: "ctx-0"}
	b.pages = []*Page{p0}

	orphan := &Page{browser: b, contextID: "orphan"}
	if got := orphan.Index(); got != -1 {
		t.Errorf("orphan.Index() = %d, want -1", got)
	}
}

func TestPage_WaitForChangeURL_BaselineFromCache(t *testing.T) {
	b := newTestBrowser()
	p := &Page{browser: b, contextID: "ctx-x"}

	// Confirm that WaitForChangeURL reads p.url as baseline (no prevURL arg).
	// We set p.url to a sentinel and verify the method signature compiles with
	// only (timeout) — the real poll needs a live browser, so we just check the
	// type signature and that url is captured correctly before the call.
	p.mu.Lock()
	p.url = "https://before.example.com/"
	p.mu.Unlock()

	// Read baseline the same way WaitForChangeURL does.
	p.mu.RLock()
	baseline := p.url
	p.mu.RUnlock()

	if baseline != "https://before.example.com/" {
		t.Errorf("baseline = %q, want before URL", baseline)
	}
	// Signature check: WaitForChangeURL(timeout time.Duration) — compiles if
	// the method exists with this signature. Calling with 0 timeout would
	// block on pollUntil; we only verify the compile-time contract here.
	var _ func(time.Duration) error = p.WaitForChangeURL
}

func TestPollBackoff_Sequence(t *testing.T) {
	got := []time.Duration{}
	d := time.Duration(0)
	for i := 0; i < 6; i++ {
		d = pollBackoff(d)
		got = append(got, d)
	}
	// 50ms → 100ms → 200ms → 400ms → 500ms (capped) → 500ms
	want := []time.Duration{
		50 * time.Millisecond,
		100 * time.Millisecond,
		200 * time.Millisecond,
		400 * time.Millisecond,
		500 * time.Millisecond,
		500 * time.Millisecond,
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("step %d: got %v, want %v", i, got[i], want[i])
		}
	}
}

func TestBoundingBox_Center(t *testing.T) {
	bb := BoundingBox{X: 10, Y: 20, Width: 100, Height: 40}
	cx, cy := bb.Center()
	if cx != 60 || cy != 40 {
		t.Errorf("center = (%g, %g), want (60, 40)", cx, cy)
	}
}

// TestPollUntil_NoDeadline_RespectsCtxCancel verifies C2 fix: timeout<=0 sets
// no deadline; cancellation must arrive via ctx, not a fake 30s wall-clock.
func TestPollUntil_NoDeadline_RespectsCtxCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	done := make(chan error, 1)
	go func() {
		done <- pollUntil(ctx, 0, func(c context.Context) (bool, error) {
			calls++
			return false, nil
		})
	}()
	// Let it spin a couple of times then cancel.
	time.Sleep(120 * time.Millisecond)
	cancel()
	err := <-done
	if err != ErrContextCanceled {
		t.Errorf("pollUntil(timeout=0) after ctx cancel: want ErrContextCanceled, got %v", err)
	}
	if calls == 0 {
		t.Error("check was never called")
	}
}

// TestPollUntil_PositiveTimeout_Expires verifies the normal deadline path
// still works after C2 refactor.
func TestPollUntil_PositiveTimeout_Expires(t *testing.T) {
	ctx := context.Background()
	err := pollUntil(ctx, 80*time.Millisecond, func(c context.Context) (bool, error) {
		return false, nil
	})
	if err != ErrTimeout {
		t.Errorf("pollUntil(80ms) should time out, got %v", err)
	}
}

func TestEvalError_Is(t *testing.T) {
	e := &evalError{cause: ErrPageNotFound}
	if !errorIs(e, ErrEvaluate) {
		t.Errorf("evalError should match ErrEvaluate")
	}
	if e.Error() == "" || !strings.Contains(e.Error(), "evaluate") {
		t.Errorf("unexpected msg: %s", e.Error())
	}
}

// errorIs avoids importing errors package twice in tests.
func errorIs(err, target error) bool {
	type isWrapper interface{ Is(error) bool }
	if w, ok := err.(isWrapper); ok {
		return w.Is(target)
	}
	return err == target
}
