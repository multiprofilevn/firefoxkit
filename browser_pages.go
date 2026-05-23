package firefoxkit

import (
	"context"
	"fmt"
	"strings"
	"time"

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
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res, err := b.client.Create(ctx, bidi.CreateParams{Type: "tab"})
	if err != nil {
		return nil, fmt.Errorf("%w: browsingContext.create: %v", ErrInvalidConfig, err)
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

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return b.client.CloseContext(ctx, bidi.CloseParams{Context: id})
}

// SwitchPageByIndex makes pages[i] current. Errors if out of range.
func (b *Browser) SwitchPageByIndex(i int) error {
	b.pagesMu.Lock()
	defer b.pagesMu.Unlock()
	if i < 0 || i >= len(b.pages) {
		return ErrPageNotFound
	}
	b.current = b.pages[i]
	return b.activateLocked(b.current.contextID)
}

// SwitchPageByURL makes the first page whose URL contains needle current.
func (b *Browser) SwitchPageByURL(needle string) error {
	b.pagesMu.Lock()
	defer b.pagesMu.Unlock()
	for _, p := range b.pages {
		if matchPage(p.URL(), needle) {
			b.current = p
			return b.activateLocked(p.contextID)
		}
	}
	return ErrPageNotFound
}

// SwitchPageByOrigin is a sugar wrapper for matching scheme://host.
func (b *Browser) SwitchPageByOrigin(origin string) error {
	origin = strings.TrimRight(origin, "/")
	return b.SwitchPageByURL(origin)
}

// activateLocked sends browsingContext.activate. Caller must hold pagesMu.
func (b *Browser) activateLocked(id string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := b.client.Activate(ctx, bidi.ActivateParams{Context: id}); err != nil {
		// activate isn't critical — log and proceed. Mirrors chromekit which
		// treats focus-failure as a warning, not a fatal.
		if b.log != nil {
			b.log.Warnf("firefoxkit: activate %s: %v", id, err)
		}
	}
	return nil
}
