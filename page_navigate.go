package firefoxkit

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/tuwibu/firefoxkit/internal/bidi"
)

// Navigate drives the page to url and waits for `complete` readiness.
// timeout <= 0 means no deadline (base context only).
//
// When the page's input backend is Native, navigation goes through Firefox's
// address bar (Ctrl+L -> Ctrl+A -> type -> End -> Enter) instead of the BiDi
// browsingContext.navigate command — this looks like a real user to
// bot-detection, mirroring chromekit's omnibox navigation. See
// navigateNative (page_navigate_native_windows.go) for the keystroke
// sequence and page_navigate_other.go for the non-Windows behavior.
//
// Scheme handling: unlike Chrome's omnibox, Firefox's address bar is a plain
// text field — it does not rewrite or duplicate scheme text as you type.
// Verified empirically (real Firefox, Windows, 2026-07-30, via a throwaway
// probe: launch native, Ctrl+L/Ctrl+A/type/End/Enter into a local TCP probe
// listener, read back the resulting URL): typing "http://host:port",
// "https://host:port", or a "file:///..." path all navigate to exactly the
// typed string with no corruption or duplication. A bare "host:port" with no
// scheme fell back to Firefox's own fixup (observed http:// for a loopback
// address — HTTPS-First excludes local/loopback targets from upgrade; a
// public domain may resolve differently, but that is Firefox's fixup
// choosing a scheme for ambiguous input, not the address bar rewriting what
// was typed). Because no chromekit-style duplication bug exists here, the
// URL is typed VERBATIM — there is no Firefox equivalent of stripScheme.
func (p *Page) Navigate(url string, timeout time.Duration) error {
	if p.browser != nil && p.browser.cfg != nil && p.browser.cfg.inputBackend == BackendNative {
		return p.navigateNative(url, timeout)
	}
	return p.navigateRemote(url, timeout)
}

// navigateRemote drives navigation through the BiDi browsingContext.navigate
// command. Pre-phase-07 behavior, unchanged.
func (p *Page) navigateRemote(url string, timeout time.Duration) error {
	ctx, cancel := p.opCtx(timeout)
	defer cancel()
	res, err := p.browser.client.Navigate(ctx, bidi.NavigateParams{
		Context: p.contextID,
		URL:     url,
		Wait:    bidi.WaitComplete,
	})
	if err != nil {
		return wrapNavErr(err)
	}
	p.mu.Lock()
	p.url = res.URL
	p.mu.Unlock()
	return nil
}

// Reload reissues the current URL. timeout <= 0 means no deadline.
func (p *Page) Reload(timeout time.Duration) error {
	ctx, cancel := p.opCtx(timeout)
	defer cancel()
	if err := p.browser.client.Reload(ctx, bidi.ReloadParams{
		Context: p.contextID,
		Wait:    bidi.WaitComplete,
	}); err != nil {
		return wrapNavErr(err)
	}
	return nil
}

// GoBack moves history -1. timeout <= 0 means no deadline.
func (p *Page) GoBack(timeout time.Duration) error {
	ctx, cancel := p.opCtx(timeout)
	defer cancel()
	return p.traverse(ctx, -1)
}

// GoForward moves history +1. timeout <= 0 means no deadline.
func (p *Page) GoForward(timeout time.Duration) error {
	ctx, cancel := p.opCtx(timeout)
	defer cancel()
	return p.traverse(ctx, +1)
}

func (p *Page) traverse(ctx context.Context, delta int) error {
	if err := p.browser.client.TraverseHistory(ctx, bidi.TraverseHistoryParams{
		Context: p.contextID,
		Delta:   delta,
	}); err != nil {
		return wrapNavErr(err)
	}
	return nil
}

// GetOrigin returns the page's scheme://host[:port] origin via location.origin.
// Blocking; uses the browser base ctx (chromekit parity).
func (p *Page) GetOrigin() (string, error) {
	var origin string
	if err := p.Evaluate(`location.origin`, &origin); err != nil {
		return "", err
	}
	return origin, nil
}

func wrapNavErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return ErrTimeout
	}
	if errors.Is(err, context.Canceled) {
		return ErrContextCanceled
	}
	return fmt.Errorf("%w: %v", ErrNavigate, err)
}
