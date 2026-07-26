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

	// initMu guards lazy construction of hdr and dl. A plain mutex (not
	// sync.Once) so cleanup() can read the fields race-free even when it runs
	// concurrently with a first-use initialization.
	initMu sync.Mutex
	hdr    *headerInjector
	dl     *downloadsAPI

	// listenerMu guards the *Subscribed flags below. A plain mutex+bool (not
	// sync.Once) so a failed session.subscribe can be retried on a later call —
	// resetting a sync.Once inside Do() fatals with "unlock of unlocked mutex".
	// The flag is set only after the subscribe RPC succeeds.
	listenerMu        sync.Mutex
	dialogSubscribed  bool
	consoleSubscribed bool

	// dialogHandler holds the current handler (replaced on each OnDialog call).
	dialogHandler atomic.Pointer[func(DialogEvent) DialogAction]

	// consoleHandler holds the current handler (replaced on each OnConsole call).
	consoleHandler atomic.Pointer[func(ConsoleEvent)]

	// subsMu guards subs and serverEvents. subs is the cumulative list of
	// local BiDi event subscriptions registered for this page (header
	// injector, downloads, OnDialog, OnConsole); serverEvents the per-context
	// event names session.subscribe'd for it. cleanup() releases both so we
	// don't leak handlers + server-side subscriptions after the tab is gone.
	subsMu       sync.Mutex
	subs         []*bidi.Subscription
	serverEvents []string

	// cleanedUp makes cleanup idempotent — it can be reached from both the
	// contextDestroyed listener and Browser.Close/ClosePage.
	cleanedUp atomic.Bool
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

// trackServerEvent records a per-context event name that was successfully
// session.subscribe'd for this page, so cleanup can session.unsubscribe it.
// Deduped: a feature that re-subscribes across install/remove cycles must not
// grow the list without bound.
func (p *Page) trackServerEvent(event string) {
	p.subsMu.Lock()
	defer p.subsMu.Unlock()
	for _, e := range p.serverEvents {
		if e == event {
			return
		}
	}
	p.serverEvents = append(p.serverEvents, event)
}

// untrackServerEvent drops an event name that was already session.unsubscribe'd
// ahead of cleanup, so cleanup doesn't unsubscribe it a second time.
func (p *Page) untrackServerEvent(event string) {
	p.subsMu.Lock()
	defer p.subsMu.Unlock()
	for i, e := range p.serverEvents {
		if e == event {
			p.serverEvents = append(p.serverEvents[:i], p.serverEvents[i+1:]...)
			return
		}
	}
}

// cleanupLocal releases the half of teardown that needs no RPC: every local
// BiDi handler registration this page holds. Left behind they stay in the
// client's dispatcher — each with its own worker goroutine keeping this *Page
// alive in a closure — for the rest of the client's life, so this must run even
// when the RPC half is skipped (cleanup drainer saturation). Reports false when
// the page was already claimed, i.e. nothing was done.
//
// Never blocks on an RPC: only locks that are held for pure bookkeeping are
// taken here. installMu is deliberately NOT one of them — SetExtraHeaders holds
// it across its install calls — so a header interceptor committed concurrently
// with this call is released by the RPC half instead (which waits on installMu).
//
// The cleanedUp claim happens BEFORE the per-feature locks: combined with each
// lazy-subscribe path checking cleanedUp under the same lock, a subscribe
// already committed under its lock is seen here (we take the same lock) and one
// starting after the claim bails out — so no subscription is ever orphaned by a
// cleanup/first-subscribe race.
func (p *Page) cleanupLocal() bool {
	if p.cleanedUp.Swap(true) {
		return false
	}
	p.initMu.Lock()
	hdr := p.hdr
	dl := p.dl
	p.initMu.Unlock()
	// Header injector handler. Only the handler goes here; the intercept id it
	// pairs with is server-side state the RPC half removes.
	if hdr != nil {
		hdr.mu.Lock()
		sub := hdr.sub
		hdr.sub = nil
		hdr.mu.Unlock()
		if sub != nil {
			sub.Unsubscribe()
		}
	}
	// Downloads subscription — dl.mu is the same lock ensureSubscribed holds.
	if dl != nil {
		dl.mu.Lock()
		sub := dl.sub
		dl.sub = nil
		dl.mu.Unlock()
		if sub != nil {
			sub.Unsubscribe()
		}
	}
	// Tracked listener subscriptions (OnDialog / OnConsole). Hold listenerMu
	// so an in-flight first OnDialog/OnConsole completes its trackSub before
	// we snapshot (and any later one sees cleanedUp and bails).
	p.listenerMu.Lock()
	p.subsMu.Lock()
	subs := p.subs
	p.subs = nil
	p.subsMu.Unlock()
	p.listenerMu.Unlock()
	for _, s := range subs {
		s.Unsubscribe()
	}
	return true
}

// cleanup unsubscribes every per-page BiDi event handler and releases any
// server-side intercept ids + event subscriptions. Idempotent — reached from
// the contextDestroyed listener, Browser.ClosePage, and Browser.Close.
// Failures during cleanup are logged but never returned — teardown must be
// best-effort to avoid wedging on partial state.
func (p *Page) cleanup(ctx context.Context) {
	// Local half first — it owns the cleanedUp claim this whole function is
	// gated on, so a second caller stops here.
	if !p.cleanupLocal() {
		return
	}
	p.initMu.Lock()
	hdr := p.hdr
	p.initMu.Unlock()
	// Header injector + intercept id — take installMu so an in-flight
	// installHeaderInterceptor finishes (and is removed) rather than being
	// orphaned by a no-op remove seeing an empty interceptID.
	if hdr != nil {
		hdr.installMu.Lock()
		if err := p.removeHeaderInterceptor(ctx); err != nil && p.log != nil {
			p.log.Warnf("firefoxkit: cleanup header injector: %v", err)
		}
		hdr.installMu.Unlock()
	}
	// Release any held keys / pressed buttons so a Down without its Up (error
	// or cancel mid-sequence) can't leave sticky modifiers in the context.
	if p.bkCache.ready.Load() {
		if r, ok := p.bkCache.bk.(interface{ Release(context.Context) error }); ok {
			if err := r.Release(ctx); err != nil && p.log != nil {
				p.log.Debugf("firefoxkit: cleanup release input: %v", err)
			}
		}
	}
	// Snapshot AFTER the header interceptor removal above, which untracks its
	// own event once it has unsubscribed it — otherwise it is unsubscribed
	// twice.
	p.subsMu.Lock()
	events := p.serverEvents
	p.serverEvents = nil
	p.subsMu.Unlock()
	// Server-side unsubscribe so per-context subscriptions don't pile up on
	// the session across page churn. Fails harmlessly for destroyed contexts.
	if len(events) > 0 {
		if err := p.browser.client.UnsubscribeEvents(ctx, bidi.SubscribeParams{
			Events:   events,
			Contexts: []string{p.contextID},
		}); err != nil && p.log != nil {
			p.log.Debugf("firefoxkit: cleanup unsubscribe %v: %v", events, err)
		}
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
