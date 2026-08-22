package firefoxkit

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/multiprofilevn/firefoxkit/internal/bidi"
)

// gracefulCloseBudget bounds the graceful phase of Browser.Close (drainer
// join + per-page cleanup sweep). On a healthy peer that phase finishes in
// milliseconds; on a dead/hung peer a cleanup RPC can wedge inside conn.Write
// (writeJSON ignores caller ctx) — the budget caps the wait, after which the
// client is torn down to error the stuck write out.
const gracefulCloseBudget = 2 * time.Second

// procExitTimeout bounds how long Close waits for the OS to actually reap the
// Firefox process after the kill. Kill() returning nil only means the request
// was accepted — the Wait owner closing procDone is the only proof the tree is
// gone. A var (not a const) so tests can shrink the wait.
var procExitTimeout = 5 * time.Second

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

	// procTree kills the Firefox process together with its content/GPU
	// children. Nil for Connect (nothing to reap).
	procTree *processTreeReaper
	// procDone is closed by the single cmd.Wait owner started in Launch when
	// the Firefox process has actually exited. Nil for Connect.
	procDone chan struct{}

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
	// cleanupDropped counts cleanups abandoned because the drainer queue was
	// full. Reported once by Close: a non-zero count means churn outran the
	// drainer, which costs best-effort RPCs but never page tracking.
	cleanupDropped atomic.Uint64

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
//
// Bounded but not instant. Against a healthy peer it finishes in milliseconds
// plus the process exit; against a dead or hung one the worst case is one
// gracefulCloseBudget for the browser.close RPC, another for the cleanup sweep
// and drainer join, plus procExitTimeout waiting for the process tree to
// actually go — roughly 9-11s with the current budgets.
func (b *Browser) Close() error {
	var firstErr error
	b.closeOnce.Do(func() {
		// Graceful quit first, Launch path only: browser.close lets Firefox
		// flush its profile and tear down its own content processes, leaving
		// the kill below as a backstop. Best-effort — the RPC runs on a
		// goroutine because a frame write does not honor the caller ctx
		// (writeJSON has its own), so a hung peer would otherwise hold Close
		// for the full write timeout. On Connect the browser belongs to the
		// caller and must survive, so nothing is sent.
		var gracefulQuit chan struct{}
		if b.closeProc && b.client != nil {
			gracefulQuit = make(chan struct{})
			go func() {
				defer close(gracefulQuit)
				qctx, qcancel := context.WithTimeout(context.Background(), gracefulCloseBudget)
				defer qcancel()
				_ = b.client.CloseBrowser(qctx)
			}()
			select {
			case <-gracefulQuit:
			case <-time.After(gracefulCloseBudget):
			}
		}
		// Signal the cleanup drainer BEFORE cancelling the base context, so
		// the stop signal is already visible when workers next reach their
		// select — otherwise which of stop / more work they pick depends on
		// cancel timing. Do NOT block on the join here: baseCancel cannot
		// unblock a worker wedged inside conn.Write (writeJSON runs under its
		// own internal ctx); only the write timeout or the client teardown
		// below frees it, so an unconditional Wait would stall Close for up to
		// the full writeTimeout on a dead peer.
		b.signalCleanupDrainer()
		// Cancel the base context so any in-flight user operation derived from
		// it unblocks before we tear down the transport.
		if b.baseCancel != nil {
			b.baseCancel()
		}
		b.pagesMu.Lock()
		pages := b.pages
		b.pages = nil
		b.current = nil
		b.pagesMu.Unlock()
		// Graceful phase, bounded: per-page cleanup sweep + drainer join. It
		// runs BEFORE the BiDi client tears down, so its RemoveIntercept /
		// Unsubscribe RPCs still have a socket to travel on.
		//
		// Full order on the Launch path: browser.close → drainer signal →
		// baseCancel → this sweep → client.Close → process reap. Sending
		// browser.close first is deliberate — only Firefox can flush the profile
		// and tear down its own content processes, and it needs a live session
		// to do it — but it costs two things: an in-flight user operation is not
		// cancelled until that RPC settles (≤ gracefulCloseBudget), and the
		// sweep here then talks to a peer that is already quitting, so its RPCs
		// often fail. They are best-effort and only logged. On the Connect path
		// no close is sent, so the sweep runs against a live browser and
		// succeeds in ms. Either way the budget caps the wait and client.Close
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
			// After a graceful browser.close the peer drops the socket on its
			// own, so a teardown error there says nothing about our shutdown.
			if err := b.client.Close(); err != nil && firstErr == nil && gracefulQuit == nil {
				firstErr = err
			}
		}
		// Final join — no goroutine may outlive Close. Fast now: with the
		// client closed, stuck writes fail immediately and both the drainer
		// workers and the sweep goroutine exit promptly.
		b.cleanupWG.Wait()
		<-graceful
		if gracefulQuit != nil {
			<-gracefulQuit
		}
		// Surface lifetime churn that outran the drainer: page tracking stayed
		// correct, but these pages never got their best-effort cleanup RPCs.
		if n := b.cleanupDropped.Load(); n > 0 && b.log != nil {
			b.log.Warnf("firefoxkit: %d page cleanups dropped over this browser's lifetime (cleanup drainer saturated)", n)
		}
		if b.closeProc && b.proc != nil {
			if err := b.reapProcess(); err != nil && firstErr == nil {
				firstErr = err
			}
		}
		if b.cleanupProfile != nil {
			b.cleanupProfile()
		}
	})
	return firstErr
}

// reapProcess terminates the Firefox process tree and blocks until the process
// has actually exited. Two invariants live here: the kill must take the content
// processes with it (they otherwise keep the profile locked and park the pipe
// readers), and success is defined by the exit being observed — a kill that
// reports nil while Firefox keeps running is a real failure the caller must see.
func (b *Browser) reapProcess() error {
	if b.procDone == nil {
		// Constructed outside Launch: no Wait owner to observe, so a plain
		// kill is all there is.
		return b.proc.Kill()
	}
	select {
	case <-b.procDone:
		return nil // already exited: graceful quit, crash or user close
	default:
	}
	killErr := b.procTree.reap()
	// Backstop for the parent in case the tree sweep missed it. An already
	// exited process is the desired outcome, not a failure.
	if err := b.proc.Kill(); err != nil && killErr == nil && !errors.Is(err, os.ErrProcessDone) {
		killErr = fmt.Errorf("kill firefox pid=%d: %w", b.proc.Pid, err)
	}
	select {
	case <-b.procDone:
		return nil // gone — how the kill was reported no longer matters
	case <-time.After(procExitTimeout):
		if killErr != nil {
			return fmt.Errorf("firefoxkit: firefox pid=%d survived kill: %w", b.proc.Pid, killErr)
		}
		return fmt.Errorf("firefoxkit: firefox pid=%d survived kill", b.proc.Pid)
	}
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
