package firefoxkit

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/tuwibu/firefoxkit/internal/bidi"
)

// gracefulCloseBudget bounds the graceful phase of Browser.Close (drainer
// join + per-page cleanup sweep). On a healthy peer that phase finishes in
// milliseconds; on a dead/hung peer a cleanup RPC can wedge inside conn.Write
// (writeJSON ignores caller ctx) — the budget caps the wait, after which the
// client is torn down to error the stuck write out.
const gracefulCloseBudget = 2 * time.Second

// Browser owns a Firefox process (Launch) or attaches to one (Connect) and
// manages the pages opened on it.
type Browser struct {
	client *bidi.Client
	cfg    *config
	log    Logger

	proc      *os.Process // nil for Connect path
	closeProc bool        // true only for Launch
	wsURL     string      // ws://127.0.0.1:N/session/<sid>
	sessionID string

	// cleanupProfile removes the firefoxkit-created temp profile dir. Nil (or
	// no-op) for Connect and user-supplied profiles. Invoked by Close() after
	// the Firefox process has been reaped so files are no longer locked.
	cleanupProfile func()

	// baseCtx is the long-lived parent context every public Page/Browser
	// method derives from. It lets the API hide context.Context from callers
	// (chromekit parity) while still supporting cancellation: Close() calls
	// baseCancel to unblock any in-flight operation. Set once by Launch/Connect.
	baseCtx    context.Context
	baseCancel context.CancelFunc

	// hwndOverride, when non-zero, is the explicit top-level window handle set
	// via SetHWND — takes precedence over PID-based lookup in HWND().
	hwndOverride uintptr

	pagesMu sync.RWMutex
	pages   []*Page
	current *Page

	// Cleanup drainer for contextDestroyed pages: detach happens synchronously
	// on the subscription worker; the blocking per-page cleanup RPCs are queued
	// here and drained by a small worker pool. Started by watchContextLifecycle,
	// joined by Close via stopCleanupDrainer. Nil when the watch never started.
	cleanupCh   chan *Page
	cleanupStop chan struct{}
	cleanupWG   sync.WaitGroup

	closeOnce sync.Once
}

// initBaseCtx wires the long-lived base context. Called by Launch/Connect right
// after the Browser literal is built. Idempotent guard not needed — each
// Browser is constructed exactly once.
func (b *Browser) initBaseCtx() {
	b.baseCtx, b.baseCancel = context.WithCancel(context.Background())
}

// ctx returns the browser-wide base context. Internal accessor for files that
// derive per-call contexts without a caller-supplied ctx.
func (b *Browser) ctx() context.Context {
	if b == nil || b.baseCtx == nil {
		return context.Background()
	}
	return b.baseCtx
}

// Current returns the active page (last opened / last switched-to).
func (b *Browser) Current() *Page {
	b.pagesMu.RLock()
	defer b.pagesMu.RUnlock()
	return b.current
}

// Pages returns a snapshot of all pages owned by this browser.
func (b *Browser) Pages() []*Page {
	b.pagesMu.RLock()
	defer b.pagesMu.RUnlock()
	out := make([]*Page, len(b.pages))
	copy(out, b.pages)
	return out
}

// InputBackend reports which input backend this Browser was configured with.
func (b *Browser) InputBackend() BackendType {
	if b == nil || b.cfg == nil {
		return BackendBiDi
	}
	return b.cfg.inputBackend
}

// PID returns the Firefox process id when chromekit-style Launch owns the
// process. 0 on the Connect path.
func (b *Browser) PID() uint32 {
	if b == nil || b.proc == nil {
		return 0
	}
	return uint32(b.proc.Pid)
}

// SetHWND overrides the Firefox top-level window handle used by native input.
// Useful on the Connect path where PID-based lookup is unavailable. A non-zero
// override takes precedence over the PID-based search in HWND().
func (b *Browser) SetHWND(hwnd uintptr) {
	if b == nil {
		return
	}
	b.hwndOverride = hwnd
}

// Done returns a channel that closes when the BiDi client disconnects —
// either via Close() or because Firefox exited.
func (b *Browser) Done() <-chan struct{} {
	if b == nil || b.client == nil {
		ch := make(chan struct{})
		close(ch)
		return ch
	}
	return b.client.Done()
}

// Close shuts down. For Launch (owns the process) it kills Firefox.
// For Connect it disconnects but leaves Firefox running.
func (b *Browser) Close() error {
	var firstErr error
	b.closeOnce.Do(func() {
		// Cancel the base context first so any in-flight user operation
		// derived from it unblocks before we tear down the transport.
		if b.baseCancel != nil {
			b.baseCancel()
		}
		// Signal the cleanup drainer to stop taking new work — but do NOT
		// block on the join here. baseCancel cannot unblock a worker wedged
		// inside conn.Write (writeJSON runs under its own internal ctx); only
		// the write timeout or the client teardown below frees it, so an
		// unconditional Wait would stall Close for up to the full writeTimeout
		// on a dead peer.
		b.signalCleanupDrainer()
		b.pagesMu.Lock()
		pages := b.pages
		b.pages = nil
		b.current = nil
		b.pagesMu.Unlock()
		// Graceful phase, bounded: per-page cleanup sweep + drainer join.
		// Runs BEFORE the BiDi client tears down so RemoveIntercept /
		// Unsubscribe RPCs still succeed on a healthy peer (finishes in ms).
		// On a dead/hung peer the budget expires and we proceed — client.Close
		// below errors any stuck write out, making the final join fast.
		graceful := make(chan struct{})
		go func() {
			defer close(graceful)
			cleanupCtx, ccancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer ccancel()
			for _, p := range pages {
				p.cleanup(cleanupCtx)
			}
			b.cleanupWG.Wait()
		}()
		select {
		case <-graceful:
		case <-time.After(gracefulCloseBudget):
		}

		if b.client != nil {
			if err := b.client.Close(); err != nil {
				firstErr = err
			}
		}
		// Final join — no goroutine may outlive Close. Fast now: with the
		// client closed, stuck writes fail immediately and both the drainer
		// workers and the sweep goroutine exit promptly.
		b.cleanupWG.Wait()
		<-graceful
		if b.closeProc && b.proc != nil {
			if err := b.proc.Kill(); err != nil && firstErr == nil {
				firstErr = fmt.Errorf("kill firefox pid=%d: %w", b.proc.Pid, err)
			}
			_, _ = b.proc.Wait()
		}
		if b.cleanupProfile != nil {
			b.cleanupProfile()
		}
	})
	return firstErr
}

// bidiClient is an internal accessor for other files in the package.
func (b *Browser) bidiClient() *bidi.Client { return b.client }

// validateConfig is the early sanity-gate run by Launch / Connect. Keeps
// the option matrix honest before any process spawn.
func validateConfig(cfg *config) error {
	if cfg.headless && cfg.inputBackend == BackendNative {
		return fmt.Errorf("%w: native input requires a visible window (headless=false)", ErrInvalidConfig)
	}
	return nil
}

// ctxWithTimeout shrink-wraps cfg.launchTimeout into a derived context.
func ctxWithTimeout(parent context.Context, cfg *config) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, cfg.launchTimeout)
}
