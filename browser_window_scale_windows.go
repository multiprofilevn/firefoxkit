//go:build windows

package firefoxkit

import (
	"math"

	"github.com/tuwibu/firefoxkit/input"
)

// windowScaleMetrics is what the page realm says about the ratio between its
// own CSS pixels and physical screen pixels, plus the window size it measured
// that ratio against.
//
// UNTRUSTED. Every field is a property of the page's `window` object and script
// can redefine all of them — window.devicePixelRatio included. Nothing here is
// used to aim the physical cursor until an out-of-band measurement has agreed
// with it.
type windowScaleMetrics struct {
	DevicePixelRatio float64 `json:"devicePixelRatio"`
	OuterWidth       int     `json:"outerWidth"`
	OuterHeight      int     `json:"outerHeight"`
}

// Seams over the two independent sources WindowScale draws on. Tests replace
// them so the spoofing case can be exercised without a real browser, and so the
// trusted bound can be moved independently of the untrusted claim. Named to
// match chromekit's, because the two kits implement the same contract.
var (
	pageWindowScaleMetrics = readPageWindowScaleMetrics
	browserWindowRect      = readBrowserWindowRect
)

// readPageWindowScaleMetrics asks the content window for its own scale.
// ok=false whenever there is no page realm to ask — the Connect path before the
// BiDi client is up, and every non-browser-backed Browser a test assembles.
func readPageWindowScaleMetrics(b *Browser) (windowScaleMetrics, bool) {
	if b == nil || b.client == nil {
		return windowScaleMetrics{}, false
	}
	page := b.Current()
	if page == nil {
		return windowScaleMetrics{}, false
	}
	var m windowScaleMetrics
	const expr = `({
		devicePixelRatio: window.devicePixelRatio,
		outerWidth: window.outerWidth, outerHeight: window.outerHeight,
	})`
	if err := page.Evaluate(expr, &m); err != nil {
		return windowScaleMetrics{}, false
	}
	return m, true
}

// readBrowserWindowRect reads the window rectangle from the window manager.
func readBrowserWindowRect(b *Browser) (input.Rect, error) {
	return b.WindowRect()
}

// windowScaleTolerance is the fraction by which the page's own
// devicePixelRatio may differ from the independent GetWindowRect estimate of
// the same ratio before the page value is refused.
//
// 2%, and the number is deliberately far above what was measured. Four real
// windows (Firefox and Chrome, each at forced scale 0.3333 and at 1.0) all
// agreed to the last digit the print carried — |dpr - rect/outerWidth| =
// 0.000000, 0.0000% relative, on every one. On this kit's own run:
// devPixelsPerPx 0.3333 gave outer 1920x1080 against a GetWindowRect of exactly
// 640x360. The only error a healthy window can produce is the browser rounding
// outerWidth to a whole CSS pixel, which caps the deviation at
// dpr*0.5/outerWidth — 0.000087 at 1920 CSS px wide. 2% is over two hundred
// times that bound and still two orders of magnitude below the failure this
// guards against, where the two numbers differ by a whole factor (1.0 vs
// 0.3333 — 200% apart).
const windowScaleTolerance = 0.02

// Plausible bounds on a device scale factor. Firefox's
// layout.css.devPixelsPerPx and Chrome's --force-device-scale-factor both live
// well inside this range, as does every shipping OS DPI setting; a page
// reporting outside it is not describing a real window.
const (
	minPlausibleScale = 0.1
	maxPlausibleScale = 10.0
)

// resolveWindowScale returns the CSS->physical-pixel ratio to map coordinates
// with, given the page's own claim and an out-of-band window rectangle.
//
// window.devicePixelRatio is BY DEFINITION that ratio, and is the only source
// that stays right in both cases that matter: with layout.css.devPixelsPerPx
// set the browser ignores OS DPI entirely, and without it it follows it. The OS
// DPI ratio alone is right only in the second case — which is exactly the bug
// this function exists to close. Measured on a real 9-window native run: a
// window tiled into a 640px cell at devPixelsPerPx 0.333 has a 1872px CSS
// viewport, and a CSS x of ~917 multiplied by the OS ratio 1.0 instead of 0.333
// aimed the physical cursor at x=2197 on a 1920-wide screen.
//
// It is also read from the page realm, where script can redefine it, so it is
// never trusted on its own. physicalWindowWidth / cssOuterWidth estimates the
// same ratio from a number the window manager owns; the page value is accepted
// only when the two independent estimates agree, and the trusted OS DPI ratio
// is used otherwise. The fallback is logged with both numbers, because a silent
// downgrade here turns into a permanent, untraceable click offset.
func resolveWindowScale(m windowScaleMetrics, rect input.Rect, osScale float64, warnf func(string, ...any)) float64 {
	if osScale <= 0 {
		osScale = 1.0
	}
	physW := float64(rect.Right - rect.Left)
	physH := float64(rect.Bottom - rect.Top)
	dpr := m.DevicePixelRatio

	if math.IsNaN(dpr) || dpr < minPlausibleScale || dpr > maxPlausibleScale ||
		m.OuterWidth < 1 || m.OuterHeight < 1 || physW < 1 || physH < 1 {
		warnf("firefoxkit: page-reported devicePixelRatio %v with outer size %dx%d and window rect %.0fx%.0f is not a usable measurement — using the OS DPI ratio %g",
			dpr, m.OuterWidth, m.OuterHeight, physW, physH, osScale)
		return osScale
	}

	estW := physW / float64(m.OuterWidth)
	estH := physH / float64(m.OuterHeight)
	if scaleAgrees(dpr, estW) && scaleAgrees(dpr, estH) {
		return dpr
	}
	warnf("firefoxkit: page-reported devicePixelRatio %g disagrees with the window-rect estimate (%g across, %g down, tolerance %g%%) — using the OS DPI ratio %g instead",
		dpr, estW, estH, windowScaleTolerance*100, osScale)
	return osScale
}

// scaleAgrees reports whether two independent estimates of the same ratio are
// within windowScaleTolerance of each other, relative to the trusted one.
func scaleAgrees(dpr, est float64) bool {
	if est <= 0 {
		return false
	}
	return math.Abs(dpr-est) <= windowScaleTolerance*est
}

// WindowScale returns the ratio between the page's CSS pixels and physical
// screen pixels — the number the native input backend multiplies CSS
// coordinates by.
//
// This is NOT simply the OS DPI ratio. With layout.css.devPixelsPerPx set
// (WithScaleFactor) Firefox ignores OS DPI and renders at the configured value
// instead, so GetDpiForWindow answers a question nobody asked. The page's own
// devicePixelRatio covers both cases, and is cross-checked against
// GetWindowRect before it is believed — see resolveWindowScale.
//
// Measured from the live window rather than taken from the caller, so it stays
// correct after the window moves to a monitor with different scaling. Falls
// back to the OS DPI ratio, without complaint, whenever there is no page to ask
// yet — the launch path, before the first navigation.
func (b *Browser) WindowScale() (float64, error) {
	hwnd, err := b.HWND()
	if err != nil {
		return 0, err
	}
	osScale := input.MeasureDPIScale(hwnd)

	m, ok := pageWindowScaleMetrics(b)
	if !ok {
		return osScale, nil
	}
	rect, rerr := browserWindowRect(b)
	if rerr != nil {
		return osScale, nil
	}
	return resolveWindowScale(m, rect, osScale, b.warnf), nil
}
