package firefoxkit

import (
	"testing"
	"time"
)

// TestPage_QuerySelector_SignatureTimeout verifies that QuerySelector accepts
// (selector string, timeout time.Duration) — compile-time contract check.
func TestPage_QuerySelector_SignatureTimeout(t *testing.T) {
	var _ func(string, time.Duration) (*ElementHandle, error) = (&Page{}).QuerySelector
}

// TestPage_QuerySelectorAll_Signature verifies []*ElementHandle return type.
func TestPage_QuerySelectorAll_Signature(t *testing.T) {
	var _ func(string, time.Duration) ([]*ElementHandle, error) = (&Page{}).QuerySelectorAll
}

// TestPage_BoundingBox_Center confirms BoundingBox.Center arithmetic is correct
// (selector-based BoundingBox delegates to EvalSelector; Center is pure math).
func TestPage_BoundingBox_Center(t *testing.T) {
	bb := BoundingBox{X: 0, Y: 0, Width: 200, Height: 100}
	cx, cy := bb.Center()
	if cx != 100 || cy != 50 {
		t.Errorf("Center() = (%g, %g), want (100, 50)", cx, cy)
	}
}

// TestPage_Type_Signature verifies Type(selector, text) — two-arg form (chromekit parity).
func TestPage_Type_Signature(t *testing.T) {
	var _ func(string, string) error = (&Page{}).Type
}

// TestPage_Evaluate_Signature verifies ctx-free form.
func TestPage_Evaluate_Signature(t *testing.T) {
	var _ func(string, any) error = (&Page{}).Evaluate
}

// TestPage_EvalSelector_Signature verifies ctx-free three-arg form.
func TestPage_EvalSelector_Signature(t *testing.T) {
	var _ func(string, string, any) error = (&Page{}).EvalSelector
}

// TestPage_EvalSelectorAll_Signature verifies ctx-free three-arg form.
func TestPage_EvalSelectorAll_Signature(t *testing.T) {
	var _ func(string, string, any) error = (&Page{}).EvalSelectorAll
}

// TestPage_GetText_Signature verifies ctx-free one-selector form.
func TestPage_GetText_Signature(t *testing.T) {
	var _ func(string) (string, error) = (&Page{}).GetText
}

// TestPage_GetAttribute_Signature verifies (selector, attribute) form.
func TestPage_GetAttribute_Signature(t *testing.T) {
	var _ func(string, string) (string, error) = (&Page{}).GetAttribute
}

// TestPage_ScrollIntoView_Signature verifies ctx-free single-selector form.
func TestPage_ScrollIntoView_Signature(t *testing.T) {
	var _ func(string) error = (&Page{}).ScrollIntoView
}

// TestPage_Click_Signature verifies ctx-free single-selector form.
func TestPage_Click_Signature(t *testing.T) {
	var _ func(string) error = (&Page{}).Click
}

// TestPage_ClickElement_Signature verifies ctx-free single-selector form.
func TestPage_ClickElement_Signature(t *testing.T) {
	var _ func(string) error = (&Page{}).ClickElement
}

// TestPage_SetInputFiles_Signature verifies ctx-free variadic-paths form.
func TestPage_SetInputFiles_Signature(t *testing.T) {
	var _ func(string, ...string) error = (&Page{}).SetInputFiles
}

// TestPage_Focus_Signature verifies ctx-free single-selector form.
func TestPage_Focus_Signature(t *testing.T) {
	var _ func(string) error = (&Page{}).Focus
}

// TestPage_Wait_Signature verifies Wait(d) — no ctx (chromekit parity).
func TestPage_Wait_Signature(t *testing.T) {
	var _ func(time.Duration) error = (&Page{}).Wait
}

// TestPage_Wait_ZeroDuration returns immediately without blocking.
func TestPage_Wait_ZeroDuration(t *testing.T) {
	b := newTestBrowser()
	p := &Page{browser: b}
	if err := p.Wait(0); err != nil {
		t.Fatalf("Wait(0): unexpected error: %v", err)
	}
}

// TestPage_Wait_NegativeDuration returns immediately.
func TestPage_Wait_NegativeDuration(t *testing.T) {
	b := newTestBrowser()
	p := &Page{browser: b}
	if err := p.Wait(-1 * time.Millisecond); err != nil {
		t.Fatalf("Wait(-1ms): unexpected error: %v", err)
	}
}

// TestPage_Wait_ShortDuration blocks for the given duration then returns nil.
func TestPage_Wait_ShortDuration(t *testing.T) {
	b := newTestBrowser()
	p := &Page{browser: b}
	if err := p.Wait(10 * time.Millisecond); err != nil {
		t.Fatalf("Wait(10ms): %v", err)
	}
}

// TestElementHandle_NilDispose verifies Dispose on nil handle is a no-op.
func TestElementHandle_NilDispose(t *testing.T) {
	var h *ElementHandle
	// Dispose should not panic on nil; it short-circuits at h == nil.
	if err := h.Dispose(nil); err != nil { //nolint:staticcheck // nil ctx is intentional no-op test
		t.Fatalf("nil Dispose: %v", err)
	}
}
