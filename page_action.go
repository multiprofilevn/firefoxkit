package firefoxkit

import "time"

// ClickAt is a Page-level shortcut for Mouse().ClickAt.
func (p *Page) ClickAt(x, y float64) error {
	return p.Mouse().ClickAt(x, y)
}

// HoverAt is sugar for Mouse().MoveTo.
func (p *Page) HoverAt(x, y float64) error {
	return p.Mouse().MoveTo(x, y)
}

// Type focuses selector then types text via Keyboard().Type.
// Mirrors chromekit Page.Type(selector, text) — selector must resolve before typing.
func (p *Page) Type(selector, text string) error {
	if err := p.Focus(selector); err != nil {
		return err
	}
	return p.Keyboard().Type(text)
}

// Click finds sel, scrolls into view, then clicks the center of its bbox.
func (p *Page) Click(selector string) error {
	if err := p.ScrollIntoView(selector); err != nil {
		return err
	}
	bb, err := p.BoundingBox(selector)
	if err != nil {
		return err
	}
	cx, cy := bb.Center()
	return p.Mouse().ClickAt(cx, cy)
}

// ClickElement is an alias for Click — kept for chromekit API parity.
func (p *Page) ClickElement(selector string) error {
	return p.Click(selector)
}

// Hover moves the mouse to the center of sel's bbox.
func (p *Page) Hover(selector string) error {
	bb, err := p.BoundingBox(selector)
	if err != nil {
		return err
	}
	cx, cy := bb.Center()
	return p.Mouse().MoveTo(cx, cy)
}


// Wait blocks for d — convenience for pacing action sequences.
func (p *Page) Wait(d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	ctx := p.ctx()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
