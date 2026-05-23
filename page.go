package firefoxkit

import (
	"context"
	"strings"
	"sync"

	"github.com/tuwibu/firefoxkit/internal/bidi"
)

// Page is a top-level browsing context (≈ a tab).
type Page struct {
	browser   *Browser
	contextID string
	log       Logger

	mu    sync.RWMutex
	url   string
	title string

	bkCache backendCache

	kbOnce sync.Once
	kb     *Keyboard

	hdrOnce sync.Once
	hdr     *headerInjector

	dlOnce sync.Once
	dl     *downloadsAPI

	// subsMu guards subs — the cumulative list of BiDi event subscriptions
	// registered for this page (header injector, downloads, OnDialog,
	// OnConsole). cleanup() walks the list on Browser.ClosePage so we don't
	// leak handlers + server-side intercept ids after the tab is gone.
	subsMu sync.Mutex
	subs   []*bidi.Subscription
}

// trackSub appends a subscription to the per-page list so cleanup() can
// unsubscribe it. Returns the input unchanged for caller chaining.
func (p *Page) trackSub(s *bidi.Subscription) *bidi.Subscription {
	if s == nil {
		return nil
	}
	p.subsMu.Lock()
	p.subs = append(p.subs, s)
	p.subsMu.Unlock()
	return s
}

// cleanup unsubscribes every per-page BiDi event handler and releases any
// server-side intercept ids. Idempotent. Called from Browser.ClosePage and
// Browser.Close. Failures during cleanup are logged but never returned —
// teardown must be best-effort to avoid wedging on partial state.
func (p *Page) cleanup(ctx context.Context) {
	// Header injector + intercept id.
	if p.hdr != nil {
		if err := p.removeHeaderInterceptor(ctx); err != nil && p.log != nil {
			p.log.Warnf("firefoxkit: cleanup header injector: %v", err)
		}
	}
	// Downloads subscription.
	if p.dl != nil {
		p.dl.mu.Lock()
		sub := p.dl.sub
		p.dl.sub = nil
		p.dl.mu.Unlock()
		if sub != nil {
			sub.Unsubscribe()
		}
	}
	// Tracked listener subscriptions (OnDialog / OnConsole).
	p.subsMu.Lock()
	subs := p.subs
	p.subs = nil
	p.subsMu.Unlock()
	for _, s := range subs {
		s.Unsubscribe()
	}
}

// ID returns the BiDi browsingContext id — opaque string, surfaced for
// debugging and for advanced callers that need to address the tab directly
// via Mouse/Keyboard helpers.
func (p *Page) ID() string {
	if p == nil {
		return ""
	}
	return p.contextID
}

// URL returns the last URL observed for this page. Refreshed by Navigate and
// will be refreshed by the browsingContext.navigationStarted listener wired
// up in phase 07. Until then call URLLive(ctx) for an authoritative value.
func (p *Page) URL() string {
	if p == nil {
		return ""
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.url
}

// Title returns the cached title. Refreshed lazily; call TitleLive for now.
func (p *Page) Title() string {
	if p == nil {
		return ""
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.title
}

// URLLive issues a fresh getTree to read the current URL.
func (p *Page) URLLive(ctx context.Context) (string, error) {
	info, err := p.lookupContext(ctx)
	if err != nil {
		return "", err
	}
	p.mu.Lock()
	p.url = info.URL
	p.mu.Unlock()
	return info.URL, nil
}

// TitleLive runs document.title via script.evaluate and updates the cache.
func (p *Page) TitleLive(ctx context.Context) (string, error) {
	var title string
	if err := p.Evaluate(ctx, `document.title`, &title); err != nil {
		return "", err
	}
	p.mu.Lock()
	p.title = title
	p.mu.Unlock()
	return title, nil
}

// lookupContext fetches the BrowsingContextInfo for this page's id from a
// fresh getTree walk. Returns ErrPageNotFound if the context has gone.
func (p *Page) lookupContext(ctx context.Context) (*bidi.BrowsingContextInfo, error) {
	tree, err := p.browser.client.GetTree(ctx)
	if err != nil {
		return nil, err
	}
	for i := range tree.Contexts {
		if c := findContext(&tree.Contexts[i], p.contextID); c != nil {
			return c, nil
		}
	}
	return nil, ErrPageNotFound
}

func findContext(node *bidi.BrowsingContextInfo, id string) *bidi.BrowsingContextInfo {
	if node.Context == id {
		return node
	}
	for i := range node.Children {
		if c := findContext(&node.Children[i], id); c != nil {
			return c
		}
	}
	return nil
}

// matchPage checks whether candidate URL matches the page's known URL by
// substring (case-insensitive). Used by Browser.SwitchPageByURL.
func matchPage(haveURL, want string) bool {
	return strings.Contains(strings.ToLower(haveURL), strings.ToLower(want))
}
