package firefoxkit

import (
	"context"
	"fmt"

	"github.com/tuwibu/firefoxkit/internal/bidi"
)

// populateInitialPage walks the BiDi context tree on startup and binds
// every top-level context as a Page. Sets b.current to the first one.
// Returns ErrNoLaunchTarget if the tree comes back empty.
func (b *Browser) populateInitialPage(ctx context.Context) error {
	tree, err := b.client.GetTree(ctx)
	if err != nil {
		return fmt.Errorf("%w: getTree: %v", ErrBiDiSessionFailed, err)
	}
	if len(tree.Contexts) == 0 {
		return ErrNoLaunchTarget
	}
	b.pagesMu.Lock()
	defer b.pagesMu.Unlock()
	for i := range tree.Contexts {
		c := &tree.Contexts[i]
		p := &Page{browser: b, contextID: c.Context, log: b.log, url: c.URL}
		b.pages = append(b.pages, p)
	}
	b.current = b.pages[0]
	return nil
}

// NewPage opens a new tab and makes it current.
func (b *Browser) NewPage() (*Page, error) {
	ctx := b.ctx()
	res, err := b.client.Create(ctx, bidi.CreateParams{Type: "tab"})
	if err != nil {
		return nil, fmt.Errorf("%w: browsingContext.create: %v", ErrBiDiSessionFailed, err)
	}
	p := &Page{browser: b, contextID: res.Context, log: b.log}
	b.pagesMu.Lock()
	b.pages = append(b.pages, p)
	b.current = p
	b.pagesMu.Unlock()
	return p, nil
}

// ClosePage closes the page with the given id. Returns ErrCannotCloseLastPage
// if it's the only page (use Browser.Close to shut down instead).
func (b *Browser) ClosePage(id string) error {
	ctx := b.ctx()
	b.pagesMu.Lock()
	if len(b.pages) <= 1 {
		b.pagesMu.Unlock()
		return ErrCannotCloseLastPage
	}
	idx := -1
	for i, p := range b.pages {
		if p.contextID == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		b.pagesMu.Unlock()
		return ErrPageNotFound
	}
	target := b.pages[idx]
	b.pages = append(b.pages[:idx], b.pages[idx+1:]...)
	if b.current == target {
		b.current = b.pages[0]
	}
	b.pagesMu.Unlock()

	target.cleanup(ctx)
	return b.client.CloseContext(ctx, bidi.CloseParams{Context: id})
}

// SwitchPageByIndex makes pages[i] current. Errors if out of range.
func (b *Browser) SwitchPageByIndex(i int) error {
	b.pagesMu.Lock()
	if i < 0 || i >= len(b.pages) {
		b.pagesMu.Unlock()
		return ErrPageNotFound
	}
	b.current = b.pages[i]
	target := b.current.contextID
	b.pagesMu.Unlock()
	return b.activate(b.ctx(), target)
}

// SwitchPageByURL makes the first page whose URL contains url current.
func (b *Browser) SwitchPageByURL(url string) error {
	b.pagesMu.Lock()
	var target string
	for _, p := range b.pages {
		if matchPage(p.URL(), url) {
			b.current = p
			target = p.contextID
			break
		}
	}
	b.pagesMu.Unlock()
	if target == "" {
		return ErrPageNotFound
	}
	return b.activate(b.ctx(), target)
}

// SwitchPageByOrigin switches to another page whose origin matches the current
// page's origin. Mirrors chromekit: no argument — the target origin is read
// from the current page.
func (b *Browser) SwitchPageByOrigin() error {
	cur := b.Current()
	if cur == nil {
		return ErrPageNotFound
	}
	origin, err := cur.GetOrigin()
	if err != nil {
		return err
	}

	// Snapshot candidate pages under RLock — do NOT call GetOrigin (JS eval)
	// while holding the lock to avoid long-hold / potential deadlock.
	b.pagesMu.RLock()
	candidates := make([]*Page, 0, len(b.pages))
	for _, p := range b.pages {
		if p != cur {
			candidates = append(candidates, p)
		}
	}
	b.pagesMu.RUnlock()

	// Eval origins outside the lock.
	var matched *Page
	for _, p := range candidates {
		if o, e := p.GetOrigin(); e == nil && o == origin {
			matched = p
			break
		}
	}
	if matched == nil {
		return ErrPageNotFound
	}

	// Short critical section: just update b.current and grab contextID.
	b.pagesMu.Lock()
	b.current = matched
	target := matched.contextID
	b.pagesMu.Unlock()

	return b.activate(b.ctx(), target)
}

// ClosePagesNotMatching closes every page whose URL does NOT contain keep,
// leaving the matching page(s) open. Never closes the last remaining page.
func (b *Browser) ClosePagesNotMatching(keep string) error {
	b.pagesMu.RLock()
	var victims []string
	for _, p := range b.pages {
		if !matchPage(p.URL(), keep) {
			victims = append(victims, p.contextID)
		}
	}
	b.pagesMu.RUnlock()
	for _, id := range victims {
		if err := b.ClosePage(id); err != nil {
			if err == ErrCannotCloseLastPage {
				break
			}
			return err
		}
	}
	return nil
}

// GoBack navigates the current page back one entry in session history.
func (b *Browser) GoBack() error {
	cur := b.Current()
	if cur == nil {
		return ErrPageNotFound
	}
	return cur.traverse(cur.ctx(), -1)
}

// activate sends browsingContext.activate. Failure logged-not-returned —
// activation is best-effort and shouldn't fail the switch operation.
func (b *Browser) activate(ctx context.Context, id string) error {
	if err := b.client.Activate(ctx, bidi.ActivateParams{Context: id}); err != nil {
		if b.log != nil {
			b.log.Warnf("firefoxkit: activate %s: %v", id, err)
		}
	}
	return nil
}
