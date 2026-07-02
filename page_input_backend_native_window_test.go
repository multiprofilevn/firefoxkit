package firefoxkit

import "testing"

// TestBuildNativeWindow_Measured: when the live HWND measurement succeeds, the
// content origin is carried as an ABSOLUTE screen position in ContentOffset
// with X=Y=0 — so cssToScreen has no window-position term to double-count
// (the dormant bug: native init previously left ContentOffset at 0).
func TestBuildNativeWindow_Measured(t *testing.T) {
	w := buildNativeWindow(nativeWindow{X: 100, Y: 200, Scale: 1.0}, 1208, 404, 1.5, true)
	if w.X != 0 || w.Y != 0 {
		t.Fatalf("measured frame must zero X/Y to avoid double-count, got (%d,%d)", w.X, w.Y)
	}
	if w.ContentOffsetX != 1208 || w.ContentOffsetY != 404 {
		t.Fatalf("ContentOffset should carry absolute origin, got (%d,%d)", w.ContentOffsetX, w.ContentOffsetY)
	}
	if w.Scale != 1.5 {
		t.Fatalf("Scale should be the measured DPI scale, got %g", w.Scale)
	}
}

// TestBuildNativeWindow_FallbackManual: when measurement fails (non-Windows or
// HWND unavailable), fall back to the manual WithNativeWindow values.
func TestBuildNativeWindow_FallbackManual(t *testing.T) {
	w := buildNativeWindow(nativeWindow{X: 100, Y: 200, Scale: 1.25}, 0, 0, 0, false)
	if w.X != 100 || w.Y != 200 {
		t.Fatalf("fallback should keep manual X/Y, got (%d,%d)", w.X, w.Y)
	}
	if w.Scale != 1.25 {
		t.Fatalf("fallback should keep manual scale, got %g", w.Scale)
	}
	if w.ContentOffsetX != 0 || w.ContentOffsetY != 0 {
		t.Fatalf("fallback has no measured offset, got (%d,%d)", w.ContentOffsetX, w.ContentOffsetY)
	}
}

// TestBuildNativeWindow_FallbackZeroScaleDefaults: a manual scale of 0 must
// default to 1.0 (never a degenerate zero-scale transform).
func TestBuildNativeWindow_FallbackZeroScaleDefaults(t *testing.T) {
	w := buildNativeWindow(nativeWindow{X: 0, Y: 0, Scale: 0}, 0, 0, 0, false)
	if w.Scale != 1.0 {
		t.Fatalf("zero manual scale should default to 1.0, got %g", w.Scale)
	}
}
