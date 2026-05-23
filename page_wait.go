package firefoxkit

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// pollBackoff steps from 50ms up to 500ms, capped. Used by every WaitFor*.
func pollBackoff(prev time.Duration) time.Duration {
	if prev == 0 {
		return 50 * time.Millisecond
	}
	next := prev * 2
	if next > 500*time.Millisecond {
		return 500 * time.Millisecond
	}
	return next
}

// pollUntil runs check until it returns (true, nil), errors, or the deadline.
// Returns ErrTimeout on deadline, ctx.Err() on cancel, check's error otherwise.
func pollUntil(ctx context.Context, timeout time.Duration, check func(context.Context) (bool, error)) error {
	deadline := time.Now().Add(timeout)
	if timeout <= 0 {
		deadline = time.Now().Add(30 * time.Second)
	}
	var wait time.Duration
	for {
		// Tight time check first so a slow check() can't extend past deadline.
		if time.Now().After(deadline) {
			return ErrTimeout
		}
		// Per-attempt sub-context: don't let a hung check block forever.
		attemptCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		ok, err := check(attemptCtx)
		cancel()
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return ErrContextCanceled
			}
			// Transient errors (selector not found, eval failed) → continue polling.
			// Caller's contextual ctx still wins via the outer select.
		}
		if ok {
			return nil
		}
		wait = pollBackoff(wait)
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return ErrTimeout
			}
			return ErrContextCanceled
		case <-time.After(wait):
		}
	}
}

// WaitForReady blocks until document.readyState === "complete".
func (p *Page) WaitForReady(ctx context.Context, timeout time.Duration) error {
	return pollUntil(ctx, timeout, func(c context.Context) (bool, error) {
		var ready string
		if err := p.Evaluate(c, `document.readyState`, &ready); err != nil {
			return false, err
		}
		return ready == "complete", nil
	})
}

// WaitForSelector blocks until document.querySelector(sel) finds a match.
func (p *Page) WaitForSelector(ctx context.Context, sel string, timeout time.Duration) error {
	return pollUntil(ctx, timeout, func(c context.Context) (bool, error) {
		expr := fmt.Sprintf(`!!document.querySelector(%q)`, sel)
		var ok bool
		if err := p.Evaluate(c, expr, &ok); err != nil {
			return false, err
		}
		return ok, nil
	})
}

// WaitForChangeURL blocks until window.location.href differs from prevURL.
func (p *Page) WaitForChangeURL(ctx context.Context, prevURL string, timeout time.Duration) error {
	return pollUntil(ctx, timeout, func(c context.Context) (bool, error) {
		var url string
		if err := p.Evaluate(c, `window.location.href`, &url); err != nil {
			return false, err
		}
		if url != prevURL {
			p.mu.Lock()
			p.url = url
			p.mu.Unlock()
			return true, nil
		}
		return false, nil
	})
}

// WaitForFunction blocks until expression evaluates to a truthy value.
func (p *Page) WaitForFunction(ctx context.Context, expression string, timeout time.Duration) error {
	return pollUntil(ctx, timeout, func(c context.Context) (bool, error) {
		var ok bool
		if err := p.Evaluate(c, fmt.Sprintf(`!!(%s)`, expression), &ok); err != nil {
			return false, err
		}
		return ok, nil
	})
}
