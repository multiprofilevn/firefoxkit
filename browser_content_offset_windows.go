//go:build windows

package firefoxkit

import (
	"github.com/tuwibu/firefoxkit/input"
)

// chromeAreaCorrection is the seam over the measured chrome-area height (title
// bar + tab strip + URL bar) that sits between the Win32 client origin and the
// first pixel of rendered content. It returns ok=false when no measurement is
// available, in which case the coarse DefaultFirefoxContentOffsetY constant is
// used instead.
//
// Firefox deliberately keeps GetClientRect + ClientToScreen as the authoritative
// base and lets a page-side measurement refine only this one term. Moving the
// whole measurement into the page realm — the way Chrome is forced to — would
// trade a trusted number for an attacker-controlled one to win ±20px.
var chromeAreaCorrection = measureChromeAreaCorrection

// measureChromeAreaCorrection asks the content window how far below the window
// top it starts. mozInnerScreenY is the screen Y of the content area in CSS
// pixels and screenY the window's own top, so the difference — scaled to device
// pixels — is the chrome-area height Firefox is currently rendering, toolbars,
// theme and bookmark bar included.
//
// Untrusted, like anything read out of a page realm; the caller clamps it and
// keeps the Win32 client origin as the base regardless of what comes back.
func measureChromeAreaCorrection(b *Browser) (int, bool) {
	// No session means no page realm to ask — the Connect path before the BiDi
	// client is up, and every non-browser-backed Browser a test assembles.
	if b == nil || b.client == nil {
		return 0, false
	}
	page := b.Current()
	if page == nil {
		return 0, false
	}
	var px float64
	const expr = `(() => {
		const dpr = window.devicePixelRatio || 1;
		return Math.round((window.mozInnerScreenY - window.screenY) * dpr);
	})()`
	if err := page.Evaluate(expr, &px); err != nil {
		b.warnf("firefoxkit: chrome-area measurement failed (%v) — using the %dpx default",
			err, input.DefaultFirefoxContentOffsetY)
		return 0, false
	}
	return int(px), true
}

// clientOriginOf is the seam over the trusted Win32 base. Tests replace it so
// the base is an input rather than a property of the machine's window layout.
var clientOriginOf = input.MeasureClientOrigin

// chromeAreaCorrectionMax is the upper bound on the correction. Anything above
// it — or more than the configured tolerance below zero — is a bad measurement
// (or a hostile one) and is discarded in favour of the constant.
const chromeAreaCorrectionMax = 200

// ContentOffset returns the ABSOLUTE screen position of the page's CSS (0,0) —
// the top-left pixel of the rendered web content. Same semantics as
// chromekit.Browser.ContentOffset.
//
// The base comes from Win32 (GetClientRect + ClientToScreen) and is trusted;
// only the chrome-area height on top of it is refined by a measurement.
func (b *Browser) ContentOffset() (int, int, error) {
	hwnd, err := b.HWND()
	if err != nil {
		return 0, 0, err
	}
	cx, cy, err := clientOriginOf(hwnd)
	if err != nil {
		return 0, 0, err
	}
	return cx, cy + b.chromeAreaHeight(), nil
}

// chromeAreaHeight returns the number of screen pixels between the Win32 client
// origin and the first pixel of rendered content.
//
// The measured value wins when it is plausible. Out-of-range means the
// measurement is broken or hostile, and the coarse constant — wrong by ~20px —
// beats a number that can be dictated by page script. The discard is logged
// because a silent fallback turns a measurement bug into a permanent click
// offset nobody can trace back to anything.
func (b *Browser) chromeAreaHeight() int {
	corr, ok := chromeAreaCorrection(b)
	if !ok {
		return input.DefaultFirefoxContentOffsetY
	}
	// A slightly negative correction is rounding at the client edge, not a tall
	// toolbar upside down; the tolerance absorbs it, anything beyond is junk.
	if corr < -b.contentOffsetTolerance() || corr > chromeAreaCorrectionMax {
		b.warnf("firefoxkit: measured chrome-area height %dpx outside [-%d, %d] — discarding it and using the %dpx default",
			corr, b.contentOffsetTolerance(), chromeAreaCorrectionMax, input.DefaultFirefoxContentOffsetY)
		return input.DefaultFirefoxContentOffsetY
	}
	if corr < 0 {
		return 0
	}
	return corr
}

// contentOffsetTolerance is the slack allowed on measurements taken inside the
// page realm. See WithContentOffsetTolerance.
func (b *Browser) contentOffsetTolerance() int {
	if b.cfg != nil && b.cfg.contentOffsetTolerance > 0 {
		return b.cfg.contentOffsetTolerance
	}
	return DefaultContentOffsetTolerance
}

// warnf logs at Warn level, tolerating a Browser assembled without a logger.
func (b *Browser) warnf(format string, args ...any) {
	if b.log != nil {
		b.log.Warnf(format, args...)
	}
}

// WindowRect returns the Firefox window's outer rectangle in absolute screen
// pixels, straight from the window manager. Callers use it as the out-of-band
// bound on coordinates that were measured inside the page.
func (b *Browser) WindowRect() (input.Rect, error) {
	hwnd, err := b.HWND()
	if err != nil {
		return input.Rect{}, err
	}
	return input.MeasureWindowRect(hwnd)
}

// InvalidateNativeWindow re-measures the coordinate frame (content origin + DPI
// scale) and pushes it into the live input backends via Backend.SetWindow.
//
// Call it after anything that can move or rescale the window. The backend
// objects are updated IN PLACE — no Page field is ever reassigned — which is
// what makes the update reach the cursor engine and the keyboard as well: both
// captured the backend behind their own sync.Once and would otherwise keep
// serving the stale frame.
func (b *Browser) InvalidateNativeWindow() error {
	ox, oy, err := b.ContentOffset()
	if err != nil {
		return err
	}
	scale, serr := b.WindowScale()
	if serr != nil || scale <= 0 {
		scale = 1.0
	}
	var manual nativeWindow
	if b.cfg != nil {
		manual = b.cfg.nativeWindow
	}
	w := buildNativeWindow(manual, ox, oy, scale, true)
	for _, p := range b.Pages() {
		p.setInputWindow(w)
	}
	return nil
}
