package firefoxkit

import (
	"context"
	"encoding/json"
	"sort"
	"sync"
	"time"

	"github.com/multiprofilevn/firefoxkit/internal/bidi"
)

// headerInjector holds the per-page intercept state for SetExtraHeaders.
type headerInjector struct {
	mu          sync.RWMutex
	headers     map[string]string
	interceptID string
	sub         *bidi.Subscription

	// installMu serializes install/remove of the interceptor. Without it two
	// concurrent SetExtraHeaders calls could both see interceptID == "" and
	// install twice — the loser's intercept + handler would be orphaned and
	// keep injecting headers even after SetExtraHeaders(nil).
	installMu sync.Mutex
}

// headerInjectorLazy returns the per-page injector, constructing it on first
// use under initMu (race-free vs. cleanup).
func (p *Page) headerInjectorLazy() *headerInjector {
	p.initMu.Lock()
	defer p.initMu.Unlock()
	if p.hdr == nil {
		p.hdr = &headerInjector{headers: map[string]string{}}
	}
	return p.hdr
}

// SetExtraHeaders merges headers into every subsequent request from this page.
// Passing an empty map removes the injector. Idempotent; later calls replace
// the active set rather than accumulating.
func (p *Page) SetExtraHeaders(headers map[string]string) error {
	hi := p.headerInjectorLazy()

	// Commit the map and pick the install/remove branch under the same
	// installMu section: the last caller to hold installMu is then also the
	// last to commit, so the intercept state always converges to the committed
	// header set. Branching on the argument instead lets a remove that loses
	// the lock race leave an interceptor pinned with an empty header set.
	// Lock order is installMu → hi.mu, never the reverse.
	hi.installMu.Lock()
	defer hi.installMu.Unlock()

	hi.mu.Lock()
	hi.headers = make(map[string]string, len(headers))
	for k, v := range headers {
		hi.headers[k] = v
	}
	empty := len(hi.headers) == 0
	installed := hi.interceptID != ""
	hi.mu.Unlock()

	if p.cleanedUp.Load() {
		return nil // page torn down — don't install an orphan interceptor
	}
	if empty {
		return p.removeHeaderInterceptor(p.ctx())
	}
	if installed {
		return nil
	}
	return p.installHeaderInterceptor(p.ctx())
}

func (p *Page) installHeaderInterceptor(ctx context.Context) error {
	res, err := p.browser.client.AddIntercept(ctx, bidi.AddInterceptParams{
		Phases:   []bidi.InterceptPhase{bidi.PhaseBeforeRequestSent},
		Contexts: []string{p.contextID},
	})
	if err != nil {
		return err
	}
	if err := p.browser.client.SubscribeEvents(ctx, bidi.SubscribeParams{
		Events:   []string{"network.beforeRequestSent"},
		Contexts: []string{p.contextID},
	}); err != nil {
		_ = p.browser.client.RemoveIntercept(ctx, bidi.RemoveInterceptParams{Intercept: res.Intercept})
		return err
	}
	p.trackServerEvent("network.beforeRequestSent")
	sub := p.browser.client.Subscribe("network.beforeRequestSent", func(params json.RawMessage) {
		var ev bidi.BeforeRequestSentEvent
		if err := json.Unmarshal(params, &ev); err != nil {
			return
		}
		if ev.Context != p.contextID {
			return
		}
		p.hdr.mu.RLock()
		hdrs := make([]bidi.Header, 0, len(ev.Request.Headers)+len(p.hdr.headers))
		// Preserve existing headers that the user didn't override.
		seen := make(map[string]struct{}, len(p.hdr.headers))
		for k := range p.hdr.headers {
			seen[k] = struct{}{}
		}
		for _, h := range ev.Request.Headers {
			if _, ok := seen[h.Name]; ok {
				continue
			}
			hdrs = append(hdrs, h)
		}
		for _, k := range sortedHeaderKeys(p.hdr.headers) {
			hdrs = append(hdrs, bidi.Header{
				Name:  k,
				Value: *bidi.StringBytes(p.hdr.headers[k]),
			})
		}
		p.hdr.mu.RUnlock()

		// Runs on this subscription's worker — safe to issue the RPC
		// synchronously. Only network.beforeRequestSent events queue behind
		// it; other subscriptions (downloads, dialogs, console) have their
		// own workers. No goroutine-per-request under load.
		cctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := p.browser.client.ContinueRequest(cctx, bidi.ContinueRequestParams{
			Request: ev.Request.Request,
			Headers: hdrs,
		}); err != nil && p.log != nil {
			p.log.Debugf("firefoxkit: continueRequest: %v", err)
		}
	})
	p.hdr.mu.Lock()
	p.hdr.interceptID = res.Intercept
	p.hdr.sub = sub
	p.hdr.mu.Unlock()
	return nil
}

func (p *Page) removeHeaderInterceptor(ctx context.Context) error {
	if p.hdr == nil {
		return nil
	}
	p.hdr.mu.Lock()
	id := p.hdr.interceptID
	sub := p.hdr.sub
	p.hdr.interceptID = ""
	p.hdr.sub = nil
	p.hdr.mu.Unlock()
	if sub != nil {
		sub.Unsubscribe()
	}
	if id == "" {
		return nil
	}
	// Drop the server-side subscription too, not just the intercept: without it
	// Firefox keeps streaming network.beforeRequestSent for this context with no
	// consumer until page cleanup, once per request.
	p.untrackServerEvent("network.beforeRequestSent")
	err := p.browser.client.RemoveIntercept(ctx, bidi.RemoveInterceptParams{Intercept: id})
	if uerr := p.browser.client.UnsubscribeEvents(ctx, bidi.SubscribeParams{
		Events:   []string{"network.beforeRequestSent"},
		Contexts: []string{p.contextID},
	}); uerr != nil && err == nil {
		err = uerr
	}
	return err
}

func sortedHeaderKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
