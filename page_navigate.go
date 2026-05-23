package firefoxkit

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/tuwibu/firefoxkit/internal/bidi"
)

// Navigate drives the page to url and waits for `complete` readiness.
// Returns ErrNavigate on protocol errors, ErrTimeout on deadline.
func (p *Page) Navigate(url string, timeout time.Duration) error {
	ctx, cancel := navCtx(timeout)
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

// Reload reissues the current URL.
func (p *Page) Reload(timeout time.Duration) error {
	ctx, cancel := navCtx(timeout)
	defer cancel()
	if err := p.browser.client.Reload(ctx, bidi.ReloadParams{
		Context: p.contextID,
		Wait:    bidi.WaitComplete,
	}); err != nil {
		return wrapNavErr(err)
	}
	return nil
}

// GoBack moves history -1. timeout=0 → 10s default.
func (p *Page) GoBack(timeout time.Duration) error { return p.traverse(-1, timeout) }

// GoForward moves history +1. timeout=0 → 10s default.
func (p *Page) GoForward(timeout time.Duration) error { return p.traverse(+1, timeout) }

func (p *Page) traverse(delta int, timeout time.Duration) error {
	ctx, cancel := navCtx(timeout)
	defer cancel()
	if err := p.browser.client.TraverseHistory(ctx, bidi.TraverseHistoryParams{
		Context: p.contextID,
		Delta:   delta,
	}); err != nil {
		return wrapNavErr(err)
	}
	return nil
}

func navCtx(timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return context.WithTimeout(context.Background(), timeout)
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
