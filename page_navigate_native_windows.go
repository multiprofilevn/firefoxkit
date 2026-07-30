//go:build windows

package firefoxkit

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/tuwibu/firefoxkit/input"
)

// navigateNative simulates a user typing url into Firefox's address bar:
// Ctrl+L (focus) -> Ctrl+A (select all) -> type -> End (cancel the address
// bar's inline autocomplete suggestion, so Enter goes to exactly what was
// typed, not a suggested history match) -> Enter. See the doc comment on
// Navigate (page_navigate.go) for the scheme-handling rationale.
//
// Every keystroke goes through the page's native input backend, which
// already carries the phase-1 ForegroundGuard (page_input_backend.go) — a
// lost foreground aborts mid-sequence with ErrForegroundLost instead of
// typing into whatever window stole focus.
func (p *Page) navigateNative(url string, timeout time.Duration) error {
	ctx, cancel := p.opCtx(timeout)
	defer cancel()

	kb := p.Keyboard().k
	if err := kb.Combo(ctx, input.VKL, input.ModCtrl); err != nil {
		return wrapNativeNavErr(err, "focus address bar")
	}
	if err := kb.Combo(ctx, input.VKA, input.ModCtrl); err != nil {
		return wrapNativeNavErr(err, "select-all")
	}
	// Stamp the pre-nav document BEFORE typing so waitNavCommitted can tell a
	// fresh document has actually loaded — readyState alone races right after
	// Enter (the old document still reports "complete").
	p.markNavToken(ctx)
	if err := kb.Type(ctx, url); err != nil {
		return wrapNativeNavErr(err, "type url")
	}
	if err := kb.Combo(ctx, input.VKEnd, 0); err != nil {
		return wrapNativeNavErr(err, "end key")
	}
	if err := kb.Combo(ctx, input.VKReturn, 0); err != nil {
		return wrapNativeNavErr(err, "enter")
	}
	return p.waitNavCommitted(ctx, timeout)
}

// wrapNativeNavErr maps a native-backend error to the package's error
// contract — the same contract wrapNavErr (page_navigate.go) applies to the
// remote path, so a deadline hit mid-keystroke (not just in the post-Enter
// wait) still surfaces as ErrTimeout instead of a generic ErrNavigate. A
// foreground loss surfaces as the shared ErrForegroundLost sentinel
// (phase 1); anything else wraps ErrNavigate with the failing step.
func wrapNativeNavErr(err error, step string) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return ErrTimeout
	}
	if errors.Is(err, context.Canceled) {
		return ErrContextCanceled
	}
	if errors.Is(err, input.ErrForegroundLost) {
		return ErrForegroundLost
	}
	return fmt.Errorf("%w: %s: %s", ErrNavigate, step, err.Error())
}

// markNavToken stamps a sentinel on the current document so waitNavCommitted
// can detect a committed navigation. Best-effort — a failed stamp just
// degrades the post-Enter wait to plain readyState polling (evaluateCtx
// errors are treated as "not ready yet" by pollUntil, see page_wait.go).
func (p *Page) markNavToken(ctx context.Context) {
	var ok bool
	_ = p.evaluateCtx(ctx, `(window.__fkNavToken=1, true)`, &ok)
}

// waitNavCommitted blocks until a native (address-bar) navigation has
// actually committed a NEW document AND that document finished loading.
// Built on pollUntil — the same polling engine WaitForReady/WaitForSelector
// use (page_wait.go) — so native and remote surface the identical
// ErrTimeout / ErrContextCanceled contract on a stalled navigation. Remote's
// wait is server-side instead, built into the BiDi Navigate command's
// Wait:complete param (navigateRemote, page_navigate.go); pollUntil is what
// both ultimately bottom out on for their timeout/cancel error shape.
func (p *Page) waitNavCommitted(ctx context.Context, timeout time.Duration) error {
	return pollUntil(ctx, timeout, func(c context.Context) (bool, error) {
		var done bool
		expr := `(document.readyState === "complete" && typeof window.__fkNavToken === "undefined")`
		if err := p.evaluateCtx(c, expr, &done); err != nil {
			return false, nil
		}
		return done, nil
	})
}
