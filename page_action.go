package firefoxkit

import (
	"context"
	"time"
)

// ClickAt is a Page-level shortcut for Mouse().ClickAt. Phase 05 will add a
// selector-based Click(sel) that resolves bbox via QuerySelector.
func (p *Page) ClickAt(ctx context.Context, x, y float64) error {
	return p.Mouse().ClickAt(ctx, x, y)
}

// HoverAt is sugar for Mouse().Hover.
func (p *Page) HoverAt(ctx context.Context, x, y float64) error {
	return p.Mouse().Hover(ctx, x, y)
}

// Type is a Page-level shortcut for Keyboard().Type. Selector-based
// TypeSelector(sel, text) lands in phase 05.
func (p *Page) Type(ctx context.Context, text string) error {
	return p.Keyboard().Type(ctx, text)
}

// TypeFast is sugar for Keyboard().TypeFast.
func (p *Page) TypeFast(ctx context.Context, text string) error {
	return p.Keyboard().TypeFast(ctx, text)
}

// Click finds sel, scrolls into view, then clicks the center of its bbox.
func (p *Page) Click(ctx context.Context, sel string) error {
	if err := p.ScrollIntoView(ctx, sel); err != nil {
		return err
	}
	bb, err := p.BoundingBox(ctx, sel)
	if err != nil {
		return err
	}
	cx, cy := bb.Center()
	return p.Mouse().ClickAt(ctx, cx, cy)
}

// ClickElement is an alias for Click — kept for chromekit API parity.
func (p *Page) ClickElement(ctx context.Context, sel string) error {
	return p.Click(ctx, sel)
}

// Hover moves the mouse to the center of sel's bbox.
func (p *Page) Hover(ctx context.Context, sel string) error {
	bb, err := p.BoundingBox(ctx, sel)
	if err != nil {
		return err
	}
	cx, cy := bb.Center()
	return p.Mouse().Hover(ctx, cx, cy)
}

// Wait blocks for d or until ctx is canceled — convenience for pacing
// action sequences without `time.Sleep` interleaving boilerplate.
func (p *Page) Wait(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
