package firefoxkit

import (
	"context"
	"errors"
	"fmt"

	"github.com/tuwibu/firefoxkit/internal/bidi"
)

// Navigate drives the page to url and waits for `complete` readiness.
// Caller controls the deadline via ctx — wrap with context.WithTimeout to
// bound the call. Returns ErrNavigate on protocol errors, ErrTimeout on
// ctx deadline, ErrContextCanceled on ctx cancel.
func (p *Page) Navigate(ctx context.Context, url string) error {
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

// Reload reissues the current URL. ctx bounds the call.
func (p *Page) Reload(ctx context.Context) error {
	if err := p.browser.client.Reload(ctx, bidi.ReloadParams{
		Context: p.contextID,
		Wait:    bidi.WaitComplete,
	}); err != nil {
		return wrapNavErr(err)
	}
	return nil
}

// GoBack moves history -1. ctx bounds the call.
func (p *Page) GoBack(ctx context.Context) error { return p.traverse(ctx, -1) }

// GoForward moves history +1. ctx bounds the call.
func (p *Page) GoForward(ctx context.Context) error { return p.traverse(ctx, +1) }

func (p *Page) traverse(ctx context.Context, delta int) error {
	if err := p.browser.client.TraverseHistory(ctx, bidi.TraverseHistoryParams{
		Context: p.contextID,
		Delta:   delta,
	}); err != nil {
		return wrapNavErr(err)
	}
	return nil
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
