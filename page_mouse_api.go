package firefoxkit

import (
	"context"
	"time"

	"github.com/tuwibu/firefoxkit/input"
)

// Mouse returns a per-page mouse namespace.
func (p *Page) Mouse() *Mouse {
	return &Mouse{p: p}
}

// Mouse groups mouse-input shortcuts.
type Mouse struct{ p *Page }

// MoveTo moves the cursor to (x,y) in viewport CSS pixels.
func (m *Mouse) MoveTo(ctx context.Context, x, y float64) error {
	return m.p.inputBackend().MouseMove(ctx, x, y)
}

// ClickAt performs MoveTo → MouseDown → MouseUp at (x,y) with the left button.
func (m *Mouse) ClickAt(ctx context.Context, x, y float64) error {
	return m.click(ctx, x, y, input.MouseLeft)
}

// RightClick is ClickAt with the right button.
func (m *Mouse) RightClick(ctx context.Context, x, y float64) error {
	return m.click(ctx, x, y, input.MouseRight)
}

// MiddleClick is ClickAt with the middle button.
func (m *Mouse) MiddleClick(ctx context.Context, x, y float64) error {
	return m.click(ctx, x, y, input.MouseMiddle)
}

func (m *Mouse) click(ctx context.Context, x, y float64, button input.MouseButton) error {
	b := m.p.inputBackend()
	if err := b.MouseDown(ctx, x, y, button); err != nil {
		return err
	}
	return b.MouseUp(ctx, x, y, button)
}

// DoubleClick fires two left-click sequences in quick succession.
func (m *Mouse) DoubleClick(ctx context.Context, x, y float64) error {
	if err := m.click(ctx, x, y, input.MouseLeft); err != nil {
		return err
	}
	// Browsers expect two clicks within ~500ms; insert a small gap.
	select {
	case <-time.After(40 * time.Millisecond):
	case <-ctx.Done():
		return ctx.Err()
	}
	return m.click(ctx, x, y, input.MouseLeft)
}

// Hover is sugar for MoveTo.
func (m *Mouse) Hover(ctx context.Context, x, y float64) error {
	return m.MoveTo(ctx, x, y)
}

// Scroll dispatches a wheel scroll centered at (x,y) by (deltaX, deltaY).
func (m *Mouse) Scroll(ctx context.Context, x, y, deltaX, deltaY float64) error {
	return m.p.inputBackend().MouseScroll(ctx, x, y, deltaX, deltaY)
}

// Drag performs press at (fromX,fromY), N intermediate moves, release at (toX,toY).
// steps == 0 produces a straight press → move → release.
func (m *Mouse) Drag(ctx context.Context, fromX, fromY, toX, toY float64, steps int) error {
	b := m.p.inputBackend()
	if err := b.MouseDown(ctx, fromX, fromY, input.MouseLeft); err != nil {
		return err
	}
	if steps > 0 {
		dx := (toX - fromX) / float64(steps+1)
		dy := (toY - fromY) / float64(steps+1)
		for i := 1; i <= steps; i++ {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := b.MouseMove(ctx, fromX+dx*float64(i), fromY+dy*float64(i)); err != nil {
				return err
			}
		}
	}
	if err := b.MouseMove(ctx, toX, toY); err != nil {
		return err
	}
	return b.MouseUp(ctx, toX, toY, input.MouseLeft)
}

// DragAndDrop is sugar for Drag with a default 10-step path.
func (m *Mouse) DragAndDrop(ctx context.Context, fromX, fromY, toX, toY float64) error {
	return m.Drag(ctx, fromX, fromY, toX, toY, 10)
}
