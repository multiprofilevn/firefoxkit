package firefoxkit

import (
	"context"
	"encoding/json"
	"sort"
	"sync"
	"time"

	"github.com/tuwibu/firefoxkit/internal/bidi"
)

// headerInjector holds the per-page intercept state for SetExtraHeaders.
type headerInjector struct {
	mu          sync.RWMutex
	headers     map[string]string
	interceptID string
	sub         *bidi.Subscription
}

// SetExtraHeaders merges headers into every subsequent request from this page.
// Passing an empty map removes the injector. Idempotent; later calls replace
// the active set rather than accumulating.
func (p *Page) SetExtraHeaders(ctx context.Context, headers map[string]string) error {
	p.hdrOnce.Do(func() { p.hdr = &headerInjector{headers: map[string]string{}} })
	hi := p.hdr

	hi.mu.Lock()
	hi.headers = make(map[string]string, len(headers))
	for k, v := range headers {
		hi.headers[k] = v
	}
	needInstall := hi.interceptID == ""
	hi.mu.Unlock()

	if len(headers) == 0 {
		return p.removeHeaderInterceptor(ctx)
	}
	if !needInstall {
		return nil
	}
	return p.installHeaderInterceptor(ctx)
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

		// Use a fresh background ctx — the original event ctx is gone by the
		// time we dispatch.
		cctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = p.browser.client.ContinueRequest(cctx, bidi.ContinueRequestParams{
			Request: ev.Request.Request,
			Headers: hdrs,
		})
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
	return p.browser.client.RemoveIntercept(ctx, bidi.RemoveInterceptParams{Intercept: id})
}

func sortedHeaderKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
