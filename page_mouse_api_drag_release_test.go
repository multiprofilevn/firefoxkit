package firefoxkit

import (
	"context"
	"sync"
	"testing"

	"github.com/tuwibu/firefoxkit/input"
)

// dragReleaseBackend records each mouse op together with whether the context it
// was handed was still alive, and kills the browser ctx from inside MouseDown —
// i.e. the caller gives up while the button is physically held.
type dragReleaseBackend struct {
	cancel context.CancelFunc

	mu    sync.Mutex
	ops   []string
	alive map[string]bool
}

func newDragReleaseBackend() *dragReleaseBackend {
	return &dragReleaseBackend{alive: map[string]bool{}}
}

func (d *dragReleaseBackend) log(ctx context.Context, op string) {
	d.mu.Lock()
	d.ops = append(d.ops, op)
	d.alive[op] = ctx.Err() == nil
	d.mu.Unlock()
}

func (d *dragReleaseBackend) MouseMove(ctx context.Context, _, _ float64) error {
	d.log(ctx, "move")
	return ctx.Err()
}

func (d *dragReleaseBackend) MouseDown(ctx context.Context, _, _ float64, _ input.MouseButton) error {
	d.log(ctx, "down")
	d.cancel()
	return nil
}

func (d *dragReleaseBackend) MouseUp(ctx context.Context, _, _ float64, _ input.MouseButton) error {
	d.log(ctx, "up")
	return ctx.Err()
}

func (d *dragReleaseBackend) MouseScroll(context.Context, float64, float64, float64, float64) error {
	return nil
}
func (d *dragReleaseBackend) KeyChar(context.Context, rune) error      { return nil }
func (d *dragReleaseBackend) InsertText(context.Context, string) error { return nil }
func (d *dragReleaseBackend) KeyCombo(context.Context, uint16, input.Modifier) error {
	return nil
}

// TestDrag_ReleasesButtonAfterCallerCtxCancelled: when the browser ctx dies
// mid-drag the release must go out on a fresh short-lived ctx. Releasing on the
// dead caller ctx is dropped by the backend and Firefox keeps the button held,
// so every later click is interpreted as a drag.
func TestDrag_ReleasesButtonAfterCallerCtxCancelled(t *testing.T) {
	b := newTestBrowser()
	p := &Page{browser: b}
	fb := newDragReleaseBackend()
	fb.cancel = b.baseCancel
	p.bkCache.bk = fb
	p.cursor = input.NewCursorWithProfile(fb, input.ProfileMachine)

	_ = p.Mouse().Drag(0, 0, 100, 100)

	fb.mu.Lock()
	defer fb.mu.Unlock()
	if len(fb.ops) == 0 || fb.ops[len(fb.ops)-1] != "up" {
		t.Fatalf("Drag must still release the button, ops=%v", fb.ops)
	}
	if !fb.alive["up"] {
		t.Fatal("Drag released on the cancelled caller ctx — the release would be dropped")
	}
}
