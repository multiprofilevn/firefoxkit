package firefoxkit

import (
	"sync"

	"github.com/tuwibu/firefoxkit/input"
)

type backendCache struct {
	once sync.Once
	bk   input.Backend
}

// inputBackend returns the input.Backend bound to this page. Lazy-initialized.
// Native path builds Window from the browser's WithNativeWindow option;
// content-offset measurement via HWND is up to the caller (use Browser.HWND +
// input.MeasureContentOffset on Windows).
func (p *Page) inputBackend() input.Backend {
	p.bkCache.once.Do(func() {
		switch p.browser.cfg.inputBackend {
		case BackendNative:
			win := input.Window{
				X:     p.browser.cfg.nativeWindow.X,
				Y:     p.browser.cfg.nativeWindow.Y,
				Scale: p.browser.cfg.nativeWindow.Scale,
			}
			p.bkCache.bk = input.NewNative(input.Config{Window: win})
		default:
			p.bkCache.bk = input.NewBiDi(p.browser.client, p.contextID)
		}
	})
	return p.bkCache.bk
}
