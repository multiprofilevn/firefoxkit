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
// timeout <= 0 means no deadline — cancellation comes from ctx (e.g. Browser.Close).
func pollUntil(ctx context.Context, timeout time.Duration, check func(context.Context) (bool, error)) error {
	var deadline time.Time
	if timeout > 0 {
		deadline = time.Now().Add(timeout)
	}
	var wait time.Duration
	for {
		// Tight time check first so a slow check() can't extend past deadline.
		if !deadline.IsZero() && time.Now().After(deadline) {
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
// timeout <= 0 means no deadline.
func (p *Page) WaitForReady(timeout time.Duration) error {
	ctx, cancel := p.opCtx(timeout)
	defer cancel()
	return pollUntil(ctx, timeout, func(c context.Context) (bool, error) {
		var ready string
		if err := p.evaluateCtx(c, `document.readyState`, &ready); err != nil {
			return false, err
		}
		return ready == "complete", nil
	})
}

// WaitForSelector blocks until document.querySelector(sel) finds a match.
// timeout <= 0 means no deadline.
func (p *Page) WaitForSelector(selector string, timeout time.Duration) error {
	ctx, cancel := p.opCtx(timeout)
	defer cancel()
	return pollUntil(ctx, timeout, func(c context.Context) (bool, error) {
		expr := fmt.Sprintf(`!!document.querySelector(%q)`, selector)
		var ok bool
		if err := p.evaluateCtx(c, expr, &ok); err != nil {
			return false, err
		}
		return ok, nil
	})
}

// WaitForSelectorGone blocks until document.querySelector(sel) returns null.
// timeout <= 0 means no deadline.
func (p *Page) WaitForSelectorGone(selector string, timeout time.Duration) error {
	ctx, cancel := p.opCtx(timeout)
	defer cancel()
	return pollUntil(ctx, timeout, func(c context.Context) (bool, error) {
		expr := fmt.Sprintf(`!document.querySelector(%q)`, selector)
		var gone bool
		if err := p.evaluateCtx(c, expr, &gone); err != nil {
			return false, err
		}
		return gone, nil
	})
}

// WaitForChangeURL blocks until window.location.href differs from the cached
// p.url at call time. The baseline URL is read from p.url (set by Navigate
// and navigation events) — no prevURL argument needed (chromekit parity).
// timeout <= 0 means no deadline.
func (p *Page) WaitForChangeURL(timeout time.Duration) error {
	p.mu.RLock()
	baseline := p.url
	p.mu.RUnlock()

	ctx, cancel := p.opCtx(timeout)
	defer cancel()
	return pollUntil(ctx, timeout, func(c context.Context) (bool, error) {
		var url string
		if err := p.evaluateCtx(c, `window.location.href`, &url); err != nil {
			return false, err
		}
		if url != baseline {
			p.mu.Lock()
			p.url = url
			p.mu.Unlock()
			return true, nil
		}
		return false, nil
	})
}

// WaitForFunction blocks until expression evaluates to a truthy value.
// timeout <= 0 means no deadline.
func (p *Page) WaitForFunction(expr string, timeout time.Duration) error {
	ctx, cancel := p.opCtx(timeout)
	defer cancel()
	return pollUntil(ctx, timeout, func(c context.Context) (bool, error) {
		var ok bool
		if err := p.evaluateCtx(c, fmt.Sprintf(`!!(%s)`, expr), &ok); err != nil {
			return false, err
		}
		return ok, nil
	})
}
