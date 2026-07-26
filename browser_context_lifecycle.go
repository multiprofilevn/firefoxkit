package firefoxkit

import (
	"context"
	"encoding/json"
	"time"

	"github.com/tuwibu/firefoxkit/internal/bidi"
)

const (
	// cleanupChCap bounds the queue of detached pages awaiting best-effort
	// cleanup RPCs. Sized well above the event bus queue (256) so a churn
	// burst never forces the synchronous fallback in practice.
	cleanupChCap = 512
	// cleanupWorkers is the size of the cleanup drainer pool. Small and fixed:
	// bounded concurrency, never goroutine-per-event.
	cleanupWorkers = 3
	// cleanupTimeout bounds each page's cleanup RPC batch. The context is
	// already gone server-side, so these calls fail harmlessly when slow.
	cleanupTimeout = 3 * time.Second
)

// watchContextLifecycle subscribes (session-wide) to
// browsingContext.contextDestroyed so pages closed by Firefox itself —
// window.close(), self-closing OAuth popups, a user closing a tab in headed
// mode — are dropped from b.pages and cleaned up. Without this, such pages
// leak: their handlers stay registered and Pages()/Switch* degrade over time.
// Also starts the cleanup drainer pool (stopped by Browser.Close).
// Called once from Launch/Connect.
func (b *Browser) watchContextLifecycle(ctx context.Context) error {
	if err := b.client.SubscribeEvents(ctx, bidi.SubscribeParams{
		Events: []string{"browsingContext.contextDestroyed"},
	}); err != nil {
		return err
	}
	b.startCleanupDrainer()
	b.client.Subscribe("browsingContext.contextDestroyed", func(params json.RawMessage) {
		var ev struct {
			Context string `json:"context"`
		}
		if err := json.Unmarshal(params, &ev); err != nil || ev.Context == "" {
			return
		}
		b.removeDestroyedPage(ev.Context)
	})
	return nil
}

// removeDestroyedPage detaches the page tracking a destroyed context and hands
// it to the cleanup drainer. The detach is synchronous and cheap, so a burst
// of contextDestroyed events can never back up the subscription queue (cap
// 256, drop-on-full) and leak entries in b.pages; the blocking cleanup RPCs
// run on the drainer pool instead of the subscription worker.
// No-op for unknown contexts (child frames also emit this event).
func (b *Browser) removeDestroyedPage(id string) {
	target := b.detachDestroyedPage(id)
	if target == nil {
		return
	}
	if b.log != nil {
		b.log.Debugf("firefoxkit: context %s destroyed — page removed", id)
	}
	select {
	case b.cleanupCh <- target:
	default:
		// Drainer saturated (extreme churn): drop the RPC half of cleanup.
		// Running it here would block the subscription worker for a full
		// cleanupTimeout per event, overflowing the 256-cap event queue — and a
		// contextDestroyed lost there leaks b.pages permanently, which is
		// strictly worse. Server-side state dies with the context anyway, so
		// those RPCs are only belt-and-suspenders. The local half still runs:
		// it needs no RPC (microseconds) and its handler registrations would
		// otherwise sit in the client dispatcher, pinning this page, for the
		// rest of the browser's life.
		target.cleanupLocal()
		n := b.cleanupDropped.Add(1)
		if b.log != nil {
			b.log.Warnf("firefoxkit: cleanup drainer saturated — dropped cleanup for context %s (%d dropped so far)", id, n)
		}
	}
}

// detachDestroyedPage removes the page tracking id from b.pages (fixing up
// b.current) and returns it for cleanup. Returns nil for unknown contexts.
// Cheap: only pagesMu work, no RPCs.
func (b *Browser) detachDestroyedPage(id string) *Page {
	b.pagesMu.Lock()
	defer b.pagesMu.Unlock()
	idx := -1
	for i, p := range b.pages {
		if p.contextID == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		return nil
	}
	target := b.pages[idx]
	b.pages = append(b.pages[:idx], b.pages[idx+1:]...)
	if b.current == target {
		if len(b.pages) > 0 {
			b.current = b.pages[0]
		} else {
			b.current = nil
		}
	}
	return target
}

// startCleanupDrainer launches the bounded worker pool that runs page cleanup
// off the subscription worker. Browser.Close signals the pool via
// signalCleanupDrainer, bounds a graceful drain by gracefulCloseBudget, and
// joins the workers after tearing the client down (which errors out any
// cleanup RPC still wedged inside conn.Write — baseCancel alone cannot
// unblock a write in flight, since writeJSON ignores the caller ctx).
func (b *Browser) startCleanupDrainer() {
	b.cleanupCh = make(chan *Page, cleanupChCap)
	b.cleanupStop = make(chan struct{})
	for i := 0; i < cleanupWorkers; i++ {
		b.cleanupWG.Add(1)
		go func() {
			defer b.cleanupWG.Done()
			for {
				select {
				case <-b.cleanupStop:
					b.drainRemainingCleanup()
					return
				case p := <-b.cleanupCh:
					// The context is already gone server-side; cleanup RPCs
					// targeting it fail harmlessly. Derived from Background,
					// like the drain path below: a page picked up while Close
					// is cancelling the base ctx must still get its shot at a
					// healthy peer instead of failing on a ctx race.
					cctx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
					p.cleanup(cctx)
					cancel()
				}
			}
		}()
	}
}

// drainRemainingCleanup empties whatever is still queued when the stop signal
// fires, so queued pages get their best-effort cleanup during Close's graceful
// phase. Base ctx is already canceled at that point, so derive from Background
// — these RPCs must still be able to reach a healthy peer. On a dead peer they
// fail fast once Browser.Close tears the client down.
func (b *Browser) drainRemainingCleanup() {
	for {
		select {
		case p := <-b.cleanupCh:
			cctx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
			p.cleanup(cctx)
			cancel()
		default:
			return
		}
	}
}

// signalCleanupDrainer tells the drainer workers to drain-then-exit WITHOUT
// blocking on the join — Browser.Close owns the (bounded) join. Safe when the
// drainer was never started (watchContextLifecycle failed or never ran).
// Called exactly once, from inside Browser.Close's closeOnce.
func (b *Browser) signalCleanupDrainer() {
	if b.cleanupStop == nil {
		return
	}
	close(b.cleanupStop)
}
