package firefoxkit

import (
	"sync"
	"sync/atomic"

	"github.com/tuwibu/firefoxkit/input"
)

type backendCache struct {
	once sync.Once
	bk   input.Backend
	// ready flips true once bk is initialized — lets cleanup() release input
	// state without forcing construction of a backend that was never used.
	ready atomic.Bool
}

// inputBackend returns the input.Backend bound to this page. Lazy-initialized.
// Native path builds Window from the browser's WithNativeWindow option;
// content-offset measurement via HWND is up to the caller (use Browser.HWND +
// input.MeasureContentOffset on Windows).
func (p *Page) inputBackend() input.Backend {
	p.bkCache.once.Do(func() {
		if p.bkCache.bk == nil { // may be pre-set by tests (fake backend injection)
			switch p.browser.cfg.inputBackend {
			case BackendNative:
				p.bkCache.bk = input.NewNative(input.Config{Window: p.nativeWindow()})
			default:
				p.bkCache.bk = input.NewBiDi(p.browser.client, p.contextID)
			}
		}
		p.bkCache.ready.Store(true)
	})
	return p.bkCache.bk
}

// nativeWindow resolves the coordinate frame for the native backend. It
// measures the live content origin + DPI scale from the Firefox HWND so
// css(0,0) maps onto the rendered content (not the title bar). Falls back to
// the manual WithNativeWindow values when the HWND is unavailable (e.g. the
// Connect path before a window exists, or non-Windows).
func (p *Page) nativeWindow() input.Window {
	ox, oy, err := p.browser.ContentOffset()
	if err != nil {
		return buildNativeWindow(p.browser.cfg.nativeWindow, 0, 0, 0, false)
	}
	scale, serr := p.browser.WindowScale()
	if serr != nil || scale <= 0 {
		scale = p.browser.cfg.nativeWindow.Scale
	}
	return buildNativeWindow(p.browser.cfg.nativeWindow, ox, oy, scale, true)
}

// buildNativeWindow chooses the coordinate frame for the native backend.
// measured=true → content origin carried as an ABSOLUTE screen position in
// ContentOffsetX/Y with X=Y=0, so cssToScreen has no window-position term to
// double-count. measured=false → use the manual WithNativeWindow values.
func buildNativeWindow(manual nativeWindow, ox, oy int, scale float64, measured bool) input.Window {
	if measured {
		if scale <= 0 {
			scale = 1.0
		}
		return input.Window{X: 0, Y: 0, ContentOffsetX: ox, ContentOffsetY: oy, Scale: scale}
	}
	s := manual.Scale
	if s <= 0 {
		s = 1.0
	}
	return input.Window{X: manual.X, Y: manual.Y, Scale: s}
}

// cursorEngine returns the human-like cursor layer over inputBackend(). Lazy.
// Mirrors chromekit's initInput: every MouseAPI primitive routes through the
// Cursor so clicks glide along a bezier path (ProfileHuman) instead of
// teleporting. The movement style comes from WithCursorProfile. A pre-set
// p.cursor (test injection) is honored as-is.
func (p *Page) cursorEngine() *input.Cursor {
	p.cursorOnce.Do(func() {
		if p.cursor != nil {
			return
		}
		profile := input.ProfileHuman
		switch p.browser.cfg.cursorProfile {
		case CursorFast:
			profile = input.ProfileFast
		case CursorMachine:
			profile = input.ProfileMachine
		}
		p.cursor = input.NewCursorWithProfileAndPath(p.inputBackend(), profile, p.browser.cfg.mousePathURL)
	})
	return p.cursor
}
