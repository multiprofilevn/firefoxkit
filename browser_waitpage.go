package firefoxkit

import (
	"fmt"
	"strings"
	"time"
)

// waitPagePollInterval is how often WaitForNewPage re-reads the context tree.
// Kept small because site-opened popups (OAuth consent) may close themselves
// quickly after posting their result — we must bind before that happens.
const waitPagePollInterval = 300 * time.Millisecond

// WaitForNewPage polls the LIVE BiDi context tree (browsingContext.getTree)
// until a top-level context that is NOT already tracked and whose URL contains
// urlSubstr appears, binds it as a Page, and returns it. Mirrors
// chromekit.WaitForNewPage (which polls /json/list).
//
// It queries getTree — a live browser query — rather than scanning b.pages: a
// popup the page opens itself (window.open, e.g. a Google Identity Services
// OAuth window) is a new top-level browsing context that getTree enumerates but
// b.pages never auto-tracks. Polling getTree also avoids a session-scoped
// contextCreated subscribe/unsubscribe (which is not ref-counted and would race
// across concurrent callers).
//
// urlSubstr == "" matches the first brand-new context regardless of URL.
func (b *Browser) WaitForNewPage(urlSubstr string, timeout time.Duration) (*Page, error) {
	if b == nil || b.client == nil {
		return nil, fmt.Errorf("%w: WaitForNewPage requires a live BiDi client", ErrBiDiSessionFailed)
	}
	known := b.knownContextIDs()
	deadline := time.Now().Add(timeout)

	for {
		ctx := b.ctx()
		if err := ctx.Err(); err != nil {
			return nil, ErrContextCanceled
		}
		if tree, err := b.client.GetTree(ctx); err == nil {
			for i := range tree.Contexts {
				c := tree.Contexts[i]
				if known[c.Context] {
					continue
				}
				if urlSubstr != "" && !strings.Contains(c.URL, urlSubstr) {
					continue
				}
				if p := b.bindNewContext(c.Context, c.URL); p != nil {
					return p, nil
				}
			}
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("%w: no new page matching %q within %s", ErrTimeout, urlSubstr, timeout)
		}
		time.Sleep(waitPagePollInterval)
	}
}

// bindNewContext registers contextID as a Page under b.pagesMu, guarding against
// a concurrent WaitForNewPage having already bound the same context. Returns nil
// if the context is already tracked (caller keeps polling).
func (b *Browser) bindNewContext(contextID, url string) *Page {
	b.pagesMu.Lock()
	defer b.pagesMu.Unlock()
	for _, p := range b.pages {
		if p.contextID == contextID {
			return nil
		}
	}
	p := &Page{browser: b, contextID: contextID, log: b.log, url: url}
	b.pages = append(b.pages, p)
	return p
}

// knownContextIDs snapshots the currently-tracked context IDs so WaitForNewPage
// can tell a freshly-opened popup from tabs that already existed.
func (b *Browser) knownContextIDs() map[string]bool {
	b.pagesMu.RLock()
	defer b.pagesMu.RUnlock()
	m := make(map[string]bool, len(b.pages))
	for _, p := range b.pages {
		m[p.contextID] = true
	}
	return m
}
