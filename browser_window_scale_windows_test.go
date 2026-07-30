//go:build windows

package firefoxkit

import (
	"strings"
	"testing"

	"github.com/tuwibu/firefoxkit/input"
)

// The numbers in this file are not invented. They come from a real Firefox
// window launched with layout.css.devPixelsPerPx = 0.3333 (WithScaleFactor) and
// tiled to a 640x360 cell, read back in the same breath from the page realm and
// from GetWindowRect:
//
//	dpr=0.333333  outer=1920x1080  inner=1872x966  screen=(1920,1080)
//	GetWindowRect=(640,360)-(1280,720)  →  640x360 physical
//	GetDpiForWindow/96 = 1.0
//
// That 1872px CSS viewport inside a 640px window is the whole bug: the kit used
// the OS DPI ratio, so a CSS x of ~917 was multiplied by 1.0 instead of 0.3333
// and the physical cursor was aimed at x=2197 on a 1920-wide screen —
//
//	cursor drift detected: expected (2197,243) got (1919,243)

const oneThird = 1.0 / 3.0

// realForcedScaleMetrics is the page-side half of the measurement above.
func realForcedScaleMetrics() windowScaleMetrics {
	return windowScaleMetrics{DevicePixelRatio: oneThird, OuterWidth: 1920, OuterHeight: 1080}
}

// realForcedScaleRect is the window-manager half of the same measurement.
func realForcedScaleRect() input.Rect {
	return input.Rect{Left: 640, Top: 360, Right: 1280, Bottom: 720}
}

// captureWarn collects warning text so a test can assert the fallback was
// announced rather than taken silently.
func captureWarn(dst *[]string) func(string, ...any) {
	return func(format string, _ ...any) { *dst = append(*dst, format) }
}

// devPixelsPerPx is invisible to GetDpiForWindow, so the OS DPI ratio says 1.0
// while the window is really rendering at a third. The page's own
// devicePixelRatio is the only source that knows, and the window rectangle
// confirms it independently.
func TestResolveWindowScale_PrefersForcedFactorOverOSDPI(t *testing.T) {
	var warns []string
	got := resolveWindowScale(realForcedScaleMetrics(), realForcedScaleRect(), 1.0, captureWarn(&warns))
	if got != oneThird {
		t.Fatalf("resolveWindowScale = %g, want %g — devPixelsPerPx was ignored", got, oneThird)
	}
	if len(warns) != 0 {
		t.Fatalf("agreeing measurements should not warn, got %v", warns)
	}
}

// Without devPixelsPerPx the two sources agree on the OS DPI ratio anyway, so
// nothing changes for the ordinary case.
func TestResolveWindowScale_UnforcedWindowKeepsOSDPIRatio(t *testing.T) {
	// Real unforced measurement: outer 640x360, rect (640,360)-(1280,720).
	m := windowScaleMetrics{DevicePixelRatio: 1, OuterWidth: 640, OuterHeight: 360}
	var warns []string
	if got := resolveWindowScale(m, realForcedScaleRect(), 1.0, captureWarn(&warns)); got != 1.0 {
		t.Fatalf("resolveWindowScale = %g, want 1.0", got)
	}
	if len(warns) != 0 {
		t.Fatalf("agreeing measurements should not warn, got %v", warns)
	}
}

// The security case. devicePixelRatio is read from the page realm and script can
// redefine it; a large one multiplies every CSS coordinate on its way to the
// PHYSICAL cursor, so believing it unchecked hands page JavaScript a lever on
// the real mouse. GetWindowRect disagrees, so the trusted OS ratio wins.
//
// Anyone tempted to drop the cross-check "because devicePixelRatio is the
// definition of the ratio" should note that being the right definition and
// being an honest number are different properties.
func TestResolveWindowScale_RefusesSpoofedDevicePixelRatio(t *testing.T) {
	m := realForcedScaleMetrics()
	m.DevicePixelRatio = 4.0 // window really renders at 1/3

	var warns []string
	got := resolveWindowScale(m, realForcedScaleRect(), 1.0, captureWarn(&warns))
	if got != 1.0 {
		t.Fatalf("resolveWindowScale = %g, want the OS DPI ratio 1.0 — a spoofed devicePixelRatio was believed", got)
	}
	if len(warns) == 0 {
		t.Fatal("fallback to the OS DPI ratio must be logged, not taken silently")
	}
	if !strings.Contains(warns[0], "disagrees") {
		t.Fatalf("warning %q should name the disagreement", warns[0])
	}
}

// A page that lies about its size to make a lie about the ratio self-consistent
// still cannot win: shrinking outerWidth to match a spoofed dpr changes the
// estimate too, and the two only agree when the page told the truth. This case
// pins the opposite direction — outer size spoofed, dpr honest.
func TestResolveWindowScale_RefusesSpoofedOuterSize(t *testing.T) {
	m := realForcedScaleMetrics()
	m.OuterWidth = 10 // would make the estimate 64 instead of 0.333

	var warns []string
	if got := resolveWindowScale(m, realForcedScaleRect(), 1.0, captureWarn(&warns)); got != 1.0 {
		t.Fatalf("resolveWindowScale = %g, want the OS DPI ratio 1.0", got)
	}
	if len(warns) == 0 {
		t.Fatal("fallback must be logged")
	}
}

// Tolerance band. The only deviation a healthy window produces is the browser
// rounding outerWidth to a whole CSS pixel; anything approaching a whole factor
// is a different window being described.
func TestResolveWindowScale_ToleranceBand(t *testing.T) {
	rect := input.Rect{Left: 0, Top: 0, Right: 1000, Bottom: 500}
	cases := []struct {
		name string
		dpr  float64
		want float64
	}{
		{"exact", 1.0, 1.0},
		{"within tolerance low", 0.99, 0.99},
		{"within tolerance high", 1.01, 1.01},
		{"just inside the tolerance edge", 1.019, 1.019},
		{"just past the edge", 1.021, 2.5},
		{"whole factor off", 3.0, 2.5},
		{"absurdly large", 1e6, 2.5},
		{"zero — no measurement", 0, 2.5},
		{"negative", -1, 2.5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := windowScaleMetrics{DevicePixelRatio: tc.dpr, OuterWidth: 1000, OuterHeight: 500}
			var warns []string
			// osScale is a value no honest path produces, so a pass cannot be
			// confused with the page value being echoed back.
			if got := resolveWindowScale(m, rect, 2.5, captureWarn(&warns)); got != tc.want {
				t.Fatalf("resolveWindowScale(dpr=%g) = %g, want %g", tc.dpr, got, tc.want)
			}
		})
	}
}

// A rectangle with no area cannot estimate anything, so the page value is not
// checkable and the trusted ratio is used.
func TestResolveWindowScale_DegenerateRect(t *testing.T) {
	var warns []string
	if got := resolveWindowScale(realForcedScaleMetrics(), input.Rect{}, 1.25, captureWarn(&warns)); got != 1.25 {
		t.Fatalf("resolveWindowScale = %g, want the OS DPI ratio 1.25", got)
	}
	if len(warns) == 0 {
		t.Fatal("an uncheckable page value must be logged when it is discarded")
	}
}

// WindowScale prefers the page value only after the rectangle has agreed, and
// keeps working — quietly — when there is no page realm to ask at all. Both
// paths go through the seams, so no browser is launched here.
func TestWindowScale_SeamsAgreeAndFallBack(t *testing.T) {
	prevMetrics, prevRect := pageWindowScaleMetrics, browserWindowRect
	t.Cleanup(func() { pageWindowScaleMetrics, browserWindowRect = prevMetrics, prevRect })

	b := &Browser{log: noopLogger{}, cfg: defaultConfig(), hwndOverride: 0x1234}
	browserWindowRect = func(*Browser) (input.Rect, error) { return realForcedScaleRect(), nil }

	pageWindowScaleMetrics = func(*Browser) (windowScaleMetrics, bool) { return realForcedScaleMetrics(), true }
	got, err := b.WindowScale()
	if err != nil {
		t.Fatalf("WindowScale: %v", err)
	}
	if got != oneThird {
		t.Fatalf("WindowScale = %g, want %g", got, oneThird)
	}

	// No page realm yet — the launch path. The OS ratio is the right answer and
	// it is not an error.
	pageWindowScaleMetrics = func(*Browser) (windowScaleMetrics, bool) { return windowScaleMetrics{}, false }
	got, err = b.WindowScale()
	if err != nil {
		t.Fatalf("WindowScale without a page: %v", err)
	}
	if got != 1.0 {
		t.Fatalf("WindowScale without a page = %g, want the OS DPI ratio 1.0", got)
	}
}
