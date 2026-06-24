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
func (p *Page) Navigate(url string, timeout time.Duration) error {
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
