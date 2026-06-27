package firefoxkit

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tuwibu/firefoxkit/input"
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

	// cursorOnce guards lazy construction of the human-like cursor engine.
	// cursor may be pre-set by tests to inject a fake backend; cursorEngine()
	// honors a non-nil value and skips building a real backend.
	cursorOnce sync.Once
	cursor     *input.Cursor

	kbOnce sync.Once
	kb     *KeyboardAPI

	mouseOnce sync.Once
	mouse     *MouseAPI

	hdrOnce sync.Once
	hdr     *headerInjector

	dlOnce sync.Once
	dl     *downloadsAPI

	// dialogOnce ensures we subscribe to userPromptOpened at most once.
	// dialogHandler holds the current handler (replaced on each OnDialog call).
	dialogOnce    sync.Once
	dialogHandler atomic.Pointer[func(DialogEvent) DialogAction]

	// consoleOnce ensures we subscribe to log.entryAdded at most once.
	// consoleHandler holds the current handler (replaced on each OnConsole call).
	consoleOnce    sync.Once
	consoleHandler atomic.Pointer[func(ConsoleEvent)]

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

// ctx returns the browser-wide base context this page derives operations from.
// Lets public methods omit a caller-supplied context.Context (chromekit parity)
// while still honoring Browser.Close() cancellation.
func (p *Page) ctx() context.Context {
	if p == nil || p.browser == nil {
		return context.Background()
	}
	return p.browser.ctx()
}

// opCtx derives a per-operation context from the base context. timeout <= 0
// means "no deadline" — returns the base ctx with a no-op cancel so callers can
// always `defer cancel()`. Used by every public method that takes an explicit
// timeout time.Duration (Navigate, WaitFor*, QuerySelector, ...).
func (p *Page) opCtx(timeout time.Duration) (context.Context, context.CancelFunc) {
	base := p.ctx()
	if timeout <= 0 {
		return base, func() {}
	}
	return context.WithTimeout(base, timeout)
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
	if err := p.Evaluate(`document.title`, &title); err != nil {
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

// GetTitle returns the live document title via script.evaluate.
// Blocking; uses the browser base ctx (chromekit parity).
func (p *Page) GetTitle() (string, error) {
	return p.TitleLive(p.ctx())
}

// GetURL returns the live window.location.href.
// Blocking; uses the browser base ctx (chromekit parity).
func (p *Page) GetURL() (string, error) {
	return p.URLLive(p.ctx())
}

// BringToFront activates the browsing context (brings the tab to the front).
// Uses the browser base ctx.
func (p *Page) BringToFront() error {
	return p.browser.activate(p.ctx(), p.contextID)
}

// Index returns the position of this page in the browser's page slice,
// or -1 if not found.
func (p *Page) Index() int {
	p.browser.pagesMu.RLock()
	defer p.browser.pagesMu.RUnlock()
	for i, pg := range p.browser.pages {
		if pg == p {
			return i
		}
	}
	return -1
}

// Ctx exposes the browser base context this page derives operations from.
// Matches chromekit parity.
func (p *Page) Ctx() context.Context {
	return p.ctx()
}

// Close closes this page best-effort. Errors are silently swallowed.
// If this is the last page, the call is a no-op (use Browser.Close instead).
func (p *Page) Close() {
	_ = p.browser.ClosePage(p.contextID)
}

// matchPage checks whether candidate URL matches the page's known URL by
// substring (case-insensitive). Used by Browser.SwitchPageByURL.
func matchPage(haveURL, want string) bool {
	return strings.Contains(strings.ToLower(haveURL), strings.ToLower(want))
}
