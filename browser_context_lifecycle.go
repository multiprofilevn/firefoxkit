package firefoxkit

import (
	"context"
	"encoding/json"
	"time"

	"github.com/tuwibu/firefoxkit/internal/bidi"
)

// watchContextLifecycle subscribes (session-wide) to
// browsingContext.contextDestroyed so pages closed by Firefox itself —
// window.close(), self-closing OAuth popups, a user closing a tab in headed
// mode — are dropped from b.pages and cleaned up. Without this, such pages
// leak: their handlers stay registered and Pages()/Switch* degrade over time.
// Called once from Launch/Connect.
func (b *Browser) watchContextLifecycle(ctx context.Context) error {
	if err := b.client.SubscribeEvents(ctx, bidi.SubscribeParams{
		Events: []string{"browsingContext.contextDestroyed"},
	}); err != nil {
		return err
	}
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

// removeDestroyedPage drops the page tracking a destroyed context and runs its
// cleanup. No-op for unknown contexts (child frames also emit this event).
func (b *Browser) removeDestroyedPage(id string) {
	b.pagesMu.Lock()
	idx := -1
	for i, p := range b.pages {
		if p.contextID == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		b.pagesMu.Unlock()
		return
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
	b.pagesMu.Unlock()

	if b.log != nil {
		b.log.Debugf("firefoxkit: context %s destroyed — page removed", id)
	}
	// The context is already gone server-side; cleanup RPCs targeting it fail
	// harmlessly. Short ctx: this runs on the subscription worker.
	cctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	target.cleanup(cctx)
	cancel()
}
