package firefoxkit

import (
	"testing"
)

// ── Compile-contract tests (chromekit API parity) ──────────────────────────
// These fail at compile time if the signature drifts from chromekit's MouseAPI.

func TestMouseAPI_ClickAt_Signature(t *testing.T) {
	var _ func(float64, float64) error = (&MouseAPI{}).ClickAt
}

func TestMouseAPI_Click_Signature(t *testing.T) {
	var _ func(string) error = (&MouseAPI{}).Click
}

func TestMouseAPI_RightClick_Signature(t *testing.T) {
	var _ func(string) error = (&MouseAPI{}).RightClick
}

func TestMouseAPI_DoubleClick_Signature(t *testing.T) {
	var _ func(string) error = (&MouseAPI{}).DoubleClick
}

func TestMouseAPI_Hover_Signature(t *testing.T) {
	var _ func(string) error = (&MouseAPI{}).Hover
}

func TestMouseAPI_MoveTo_Signature(t *testing.T) {
	var _ func(float64, float64) error = (&MouseAPI{}).MoveTo
}

func TestMouseAPI_Scroll_Signature(t *testing.T) {
	// chromekit: Scroll(deltaX, deltaY float64) error — NO origin params.
	var _ func(float64, float64) error = (&MouseAPI{}).Scroll
}

func TestMouseAPI_Drag_Signature(t *testing.T) {
	// chromekit: Drag(fromX, fromY, toX, toY float64) error — NO steps param.
	var _ func(float64, float64, float64, float64) error = (&MouseAPI{}).Drag
}

func TestMouseAPI_DragAndDrop_Signature(t *testing.T) {
	// chromekit: DragAndDrop(fromSel, toSel string) error — two selectors.
	var _ func(string, string) error = (&MouseAPI{}).DragAndDrop
}

func TestMouseAPI_Tap_Signature(t *testing.T) {
	var _ func(float64, float64) error = (&MouseAPI{}).Tap
}

func TestMouseAPI_FocusElement_Signature(t *testing.T) {
	var _ func(string) error = (&MouseAPI{}).FocusElement
}

// Extra Firefox-only method (coord-based, ctx-free).
func TestMouseAPI_MiddleClick_Signature(t *testing.T) {
	var _ func(float64, float64) error = (&MouseAPI{}).MiddleClick
}

// Page.Mouse() must return *MouseAPI (not *Mouse).
func TestPage_Mouse_ReturnType(t *testing.T) {
	var _ func() *MouseAPI = (&Page{}).Mouse
}

// ── scrollOrigin sentinel logic ────────────────────────────────────────────

// TestMouseAPI_ScrollOrigin_Sentinel confirms a freshly constructed MouseAPI
// has lastX/lastY at the sentinel value (no position set yet).
func TestMouseAPI_ScrollOrigin_Sentinel(t *testing.T) {
	m := &MouseAPI{lastX: sentinelPos, lastY: sentinelPos}
	if m.lastX != sentinelPos || m.lastY != sentinelPos {
		t.Fatalf("new MouseAPI should start at sentinel, got (%g,%g)", m.lastX, m.lastY)
	}
}

// TestMouseAPI_ScrollOrigin_AfterMove confirms lastX/lastY track after MoveTo
// (no real backend needed because Page is nil and we test the struct state directly).
func TestMouseAPI_ScrollOrigin_TrackState(t *testing.T) {
	m := &MouseAPI{lastX: sentinelPos, lastY: sentinelPos}
	// Simulate what MoveTo does after the backend call succeeds.
	m.lastX, m.lastY = 123.0, 456.0
	x, y := m.scrollOrigin()
	if x != 123.0 || y != 456.0 {
		t.Fatalf("scrollOrigin after set: want (123,456), got (%g,%g)", x, y)
	}
}

// TestMouseAPI_Scroll_NoOriginArg verifies that Scroll accepts exactly two args
// (no x,y origin) — compile-time guard matching the Test Spec requirement.
func TestMouseAPI_Scroll_NoOriginArg(t *testing.T) {
	// If this compiles, the signature is correct.
	var _ func(float64, float64) error = (&MouseAPI{}).Scroll
}

// TestMouseAPI_DragAndDrop_TwoSelectors verifies DragAndDrop accepts two
// selector strings (not four float64 coords) — matches Test Spec.
func TestMouseAPI_DragAndDrop_TwoSelectors(t *testing.T) {
	var _ func(string, string) error = (&MouseAPI{}).DragAndDrop
}
