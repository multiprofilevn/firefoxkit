package firefoxkit

import (
	"context"
	"fmt"
	"strings"

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

// NewPage opens a new tab and makes it current. Honors ctx for the
// browsingContext.create RPC.
func (b *Browser) NewPage(ctx context.Context) (*Page, error) {
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
// if it's the only page (use Browser.Close to shut down instead). Honors ctx
// for both the per-page cleanup and the CloseContext RPC.
func (b *Browser) ClosePage(ctx context.Context, id string) error {
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

// SwitchPageByIndex makes pages[i] current. Errors if out of range. Honors
// ctx for the activate RPC.
func (b *Browser) SwitchPageByIndex(ctx context.Context, i int) error {
	b.pagesMu.Lock()
	if i < 0 || i >= len(b.pages) {
		b.pagesMu.Unlock()
		return ErrPageNotFound
	}
	b.current = b.pages[i]
	target := b.current.contextID
	b.pagesMu.Unlock()
	return b.activate(ctx, target)
}

// SwitchPageByURL makes the first page whose URL contains needle current.
func (b *Browser) SwitchPageByURL(ctx context.Context, needle string) error {
	b.pagesMu.Lock()
	var target string
	for _, p := range b.pages {
		if matchPage(p.URL(), needle) {
			b.current = p
			target = p.contextID
			break
		}
	}
	b.pagesMu.Unlock()
	if target == "" {
		return ErrPageNotFound
	}
	return b.activate(ctx, target)
}

// SwitchPageByOrigin is a sugar wrapper for matching scheme://host.
func (b *Browser) SwitchPageByOrigin(ctx context.Context, origin string) error {
	return b.SwitchPageByURL(ctx, strings.TrimRight(origin, "/"))
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
