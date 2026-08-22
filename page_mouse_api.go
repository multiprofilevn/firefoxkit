package firefoxkit

import (
	"context"
	"sync"
	"time"

	"github.com/multiprofilevn/firefoxkit/input"
)

const dragSteps = 10

// sentinelPos marks "cursor position not yet set" for scroll origin fallback.
const sentinelPos = -1e9

// Mouse returns the cached per-page MouseAPI so lastX/lastY persist across calls.
func (p *Page) Mouse() *MouseAPI {
	p.mouseOnce.Do(func() {
		p.mouse = &MouseAPI{p: p, lastX: sentinelPos, lastY: sentinelPos}
	})
	return p.mouse
}

// MouseAPI groups mouse-input shortcuts — both raw-coordinate (low-level) and
// selector-based variants. Mirrors chromekit MouseAPI public surface exactly,
// plus extra Firefox-specific MiddleClick. Safe for concurrent use: position
// state is mutex-guarded and gesture paths serialize on the cursor engine.
type MouseAPI struct {
	p *Page

	posMu sync.Mutex
	lastX float64 // last known cursor X; sentinelPos if never moved
	lastY float64 // last known cursor Y; sentinelPos if never moved
}

// setLast records the cursor position under lock.
func (m *MouseAPI) setLast(x, y float64) {
	m.posMu.Lock()
	m.lastX, m.lastY = x, y
	m.posMu.Unlock()
}

// MoveTo glides the cursor to (x,y) in viewport CSS pixels along a human-like
// bezier path (ProfileHuman) via the cursor engine.
func (m *MouseAPI) MoveTo(x, y float64) error {
	if err := m.p.cursorEngine().MoveTo(m.p.ctx(), x, y); err != nil {
		return err
	}
	m.setLast(x, y)
	return nil
}

// ClickAt glides to (x,y) then clicks with the left button.
func (m *MouseAPI) ClickAt(x, y float64) error {
	if err := m.clickButton(x, y, input.MouseLeft); err != nil {
		return err
	}
	m.setLast(x, y)
	return nil
}

// Click moves to the center of the element matching selector and left-clicks.
func (m *MouseAPI) Click(selector string) error {
	bb, err := m.p.BoundingBox(selector)
	if err != nil {
		return err
	}
	cx, cy := bb.Center()
	return m.ClickAt(cx, cy)
}

// RightClick clicks the center of the element matching selector with the right button.
func (m *MouseAPI) RightClick(selector string) error {
	bb, err := m.p.BoundingBox(selector)
	if err != nil {
		return err
	}
	cx, cy := bb.Center()
	if err := m.clickButton(cx, cy, input.MouseRight); err != nil {
		return err
	}
	m.setLast(cx, cy)
	return nil
}

// MiddleClick is a Firefox-extra: glides to (x,y) then clicks the middle button.
// Not in chromekit MouseAPI but retained as coord-based extra.
func (m *MouseAPI) MiddleClick(x, y float64) error {
	if err := m.clickButton(x, y, input.MouseMiddle); err != nil {
		return err
	}
	m.setLast(x, y)
	return nil
}

// clickButton routes through the cursor engine: glide to (x,y) along a bezier
// path, then press/release with a human-like hold. Replaces the old direct
// backend MouseDown/MouseUp teleport-click so Firefox matches chromekit.
func (m *MouseAPI) clickButton(x, y float64, button input.MouseButton) error {
	return m.p.cursorEngine().Click(m.p.ctx(), x, y, button)
}

// DoubleClick fires two left clicks at the element's center within ~80ms.
func (m *MouseAPI) DoubleClick(selector string) error {
	bb, err := m.p.BoundingBox(selector)
	if err != nil {
		return err
	}
	cx, cy := bb.Center()
	if err := m.clickButton(cx, cy, input.MouseLeft); err != nil {
		return err
	}
	m.setLast(cx, cy)
	// Browsers expect two clicks within ~500ms; insert a small gap.
	time.Sleep(40 * time.Millisecond)
	if err := m.clickButton(cx, cy, input.MouseLeft); err != nil {
		return err
	}
	return nil
}

// Hover moves the cursor to the center of the element matching selector.
func (m *MouseAPI) Hover(selector string) error {
	bb, err := m.p.BoundingBox(selector)
	if err != nil {
		return err
	}
	cx, cy := bb.Center()
	return m.MoveTo(cx, cy)
}

// Scroll dispatches a wheel event with the given deltas. The origin is the last
// known cursor position, or the viewport center if the cursor has not been moved.
// Takes the gesture lock: the native wheel lands wherever the shared OS pointer
// currently is, so it must not fire inside another Browser's gesture. The origin
// is resolved BEFORE the lock — scrollOrigin may hit the browser over BiDi.
func (m *MouseAPI) Scroll(deltaX, deltaY float64) error {
	x, y := m.scrollOrigin()
	b := m.p.inputBackend()
	return input.WithGesture(m.p.ctx(), b, func(ctx context.Context) error {
		return b.MouseScroll(ctx, x, y, deltaX, deltaY)
	})
}

// scrollOrigin returns the wheel origin: last cursor pos, else viewport center.
func (m *MouseAPI) scrollOrigin() (float64, float64) {
	m.posMu.Lock()
	lx, ly := m.lastX, m.lastY
	m.posMu.Unlock()
	if lx != sentinelPos {
		return lx, ly
	}
	// Fallback: read viewport dimensions via JS.
	var dims [2]float64
	if err := m.p.Evaluate("[window.innerWidth/2, window.innerHeight/2]", &dims); err == nil {
		return dims[0], dims[1]
	}
	// Last resort: (0,0) — still valid (BiDi ignores origin for scroll).
	return 0, 0
}

// Drag glides to (fromX,fromY), presses, moves through dragSteps intermediate
// points, then releases at (toX,toY). The approach glide makes the gesture
// human-like; the press-hold-move portion stays on the raw backend because a
// button must remain held across the intermediate moves.
//
// The whole sequence runs inside one gesture (input.WithGesture): on the native
// backend another Browser must not move the shared OS pointer between the grab
// and the drop. Every release goes through input.ReleaseCtx so a caller ctx
// that dies mid-drag still lifts the button.
func (m *MouseAPI) Drag(fromX, fromY, toX, toY float64) error {
	b := m.p.inputBackend()
	return input.WithGesture(m.p.ctx(), b, func(ctx context.Context) error {
		release := func(x, y float64) {
			rctx, cancel := input.ReleaseCtx(ctx)
			_ = b.MouseUp(rctx, x, y, input.MouseLeft)
			cancel()
		}
		// Glide to the grab point before pressing (matches click behavior).
		if err := m.p.cursorEngine().MoveTo(ctx, fromX, fromY); err != nil {
			return err
		}
		if err := b.MouseDown(ctx, fromX, fromY, input.MouseLeft); err != nil {
			return err
		}
		dx := (toX - fromX) / float64(dragSteps+1)
		dy := (toY - fromY) / float64(dragSteps+1)
		for i := 1; i <= dragSteps; i++ {
			if err := b.MouseMove(ctx, fromX+dx*float64(i), fromY+dy*float64(i)); err != nil {
				release(fromX+dx*float64(i), fromY+dy*float64(i))
				return err
			}
		}
		if err := b.MouseMove(ctx, toX, toY); err != nil {
			release(toX, toY)
			return err
		}
		m.setLast(toX, toY)
		// Sync the cursor engine to the drag end-point — the press-hold-move
		// above bypassed it, so without this the next glide would seed from the
		// drag source (visible backward jump on the BiDi backend, which has no
		// OS cursor to re-seed from).
		m.p.cursorEngine().SetPosition(toX, toY)
		rctx, cancel := input.ReleaseCtx(ctx)
		defer cancel()
		return b.MouseUp(rctx, toX, toY, input.MouseLeft)
	})
}

// DragAndDrop resolves fromSel and toSel to their center points then drags.
func (m *MouseAPI) DragAndDrop(fromSel, toSel string) error {
	fbx, err := m.p.BoundingBox(fromSel)
	if err != nil {
		return err
	}
	tbx, err := m.p.BoundingBox(toSel)
	if err != nil {
		return err
	}
	fx, fy := fbx.Center()
	tx, ty := tbx.Center()
	return m.Drag(fx, fy, tx, ty)
}

// Tap dispatches a touch tap at (x,y) using TouchScreen primitives.
func (m *MouseAPI) Tap(x, y float64) error {
	ts := input.NewTouchScreen(m.p.inputBackend())
	return ts.Tap(m.p.ctx(), x, y)
}

// FocusElement scrolls the element into view and clicks it to give it focus.
// If the click does not move document.activeElement to the target, falls back
// to a JS .focus() call via Evaluate.
func (m *MouseAPI) FocusElement(selector string) error {
	_ = m.p.ScrollIntoView(selector)
	if err := m.Click(selector); err != nil {
		return err
	}
	// Verify focus; JS .focus() fallback if click didn't take.
	var focused bool
	expr := `(function(sel){
		const el = document.querySelector(sel);
		return !!el && el === document.activeElement;
	})(` + quoteJS(selector) + `)`
	if err := m.p.Evaluate(expr, &focused); err == nil && focused {
		return nil
	}
	return m.p.Focus(selector)
}

// quoteJS wraps s in a JS string literal (double-quoted, backslash-escaped).
func quoteJS(s string) string {
	out := make([]byte, 0, len(s)+2)
	out = append(out, '"')
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '"', '\\':
			out = append(out, '\\', c)
		case '\n':
			out = append(out, '\\', 'n')
		case '\r':
			out = append(out, '\\', 'r')
		default:
			out = append(out, c)
		}
	}
	out = append(out, '"')
	return string(out)
}
