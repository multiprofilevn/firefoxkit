//go:build windows

package input

import "testing"

// TestCSSToScreen_AbsoluteOffsetConvention documents the coordinate frame the
// native backend uses after the wiring fix: Window.X/Y = 0 and ContentOffset
// carries the ABSOLUTE screen position of the page's content origin. css(0,0)
// must therefore map straight onto the content origin with no window-position
// term to double-count.
func TestCSSToScreen_AbsoluteOffsetConvention(t *testing.T) {
	win := Window{X: 0, Y: 0, ContentOffsetX: 1208, ContentOffsetY: 404, Scale: 1.5}

	sx, sy := cssToScreen(0, 0, win)
	if sx != 1208 || sy != 404 {
		t.Fatalf("css(0,0) should map to content origin (1208,404), got (%g,%g)", sx, sy)
	}
	sx, sy = cssToScreen(100, 200, win)
	if sx != 1208+150 || sy != 404+300 {
		t.Fatalf("css(100,200)@1.5 → (1358,704), got (%g,%g)", sx, sy)
	}
}

// TestCSSScreen_RoundTrip verifies screenToCSS is the exact inverse of
// cssToScreen for the absolute-offset frame — the invariant the OSPositionCSS
// seeding relies on (a broken inverse leaks phantom drift into the bezier).
func TestCSSScreen_RoundTrip(t *testing.T) {
	win := Window{X: 0, Y: 0, ContentOffsetX: 1208, ContentOffsetY: 404, Scale: 1.5}
	for _, p := range [][2]float64{{0, 0}, {100, 200}, {37.5, 980}, {1919, 1079}} {
		sx, sy := cssToScreen(p[0], p[1], win)
		cx, cy := screenToCSS(sx, sy, win)
		if cx != p[0] || cy != p[1] {
			t.Fatalf("round-trip css(%g,%g) → screen(%g,%g) → css(%g,%g)", p[0], p[1], sx, sy, cx, cy)
		}
	}
}
